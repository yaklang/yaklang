package mutate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"yaklang.io/yaklang/common/consts"
	"yaklang.io/yaklang/common/go-funk"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/utils/lowhttp"
	"yaklang.io/yaklang/common/utils/mixer"

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
	var values []string
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
	}
	return values
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
	var originUrlPath = req.URL.Path
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
			req.URL.Path = targetPath
			_req, err = rebuildHTTPRequest(req, 0)
			if err != nil {
				log.Errorf("rebuild http request path[%s] failed: %s", req.URL, err)
				continue
			}
			req.URL.Path = originUrlPath
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

func (f *FuzzHTTPRequest) fuzzGetParams(key interface{}, value interface{}) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}
	vals := req.URL.Query()
	if vals == nil {
		vals = make(url.Values)
	}

	//originVals, err := deepCopyUrlValues(vals)
	//if err != nil {
	//	return nil, utils.Errorf("copy url.Values failed: %s", err.Error())
	//}

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

func (f *FuzzHTTPRequest) fuzzPostParams(k, v interface{}) ([]*http.Request, error) {
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

	var originParam map[string]interface{}
	_ = json.Unmarshal(bytes.TrimSpace(rawBody), &originParam)
	if originParam == nil {
		originParam = make(map[string]interface{})
	}

	keys, values := InterfaceToFuzzResults(p.param), InterfaceToFuzzResults(originValue)
	if keys == nil || values == nil {
		return nil, utils.Errorf("key or value is empty...")
	}

	m, err := mixer.NewMixer(keys, values)
	if err != nil {
		return nil, err
	}

	// find last map
	tempParam := originParam
	splitJsonPath := strings.Split(p.jsonPath, ".")
	for _, path := range strings.Split(p.jsonPath, ".")[:len(splitJsonPath)-1] {
		val, ok := tempParam[path]
		if !ok {
			return nil, utils.Errorf("no such post-json params: %s", p.jsonPath)
		}
		switch v := val.(type) {
		case map[string]interface{}:
			tempParam = v
		default:
			return nil, utils.Errorf("no such post-json params: %s", p.jsonPath)
		}
	}

	var reqs []*http.Request
	for {
		pair := m.Value()
		key, value := pair[0], pair[1]

		tempParam[key] = value
		raw, _ := json.Marshal(originParam)
		_req, _ := rebuildHTTPRequest(req, int64(len(raw)))
		_req.Body = ioutil.NopCloser(bytes.NewBuffer(raw))
		if _req != nil {
			reqs = append(reqs, _req)
		}
		tempParam[key] = originValue

		if err = m.Next(); err != nil {
			break
		}
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

func (f *FuzzHTTPRequest) fuzzCookie(k, v interface{}) ([]*http.Request, error) {
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
	return executeOne(f, opts...)
}
