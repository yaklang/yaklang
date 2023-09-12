package mutate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/asaskevich/govalidator"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/mixer"
	"github.com/yaklang/yaklang/common/yak/cartesian"

	"github.com/davecgh/go-spew/spew"
)

var dump = spew.Dump

func QuickMutateSimple(target ...string) []string {
	var finalResults []string
	for _, targetItem := range target {
		retResults, err := QuickMutate(targetItem, nil)
		if err != nil {
			finalResults = append(finalResults, targetItem)
			continue
		}
		finalResults = append(finalResults, retResults...)
	}
	return finalResults
}

func InterfaceToFuzzResults(value interface{}, conds ...*RegexpMutateCondition) []string {
	switch ret := value.(type) {
	case []byte:
		return InterfaceToFuzzResults(string(ret), conds...)
	case string:
		results, err := QuickMutate(ret, consts.GetGormProfileDatabase(), conds...)
		if err != nil {
			log.Errorf("quick mutate string failed: %s", err)
		}
		return results
	case []string:
		return funk.FlatMap(funk.Map(ret, func(i string) []string {
			return InterfaceToFuzzResults(i)
		}), func(v []string) []string {
			return v
		}).([]string)
	case []interface{}:
		return InterfaceToFuzzResults(funk.Map(ret, func(i interface{}) string {
			return utils.InterfaceToString(i)
		}))
	default:
		return InterfaceToFuzzResults(utils.InterfaceToString(value), conds...)
	}
}

func (f *FuzzHTTPRequest) Results() ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}
	if req != nil {
		req = _fixReq(req, f.isHttps)
		return []*http.Request{req}, nil
	}
	return nil, utils.Errorf("BUG: http.Request is nil...")
}

func reqToOpts(req *http.Request) []BuildFuzzHTTPRequestOption {
	if req == nil {
		return nil
	}

	if req.URL != nil {
		if req.URL.Scheme == "https" {
			return []BuildFuzzHTTPRequestOption{OptHTTPS(true)}
		}
	}
	return nil
}

func _fixReq(req *http.Request, isHttps bool) *http.Request {
	if isHttps {
		req.URL.Scheme = "https"
	} else {
		req.URL.Scheme = "http"
	}

	if req.Host == "" {
		req.Host = req.Header.Get("Host")
		req.URL.Host = req.Header.Get("Host")
	}

	// fix: single "Connection: close"
	if connection, ok := req.Header["Connection"]; ok {
		if len(connection) > 0 {
			req.Header["Connection"] = []string{connection[0]}
		}
	}

	return req
}

func (f *FuzzHTTPRequest) fuzzMethod(methods ...string) ([]*http.Request, error) {
	req, err := lowhttp.ParseBytesToHttpRequest(f.originRequest)
	if err != nil {
		return nil, utils.Errorf("BUG: fetch origin request failed: %s", err)
	}
	_ = req

	var reqs []*http.Request
	for _, method := range methods {
		newReq, err := rebuildHTTPRequest(req, 0)
		newReq.Method = method
		if err != nil {
			log.Errorf("invalid method: %v to fuzz: %v", method, err)
			continue
		}
		reqs = append(reqs, newReq)
	}
	return reqs, nil
}

func (f *FuzzHTTPRequest) Repeat(i int) FuzzHTTPRequestIf {
	var r = make([]int, i)
	var req []*http.Request
	for range r {
		nReq, err := lowhttp.ParseBytesToHttpRequest(f.originRequest)
		if err != nil {
			continue
		}
		req = append(req, nReq)
	}

	return NewFuzzHTTPRequestBatch(f, req...)
}

