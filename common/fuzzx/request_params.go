package fuzzx

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/antchfx/xmlquery"
	"github.com/asaskevich/govalidator"
	"github.com/samber/lo"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type FuzzParam struct {
	origin   *FuzzRequest
	position lowhttp.HttpParamPositionType
	param    string
	value    any    // usually a string, or other type(int, bool)
	pathKey  string // xpath or jsonpath key
	path     string // xpath or jsonpath
	n        int    // If there are duplicate parameters, it indicates which parameter it is
}

func (p *FuzzParam) IsPostParams() bool {
	switch p.position {
	case lowhttp.PosPostJson, lowhttp.PosPostQuery, lowhttp.PosPostQueryBase64,
		lowhttp.PosPostQueryJson, lowhttp.PosPostQueryBase64Json, lowhttp.PosPostXML:
		return true
	}
	return false
}

func (p *FuzzParam) IsGetParams() bool {
	switch p.position {
	case lowhttp.PosGetQuery, lowhttp.PosGetQueryBase64, lowhttp.PosGetQueryJson,
		lowhttp.PosGetQueryBase64Json:
		return true
	}
	return false
}

func (p *FuzzParam) IsGetValueJSON() bool {
	if p == nil {
		return false
	}

	switch p.position {
	case lowhttp.PosGetQueryJson, lowhttp.PosGetQueryBase64Json:
		return true
	}
	return false
}

func (p *FuzzParam) IsCookieParams() bool {
	switch p.position {
	case lowhttp.PosCookie, lowhttp.PosCookieJson, lowhttp.PosCookieBase64,
		lowhttp.PosCookieBase64Json:
		return true
	}
	return false
}

func (p *FuzzParam) Name() string {
	return p.param
}

func (p *FuzzParam) Value() interface{} {
	return p.value
}

func (p *FuzzParam) Position() string {
	return string(p.position)
}

func (p *FuzzParam) PositionVerbose() string {
	return mutate.PositionTypeVerbose(p.position)
}

func (p *FuzzParam) Path() string {
	return p.path
}

func (p *FuzzParam) String() string {
	if p.path != "" {
		pathName := "JsonPath"
		if p.position == lowhttp.PosPostXML {
			pathName = "XPath"
		}
		return fmt.Sprintf("Name:%-20s %s: %-12s Position:[%v(%v)]\n", p.Name(), pathName, p.path, p.PositionVerbose(), p.Position())
	}
	return fmt.Sprintf("Name:%-20s Position:[%v(%v)]\n", p.Name(), p.PositionVerbose(), p.Position())
}

func (p *FuzzParam) Exec(opts ...mutate.HttpPoolConfigOption) (chan *mutate.HttpResult, error) {
	return p.origin.Exec(opts...)
}

func (p *FuzzParam) ExecFirst(opts ...mutate.HttpPoolConfigOption) (result *mutate.HttpResult, err error) {
	return p.origin.ExecFirst(opts...)
}

func (p *FuzzParam) Results() [][]byte {
	return p.origin.requests
}

func (p *FuzzParam) Show() *FuzzParam {
	p.origin.Show()
	return p
}

func (p *FuzzParam) Clone() *FuzzParam {
	return &FuzzParam{
		origin:   p.origin.Clone(),
		position: p.position,
		param:    p.param,
		value:    p.value,
		pathKey:  p.pathKey,
	}
}

func (p *FuzzParam) FirstFuzzRequestBytes() []byte {
	return p.origin.FirstFuzzRequestBytes()
}

func (p *FuzzParam) Fuzz(values ...string) *FuzzParam {
	np := p.Clone()
	req := np.origin
	for _, i := range values {
		switch p.position {
		case lowhttp.PosPath:
			req.FuzzPath(i)
		case lowhttp.PosPathAppend:
			req.FuzzPathAppend(i)
		case lowhttp.PosPathBlock:
			req.FuzzPathBlock(i)
		case lowhttp.PosMethod:
			req.FuzzMethod(i)
		case lowhttp.PosGetQuery:
			req.fuzzGetParams(p.param, i, nil, p.n)
		case lowhttp.PosGetQueryBase64:
			req.fuzzGetParams(p.param, i, codec.EncodeBase64, p.n)
		case lowhttp.PosGetQueryJson:
			req.fuzzGetJsonPathParams(p.param, p.path, i, nil, p.n)
		case lowhttp.PosGetQueryBase64Json:
			req.fuzzGetJsonPathParams(p.param, p.path, i, codec.EncodeBase64, p.n)
		case lowhttp.PosHeader:
			req.FuzzHTTPHeader(p.param, i)
		case lowhttp.PosCookie:
			req.FuzzCookie(p.param, i)
		case lowhttp.PosCookieBase64:
			req.FuzzCookieBase64(p.param, i)
		case lowhttp.PosCookieJson:
			req.FuzzCookieJsonPath(p.param, p.path, i)
		case lowhttp.PosCookieBase64Json:
			req.FuzzCookieBase64JsonPath(p.param, p.path, i)
		case lowhttp.PosPostJson:
			req.FuzzPostJson(p.path, i)
		case lowhttp.PosPostQuery:
			req.fuzzPostParams(p.param, i, nil, p.n)
		case lowhttp.PosPostQueryBase64:
			req.fuzzPostParams(p.param, i, codec.EncodeBase64, p.n)
		case lowhttp.PosPostQueryJson:
			req.FuzzPostJsonPathParams(p.param, p.path, i)
		case lowhttp.PosPostQueryBase64Json:
			req.FuzzPostBase64JsonPathParams(p.param, p.path, i)
		case lowhttp.PosPostXML:
			req.FuzzPostXMLParams(p.path, i)
		case lowhttp.PosBody:
			req.FuzzPostRaw(i)
		default:
			log.Warnf("cannot found fuzz params method identify: %v", p.position)
		}
	}
	return np
}

