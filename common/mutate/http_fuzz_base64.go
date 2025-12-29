package mutate

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/utils/mixer"
	"github.com/yaklang/yaklang/common/yak/cartesian"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func IsBase64JSON(raw string) (string, bool) {
	if raw == "" {
		return raw, false
	}
	decoded, err := codec.DecodeBase64(raw)
	if err != nil {
		return raw, false
	}
	return utils.IsJSON(string(decoded))
}

func allPlain(i []byte) bool {
	for _, v := range i {
		if v >= 0x7f || v <= 0x20 {
			return false
		}
	}
	return true
}

func IsStrictBase64(raw string) (string, bool) {
	if raw == "" {
		return raw, false
	}

	if len([]byte(raw))%4 != 0 {
		return raw, false
	}

	decoded, err := codec.DecodeBase64(raw)
	if err != nil {
		return raw, false
	}

	if !allPlain(decoded) {
		return raw, false
	}
	return string(decoded), true
}

func IsBase64(raw string) (string, bool) {
	if raw == "" {
		return raw, false
	}
	decoded, err := codec.DecodeBase64(raw)
	if err != nil {
		return raw, false
	}

	if !allPlain(decoded) {
		return raw, false
	}
	return string(decoded), true
}

func (f *FuzzHTTPRequest) fuzzGetBase64Params(key interface{}, value interface{}) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}
	vals := req.URL.Query()
	if vals == nil {
		vals = make(url.Values)
	}

	keys, values := InterfaceToFuzzResults(key), InterfaceToFuzzResults(value)
	if len(keys) <= 0 || len(values) <= 0 {
		return nil, utils.Errorf("GetQuery key or Values are empty...")
	}
	mix, err := mixer.NewMixer(keys, values)
	if err != nil {
		return nil, err
	}

	var reqs []*http.Request
	for {
		pairs := mix.Value()
		key, value := pairs[0], codec.EncodeBase64(pairs[1])
		req.RequestURI = ""
		newVals, err := deepCopyUrlValues(vals)
		if err != nil {
			continue
		}
		newVals.Set(key, value)
		req.URL.RawQuery = newVals.Encode()

		_req, err := rebuildHTTPRequest(req, 0)
		if err != nil {
			continue
		}
		req.URL.RawQuery = vals.Encode()
		reqs = append(reqs, _req)

		err = mix.Next()
		if err != nil {
			break
		}
	}
	return reqs, nil
}

