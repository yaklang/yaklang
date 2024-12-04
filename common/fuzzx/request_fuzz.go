package fuzzx

import (
	"bytes"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/antchfx/xmlquery"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/mixer"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func (f *FuzzRequest) Repeat(n int) *FuzzRequest {
	for i := 0; i < n; i++ {
		f.requests = append(f.requests, f.origin)
	}
	return f
}

func (f *FuzzRequest) FuzzMethod(methods ...string) *FuzzRequest {
	methods = QuickMutateSimple(methods...)
	origin := f.origin
	for _, method := range methods {
		f.requests = append(f.requests, lowhttp.ReplaceHTTPPacketMethod(origin, method))
	}
	return f
}

func (f *FuzzRequest) FuzzPath(paths ...string) *FuzzRequest {
	paths = QuickMutateSimple(paths...)
	origin := f.origin
	for _, p := range paths {
		f.requests = append(f.requests, lowhttp.ReplaceHTTPPacketPathWithoutEncoding(origin, p))
	}
	return f
}

func (f *FuzzRequest) FuzzPathAppend(paths ...string) *FuzzRequest {
	paths = QuickMutateSimple(paths...)
	origin := f.origin
	originPath := lowhttp.GetHTTPRequestPathWithoutQuery(origin)
	for _, p := range paths {
		f.requests = append(f.requests, lowhttp.ReplaceHTTPPacketPathWithoutEncoding(origin, path.Join(originPath, p)))
	}
	return f
}

func (f *FuzzRequest) FuzzPathBlock(paths ...string) *FuzzRequest {
	originPath := lowhttp.GetHTTPRequestPathWithoutQuery(f.origin)
	splited := lo.Filter(strings.Split(originPath, "/"), func(s string, _ int) bool {
		return s != ""
	})
	results := make([]string, 0)
	for _, p := range paths {
		for i := 0; i < len(splited); i++ {
			old := splited[i]
			splited[i] = p
			results = append(results, QuickMutateSimple(strings.Join(splited, "/"))...)
			splited[i] = old
		}
	}
	for _, r := range results {
		f.requests = append(f.requests, lowhttp.ReplaceHTTPPacketPathWithoutEncoding(f.origin, r))
	}
	return f
}

func (f *FuzzRequest) FuzzHTTPHeader(key, value string) *FuzzRequest {
	keys := QuickMutateSimple(key)
	values := QuickMutateSimple(value)

	m, err := mixer.NewMixer(keys, values)
	if err != nil {
		log.Errorf("fuzzx: failed to create mixer: %v", err)
		return f
	}
	origin := f.origin
	for err == nil {
		pair := m.Value()
		k, v := pair[0], pair[1]
		f.requests = append(f.requests, lowhttp.ReplaceHTTPPacketHeader(origin, k, v))
		err = m.Next()
	}
	return f
}

func (f *FuzzRequest) fuzzCookie(key, value string, encoded codec.EncodedFunc) *FuzzRequest {
	keys := QuickMutateSimple(key)
	values := QuickMutateSimple(value)

	m, err := mixer.NewMixer(keys, values)
	if err != nil {
		log.Errorf("fuzzx: failed to create mixer: %v", err)
		return f
	}
	origin := f.origin
	for err == nil {
		pair := m.Value()
		k, v := pair[0], pair[1]
		if encoded != nil {
			v = encoded(v)
		}
		f.requests = append(f.requests, lowhttp.ReplaceHTTPPacketCookie(origin, k, v))
		err = m.Next()
	}
	return f
}

func (f *FuzzRequest) FuzzCookie(key, value string) *FuzzRequest {
	return f.fuzzCookie(key, value, nil)
}

func (f *FuzzRequest) FuzzCookieBase64(key, value string) *FuzzRequest {
	return f.fuzzCookie(key, value, codec.EncodeBase64)
}

func (f *FuzzRequest) fuzzCookieJsonPath(key, jsonPath, value string, encoded codec.EncodedFunc) *FuzzRequest {
	origin := f.origin

	keyToValueMap := make(map[string]string)

	// check if origin value is json
	cookies := lowhttp.ParseCookie("cookie", lowhttp.GetHTTPPacketHeader(f.origin, "Cookie"))
	cookiesMap := lo.SliceToMap(cookies, func(c *http.Cookie) (string, *http.Cookie) {
		return c.Name, c
	})

	keys := lo.FilterMap(QuickMutateSimple(key), func(k string, _ int) (string, bool) {
		c, ok := cookiesMap[k]
		if !ok {
			keyToValueMap[k] = "{}"
			return k, true // if not found, just append
		}
		v := c.Value

		if _, ok := utils.IsJSON(v); ok {
			keyToValueMap[k] = v
			return k, true
		} else if decoded, ok := mutate.IsBase64JSON(v); ok {
			keyToValueMap[k] = decoded
			return k, true
		}
		log.Warnf("fuzzx: cookie[%s] is invalid json", k)
		return "", false
	})
	values := QuickMutateSimple(value)

	m, err := mixer.NewMixer(keys, values)
	if err != nil {
		log.Errorf("fuzzx: failed to create mixer: %v", err)
		return f
	}
	newValue := ""

	for err == nil {
		pair := m.Value()
		k, v := pair[0], pair[1]
		rawJson := keyToValueMap[k]
		newValue, err = jsonpath.ReplaceStringWithError(rawJson, jsonPath, v)
		if encoded != nil {
			newValue = encoded(newValue)
		}
		if err != nil {
			log.Warnf("fuzzx: failed to replace jsonPath: %v", err)
			err = m.Next()
			continue
		}
		f.requests = append(f.requests, lowhttp.ReplaceHTTPPacketCookie(origin, k, newValue))
		err = m.Next()
	}
	return f
}

func (f *FuzzRequest) FuzzCookieJsonPath(key, jsonPath, value string) *FuzzRequest {
	return f.fuzzCookieJsonPath(key, jsonPath, value, nil)
}

func (f *FuzzRequest) FuzzCookieBase64JsonPath(key, jsonPath, value string) *FuzzRequest {
	return f.fuzzCookieJsonPath(key, jsonPath, value, codec.EncodeBase64)
}

func (f *FuzzRequest) FuzzGetParamsRaw(raw ...string) *FuzzRequest {
	raw = QuickMutateSimple(raw...)
	origin := f.origin
	for _, r := range raw {
		f.requests = append(f.requests, lowhttp.ReplaceHTTPPacketQueryParamRaw(origin, r))
	}
	return f
}

func (f *FuzzRequest) fuzzGetParams(key, value string, encoded codec.EncodedFunc, n int) *FuzzRequest {
	keys := QuickMutateSimple(key)
	values := QuickMutateSimple(value)

	m, err := mixer.NewMixer(keys, values)
	if err != nil {
		log.Errorf("fuzzx: failed to create mixer: %v", err)
		return f
	}
	origin := f.origin
	for err == nil {
		pair := m.Value()
		k, v := pair[0], pair[1]
		if encoded != nil {
			v = encoded(v)
		}

		f.requests = append(f.requests, lowhttp.ReplaceHTTPPacketQueryParamWithoutEncoding(origin, k, v, n))
		err = m.Next()
	}
	return f
}

func (f *FuzzRequest) FuzzGetParams(key, value string, n ...int) *FuzzRequest {
	i := 0
	if len(n) > 0 {
		i = n[0]
	}
	return f.fuzzGetParams(key, value, nil, i)
}

func (f *FuzzRequest) FuzzGetBase64Params(key, value string, n ...int) *FuzzRequest {
	i := 0
	if len(n) > 0 {
		i = n[0]
	}
	return f.fuzzGetParams(key, value, codec.EncodeBase64, i)
}

func (f *FuzzRequest) fuzzGetJsonPathParams(key, jsonPath, value string, encoded codec.EncodedFunc, n int) *FuzzRequest {
	origin := f.origin

	keyToValueMap := make(map[string]string)

	// check if origin value is json
	keys := lo.FilterMap(QuickMutateSimple(key), func(k string, _ int) (string, bool) {
		vs := lowhttp.GetHTTPRequestQueryParamFull(origin, k)
		v := vs[n]

		if _, ok := utils.IsJSON(v); ok {
			keyToValueMap[k] = v
			return k, true
		} else if decoded, ok := mutate.IsBase64JSON(v); ok {
			keyToValueMap[k] = decoded
			return k, true
		}
		log.Warnf("fuzzx: query-params[%s] is invalid json", k)
		return "", false
	})
	values := QuickMutateSimple(value)

	m, err := mixer.NewMixer(keys, values)
	if err != nil {
		log.Errorf("fuzzx: failed to create mixer: %v", err)
		return f
	}
	newValue := ""
	for err == nil {
		pair := m.Value()
		k, v := pair[0], pair[1]
		rawJson := keyToValueMap[k]
		newValue, err = jsonpath.ReplaceStringWithError(rawJson, jsonPath, v)
		if encoded != nil {
			newValue = encoded(newValue)
		}
		if err != nil {
			log.Warnf("fuzzx: failed to replace jsonPath: %v", err)
			err = m.Next()
			continue
		}
		f.requests = append(f.requests, lowhttp.ReplaceHTTPPacketQueryParamWithoutEncoding(origin, k, newValue, n))
		err = m.Next()
	}
	return f
}

func (f *FuzzRequest) FuzzGetJsonPathParams(key, jsonPath, value string, n ...int) *FuzzRequest {
	i := 0
	if len(n) > 0 {
		i = n[0]
	}
	return f.fuzzGetJsonPathParams(key, jsonPath, value, nil, i)
}

func (f *FuzzRequest) FuzzGetBase64JsonPathParams(key, jsonPath, value string, n ...int) *FuzzRequest {
	i := 0
	if len(n) > 0 {
		i = n[0]
	}
	return f.fuzzGetJsonPathParams(key, jsonPath, value, codec.EncodeBase64, i)
}

func (f *FuzzRequest) FuzzPostRaw(raw ...string) *FuzzRequest {
	raw = QuickMutateSimple(raw...)
	origin := f.origin
	for _, r := range raw {
		f.requests = append(f.requests, lowhttp.ReplaceHTTPPacketBodyFast(origin, []byte(r)))
	}
	return f
}

func (f *FuzzRequest) fuzzPostParams(key, value string, encoded codec.EncodedFunc, n int) *FuzzRequest {
	keys := QuickMutateSimple(key)
	values := QuickMutateSimple(value)

	m, err := mixer.NewMixer(keys, values)
	if err != nil {
		log.Errorf("fuzzx: failed to create mixer: %v", err)
		return f
	}
	origin := f.origin
	for err == nil {
		pair := m.Value()
		k, v := pair[0], pair[1]
		if encoded != nil {
			v = encoded(v)
		}
		f.requests = append(f.requests, lowhttp.ReplaceHTTPPacketPostParamWithoutEncoding(origin, k, v, n))
		err = m.Next()
	}
	return f
}

func (f *FuzzRequest) FuzzPostParams(key, value string, n ...int) *FuzzRequest {
	i := 0
	if len(n) > 0 {
		i = n[0]
	}
	return f.fuzzPostParams(key, value, nil, i)
}

func (f *FuzzRequest) FuzzPostBase64Params(key, value string, n ...int) *FuzzRequest {
	i := 0
	if len(n) > 0 {
		i = n[0]
	}
	return f.fuzzPostParams(key, value, codec.EncodeBase64, i)
}

func (f *FuzzRequest) FuzzPostJson(jsonPath, value string) *FuzzRequest {
	origin := f.origin

	// check if origin value is json
	body := lowhttp.GetHTTPPacketBody(origin)
	if len(body) == 0 {
		body = []byte("{}")
	}
	bodyStr := string(body)
	if _, ok := utils.IsJSON(bodyStr); !ok {
		log.Warnf("fuzzx: post-body is invalid json")
		return f
	}
	values := QuickMutateSimple(value)

	for _, v := range values {
		newBodyStr, err := jsonpath.ReplaceStringWithError(bodyStr, jsonPath, v)
		if err != nil {
			log.Warnf("fuzzx: failed to replace jsonPath: %v", err)
			err = nil
			continue
		}

		f.requests = append(f.requests, lowhttp.ReplaceHTTPPacketBodyFast(f.origin, []byte(newBodyStr)))
	}
	return f
}

func (f *FuzzRequest) fuzzPostJsonPathParams(key, jsonPath, value string, encoded codec.EncodedFunc, n int) *FuzzRequest {
	origin := f.origin

	keyToValueMap := make(map[string]string)

	// check if origin value is json
	keys := lo.FilterMap(QuickMutateSimple(key), func(k string, _ int) (string, bool) {
		vs := lowhttp.GetHTTPRequestPostParamFull(origin, k)
		v := vs[n]
		if _, ok := utils.IsJSON(v); ok {
			keyToValueMap[k] = v
			return k, true
		} else if decoded, ok := mutate.IsBase64JSON(v); ok {
			keyToValueMap[k] = decoded
			return k, true
		}
		log.Warnf("fuzzx: post-params[%s] is invalid json", k)
		return "", false
	})
	values := QuickMutateSimple(value)

	m, err := mixer.NewMixer(keys, values)
	if err != nil {
		log.Errorf("fuzzx: failed to create mixer: %v", err)
		return f
	}
	newValue := ""
	for err == nil {
		pair := m.Value()
		k, v := pair[0], pair[1]
		rawJson := keyToValueMap[k]
		newValue, err = jsonpath.ReplaceStringWithError(rawJson, jsonPath, v)
		if encoded != nil {
			newValue = encoded(newValue)
		}
		if err != nil {
			log.Warnf("fuzzx: failed to replace jsonPath: %v", err)
			err = m.Next()
			continue
		}

		f.requests = append(f.requests, lowhttp.ReplaceHTTPPacketPostParamWithoutEncoding(origin, k, newValue, n))
		err = m.Next()
	}
	return f
}

func (f *FuzzRequest) FuzzPostJsonPathParams(key, jsonPath, value string, n ...int) *FuzzRequest {
	i := 0
	if len(n) > 0 {
		i = n[0]
	}
	return f.fuzzPostJsonPathParams(key, jsonPath, value, nil, i)
}

func (f *FuzzRequest) FuzzPostBase64JsonPathParams(key, jsonPath, value string, n ...int) *FuzzRequest {
	i := 0
	if len(n) > 0 {
		i = n[0]
	}
	return f.fuzzPostJsonPathParams(key, jsonPath, value, codec.EncodeBase64, i)
}

func (f *FuzzRequest) FuzzPostXMLParams(xpath, value string) *FuzzRequest {
	origin := f.origin

	body := lowhttp.GetHTTPPacketBody(origin)
	if len(body) == 0 {
		log.Warnf("fuzzx: post-body is empty")
		return f
	}

	rootNode, err := xmlquery.Parse(bytes.NewReader(body))
	if err != nil {
		log.Warnf("fuzzx: failed to parse xml: %v", err)
		return f
	}

	values := QuickMutateSimple(value)
	m, err := mixer.NewMixer([]string{xpath}, values)
	if err != nil {
		log.Errorf("fuzzx: failed to create mixer: %v", err)
		return f
	}
	var nodes []*xmlquery.Node

	for err == nil {
		pair := m.Value()
		k, v := pair[0], pair[1]

		nodes, err = xmlquery.QueryAll(rootNode, xpath)
		if err != nil || len(nodes) == 0 {
			nodes, _ = xmlquery.QueryAll(rootNode, fmt.Sprintf("//%s", k))
		}
		if len(nodes) > 0 {
			for _, node := range nodes {
				value := mutate.ConvertValue(node.InnerText(), v)
				oldChild := node.FirstChild
				node.FirstChild = &xmlquery.Node{
					Data: value,
					Type: xmlquery.TextNode,
				}
				newBodyStr := rootNode.OutputXML(false)
				f.requests = append(f.requests, lowhttp.ReplaceHTTPPacketBodyFast(origin, []byte(newBodyStr)))
				node.FirstChild = oldChild
			}
		}

		err = m.Next()
	}
	return f
}
