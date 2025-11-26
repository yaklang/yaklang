package mutate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/antchfx/xmlquery"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

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
		req = f._fixReq(req, f.isHttps)
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

func (f *FuzzHTTPRequest) _fixReq(req *http.Request, isHttps bool) *http.Request {
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
		if len(connection) > 1 {
			f.mutex.Lock()
			defer f.mutex.Unlock()

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
	r := make([]int, i)
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
		return f.toFuzzHTTPRequestBatch()
	}

	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) fuzzPath(paths ...string) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}

	pathTotal := QuickMutateSimple(paths...)

	results := make([]*http.Request, 0, len(pathTotal))
	origin := httpctx.GetBareRequestBytes(req)
	pos := lowhttp.PosGetQuery
	if f.queryParams == nil || f.queryParams.IsEmpty() || f.mode == paramsFuzz {
		f.queryParams = lowhttp.ParseQueryParams(
			f.GetQueryRaw(),
			lowhttp.WithDisableAutoEncode(f.noAutoEncode),
			lowhttp.WithFriendlyDisplay(f.friendlyDisplay),
			lowhttp.WithPosition(pos),
		)
	}

	for _, targetPath := range pathTotal {
		var path, query string
		var params *lowhttp.QueryParams
		if strings.Contains(targetPath, "?") {
			path, query, _ = strings.Cut(targetPath, "?")
			if query != "" {
				// 每个 path?query 应该都是独立的 queryParams
				params = lowhttp.ParseQueryParams(query)
				params.SetPosition(pos)
				params.SetFriendlyDisplay(f.friendlyDisplay)
				params.DisableAutoEncode(f.noAutoEncode)
				for _, v := range params.Items {
					params.Set(v.Key, v.Value)
				}
				f.queryParams.SetPosition(pos)
				f.queryParams.SetFriendlyDisplay(f.friendlyDisplay)
				f.queryParams.DisableAutoEncode(f.noAutoEncode)
				for _, v := range f.queryParams.Items {
					params.Set(v.Key, v.Value)
				}
			}
		} else {
			path = targetPath
		}
		var replaced []byte
		if f.NoAutoEncode() {
			replaced = lowhttp.ReplaceHTTPPacketPathWithoutEncoding(origin, path)
		} else {
			replaced = lowhttp.ReplaceHTTPPacketPath(origin, path)
		}
		var reqIns *http.Request
		if params != nil {
			reqIns, err = lowhttp.ParseBytesToHttpRequest(
				lowhttp.ReplaceHTTPPacketQueryParamRaw(replaced, params.EncodeByPos(pos)),
			)
		} else {
			reqIns, err = lowhttp.ParseBytesToHttpRequest(replaced)
		}
		if err != nil {
			log.Infof("parse (in FuzzPath) request failed: %v", err)
			continue
		}
		results = append(results, reqIns)
	}
	if len(results) == 0 {
		return []*http.Request{req}, nil
	}
	return results, nil
}

func (f *FuzzHTTPRequest) FuzzPath(paths ...string) FuzzHTTPRequestIf {
	f.position = lowhttp.PosPath
	reqs, err := f.fuzzPath(paths...)
	if err != nil {
		return f.toFuzzHTTPRequestBatch()
	}

	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) FuzzPathAppend(paths ...string) FuzzHTTPRequestIf {
	path := f.GetPath()
	if path == "" {
		path = "/"
	}
	result := make([]string, len(paths))
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
	origin := httpctx.GetBareRequestBytes(req)
	for {
		vals := m.Value()
		key, value := vals[0], vals[1]

		reqIns, err := lowhttp.ParseBytesToHttpRequest(lowhttp.ReplaceHTTPPacketHeaderStrict(origin, key, value))
		if err != nil {
			log.Infof("parse (in FuzzHTTPHeader) request failed: %v", err)
			continue
		}
		switch strings.ToLower(key) {
		case "host":
			reqIns.Host = value
		case "transfer-encoding":
			if strings.Contains(strings.ToLower(fmt.Sprint(value)), "chunked") {
				f.chunked = true
			}
		}
		reqs = append(reqs, reqIns)

		err = m.Next()
		if err != nil {
			break
		}
	}

	return reqs, nil
}

