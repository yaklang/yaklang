package mutate

import (
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/cartesian"
	"net/http"
)

func cloneCookies(i []*http.Cookie) []*http.Cookie {
	return funk.Map(i, func(in *http.Cookie) *http.Cookie {
		return &http.Cookie{
			Name:       in.Name,
			Value:      in.Value,
			Path:       in.Path,
			Domain:     in.Domain,
			Expires:    in.Expires,
			RawExpires: in.RawExpires,
			MaxAge:     in.MaxAge,
			Secure:     in.Secure,
			HttpOnly:   in.HttpOnly,
			SameSite:   in.SameSite,
			Raw:        in.Raw,
			Unparsed:   in.Unparsed,
		}
	}).([]*http.Cookie)
}

func valueToJsonValue(i string) []any {
	if utils.IsValidInteger(i) {
		return []any{utils.Atoi(i), i}
	}

	if utils.IsValidFloat(i) {
		return []any{utils.Atof(i), i}
	}

	if i == "true" || i == "false" {
		return []any{utils.Atob(i), i}
	}

	if i == "undefined" || i == "null" {
		return []any{nil, i, "", 0}
	}

	return []any{i}
}

func (f *FuzzHTTPRequest) fuzzCookieJsonPath(key any, jsonPath string, val any) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}

	var cookies []*http.Cookie
	cookies = lowhttp.ParseCookie(req.Header.Get("Cookie"))
	if cookies == nil {
		return nil, utils.Error("empty cookie")
	}

	var kStr = utils.InterfaceToString(key)
	var originJson string
	for _, k := range cookies {
		if k.Name != kStr {
			continue
		}
		var ok bool
		originJson, ok = utils.IsJSON(k.Value)
		if !ok {
			continue
		}
	}
	if originJson == "" {
		return nil, utils.Error("empty json")
	}

	var reqs []*http.Request
	err = cartesian.ProductEx([][]string{
		{kStr}, InterfaceToFuzzResults(val),
	}, func(result []string) error {
		k, v := result[0], result[1]
		_ = k
		var replaced = valueToJsonValue(v)
		for _, i := range replaced {
			replacedOrigin := jsonpath.ReplaceString(originJson, jsonPath, i)
			cloned := cloneCookies(cookies)
			for _, cookie := range cloned {
				if cookie.Name == kStr {
					cookie.Value = replacedOrigin
					break
				}
			}
			_req := lowhttp.CopyRequest(req)
			_req.Header.Del("Cookie")
			_req.Header["Cookie"] = []string{lowhttp.CookiesToString(cloned)}
			reqs = append(reqs, _req)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return reqs, nil
}

func (f *FuzzHTTPRequest) FuzzCookieJsonPath(k any, jp string, v any) FuzzHTTPRequestIf {
	reqs, err := f.fuzzCookieJsonPath(k, jp, v)
	if err != nil {
		return &FuzzHTTPRequestBatch{fallback: f, originRequest: f}
	}
	return NewFuzzHTTPRequestBatch(f, reqs...)
}

func (f *FuzzHTTPRequestBatch) FuzzCookieJsonPath(k any, jp string, v any) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzCookieJsonPath(k, jp, v)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzCookieJsonPath(k, jp, v))
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