func (f *FuzzRequest) GetCommonParams() []*FuzzParam {
	params := make([]*FuzzParam, 0)
	params = append(params, f.GetQueryParams()...)
	params = append(params, f.GetPostCommonParams()...)
	params = append(params, f.GetCookieParams()...)
	return params
}

func (f *FuzzRequest) GetHeaderParams() []*FuzzParam {
	headers := lowhttp.GetHTTPPacketHeaders(f.origin)
	return lo.MapToSlice(headers, func(k, v string) *FuzzParam {
		return &FuzzParam{
			position: lowhttp.PosHeader,
			param:    k,
			value:    v,
			origin:   f,
		}
	})
}

func (f *FuzzRequest) GetPathParams() []*FuzzParam {
	u, err := lowhttp.ExtractURLFromHTTPRequestRaw(f.origin, false)
	if err != nil {
		log.Errorf("extract url from request raw failed: %s", err)
		return nil
	}
	path := u.Path
	return []*FuzzParam{
		{
			position: lowhttp.PosPath,
			param:    path,
			origin:   f,
		},
		{
			position: lowhttp.PosPathAppend,
			param:    path,
			origin:   f,
		},
		{
			position: lowhttp.PosPathBlock,
			param:    path,
			origin:   f,
		},
	}
}

func (f *FuzzRequest) GetMethodParams() []*FuzzParam {
	return []*FuzzParam{
		{
			position: lowhttp.PosMethod,
			origin:   f,
		},
	}
}

func (f *FuzzRequest) GetRawBodyParams() []*FuzzParam {
	return []*FuzzParam{
		{
			position: lowhttp.PosBody,
			origin:   f,
		},
	}
}

func (f *FuzzRequest) GetQueryParams() []*FuzzParam {
	fuzzParams := make([]*FuzzParam, 0)
	params := lowhttp.GetFullHTTPRequestQueryParams(f.origin)

	for key, values := range params {
		for i, value := range values {
			if raw, ok := utils.IsJSON(value); ok {
				fixRaw := strings.TrimSpace(raw)
				walkJson([]byte(fixRaw), func(k, v gjson.Result, jsonPath string) {
					fuzzParams = append(fuzzParams, &FuzzParam{
						position: lowhttp.PosGetQueryJson,
						param:    key,
						path:     jsonPath,
						pathKey:  k.String(),
						value:    v.String(),
						origin:   f,
						n:        i,
					})
				})
			}

			if bs64Raw, ok := mutate.IsStrictBase64(value); ok && govalidator.IsPrintableASCII(bs64Raw) {
				if raw, ok := utils.IsJSON(bs64Raw); ok {
					fixRaw := strings.TrimSpace(raw)
					walkJson([]byte(fixRaw), func(k, v gjson.Result, jsonPath string) {
						fuzzParams = append(fuzzParams, &FuzzParam{
							position: lowhttp.PosGetQueryBase64Json,
							param:    key,
							path:     jsonPath,
							pathKey:  k.String(),
							value:    v.String(),
							origin:   f,
							n:        i,
						})
					})
				}
				// 优化显示效果
				fuzzParams = append(fuzzParams, &FuzzParam{
					position: lowhttp.PosGetQueryBase64,
					param:    key,
					value:    bs64Raw,
					origin:   f,
					n:        i,
				})
			}

			param := &FuzzParam{
				position: lowhttp.PosGetQuery,
				param:    key,
				value:    value,
				origin:   f,
				n:        i,
			}
			fuzzParams = append(fuzzParams, param)
		}
	}
	return fuzzParams
}