func (f *FuzzHTTPRequest) FuzzHTTPHeader(k, v interface{}) FuzzHTTPRequestIf {
	reqs, err := f.fuzzHTTPHeader(k, v)
	if err != nil {
		return f.toFuzzHTTPRequestBatch()
	}

	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) fuzzGetParamsRaw(queryRaw ...string) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}

	raws := QuickMutateSimple(queryRaw...)
	if f.NoAutoEncode() {
		originBytes := f.GetBytes()
		method, pathStr, proto := lowhttp.GetHTTPPacketFirstLine(originBytes)
		if !strings.HasPrefix(proto, "HTTP/") {
			proto = "HTTP/1.1"
		}

		originPath, _, _ := strings.Cut(pathStr, "?")
		results := make([]*http.Request, 0, len(raws))
		for i := 0; i < len(raws); i++ {
			rawQuery := raws[i]
			if rawQuery == "" {
				continue
			}
			firstLine := fmt.Sprintf("%v %v?%v %v", method, originPath, rawQuery, proto)
			reqInstance, err := lowhttp.ParseBytesToHttpRequest(lowhttp.ReplaceHTTPPacketFirstLine(originBytes, firstLine))
			if err != nil {
				log.Infof("parse (in FuzzGetParamsRaw) request failed: %v", err)
				continue
			}
			results = append(results, reqInstance)
		}
		if len(results) <= 0 {
			return []*http.Request{req}, nil
		}
		return results, nil
	}

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
		return f.toFuzzHTTPRequestBatch()
	}
	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) FuzzGetJsonPathParams(key any, jsonPath string, val any) FuzzHTTPRequestIf {
	f.position = lowhttp.PosGetQueryJson
	reqs, err := f.fuzzGetParamsJsonPath(key, jsonPath, val)
	if err != nil {
		return f.toFuzzHTTPRequestBatch()
	}
	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) FuzzPostJsonPathParams(key any, jsonPath string, val any) FuzzHTTPRequestIf {
	f.position = lowhttp.PosPostQueryJson
	reqs, err := f.fuzzPostParamsJsonPath(key, jsonPath, val)
	if err != nil {
		return f.toFuzzHTTPRequestBatch()
	}
	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) fuzzPostParamsJsonPath(key any, jsonPath string, val any) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}
	pos := f.position
	if f.queryParams == nil || f.queryParams.IsEmpty() || f.mode == paramsFuzz {
		f.queryParams = lowhttp.ParseQueryParams(
			f.GetPostQuery(),
			lowhttp.WithDisableAutoEncode(f.noAutoEncode),
			lowhttp.WithFriendlyDisplay(f.friendlyDisplay),
			lowhttp.WithPosition(pos),
		)
	}

	keyStr := utils.InterfaceToString(key)
	keys, values := []string{keyStr}, InterfaceToFuzzResults(val)

	valueOrigin := f.queryParams.Get(keyStr)

	if valueOrigin == "" {
		return nil, utils.Errorf("empty HTTP Post Params[%v] Values", key)
	}
	rawJson, isJsonOk := utils.IsJSON(valueOrigin)
	if !isJsonOk {
		return nil, utils.Errorf("HTTP Post Params[%v] Values is not json", key)
	}

	var reqs []*http.Request
	origin := httpctx.GetBareRequestBytes(req)
	valueIndex := 0
	err = cartesian.ProductEx([][]string{keys, values}, func(result []string) error {
		key, value := result[0], result[1]

		modifiedParams, err := modifyJSONValue(rawJson, jsonPath, value, val, valueIndex)
		if err != nil {
			return err
		}
		f.queryParams.SetPosition(pos)
		f.queryParams.SetFriendlyDisplay(f.friendlyDisplay)
		f.queryParams.DisableAutoEncode(f.noAutoEncode)
		f.queryParams.Set(key, modifiedParams)

		reqIns, err := lowhttp.ParseBytesToHttpRequest(
			lowhttp.ReplaceHTTPPacketBodyFast(origin, []byte(f.queryParams.EncodeByPos(pos))))
		if err != nil {
			return nil
		}
		reqs = append(reqs, reqIns)

		return nil
	})
	if err != nil {
		return nil, err
	}
	return reqs, nil
}

