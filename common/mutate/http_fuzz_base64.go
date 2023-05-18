package mutate

import (
	"bytes"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/cartesian"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func isBase64JSON(raw string) (string, bool) {
	decoded, err := codec.DecodeBase64Url(raw)
	if err != nil {
		return raw, false
	}
	return utils.IsJSON(string(decoded))
}

func (f *FuzzHTTPRequest) fuzzPostBase64JsonPath(key any, jsonPath string, val any) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}

	keyStr := utils.InterfaceToString(key)
	vals, err := url.ParseQuery(string(f.GetBody()))
	if err != nil {
		return nil, utils.Errorf("url.ParseQuery: %s", err)
	}
	originValue := vals.Get(keyStr)
	if strings.Contains(originValue, "%") {
		unescaped, err := url.QueryUnescape(originValue)
		if err == nil {
			originValue = unescaped
		}
	}
	if ret, ok := isBase64JSON(originValue); !ok {
		return nil, utils.Errorf("invalid base64 json: %s", ret)
	} else {
		originValue = ret
	}

	var reqs []*http.Request
	err = cartesian.ProductEx([][]string{
		{keyStr}, InterfaceToFuzzResults(val),
	}, func(result []string) error {
		value := result[1]
		var replaced = valueToJsonValue(value)
		for _, i := range replaced {
			_req := lowhttp.CopyRequest(req)
			originVals := make(url.Values)
			for k, v := range vals {
				if k == keyStr {
					originVals.Set(
						k,
						codec.EncodeBase64(jsonpath.ReplaceString(originValue, jsonPath, i)))
				} else {
					originVals[k] = v
				}
			}
			_req.Body = io.NopCloser(bytes.NewBufferString(originVals.Encode()))
			reqs = append(reqs, _req)
		}
		return nil
	})
	if err != nil {
		return nil, utils.Errorf("cartesian.ProductEx: %s", err)
	}
	return reqs, nil
}

func (f *FuzzHTTPRequest) fuzzGetBase64JsonPath(key any, jsonPath string, val any) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}

	keyStr := utils.InterfaceToString(key)
	vals := req.URL.Query()
	originValue := vals.Get(keyStr)
	if strings.Contains(originValue, "%") {
		unescaped, err := url.QueryUnescape(originValue)
		if err == nil {
			originValue = unescaped
		}
	}
	if ret, ok := isBase64JSON(originValue); !ok {
		return nil, utils.Errorf("invalid base64 json: %s", ret)
	} else {
		originValue = ret
	}

	var reqs []*http.Request
	err = cartesian.ProductEx([][]string{
		{keyStr}, InterfaceToFuzzResults(val),
	}, func(result []string) error {
		value := result[1]
		var replaced = valueToJsonValue(value)
		for _, i := range replaced {
			_req := lowhttp.CopyRequest(req)
			newVals := _req.URL.Query()
			newVals.Set(keyStr, codec.EncodeBase64(jsonpath.ReplaceString(originValue, jsonPath, i)))
			_req.URL.RawQuery = newVals.Encode()
			_req.RequestURI = _req.URL.RequestURI()
			reqs = append(reqs, _req)
		}
		return nil
	})
	if err != nil {
		return nil, utils.Errorf("cartesian.ProductEx: %s", err)
	}
	return reqs, nil
}

func (f *FuzzHTTPRequest) FuzzGetBase64JsonPath(key any, jsonPath string, val any) FuzzHTTPRequestIf {
	reqs, err := f.fuzzGetBase64JsonPath(key, jsonPath, val)
	if err != nil {
		return &FuzzHTTPRequestBatch{
			fallback:      f,
			originRequest: f,
		}
	}
	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) FuzzPostBase64JsonPath(key any, jsonPath string, val any) FuzzHTTPRequestIf {
	reqs, err := f.fuzzPostBase64JsonPath(key, jsonPath, val)
	if err != nil {
		return &FuzzHTTPRequestBatch{
			fallback:      f,
			originRequest: f,
		}
	}
	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequestBatch) FuzzPostBase64JsonPath(key any, jsonPath string, val any) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzPostBase64JsonPath(key, jsonPath, val)
	}

	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzPostBase64JsonPath(key, jsonPath, val))
	}

	if len(reqs) <= 0 {
		return &FuzzHTTPRequestBatch{
			fallback:      f.fallback,
			originRequest: f.GetOriginRequest(),
		}
	}

	return &FuzzHTTPRequestBatch{
		nextFuzzRequests: reqs,
		originRequest:    f.GetOriginRequest(),
	}
}

func (f *FuzzHTTPRequestBatch) FuzzGetBase64JsonPath(key any, jsonPath string, val any) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzGetBase64JsonPath(key, jsonPath, val)
	}

	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzGetBase64JsonPath(key, jsonPath, val))
	}

	if len(reqs) <= 0 {
		return &FuzzHTTPRequestBatch{
			fallback:      f.fallback,
			originRequest: f.GetOriginRequest(),
		}
	}

	return &FuzzHTTPRequestBatch{
		nextFuzzRequests: reqs,
		originRequest:    f.GetOriginRequest(),
	}
}