func (f *FuzzHTTPRequest) FuzzMethod(methods ...string) FuzzHTTPRequestIf {
	reqs, err := f.fuzzMethod(methods...)
	if err != nil {
		return &FuzzHTTPRequestBatch{fallback: f, originRequest: f}
	}

	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) fuzzPath(paths ...string) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}

	var pathTotal = QuickMutateSimple(paths...)
	var reqs []*http.Request
	var originUrl = req.URL
	req.RequestURI = ""

	for _, targetPath := range pathTotal {
		if !strings.HasPrefix(targetPath, "/") {
			targetPath = "/" + targetPath
		}
		var _req *http.Request
		if strings.HasPrefix(targetPath, "https://") || strings.HasPrefix(targetPath, "http://") {
			targetUrl, err := url.Parse(targetPath)
			if err != nil {
				log.Errorf("parse url[%v] failed: %s", targetPath, err)
				continue
			}

			req.URL = targetUrl
			_req, err = rebuildHTTPRequest(req, 0)
			if err != nil {
				log.Errorf("rebuild http request[%s] failed: %s", req.URL, err)
				continue
			}
			reqs = append(reqs, _req)
			req.URL = originUrl
			continue
		} else {
			_req, err = rebuildHTTPRequest(req, 0)
			if err != nil {
				log.Errorf("rebuild http request path[%s] failed: %s", req.URL, err)
				continue
			}
			vals := _req.URL.Query()
			if vals == nil || len(vals) <= 0 {
				vals = make(url.Values)
			}
			if strings.Contains(targetPath, "?") {
				path, query, _ := strings.Cut(targetPath, "?")
				if query != "" {
					if newVals, err := url.ParseQuery(query); err == nil {
						for k, v := range newVals {
							if len(v) > 0 {
								vals.Set(k, v[0])
							}
						}
					}
				}
				_req.URL.Path = path
				_req.URL.RawQuery = vals.Encode()
			} else {
				_req.URL.Path = targetPath
			}
			_req.RequestURI = ""
			reqs = append(reqs, _req)
			continue
		}
	}
	return reqs, nil
}

func (f *FuzzHTTPRequest) FuzzPath(paths ...string) FuzzHTTPRequestIf {
	reqs, err := f.fuzzPath(paths...)
	if err != nil {
		return &FuzzHTTPRequestBatch{fallback: f, originRequest: f}
	}

	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) FuzzPathAppend(paths ...string) FuzzHTTPRequestIf {
	path := f.GetPath()
	if path == "" {
		path = "/"
	}
	var result = make([]string, len(paths))
	for i, v := range paths {
		result[i] = path + v
	}
	return f.FuzzPath(result...)
}

func (f *FuzzHTTPRequest) fuzzHTTPHeader(key interface{}, value interface{}) ([]*http.Request, error) {
	var keys, values []string = InterfaceToFuzzResults(key), InterfaceToFuzzResults(value)
	if len(keys) <= 0 {
		return nil, utils.Errorf("empty HTTP Header keys")
	}

	if len(values) <= 0 {
		return nil, utils.Errorf("empty HTTP Header Values")
	}

	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, utils.Errorf("init origin request failed: %s", err)
	}

	m, err := mixer.NewMixer(keys, values)
	if err != nil {
		return nil, utils.Errorf("create mixer failed: %s", err)
	}

	var reqs []*http.Request
	var originHeader = req.Header
	for {
		vals := m.Value()
		req.Header, _ = deepCopyHeader(originHeader)
		key, value := vals[0], vals[1]
		if value == "" {
			req.Header.Del(key)
		} else {
			req.Header[key] = []string{value}
		}
		switch strings.ToLower(key) {
		case "host":
			req.Host = value
		case "transfer-encoding":
			if strings.Contains(strings.ToLower(fmt.Sprint(value)), "chunked") {
				f.chunked = true
			}
		}

		_req, _ := rebuildHTTPRequest(req, 0)
		if _req != nil {
			reqs = append(reqs, _req)
		}
		req.Header = originHeader

		err := m.Next()
		if err != nil {
			break
		}
	}

	return reqs, nil
}

func (f *FuzzHTTPRequest) FuzzHTTPHeader(k, v interface{}) FuzzHTTPRequestIf {
	reqs, err := f.fuzzHTTPHeader(k, v)
	if err != nil {
		return &FuzzHTTPRequestBatch{fallback: f, originRequest: f}
	}

	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) fuzzGetParamsRaw(queryRaw ...string) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}

	var raws = QuickMutateSimple(queryRaw...)

	originRawQuery := req.URL.RawQuery

	var reqs []*http.Request
	for _, targetQuery := range raws {
		req.RequestURI = ""
		req.URL.RawQuery = targetQuery
		_req, err := rebuildHTTPRequest(req, 0)
		req.URL.RawQuery = originRawQuery
		if err != nil {
			continue
		}
		reqs = append(reqs, _req)
	}
	return reqs, nil
}