func modifyJSONValue(rawJson, jsonPath, value string, val any, index int) (string, error) {
	defer func() {
		index++
	}()
	if !strings.HasPrefix(jsonPath, "$.") && !strings.HasPrefix(jsonPath, "$[") {
		jsonPath = "$." + jsonPath
	}
	var newValue interface{} = value
	// originalValue := jsonpath.Find(rawJson, jsonPath)

	// 如果原始值为空，或者原始值类型和新值类型一致，或者原始值是float64类型，新值是int类型就进入下面的逻辑
	//if originalValue == nil ||
	//	reflect.TypeOf(originalValue) == reflect.TypeOf(val) ||
	//	(reflect.TypeOf(originalValue).AssignableTo(reflect.TypeOf(0.0)) &&
	//		reflect.TypeOf(val).AssignableTo(reflect.TypeOf(0))) {
	//	switch originalValue.(type) {
	//	case float64:
	//		if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
	//			newValue = floatVal
	//		}
	//	case bool:
	//		if boolVal, err := strconv.ParseBool(value); err == nil {
	//			newValue = boolVal
	//		}
	//	case string:
	//		newValue = value
	//	case map[string]interface{}, []interface{}:
	//		if gjson.Valid(value) {
	//			newValue = gjson.Parse(value).Value()
	//		}
	//	case nil:
	//		switch val.(type) {
	//		case string:
	//			newValue = value
	//		default:
	//			newValue = gjson.Parse(value).Value()
	//		}
	//	default:
	//		return "", utils.Wrap(errors.New("unrecognized json value type"), "json value type")
	//	}
	//} else {
	//	log.Errorf("original value type: %v, new value type: %v", reflect.TypeOf(originalValue), reflect.TypeOf(val))
	// 该不该 fuzz 类型
	// newValue = originalValue
	//}

	newValue, valIndex := handleJSONVal(value, val, index)

	// 如果原始值类型不为 nil，且新值为 nil，则说明 value 和 val 的类型可能不一致，尝试直接转换 value 为 json value
	if valIndex != nil && newValue == nil {
		jsonStr, _ := json.Marshal(value)
		newValue = gjson.ParseBytes(jsonStr).Value()
	}

	return jsonpath.ReplaceString(rawJson, jsonPath, newValue), nil
}

func handleJSONVal(value string, val any, index int) (any, any) {
	switch v := val.(type) {
	case string:
		jsonStr, _ := json.Marshal(value)
		return gjson.ParseBytes(jsonStr).Value(), v
	case nil:
		return nil, nil
	case []interface{}:
		return handleJSONVal(value, v[index], index)
	default:
		// 解析字符串时先判断是否为合法 json，如果不是则返回 nil
		// `"abcd"` 才是合法的 json 字符串，`abcd` 不是
		// `"abcd"` -> "abcd"
		// `123` -> 123 `true` -> true
		// `abcd` -> nil
		p := gjson.Parse(value).Get("@valid")
		return p.Value(), v
	}
}

