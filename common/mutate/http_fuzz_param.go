package mutate

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils/lowhttp"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func PositionTypeVerbose(pos lowhttp.HttpParamPositionType) string {
	switch pos {
	case lowhttp.PosMethod:
		return "HTTP方法"
	case lowhttp.PosBody:
		return "Body"
	case lowhttp.PosGetQuery:
		return "GET参数"
	case lowhttp.PosGetQueryBase64:
		return "GET参数(Base64)"
	case lowhttp.PosGetQueryJson:
		return "GET参数(JSON)"
	case lowhttp.PosGetQueryBase64Json:
		return "GET参数(Base64+JSON)"
	case lowhttp.PosPathAppend:
		return "URL路径(追加)"
	case lowhttp.PosPathBlock:
		return "URL路径(分块)"
	case lowhttp.PosPath:
		return "URL路径"
	case lowhttp.PosHeader:
		return "Header"
	case lowhttp.PosPostQuery:
		return "POST参数"
	case lowhttp.PosPostXML:
		return "POST参数(XML)"
	case lowhttp.PosPostQueryBase64:
		return "POST参数(Base64)"
	case lowhttp.PosPostQueryJson:
		return "POST参数(JSON)"
	case lowhttp.PosPostQueryBase64Json:
		return "POST参数(Base64+JSON)"
	case lowhttp.PosPostJson:
		return "JSON-Body参数"
	case lowhttp.PosCookie:
		return "Cookie参数"
	case lowhttp.PosCookieBase64:
		return "Cookie参数(Base64)"
	case lowhttp.PosCookieJson:
		return "Cookie参数(JSON)"
	case lowhttp.PosCookieBase64Json:
		return "Cookie参数(Base64+JSON)"
	default:
		return string(pos)
	}
}

type FuzzHTTPRequestParam struct {
	position lowhttp.HttpParamPositionType
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
	case lowhttp.PosPostJson, lowhttp.PosPostQuery, lowhttp.PosPostQueryBase64,
		lowhttp.PosPostQueryJson, lowhttp.PosPostQueryBase64Json, lowhttp.PosPostXML:
		return true
	}
	return false
}

func (p *FuzzHTTPRequestParam) IsGetParams() bool {
	switch p.position {
	case lowhttp.PosGetQuery, lowhttp.PosGetQueryBase64, lowhttp.PosGetQueryJson,
		lowhttp.PosGetQueryBase64Json:
		return true
	}
	return false
}

func (p *FuzzHTTPRequestParam) IsGetValueJSON() bool {
	if p == nil {
		return false
	}

	switch p.position {
	case lowhttp.PosGetQueryJson, lowhttp.PosGetQueryBase64Json:
		return true
	}
	return false
}

func (p *FuzzHTTPRequestParam) IsCookieParams() bool {
	switch p.position {
	case lowhttp.PosCookie, lowhttp.PosCookieJson, lowhttp.PosCookieBase64,
		lowhttp.PosCookieBase64Json:
		return true
	}
	return false
}