func (f *FuzzHTTPRequest) FuzzGetParamsRaw(queryRaws ...string) FuzzHTTPRequestIf {
	reqs, err := f.fuzzGetParamsRaw(queryRaws...)
	if err != nil {
		return &FuzzHTTPRequestBatch{fallback: f, originRequest: f}
	}
	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) FuzzGetJsonPathParams(key any, jsonPath string, val any) FuzzHTTPRequestIf {
	reqs, err := f.fuzzGetParamsJsonPath(key, jsonPath, val)
	if err != nil {
		return &FuzzHTTPRequestBatch{fallback: f, originRequest: f}
	}
	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) FuzzPostJsonPathParams(key any, jsonPath string, val any) FuzzHTTPRequestIf {
	reqs, err := f.fuzzPostParamsJsonPath(key, jsonPath, val)
	if err != nil {
		return &FuzzHTTPRequestBatch{fallback: f, originRequest: f}
	}
	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) fuzzPostParamsJsonPath(key any, jsonPath string, val any) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}

	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(f.originRequest)
	if body == nil {
		return nil, errors.New("empty body")
	}

	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, errors.New("parse body failed")
	}

	var reqs []*http.Request
	var keyStr = utils.InterfaceToString(key)
	keys, values := []string{keyStr}, InterfaceToFuzzResults(val)
	var valueOrigin = vals.Get(keyStr)
	if valueOrigin == "" {
		return nil, utils.Errorf("empty HTTP Get Params[%v] Values", key)
	}
	rawJson, isJsonOk := utils.IsJSON(valueOrigin)
	if !isJsonOk {
		return nil, utils.Errorf("HTTP Get Params[%v] Values is not json", key)
	}

	err = cartesian.ProductEx([][]string{keys, values}, func(result []string) error {
		key, value := result[0], result[1]
		var replacedValue []string
		if govalidator.IsIn(value) {
			replacedValue = append(replacedValue, jsonpath.ReplaceString(rawJson, jsonPath, codec.Atoi(value)))
		}
		if govalidator.IsFloat(value) {
			replacedValue = append(replacedValue, jsonpath.ReplaceString(rawJson, jsonPath, codec.Atof(value)))
		}
		if value == `true` || value == `false` {
			replacedValue = append(replacedValue, jsonpath.ReplaceString(rawJson, jsonPath, codec.Atob(value)))
		}
		replacedValue = append(replacedValue, jsonpath.ReplaceString(rawJson, jsonPath, value))

		for _, i := range replacedValue {
			newReq := lowhttp.CopyRequest(req)
			if newReq == nil {
				continue
			}

			newVals, err := deepCopyUrlValues(vals)
			if err != nil {
				continue
			}
			newVals.Set(key, i)
			newReq.Body = ioutil.NopCloser(bytes.NewBufferString(newVals.Encode()))
			reqs = append(reqs, newReq)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return reqs, nil
}

func (f *FuzzHTTPRequest) fuzzGetParamsJsonPath(key any, jsonPath string, val any) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}

	vals := req.URL.Query()
	if vals == nil {
		vals = make(url.Values)
	}
	keyStr := utils.InterfaceToString(key)
	valueOrigin := vals.Get(keyStr)
	if valueOrigin == "" {
		return nil, utils.Errorf("empty HTTP Get Params[%v] Values", key)
	}
	rawJson, isJsonOk := utils.IsJSON(valueOrigin)
	if !isJsonOk {
		return nil, utils.Errorf("HTTP Get Params[%v] Values is not json", key)
	}

	var reqs []*http.Request
	keys, values := []string{keyStr}, InterfaceToFuzzResults(val)
	err = cartesian.ProductEx([][]string{keys, values}, func(result []string) error {
		key, value := result[0], result[1]
		var replacedValue = []string{
			jsonpath.ReplaceString(rawJson, jsonPath, value),
		}
		if govalidator.IsInt(value) {
			replacedValue = append(replacedValue, jsonpath.ReplaceString(rawJson, jsonPath, codec.Atoi(value)))
		}
		for _, i := range replacedValue {
			newReq := lowhttp.CopyRequest(req)
			if newReq == nil {
				continue
			}
			newVals := newReq.URL.Query()
			newVals.Set(key, i)
			newReq.URL.RawQuery = newVals.Encode()
			newReq.RequestURI = newReq.URL.RequestURI()
			reqs = append(reqs, newReq)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return reqs, nil
}

func (f *FuzzHTTPRequest) fuzzGetParams(key interface{}, value interface{}, encoded ...EncodedFunc) ([]*http.Request, error) {
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
		key, value := pairs[0], pairs[1]
		for _, e := range encoded {
			value = e(value)
		}

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

func (f *FuzzHTTPRequest) FuzzGetParams(k, v interface{}) FuzzHTTPRequestIf {
	reqs, err := f.fuzzGetParams(k, v)
	if err != nil {
		return &FuzzHTTPRequestBatch{fallback: f, originRequest: f}
	}

	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) fuzzPostRaw(body ...string) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}

	var raws = QuickMutateSimple(body...)
	var reqs []*http.Request
	for _, tBody := range raws {
		_req, err := rebuildHTTPRequest(req, int64(len(tBody)))
		if err != nil {
			continue
		}
		_req.Body = ioutil.NopCloser(bytes.NewBufferString(tBody))
		reqs = append(reqs, _req)
	}
	return reqs, nil
}

func (f *FuzzHTTPRequest) FuzzPostRaw(body ...string) FuzzHTTPRequestIf {
	reqs, err := f.fuzzPostRaw(body...)
	if err != nil {
		return &FuzzHTTPRequestBatch{fallback: f, originRequest: f}
	}

	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) fuzzPostParams(k, v interface{}, encoded ...EncodedFunc) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}

	var vals url.Values
	var rawBody []byte
	if req.Body != nil {
		rawBody = httpRequestReadBody(req)
		if rawBody != nil {
			vals, _ = url.ParseQuery(string(rawBody))
		}
	}
	if vals == nil {
		vals = make(url.Values)
	}

	keys, values := InterfaceToFuzzResults(k), InterfaceToFuzzResults(v)
	if keys == nil || values == nil {
		return nil, utils.Errorf("empty keys or Values")
	}

	m, err := mixer.NewMixer(keys, values)
	if err != nil {
		return nil, utils.Errorf("BUG: mixer failed: %s", err)
	}

	var reqs []*http.Request
	for {
		pair := m.Value()
		key, value := pair[0], pair[1]
		for _, e := range encoded {
			value = e(value)
		}

		newVals, _ := deepCopyUrlValues(vals)
		if newVals != nil {
			newVals.Set(key, value)
			raw := newVals.Encode()
			newBody := bytes.NewBufferString(raw)
			_req, _ := rebuildHTTPRequest(req, int64(len(raw)))
			_req.Body = ioutil.NopCloser(newBody)
			if _req != nil {
				reqs = append(reqs, _req)
			}
		}

		err = m.Next()
		if err != nil {
			break
		}
	}

	return reqs, nil
}