func (f *FuzzHTTPRequest) fuzzPostBase64JsonPath(key any, jsonPath string, val any) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}

	keyStr := utils.InterfaceToString(key)
	vals := lowhttp.ParseQueryParams(f.GetPostQuery())
	originValue := vals.Get(keyStr)

	if ret, ok := IsBase64JSON(originValue); !ok {
		return nil, utils.Errorf("invalid base64 json: %s", ret)
	} else {
		originValue = ret
	}

	var reqs []*http.Request
	origin := httpctx.GetBareRequestBytes(req)

	valueIndex := 0
	err = cartesian.ProductEx([][]string{
		{keyStr}, InterfaceToFuzzResults(val),
	}, func(result []string) error {
		value := result[1]
		modifiedParams, err := modifyJSONValue(originValue, jsonPath, value, val, valueIndex, f.noEscapeHTML)
		if err != nil {
			return err
		}
		if f.friendlyDisplay {
			modifiedParams = fmt.Sprintf("{{base64(%s)}}", modifiedParams)
		} else {
			modifiedParams = codec.EncodeBase64(modifiedParams)
		}

		vals.DisableAutoEncode(f.NoAutoEncode())
		vals.SetFriendlyDisplay(f.friendlyDisplay)
		vals.Set(keyStr, modifiedParams)
		reqIns, err := lowhttp.ParseBytesToHttpRequest(lowhttp.ReplaceHTTPPacketQueryParamRaw(origin, vals.Encode()))
		if err != nil {
			log.Errorf("parse (in FuzzGetParams) request failed: %v", err)
			return nil
		}
		reqs = append(reqs, reqIns)
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
	vals := lowhttp.ParseQueryParams(f.GetQueryRaw())
	originValue := vals.Get(keyStr)

	if ret, ok := IsBase64JSON(originValue); !ok {
		return nil, utils.Errorf("invalid base64 json: %s", ret)
	} else {
		originValue = ret
	}

	var reqs []*http.Request
	origin := httpctx.GetBareRequestBytes(req)
	valueIndex := 0
	err = cartesian.ProductEx([][]string{{keyStr}, InterfaceToFuzzResults(val)}, func(result []string) error {
		value := result[1]
		// var replaced = valueToJsonValue(value)

		modifiedParams, err := modifyJSONValue(originValue, jsonPath, value, val, valueIndex, f.noEscapeHTML)
		if err != nil {
			return err
		}
		if f.friendlyDisplay {
			modifiedParams = fmt.Sprintf("{{base64(%s)}}", modifiedParams)
		} else {
			modifiedParams = codec.EncodeBase64(modifiedParams)
		}

		vals.DisableAutoEncode(f.NoAutoEncode())
		vals.SetFriendlyDisplay(f.friendlyDisplay)
		vals.Set(keyStr, modifiedParams)
		reqIns, err := lowhttp.ParseBytesToHttpRequest(lowhttp.ReplaceHTTPPacketQueryParamRaw(origin, vals.Encode()))
		if err != nil {
			log.Errorf("parse (in FuzzGetParams) request failed: %v", err)
			return nil
		}
		reqs = append(reqs, reqIns)
		return nil
	})
	if err != nil {
		return nil, utils.Errorf("cartesian.ProductEx: %s", err)
	}
	return reqs, nil
}

func (f *FuzzHTTPRequest) toFuzzHTTPRequestBatch() *FuzzHTTPRequestBatch {
	return &FuzzHTTPRequestBatch{fallback: f, originRequest: f, noAutoEncode: f.noAutoEncode}
}

func (f *FuzzHTTPRequest) FuzzGetBase64Params(key, val any) FuzzHTTPRequestIf {
	f.position = lowhttp.PosGetQueryBase64
	encode := func(v any) string {
		if f.friendlyDisplay {
			return fmt.Sprintf("{{base64(%s)}}", v)
		}
		return codec.EncodeBase64(v)
	}

	reqs, err := f.fuzzGetParams(key, val, encode)
	if err != nil {
		return f.toFuzzHTTPRequestBatch()
	}
	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) FuzzPostBase64Params(key, val any) FuzzHTTPRequestIf {
	encode := func(v any) string {
		if f.friendlyDisplay {
			return fmt.Sprintf("{{base64(%s)}}", v)
		}
		return codec.EncodeBase64(v)
	}
	reqs, err := f.fuzzPostParams(key, val, encode)
	if err != nil {
		return f.toFuzzHTTPRequestBatch()
	}
	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) FuzzCookieBase64(key, val any) FuzzHTTPRequestIf {
	encode := func(v any) string {
		if f.friendlyDisplay {
			return fmt.Sprintf("{{base64(%s)}}", v)
		}
		return codec.EncodeBase64(v)
	}
	reqs, err := f.fuzzCookie(key, val, encode)
	if err != nil {
		return f.toFuzzHTTPRequestBatch()
	}
	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) FuzzGetBase64JsonPath(key any, jsonPath string, val any) FuzzHTTPRequestIf {
	reqs, err := f.fuzzGetBase64JsonPath(key, jsonPath, val)
	if err != nil {
		return f.toFuzzHTTPRequestBatch()
	}
	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) FuzzPostBase64JsonPath(key any, jsonPath string, val any) FuzzHTTPRequestIf {
	reqs, err := f.fuzzPostBase64JsonPath(key, jsonPath, val)
	if err != nil {
		return f.toFuzzHTTPRequestBatch()
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

	return f.toFuzzHTTPRequestIf(reqs)
}

func (f *FuzzHTTPRequestBatch) FuzzGetBase64JsonPath(key any, jsonPath string, val any) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzGetBase64JsonPath(key, jsonPath, val)
	}

	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzGetBase64JsonPath(key, jsonPath, val))
	}

	return f.toFuzzHTTPRequestIf(reqs)
}