func (f *FuzzHTTPRequest) fuzzGetParamsJsonPath(key any, jsonPath string, val any) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}
	pos := f.position
	if f.queryParams == nil || f.queryParams.IsEmpty() || f.mode == paramsFuzz {
		f.queryParams = lowhttp.ParseQueryParams(
			f.GetQueryRaw(),
			lowhttp.WithDisableAutoEncode(f.noAutoEncode),
			lowhttp.WithFriendlyDisplay(f.friendlyDisplay),
			lowhttp.WithPosition(pos),
		)
	}
	keyStr := utils.InterfaceToString(key)
	valueOrigin := f.queryParams.Get(keyStr)
	if valueOrigin == "" {
		return nil, utils.Errorf("empty HTTP Get Params[%v] Values", key)
	}
	rawJson, isJsonOk := utils.IsJSON(valueOrigin)
	if !isJsonOk {
		return nil, utils.Errorf("HTTP Get Params[%v] Values is not json", key)
	}

	keys, values := []string{keyStr}, InterfaceToFuzzResults(val)

	var reqs []*http.Request
	origin := httpctx.GetBareRequestBytes(req)
	valueIndex := 0
	err = cartesian.ProductEx([][]string{keys, values}, func(result []string) error {
		key, value := result[0], result[1]
		modifiedParams, err := modifyJSONValue(rawJson, jsonPath, value, val, valueIndex)
		if err != nil {
			log.Errorf("modify json value failed: %s", err)
			return nil
		}

		f.queryParams.SetPosition(pos)
		f.queryParams.SetFriendlyDisplay(f.friendlyDisplay)
		f.queryParams.DisableAutoEncode(f.noAutoEncode)
		f.queryParams.Set(key, modifiedParams)
		reqIns, err := lowhttp.ParseBytesToHttpRequest(
			lowhttp.ReplaceHTTPPacketQueryParamRaw(origin, f.queryParams.EncodeByPos(pos)))
		if err != nil {
			log.Errorf("parse (in FuzzGetParams) request failed: %v", err)
			return nil
		}

		reqs = append(reqs, reqIns)

		return nil
	})
	if err != nil {
		return nil, err
	}
	return reqs, nil
}