func (p *FuzzHTTPRequestParam) Name() string {
	//if p.param2nd != nil {
	//	return ""
	//}
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

func (p *FuzzHTTPRequestParam) GetFirstValue() any {
	vals := utils.InterfaceToSliceInterface(p.Value())
	if len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func (p *FuzzHTTPRequestParam) GetPostJsonPath() string {
	if p.Position() == string(lowhttp.PosPostJson) {
		return p.path
	}
	return ""
}

func (p *FuzzHTTPRequestParam) FirstValueIsNumber() bool {
	ret := p.GetFirstValue()
	return utils.IsInt(ret) || utils.IsFloat(ret)
}

func (p *FuzzHTTPRequestParam) OriginValue() interface{} {
	return p.raw
}

func (p *FuzzHTTPRequestParam) Value() interface{} {
	//switch p.position {
	//case PosGetQueryBase64, PosPostQueryBase64, PosCookieBase64:
	//	switch paramOriginValue := p.raw.(type) {
	//	case []string:
	//		if len(paramOriginValue) > 0 {
	//			decoded, err := codec.DecodeBase64Url(paramOriginValue[0])
	//			if err != nil {
	//				break
	//			}
	//			return utils.InterfaceToStringSlice(decoded)
	//		} else {
	//			return utils.InterfaceToStringSlice("")
	//		}
	//	case string:
	//		decoded, err := codec.DecodeBase64Url(paramOriginValue)
	//		if err != nil {
	//			break
	//		}
	//		return utils.InterfaceToStringSlice(decoded)
	//	default:
	//		log.Error("unrecognized param value type")
	//		return p.raw
	//	}
	//case PosGetQueryJson, PosPostJson, PosCookieJson:
	//	var path string
	//	if p.gpath == "" {
	//		path = p.path
	//	} else {
	//		path = p.gpath
	//	}
	//	switch paramOriginValue := p.raw.(type) {
	//	case []string:
	//		if len(paramOriginValue) > 0 {
	//			return utils.InterfaceToStringSlice(jsonpath.Find(paramOriginValue[0], path))
	//		} else {
	//			return utils.InterfaceToStringSlice("")
	//		}
	//	case string:
	//		return utils.InterfaceToStringSlice(gjson.Get(paramOriginValue, path))
	//	default:
	//		log.Error("unrecognized param value type")
	//		return p.raw
	//	}
	//
	//case PosGetQueryBase64Json, PosPostQueryBase64Json, PosCookieBase64Json:
	//	switch paramOriginValue := p.raw.(type) {
	//	case []string:
	//		if len(paramOriginValue) > 0 {
	//			jsonStr, err := codec.DecodeBase64Url(paramOriginValue[0])
	//			if err != nil {
	//				break
	//			}
	//			return utils.InterfaceToStringSlice(jsonpath.Find(jsonStr, p.path))
	//		} else {
	//			return utils.InterfaceToStringSlice("")
	//		}
	//	case string:
	//		jsonStr, err := codec.DecodeBase64Url(paramOriginValue)
	//		if err != nil {
	//			break
	//		}
	//		return utils.InterfaceToStringSlice(jsonpath.Find(jsonStr, p.path))
	//
	//	default:
	//		log.Error("unrecognized param value type")
	//		return p.paramValue
	//	}
	//
	//}
	return p.paramValue
}

func (p *FuzzHTTPRequestParam) ValueString() string {
	return utils.InterfaceToString(p.Value())
}

func (p *FuzzHTTPRequestParam) Repeat(i int) FuzzHTTPRequestIf {
	return p.origin.Repeat(i)
}

func (p *FuzzHTTPRequestParam) DisableAutoEncode(b bool) *FuzzHTTPRequestParam {
	p.origin.DisableAutoEncode(b)
	return p
}

func (p *FuzzHTTPRequestParam) FriendlyDisplay() *FuzzHTTPRequestParam {
	p.origin.FriendlyDisplay()
	return p
}

func (p *FuzzHTTPRequestParam) Fuzz(i ...interface{}) FuzzHTTPRequestIf {
	p.origin.mode = paramsFuzz
	switch p.position {
	case lowhttp.PosMethod:
		return p.origin.FuzzMethod(InterfaceToFuzzResults(i)...)
	case lowhttp.PosGetQuery:
		return p.origin.FuzzGetParams(p.param, i)
	case lowhttp.PosGetQueryJson:
		return p.origin.FuzzGetJsonPathParams(p.param, p.path, i)
	case lowhttp.PosGetQueryBase64:
		return p.origin.FuzzGetBase64Params(p.param, i)
	case lowhttp.PosGetQueryBase64Json:
		return p.origin.FuzzGetBase64JsonPath(p.param, p.path, i)
	case lowhttp.PosHeader:
		return p.origin.FuzzHTTPHeader(p.param, i)
	case lowhttp.PosPath:
		return p.origin.FuzzPath(InterfaceToFuzzResults(i)...)
	case lowhttp.PosCookie:
		return p.origin.FuzzCookie(p.param, InterfaceToFuzzResults(i))
	case lowhttp.PosCookieBase64:
		return p.origin.FuzzCookieBase64(p.param, InterfaceToFuzzResults(i))
	case lowhttp.PosCookieJson:
		return p.origin.FuzzCookieJsonPath(p.param, p.path, i)
	case lowhttp.PosCookieBase64Json:
		return p.origin.FuzzCookieBase64JsonPath(p.param, p.path, i)
	case lowhttp.PosPostJson:
		return p.origin.FuzzPostJsonParams(p, i)
	case lowhttp.PosPostQuery:
		return p.origin.FuzzPostParams(p.param, i)
	case lowhttp.PosPostXML:
		return p.origin.FuzzPostXMLParams(p.path, i)
	case lowhttp.PosPostQueryBase64:
		return p.origin.FuzzPostBase64Params(p.param, i)
	case lowhttp.PosPostQueryJson:
		return p.origin.FuzzPostJsonPathParams(p.param, p.path, i)
	case lowhttp.PosPostQueryBase64Json:
		return p.origin.FuzzPostBase64JsonPath(p.param, p.path, i)
	case lowhttp.PosPathAppend:
		return p.origin.FuzzPath(funk.Map(InterfaceToFuzzResults(i), func(s string) string {
			if !strings.HasPrefix(s, "/") {
				s = "/" + s
			}
			return p.origin.GetPath() + s
		}).([]string)...)
	case lowhttp.PosBody:
		return p.origin.FuzzPostRaw(InterfaceToFuzzResults(i)...)
	case lowhttp.PosPathBlock:
		result := strings.Split(p.origin.GetPath(), "/")
		if len(result) <= 0 {
			return p.origin.FuzzPath(InterfaceToFuzzResults(i)...)
		}
		var templates []string
		for i := 1; i < len(result); i++ {
			resultCopy := make([]string, len(result))
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
		log.Warnf("cannot found fuzz params method identify: %v", lowhttp.PosGetQueryJson)
		return p.origin
	}
}

func (p *FuzzHTTPRequestParam) String() string {
	if p.path != "" {
		pathName := "JsonPath"
		if p.position == lowhttp.PosPostXML {
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
