package mutate

import (
	"fmt"
	"net/http"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yak/cartesian"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func valueToJsonValue(i string) []any {
	if utils.IsValidInteger(i) {
		return []any{codec.Atoi(i), i}
	}

	if utils.IsValidFloat(i) {
		return []any{codec.Atof(i), i}
	}

	if i == "true" || i == "false" {
		return []any{codec.Atob(i), i}
	}

	if i == "undefined" || i == "null" {
		return []any{nil, i, "", 0}
	}

	return []any{i}
}

func (f *FuzzHTTPRequest) fuzzCookieJsonPath(key any, jsonPath string, val any, encoded ...codec.EncodedFunc) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}

	var cookies []*http.Cookie
	cookies = lowhttp.ParseCookie("cookie", req.Header.Get("Cookie"))
	if cookies == nil {
		return nil, utils.Error("empty cookie")
	}

	kStr, values := utils.InterfaceToString(key), InterfaceToFuzzResults(val)
	var originJson string
	for _, k := range cookies {
		if k.Name != kStr {
			continue
		}
		var ok bool
		if originJson, ok = IsBase64JSON(k.Value); !ok {
			originJson, ok = utils.IsJSON(k.Value)
			if !ok {
				continue
			}
		}
	}
	// 如果没有找到对应的cookie key,则认为是追加
	if originJson == "" {
		originJson = "{}"
	}

	results := make([]*http.Request, 0, len(kStr)*len(values))
	origin := httpctx.GetBareRequestBytes(req)
	valueIndex := 0
	err = cartesian.ProductEx([][]string{
		{kStr}, values,
	}, func(result []string) error {
		_, value := result[0], result[1]
		modifiedParams, err := modifyJSONValue(originJson, jsonPath, value, val, valueIndex, f.noEscapeHTML)
		if err != nil {
			return err
		}
		var newModifiedParams string

		if f.friendlyDisplay {
			newModifiedParams = lowhttp.CookieSafeFriendly(modifiedParams)
		}
		if f.NoAutoEncode() {
			newModifiedParams = lowhttp.CookieSafeString(modifiedParams)
		} else if !f.friendlyDisplay {
			newModifiedParams = lowhttp.CookieSafeQuoteString(modifiedParams)
		}

		for _, e := range encoded {
			newModifiedParams = e(newModifiedParams)
		}

		reqIns, err := lowhttp.ParseBytesToHttpRequest(
			lowhttp.ReplaceHTTPPacketCookie(origin, kStr, newModifiedParams),
		)
		if err != nil {
			return err
		}

		results = append(results, reqIns)

		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

func (f *FuzzHTTPRequest) FuzzCookieJsonPath(k any, jp string, v any) FuzzHTTPRequestIf {
	reqs, err := f.fuzzCookieJsonPath(k, jp, v)
	if err != nil {
		return f.toFuzzHTTPRequestBatch()
	}
	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequest) FuzzCookieBase64JsonPath(k any, jp string, v any) FuzzHTTPRequestIf {
	encode := func(v any) string {
		if f.friendlyDisplay {
			return fmt.Sprintf("{{base64(%s)}}", v)
		}
		return codec.EncodeBase64(v)
	}
	// 被base64编码的json 应当不再被 url 编码
	f.DisableAutoEncode(true)
	reqs, err := f.fuzzCookieJsonPath(k, jp, v, encode)
	if err != nil {
		return f.toFuzzHTTPRequestBatch()
	}
	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequestBatch) FuzzCookieBase64JsonPath(k any, jp string, v any) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzCookieBase64JsonPath(k, jp, v)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzCookieBase64JsonPath(k, jp, v))
	}

	return f.toFuzzHTTPRequestIf(reqs)
}

func (f *FuzzHTTPRequestBatch) FuzzCookieJsonPath(k any, jp string, v any) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzCookieJsonPath(k, jp, v)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzCookieJsonPath(k, jp, v))
	}

	return f.toFuzzHTTPRequestIf(reqs)
}