func (f *FuzzHTTPRequest) fuzzGetParams(key interface{}, value interface{}, encoded ...codec.EncodedFunc) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}
	pos := f.position
	if f.queryParams == nil || f.queryParams.IsEmpty() || f.mode == paramsFuzz {
		f.queryParams = lowhttp.ParseQueryParams(f.GetQueryRaw(),
			lowhttp.WithDisableAutoEncode(f.noAutoEncode),
			lowhttp.WithFriendlyDisplay(f.friendlyDisplay),
			lowhttp.WithPosition(pos),
		)

	}

	keys, values := InterfaceToFuzzResults(key), InterfaceToFuzzResults(value)
	if len(keys) <= 0 || len(values) <= 0 {
		return nil, utils.Errorf("GetQuery key or Values are empty...")
	}

	origin := httpctx.GetBareRequestBytes(req)
	results := make([]*http.Request, 0, len(keys)*len(values))
	err = cartesian.ProductEx([][]string{keys, values}, func(result []string) error {
		if len(result) != 2 {
			return utils.Error("BUG in fuzz GetQuery KeyValue")
		}

		key := result[0]
		value := result[1]
		for _, e := range encoded {
			value = e(value)
		}

		f.queryParams.SetPosition(pos)
		f.queryParams.SetFriendlyDisplay(f.friendlyDisplay)
		f.queryParams.DisableAutoEncode(f.noAutoEncode)
		f.queryParams.Set(key, value)

		reqIns, err := lowhttp.ParseBytesToHttpRequest(
			lowhttp.ReplaceHTTPPacketQueryParamRaw(origin, f.queryParams.EncodeByPos(pos)))
		if err != nil {
			log.Infof("parse (in FuzzGetParams) request failed: %v", err)
			return nil
		}
		results = append(results, reqIns)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(results) <= 0 {
		return []*http.Request{req}, nil
	}
	return results, nil
}

func (f *FuzzHTTPRequest) FuzzGetParams(k, v interface{}) FuzzHTTPRequestIf {
	f.position = lowhttp.PosGetQuery
	reqs, err := f.fuzzGetParams(k, v)
	if err != nil {
		return f.toFuzzHTTPRequestBatch()
	}

	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) fuzzPostRaw(body ...string) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}

	raws := QuickMutateSimple(body...)
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
		return f.toFuzzHTTPRequestBatch()
	}

	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) fuzzPostParams(k, v interface{}, encoded ...codec.EncodedFunc) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}
	pos := f.position
	if f.queryParams == nil || f.queryParams.IsEmpty() || f.mode == paramsFuzz {
		f.queryParams = lowhttp.ParseQueryParams(
			f.GetPostQuery(),
			lowhttp.WithDisableAutoEncode(f.noAutoEncode),
			lowhttp.WithFriendlyDisplay(f.friendlyDisplay),
			lowhttp.WithPosition(pos),
		)
	}
	keys, values := InterfaceToFuzzResults(k), InterfaceToFuzzResults(v)
	if keys == nil || values == nil {
		return nil, utils.Errorf("empty keys or Values")
	}

	var results []*http.Request
	origin := httpctx.GetBareRequestBytes(req)
	err = cartesian.ProductEx([][]string{keys, values}, func(result []string) error {
		if len(result) != 2 {
			return utils.Error("BUG in fuzz PostParams KeyValue")
		}

		key := result[0]
		value := result[1]
		for _, e := range encoded {
			value = e(value)
		}

		f.queryParams.SetPosition(pos)
		f.queryParams.SetFriendlyDisplay(f.friendlyDisplay)
		f.queryParams.DisableAutoEncode(f.noAutoEncode)
		f.queryParams.Set(key, value)
		reqIns, err := lowhttp.ParseBytesToHttpRequest(
			lowhttp.ReplaceHTTPPacketBodyFast(origin,
				[]byte(f.queryParams.EncodeByPos(pos)),
			),
		)
		if err != nil {
			return nil
		}
		results = append(results, reqIns)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

func (f *FuzzHTTPRequest) FuzzPostParams(k, v interface{}) FuzzHTTPRequestIf {
	f.position = lowhttp.PosPostQuery
	reqs, err := f.fuzzPostParams(k, v)
	if err != nil {
		return f.toFuzzHTTPRequestBatch()
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
	jsonBody := string(rawBody)
	if _, ok := utils.IsJSON(jsonBody); !ok {
		return nil, utils.Errorf("body is not json")
	}

	values := InterfaceToFuzzResults(originValue)
	if values == nil {
		return nil, utils.Errorf("key or value is empty...")
	}

	var reqs []*http.Request
	origin := httpctx.GetBareRequestBytes(req)
	valueIndex := 0
	err = cartesian.ProductEx([][]string{
		values,
	}, func(result []string) error {
		value := result[0]

		modifiedBody, err := modifyJSONValue(jsonBody, p.path, value, originValue, valueIndex)
		if err != nil {
			return err
		}
		reqIns, err := lowhttp.ParseBytesToHttpRequest(lowhttp.ReplaceHTTPPacketBodyFast(origin, []byte(modifiedBody)))
		if err != nil {
			return err
		}
		reqs = append(reqs, reqIns)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return reqs, nil
}

func (f *FuzzHTTPRequest) fuzzXMLWithRaw(k, v any) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}

	var rawBody []byte
	if req.Body != nil {
		rawBody = f.GetBody()
	} else {
		return nil, utils.Errorf("empty body")
	}

	rootNode, err := xmlquery.Parse(bytes.NewReader(rawBody))
	if err != nil {
		return nil, utils.Wrap(err, "parse body as xml failed")
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
	var nodes []*xmlquery.Node
	origin := httpctx.GetBareRequestBytes(req)
	for {
		pair := m.Value()
		key, value := pair[0], pair[1]
		_ = value

		nodes, err = xmlquery.QueryAll(rootNode, key)
		if err != nil || len(nodes) == 0 {
			nodes, _ = xmlquery.QueryAll(rootNode, fmt.Sprintf("//%s", key))
		}

		if len(nodes) > 0 {
			for _, node := range nodes {
				value := ConvertValue(node.InnerText(), value)
				oldChild := node.FirstChild
				node.FirstChild = &xmlquery.Node{
					Data: value,
					Type: xmlquery.TextNode,
				}
				raw := rootNode.OutputXML(false)
				reqIns, err := lowhttp.ParseBytesToHttpRequest(lowhttp.ReplaceHTTPPacketBodyFast(origin, []byte(raw)))
				if err != nil {
					continue
				}
				reqs = append(reqs, reqIns)
				node.FirstChild = oldChild
			}
		}

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

	originBody := rawBody
	keys, values := InterfaceToFuzzResults(k), InterfaceToFuzzResults(v)
	if keys == nil || values == nil {
		return nil, utils.Wrapf(err, "keys or Values is empty...")
	}

	m, err := mixer.NewMixer(keys, values)
	if err != nil {
		return nil, err
	}

	origin := httpctx.GetBareRequestBytes(req)
	var reqs []*http.Request
	valueIndex := 0
	for {
		pair := m.Value()
		key, value := pair[0], pair[1]

		modifiedBody, err := modifyJSONValue(string(originBody), key, value, v, valueIndex)
		if err != nil {
			break
		}
		reqIns, err := lowhttp.ParseBytesToHttpRequest(lowhttp.ReplaceHTTPPacketBodyFast(origin, []byte(modifiedBody)))
		if err != nil {
			break
		}
		reqs = append(reqs, reqIns)

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
		return f.toFuzzHTTPRequestBatch()
	}
	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) FuzzPostXMLParams(k, v interface{}) FuzzHTTPRequestIf {
	reqs, err := f.fuzzXMLWithRaw(k, v)
	if err != nil {
		return f.toFuzzHTTPRequestBatch()
	}
	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) FuzzCookieRaw(v interface{}) FuzzHTTPRequestIf {
	return f.FuzzHTTPHeader("Cookie", v)
}

func (f *FuzzHTTPRequest) FuzzCookie(k, v interface{}) FuzzHTTPRequestIf {
	reqs, err := f.fuzzCookie(k, v)
	if err != nil {
		return f.toFuzzHTTPRequestBatch()
	}
	r := NewFuzzHTTPRequestBatch(f, reqs...)
	return r
}

func (f *FuzzHTTPRequest) FuzzFormEncoded(k, v interface{}) FuzzHTTPRequestIf {
	reqs, err := f.fuzzFormEncoded(k, v)
	if err != nil {
		return f.toFuzzHTTPRequestBatch()
	}
	r := NewFuzzHTTPRequestBatch(f, reqs...)
	return r
}

func (f *FuzzHTTPRequest) FuzzUploadFile(k, v interface{}, raw []byte) FuzzHTTPRequestIf {
	reqs, err := f.fuzzUploadedFile(k, v, raw)
	if err != nil {
		return f.toFuzzHTTPRequestBatch()
	}
	r := NewFuzzHTTPRequestBatch(f, reqs...)
	return r
}

func (f *FuzzHTTPRequest) FuzzUploadKVPair(k, v interface{}) FuzzHTTPRequestIf {
	reqs, err := f.fuzzMultipartKeyValue(k, v)
	if err != nil {
		return f.toFuzzHTTPRequestBatch()
	}
	r := NewFuzzHTTPRequestBatch(f, reqs...)
	return r
}

func (f *FuzzHTTPRequest) FuzzUploadFileName(k, v interface{}) FuzzHTTPRequestIf {
	reqs, err := f.fuzzUploadedFileName(k, v)
	if err != nil {
		return f.toFuzzHTTPRequestBatch()
	}
	r := NewFuzzHTTPRequestBatch(f, reqs...)
	return r
}

func (f *FuzzHTTPRequest) fuzzCookie(k, v interface{}, encoded ...codec.EncodedFunc) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
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

	origin := httpctx.GetBareRequestBytes(req)
	results := make([]*http.Request, 0, len(keys)*len(values))
	for {
		pair := m.Value()
		key, value := pair[0], pair[1]
		var rspIns *http.Request
		var err error
		var newValue string
		if encoded == nil {
			if f.friendlyDisplay {
				newValue = lowhttp.CookieSafeFriendly(value)
			}
			if f.NoAutoEncode() {
				newValue = lowhttp.CookieSafeString(value)
			} else if !f.friendlyDisplay {
				newValue = lowhttp.CookieSafeQuoteString(value)
			}
		} else {
			newValue = value
		}

		for _, e := range encoded {
			newValue = e(newValue)
		}
		rspIns, err = lowhttp.ParseBytesToHttpRequest(
			lowhttp.ReplaceHTTPPacketCookie(origin, key, newValue),
		)
		if err != nil {
			log.Infof("parse (in FuzzCookie) request failed: %v", err)
			continue
		}
		results = append(results, rspIns)

		err = m.Next()
		if err != nil {
			break
		}
	}
	return results, nil
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

func (f *FuzzHTTPRequest) ExecFirst(opts ...HttpPoolConfigOption) (*HttpResult, error) {
	opts = append(opts, WithPoolOpt_RequestCountLimiter(1))
	resultCh, err := f.Exec(opts...)
	if err != nil {
		return nil, err
	}

	var result *HttpResult
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