func (f *FuzzHTTPRequest) FuzzPostParams(k, v interface{}) FuzzHTTPRequestIf {
	reqs, err := f.fuzzPostParams(k, v)
	if err != nil {
		return &FuzzHTTPRequestBatch{fallback: f, originRequest: f}
	}
	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) fuzzPostJsonParams(k, v interface{}) ([]*http.Request, error) {
	switch param := k.(type) {
	case *FuzzHTTPRequestParam:
		return f.fuzzPostJsonParamsWithFuzzParam(param, v)
	default:
		return f.fuzzPostJsonParamsWithRaw(k, v)
	}
}

func (f *FuzzHTTPRequest) fuzzPostJsonParamsWithFuzzParam(p *FuzzHTTPRequestParam, originValue interface{}) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}

	var rawBody []byte
	if req.Body != nil {
		rawBody = httpRequestReadBody(req)
	}

	if rawBody == nil {
		rawBody = []byte("{}")
	}

	values := InterfaceToFuzzResults(originValue)
	if values == nil {
		return nil, utils.Errorf("key or value is empty...")
	}

	var reqs []*http.Request

	err = cartesian.ProductEx([][]string{
		values,
	}, func(result []string) error {
		value := result[0]
		raw := jsonpath.ReplaceString(string(rawBody), p.jsonPath, value)
		_req, _ := rebuildHTTPRequest(req, int64(len(raw)))
		_req.Body = io.NopCloser(bytes.NewBufferString(raw))
		if _req != nil {
			reqs = append(reqs, _req)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return reqs, nil
}

func (f *FuzzHTTPRequest) fuzzPostJsonParamsWithRaw(k, v interface{}) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}

	var rawBody []byte
	if req.Body != nil {
		rawBody = httpRequestReadBody(req)
	}

	if rawBody == nil {
		rawBody = []byte("{}")
	}

	var originParam map[string]interface{}
	_ = json.Unmarshal(bytes.TrimSpace(rawBody), &originParam)
	if originParam == nil {
		originParam = make(map[string]interface{})
	}

	keys, values := InterfaceToFuzzResults(k), InterfaceToFuzzResults(v)
	if keys == nil || values == nil {
		return nil, utils.Errorf("key or value is empty...")
	}

	m, err := mixer.NewMixer(keys, values)
	if err != nil {
		return nil, err
	}

	var reqs []*http.Request
	for {
		pair := m.Value()
		key, value := pair[0], pair[1]
		newParam, _ := deepCopyMapRaw(originParam)
		if newParam == nil {
			newParam = make(map[string]interface{})
		}

		var isInteger bool
		var isFloat bool
		originValue, ok := newParam[key]
		if ok {
			isInteger = utils.IsValidInteger(fmt.Sprintf("%#v", originValue))
			isFloat = utils.IsValidFloat(fmt.Sprintf("%#v", originValue))
		}

		if utils.IsValidInteger(value) {
			forkedMap, _ := deepCopyMapRaw(newParam)
			if forkedMap != nil {
				forkedMap[key] = codec.Atoi(value)
				raw, _ := json.Marshal(forkedMap)
				_req, _ := rebuildHTTPRequest(req, int64(len(raw)))
				_req.Body = ioutil.NopCloser(bytes.NewBuffer(raw))
				if _req != nil {
					reqs = append(reqs, _req)
				}
			}
		}

		if utils.IsValidFloat(value) && strings.Contains(strings.Trim(value, `.`), ".") {
			forkedMap, _ := deepCopyMapRaw(newParam)
			if forkedMap != nil {
				forkedMap[key] = codec.Atof(value)
				raw, _ := json.Marshal(forkedMap)
				_req, _ := rebuildHTTPRequest(req, int64(len(raw)))
				_req.Body = ioutil.NopCloser(bytes.NewBuffer(raw))
				if _req != nil {
					reqs = append(reqs, _req)
				}
			}
		}

		if value == "true" || value == "false" {
			forkedMap, _ := deepCopyMapRaw(newParam)
			if forkedMap != nil {
				forkedMap[key] = codec.Atob(value)
				raw, _ := json.Marshal(forkedMap)
				_req, _ := rebuildHTTPRequest(req, int64(len(raw)))
				_req.Body = io.NopCloser(bytes.NewBuffer(raw))
				if _req != nil {
					reqs = append(reqs, _req)
				}
			}
		}

		switch true {
		case isFloat:
			fallthrough
		case isInteger && isFloat:
			// isNumber
			f, err := strconv.ParseFloat(value, 64)
			if err != nil {
				newParam[key] = value
				break
			}
			newParam[key] = f
		case isInteger:
			i, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				newParam[key] = value
				break
			}
			newParam[key] = i
		default:
			newParam[key] = value
		}

		raw, _ := json.Marshal(newParam)
		_req, _ := rebuildHTTPRequest(req, int64(len(raw)))
		_req.Body = ioutil.NopCloser(bytes.NewBuffer(raw))
		if _req != nil {
			reqs = append(reqs, _req)
		}

		if err = m.Next(); err != nil {
			break
		}
	}
	if rawBody != nil {
		req.Body = ioutil.NopCloser(bytes.NewReader(rawBody))
	}
	return reqs, nil
}