func (f *FuzzRequest) GetCookieParams() []*FuzzParam {
	fuzzParams := make([]*FuzzParam, 0)
	cookies := lowhttp.ParseCookie("cookie", lowhttp.GetHTTPPacketHeader(f.origin, "Cookie"))
	for _, c := range cookies {
		if mutate.ShouldIgnoreCookie(c.Name) {
			continue
		}
		if raw, ok := utils.IsJSON(c.Value); ok {
			fixRaw := strings.TrimSpace(raw)
			walkJson([]byte(fixRaw), func(k, v gjson.Result, jsonPath string) {
				fuzzParams = append(fuzzParams, &FuzzParam{
					position: lowhttp.PosCookieJson,
					param:    c.Name,
					pathKey:  k.String(),
					value:    v.String(),
					path:     jsonPath,
					origin:   f,
				})
			})
		}
		if bs64Raw, ok := mutate.IsStrictBase64(c.Value); ok && govalidator.IsPrintableASCII(bs64Raw) {
			if raw, ok := utils.IsJSON(bs64Raw); ok {
				fixRaw := strings.TrimSpace(raw)
				walkJson([]byte(fixRaw), func(k, v gjson.Result, jsonPath string) {
					fuzzParams = append(fuzzParams, &FuzzParam{
						position: lowhttp.PosCookieBase64Json,
						param:    c.Name,
						pathKey:  k.String(),
						value:    v.String(),
						path:     jsonPath,
						origin:   f,
					})
				})
			}
			fuzzParams = append(fuzzParams, &FuzzParam{
				position: lowhttp.PosCookieBase64,
				param:    c.Name,
				value:    bs64Raw,
				origin:   f,
			})
		}

		fuzzParams = append(fuzzParams, &FuzzParam{
			position: lowhttp.PosCookie,
			param:    c.Name,
			value:    c.Value,
			origin:   f,
		})
	}
	return fuzzParams
}

func (f *FuzzRequest) GetPostCommonParams() []*FuzzParam {
	postParams := f.GetPostJsonParams()
	if len(postParams) <= 0 {
		postParams = f.GetPostXMLParams()
	}
	if len(postParams) <= 0 {
		postParams = f.GetPostParams()
	}
	return postParams
}

func (f *FuzzRequest) GetPostJsonParams() []*FuzzParam {
	fuzzParams := make([]*FuzzParam, 0)

	bodyRaw := lowhttp.GetHTTPPacketBody(f.origin)

	if bodyRaw == nil || len(bodyRaw) == 0 {
		return fuzzParams
	}
	bodyStr := string(bytes.TrimSpace(bodyRaw))
	if _, ok := utils.IsJSON(bodyStr); !ok {
		return fuzzParams
	}
	walkJson(bodyRaw, func(key, val gjson.Result, jsonPath string) {
		var paramValue interface{}
		if val.IsObject() || val.IsArray() {
			paramValue = val.String()
		} else {
			paramValue = val.Value()
		}

		fuzzParams = append(fuzzParams, &FuzzParam{
			position: lowhttp.PosPostJson,
			param:    key.String(),
			value:    paramValue,
			path:     jsonPath,
			origin:   f,
		})
	})
	return fuzzParams
}

func (f *FuzzRequest) GetPostXMLParams() []*FuzzParam {
	fuzzParams := make([]*FuzzParam, 0)

	bodyRaw := lowhttp.GetHTTPPacketBody(f.origin)

	if bodyRaw == nil || len(bodyRaw) == 0 {
		return fuzzParams
	}
	rootNode, err := xmlquery.Parse(bytes.NewReader(bodyRaw))
	if err != nil {
		return nil
	}

	RecursiveXMLNode(rootNode, func(node *xmlquery.Node) {
		fuzzParams = append(fuzzParams, &FuzzParam{
			position: lowhttp.PosPostXML,
			param:    node.Data,
			value:    node.InnerText(),
			path:     GetXpathFromNode(node),
			origin:   f,
		})
	})
	return fuzzParams
}

func (f *FuzzRequest) GetPostParams() []*FuzzParam {
	fuzzParams := make([]*FuzzParam, 0)
	params := lowhttp.GetFullHTTPRequestPostParams(f.origin)

	for key, values := range params {
		for i, value := range values {
			if raw, ok := utils.IsJSON(value); ok {
				fixRaw := strings.TrimSpace(raw)
				walkJson([]byte(fixRaw), func(k, v gjson.Result, jsonPath string) {
					fuzzParams = append(fuzzParams, &FuzzParam{
						position: lowhttp.PosPostQueryJson,
						param:    key,
						path:     jsonPath,
						pathKey:  k.String(),
						value:    v.String(),
						origin:   f,
						n:        i,
					})
				})
			}

			if bs64Raw, ok := mutate.IsStrictBase64(value); ok && govalidator.IsPrintableASCII(bs64Raw) {
				if raw, ok := utils.IsJSON(bs64Raw); ok {
					fixRaw := strings.TrimSpace(raw)
					walkJson([]byte(fixRaw), func(k, v gjson.Result, jsonPath string) {
						fuzzParams = append(fuzzParams, &FuzzParam{
							position: lowhttp.PosPostQueryBase64Json,
							param:    key,
							path:     jsonPath,
							pathKey:  k.String(),
							value:    v.String(),
							origin:   f,
							n:        i,
						})
					})
				}
				// 优化显示效果
				fuzzParams = append(fuzzParams, &FuzzParam{
					position: lowhttp.PosPostQueryBase64,
					param:    key,
					value:    bs64Raw,
					origin:   f,
					n:        i,
				})
			}

			param := &FuzzParam{
				position: lowhttp.PosPostQuery,
				param:    key,
				value:    value,
				origin:   f,
				n:        i,
			}
			fuzzParams = append(fuzzParams, param)
		}

	}
	return fuzzParams
}
