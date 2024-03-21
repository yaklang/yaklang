package mutate

import (
	"fmt"
	"github.com/tidwall/gjson"
	"strings"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type httpParamPositionType string

var (
	posMethod              httpParamPositionType = "method"
	posBody                httpParamPositionType = "body"
	posGetQuery            httpParamPositionType = "get-query"
	posGetQueryBase64      httpParamPositionType = "get-query-base64"
	posGetQueryJson        httpParamPositionType = "get-query-json"
	posGetQueryBase64Json  httpParamPositionType = "get-query-base64-json"
	posPath                httpParamPositionType = "path"
	posHeader              httpParamPositionType = "header"
	posPostQuery           httpParamPositionType = "post-query"
	posPostXML             httpParamPositionType = "post-xml"
	posPostQueryBase64     httpParamPositionType = "post-query-base64"
	posPostQueryJson       httpParamPositionType = "post-query-json"
	posPostQueryBase64Json httpParamPositionType = "post-query-base64-json"
	posPostJson            httpParamPositionType = "post-json"
	posCookie              httpParamPositionType = "cookie"
	posCookieBase64        httpParamPositionType = "cookie-base64"
	posCookieJson          httpParamPositionType = "cookie-json"
	posCookieBase64Json    httpParamPositionType = "cookie-base64-json"
	posPathAppend          httpParamPositionType = "path-append"
	posPathBlock           httpParamPositionType = "path-block"
)

func PositionTypeVerbose(pos httpParamPositionType) string {
	switch pos {
	case posMethod:
		return "HTTP方法"
	case posBody:
		return "Body"
	case posGetQuery:
		return "GET参数"
	case posGetQueryBase64:
		return "GET参数(Base64)"
	case posGetQueryJson:
		return "GET参数(JSON)"
	case posGetQueryBase64Json:
		return "GET参数(Base64+JSON)"
	case posPathAppend:
		return "URL路径(追加)"
	case posPathBlock:
		return "URL路径(分块)"
	case posPath:
		return "URL路径"
	case posHeader:
		return "Header"
	case posPostQuery:
		return "POST参数"
	case posPostXML:
		return "POST参数(XML)"
	case posPostQueryBase64:
		return "POST参数(Base64)"
	case posPostQueryJson:
		return "POST参数(JSON)"
	case posPostQueryBase64Json:
		return "POST参数(Base64+JSON)"
	case posPostJson:
		return "JSON-Body参数"
	case posCookie:
		return "Cookie参数"
	case posCookieBase64:
		return "Cookie参数(Base64)"
	case posCookieJson:
		return "Cookie参数(JSON)"
	case posCookieBase64Json:
		return "Cookie参数(Base64+JSON)"
	default:
		return string(pos)
	}
}

type FuzzHTTPRequestParam struct {
	position httpParamPositionType
	param    interface{}
	param2nd interface{}
	// key 值对应的 value 值
	paramValue interface{}
	raw        interface{}
	path       string
	gpath      string
	origin     *FuzzHTTPRequest
}

func (p *FuzzHTTPRequestParam) IsPostParams() bool {
	switch p.position {
	case posPostJson, posPostQuery, posPostQueryBase64, posPostQueryJson, posPostQueryBase64Json, posPostXML:
		return true
	}
	return false
}

func (p *FuzzHTTPRequestParam) IsGetParams() bool {
	switch p.position {
	case posGetQuery, posGetQueryBase64, posGetQueryJson, posGetQueryBase64Json:
		return true
	}
	return false
}

func (p *FuzzHTTPRequestParam) IsGetValueJSON() bool {
	if p == nil {
		return false
	}

	switch p.position {
	case posGetQueryJson, posGetQueryBase64Json:
		return true
	}
	return false
}

func (p *FuzzHTTPRequestParam) IsCookieParams() bool {
	switch p.position {
	case posCookie, posCookieJson, posCookieBase64, posCookieBase64Json:
		return true
	}
	return false
}

func (p *FuzzHTTPRequestParam) Name() string {
	if p.param2nd != nil {
		return ""
	}
	return fmt.Sprintf("%v", p.param)
}

func (p *FuzzHTTPRequestParam) Position() string {
	return string(p.position)
}

func (p *FuzzHTTPRequestParam) Path() string {
	return p.path
}

func (p *FuzzHTTPRequestParam) GPath() string {
	return p.gpath
}

func (p *FuzzHTTPRequestParam) PositionVerbose() string {
	if p.gpath != "" {
		return fmt.Sprintf("%s-[%v]", PositionTypeVerbose(p.position), p.gpath)
	}
	return PositionTypeVerbose(p.position)
}

func (p *FuzzHTTPRequestParam) GetFirstValue() string {
	vals := utils.InterfaceToSliceInterface(p.Value())
	if len(vals) > 0 {
		return utils.InterfaceToString(vals[0])
	}
	return ""
}

func (p *FuzzHTTPRequestParam) OriginValue() interface{} {
	return p.raw
}

func (p *FuzzHTTPRequestParam) Value() interface{} {
	switch p.position {
	case posGetQueryBase64, posPostQueryBase64, posCookieBase64:
		switch paramOriginValue := p.raw.(type) {
		case []string:
			if len(paramOriginValue) > 0 {
				decoded, err := codec.DecodeBase64Url(paramOriginValue[0])
				if err != nil {
					break
				}
				return utils.InterfaceToStringSlice(decoded)
			} else {
				return utils.InterfaceToStringSlice("")
			}
		case string:
			decoded, err := codec.DecodeBase64Url(paramOriginValue)
			if err != nil {
				break
			}
			return utils.InterfaceToStringSlice(decoded)
		default:
			log.Error("unrecognized param value type")
			return p.raw
		}
	case posGetQueryJson, posPostJson, posCookieJson:
		var path string
		if p.gpath == "" {
			path = p.path
		} else {
			path = p.gpath
		}
		switch paramOriginValue := p.raw.(type) {
		case []string:
			if len(paramOriginValue) > 0 {
				return utils.InterfaceToStringSlice(jsonpath.Find(paramOriginValue[0], path))
			} else {
				return utils.InterfaceToStringSlice("")
			}
		case string:
			return utils.InterfaceToStringSlice(gjson.Get(paramOriginValue, path))
		default:
			log.Error("unrecognized param value type")
			return p.raw
		}

	case posGetQueryBase64Json, posPostQueryBase64Json, posCookieBase64Json:
		switch paramOriginValue := p.raw.(type) {
		case []string:
			if len(paramOriginValue) > 0 {
				jsonStr, err := codec.DecodeBase64Url(paramOriginValue[0])
				if err != nil {
					break
				}
				return utils.InterfaceToStringSlice(jsonpath.Find(jsonStr, p.path))
			} else {
				return utils.InterfaceToStringSlice("")
			}
		case string:
			jsonStr, err := codec.DecodeBase64Url(paramOriginValue)
			if err != nil {
				break
			}
			return utils.InterfaceToStringSlice(jsonpath.Find(jsonStr, p.path))

		default:
			log.Error("unrecognized param value type")
			return p.paramValue
		}

	}
	return p.paramValue
}

func (p *FuzzHTTPRequestParam) Repeat(i int) FuzzHTTPRequestIf {
	return p.origin.Repeat(i)
}

func (p *FuzzHTTPRequestParam) DisableAutoEncode(b bool) *FuzzHTTPRequestParam {
	p.origin.DisableAutoEncode(b)
	return p
}

func (p *FuzzHTTPRequestParam) Fuzz(i ...interface{}) FuzzHTTPRequestIf {
	switch p.position {
	case posMethod:
		return p.origin.FuzzMethod(InterfaceToFuzzResults(i)...)
	case posGetQuery:
		return p.origin.FuzzGetParams(p.param, i)
	case posGetQueryJson:
		return p.origin.FuzzGetJsonPathParams(p.param, p.path, i)
	case posGetQueryBase64:
		return p.origin.FuzzGetBase64Params(p.param, i)
	case posGetQueryBase64Json:
		return p.origin.FuzzGetBase64JsonPath(p.param, p.path, i)
	case posHeader:
		return p.origin.FuzzHTTPHeader(p.param, i)
	case posPath:
		return p.origin.FuzzPath(InterfaceToFuzzResults(i)...)
	case posPostJson:
		return p.origin.FuzzPostJsonParams(p, i)
	case posCookie:
		return p.origin.FuzzCookie(p.param, InterfaceToFuzzResults(i))
	case posCookieBase64:
		return p.origin.FuzzCookieBase64(p.param, InterfaceToFuzzResults(i))
	case posCookieJson:
		return p.origin.FuzzCookieJsonPath(p.param, p.path, i)
	case posCookieBase64Json:
		return p.origin.FuzzCookieBase64JsonPath(p.param, p.path, i)
	case posPostQuery:
		return p.origin.FuzzPostParams(p.param, i)
	case posPostXML:
		return p.origin.FuzzPostXMLParams(p.path, i)
	case posPostQueryBase64:
		return p.origin.FuzzPostBase64Params(p.param, i)
	case posPostQueryJson:
		return p.origin.FuzzPostJsonPathParams(p.param, p.path, i)
	case posPostQueryBase64Json:
		return p.origin.FuzzPostBase64JsonPath(p.param, p.path, i)
	case posPathAppend:
		return p.origin.FuzzPath(funk.Map(InterfaceToFuzzResults(i), func(s string) string {
			if !strings.HasPrefix(s, "/") {
				s = "/" + s
			}
			return p.origin.GetPath() + s
		}).([]string)...)
	case posBody:
		return p.origin.FuzzPostRaw(InterfaceToFuzzResults(i)...)
	case posPathBlock:
		result := strings.Split(p.origin.GetPath(), "/")
		if len(result) <= 0 {
			return p.origin.FuzzPath(InterfaceToFuzzResults(i)...)
		}
		var templates []string
		for i := 1; i < len(result); i++ {
			var resultCopy = make([]string, len(result))
			copy(resultCopy, result)
			resultCopy[i] = `{{params(placeholder)}}`
			templates = append(templates, strings.Join(resultCopy, "/"))
		}
		fuzzResults := InterfaceToFuzzResults(i)
		var finalResults []string
		for _, t := range templates {
			finalResults = append(finalResults, InterfaceToFuzzResults(t, MutateWithExtraParams(map[string][]string{
				"placeholder": fuzzResults,
			}))...)
		}
		return p.origin.FuzzPath(finalResults...)
	default:
		log.Warnf("cannot found fuzz params method identify: %v", posGetQueryJson)
		return p.origin
	}
}

func (p *FuzzHTTPRequestParam) String() string {
	if p.path != "" {
		pathName := "JsonPath"
		if p.position == posPostXML {
			pathName = "XPath"
		}
		return fmt.Sprintf("Name:%-20s %s: %-12s Position:[%v(%v)]\n", p.Name(), pathName, p.path, p.PositionVerbose(), p.Position())
	}
	return fmt.Sprintf("Name:%-20s Position:[%v(%v)]\n", p.Name(), p.PositionVerbose(), p.Position())
}

func (p *FuzzHTTPRequestParam) Debug() *FuzzHTTPRequestParam {
	fmt.Print(p.String())
	return p
}