func (f *FuzzHTTPRequest) FuzzPostJsonParams(k, v interface{}) FuzzHTTPRequestIf {
	reqs, err := f.fuzzPostJsonParams(k, v)
	if err != nil {
		return &FuzzHTTPRequestBatch{fallback: f, originRequest: f}
	}
	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) FuzzCookieRaw(v interface{}) FuzzHTTPRequestIf {
	return f.FuzzHTTPHeader("Cookie", v)
}

func (f *FuzzHTTPRequest) FuzzCookie(k, v interface{}) FuzzHTTPRequestIf {
	reqs, err := f.fuzzCookie(k, v)
	if err != nil {
		return &FuzzHTTPRequestBatch{fallback: f, originRequest: f}
	}
	r := NewFuzzHTTPRequestBatch(f, reqs...)
	return r
}

func (f *FuzzHTTPRequest) FuzzFormEncoded(k, v interface{}) FuzzHTTPRequestIf {
	reqs, err := f.fuzzFormEncoded(k, v)
	if err != nil {
		return &FuzzHTTPRequestBatch{fallback: f, originRequest: f}
	}
	r := NewFuzzHTTPRequestBatch(f, reqs...)
	return r
}

func (f *FuzzHTTPRequest) FuzzUploadFile(k, v interface{}, raw []byte) FuzzHTTPRequestIf {
	reqs, err := f.fuzzUploadedFile(k, v, raw)
	if err != nil {
		return &FuzzHTTPRequestBatch{fallback: f, originRequest: f}
	}
	r := NewFuzzHTTPRequestBatch(f, reqs...)
	return r
}

func (f *FuzzHTTPRequest) FuzzUploadKVPair(k, v interface{}) FuzzHTTPRequestIf {
	reqs, err := f.fuzzMultipartKeyValue(k, v)
	if err != nil {
		return &FuzzHTTPRequestBatch{fallback: f, originRequest: f}
	}
	r := NewFuzzHTTPRequestBatch(f, reqs...)
	return r
}

func (f *FuzzHTTPRequest) FuzzUploadFileName(k, v interface{}) FuzzHTTPRequestIf {
	reqs, err := f.fuzzUploadedFileName(k, v)
	if err != nil {
		return &FuzzHTTPRequestBatch{fallback: f, originRequest: f}
	}
	r := NewFuzzHTTPRequestBatch(f, reqs...)
	return r
}

func (f *FuzzHTTPRequest) fuzzCookie(k, v interface{}, encoded ...EncodedFunc) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}

	var cookies = new(sync.Map)
	for _, c := range req.Cookies() {
		cookies.Store(c.Name, c)
	}

	keys := InterfaceToFuzzResults(k)
	values := InterfaceToFuzzResults(v)
	if keys == nil || values == nil {
		return nil, utils.Errorf("keys or Values is empty...")
	}

	m, err := mixer.NewMixer(keys, values)
	if err != nil {
		return nil, err
	}

	var reqs []*http.Request
	for {
		pair := m.Value()
		key, value := pair[0], pair[1]
		for _, e := range encoded {
			value = e(value)
		}
		newCookie, _ := deepCopySyncMapCookie(cookies)
		if newCookie == nil {
			newCookie = new(sync.Map)
		}
		newCookie.Store(key, &http.Cookie{Name: key, Value: value})

		_req, _ := rebuildHTTPRequest(req, 0)
		// 增加新的 Cookie
		_req.Header.Del("Cookie")
		newCookie.Range(func(key, value interface{}) bool {
			c, _ := value.(*http.Cookie)
			if c == nil {
				return true
			}

			_req.AddCookie(c)
			return true
		})
		if _req != nil {
			reqs = append(reqs, _req)
		}

		err = m.Next()
		if err != nil {
			break
		}
	}
	return reqs, nil
}

func (f *FuzzHTTPRequest) Show() FuzzHTTPRequestIf {
	reqs, err := f.Results()
	if err != nil {
		log.Errorf("fetch results failed: %s", err)
	}

	for _, req := range reqs {
		utils.HttpShow(req)
	}
	return f
}

func (f *FuzzHTTPRequest) ExecFirst(opts ...HttpPoolConfigOption) (*_httpResult, error) {
	opts = append(opts, WithPoolOpt_RequestCountLimiter(1))
	resultCh, err := f.Exec(opts...)
	if err != nil {
		return nil, err
	}

	var result *_httpResult
	for i := range resultCh {
		result = i
	}
	if result == nil {
		return nil, utils.Error("empty result for FuzzHTTPRequest")
	}
	if result.Error != nil {
		return result, result.Error
	}

	return result, nil
}
