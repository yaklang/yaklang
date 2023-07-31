package mutate

import (
	"fmt"
	"strings"

	"github.com/asaskevich/govalidator"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/jsonextractor"
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
	typePosition     httpParamPositionType
	param            interface{}
	param2nd         interface{}
	paramOriginValue interface{}
	jsonPath         string
	origin           *FuzzHTTPRequest
}

func (p *FuzzHTTPRequestParam) IsPostParams() bool {
	if p.typePosition == posPostJson {
		return true
	}

	if p.typePosition == posPostQuery {
		return true
	}

	return false
}

func (p *FuzzHTTPRequestParam) IsGetParams() bool {
	if p.typePosition == posGetQuery {
		return true
	}

	return false
}

func (p *FuzzHTTPRequestParam) IsGetValueJSON() bool {
	if p == nil {
		return false
	}

	if !p.IsGetParams() {
		return false
	}

	valStr := utils.InterfaceToString(utils.InterfaceToString(p.Value()))
	fixedJson := jsonextractor.FixJson([]byte(valStr))
	return govalidator.IsJSON(string(fixedJson))
}

func (p *FuzzHTTPRequestParam) IsCookieParams() bool {
	if p.typePosition == posCookie {
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
	return string(p.typePosition)
}

func (p *FuzzHTTPRequestParam) PositionVerbose() string {
	return PositionTypeVerbose(p.typePosition)
}

func (p *FuzzHTTPRequestParam) Value() interface{} {
	switch p.typePosition {
	case posGetQueryBase64, posPostQueryBase64, posCookieBase64:
		switch paramOriginValue := p.paramOriginValue.(type) {
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
			return p.paramOriginValue
		}
	case posGetQueryJson, posPostJson, posCookieJson:
		switch paramOriginValue := p.paramOriginValue.(type) {
		case []string:
			if len(paramOriginValue) > 0 {
				return utils.InterfaceToStringSlice(jsonpath.Find(paramOriginValue[0], p.jsonPath))
			} else {
				return utils.InterfaceToStringSlice("")
			}
		case string:
			return utils.InterfaceToStringSlice(jsonpath.Find(paramOriginValue, p.jsonPath))
		default:
			log.Error("unrecognized param value type")
			return p.paramOriginValue
		}

	case posGetQueryBase64Json, posPostQueryBase64Json, posCookieBase64Json:
		switch paramOriginValue := p.paramOriginValue.(type) {
		case []string:
			if len(paramOriginValue) > 0 {
				jsonStr, err := codec.DecodeBase64Url(paramOriginValue[0])
				if err != nil {
					break
				}
				return utils.InterfaceToStringSlice(jsonpath.Find(jsonStr, p.jsonPath))
			} else {
				return utils.InterfaceToStringSlice("")
			}
		case string:
			jsonStr, err := codec.DecodeBase64Url(paramOriginValue)
			if err != nil {
				break
			}
			return utils.InterfaceToStringSlice(jsonpath.Find(jsonStr, p.jsonPath))

		default:
			log.Error("unrecognized param value type")
			return p.paramOriginValue
		}

	}
	return p.paramOriginValue
}

func (p *FuzzHTTPRequestParam) Repeat(i int) FuzzHTTPRequestIf {
	return p.origin.Repeat(i)
}

func (p *FuzzHTTPRequestParam) Fuzz(i ...interface{}) FuzzHTTPRequestIf {
	switch p.typePosition {
	case posMethod:
		return p.origin.FuzzMethod(InterfaceToFuzzResults(i)...)
	case posGetQuery:
		return p.origin.FuzzGetParams(p.param, i)
	case posGetQueryJson:
		return p.origin.FuzzGetJsonPathParams(p.param, p.jsonPath, i)
	case posGetQueryBase64:
		return p.origin.FuzzGetBase64Params(p.param, i)
	case posGetQueryBase64Json:
		return p.origin.FuzzGetBase64JsonPath(p.param, p.jsonPath, i)
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
		return p.origin.FuzzCookieJsonPath(p.param, p.jsonPath, i)
	case posCookieBase64Json:
		return p.origin.FuzzCookieBase64JsonPath(p.param, p.jsonPath, i)
	case posPostQuery:
		return p.origin.FuzzPostParams(p.param, i)
	case posPostQueryBase64:
		return p.origin.FuzzPostBase64Params(p.param, i)
	case posPostQueryJson:
		return p.origin.FuzzPostJsonPathParams(p.param, p.jsonPath, i)
	case posPostQueryBase64Json:
		return p.origin.FuzzPostBase64JsonPath(p.param, p.jsonPath, i)
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
		var result = strings.Split(p.origin.GetPath(), "/")
		if len(result) <= 0 {
			return p.origin.FuzzPath(InterfaceToFuzzResults(i)...)
		}
		var templates []string
		for i := 1; i < len(result); i++ {
			resultCopy := result[:]
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
	if p.jsonPath != "" {
		return fmt.Sprintf("Name:%-20s JsonPath: %-12s Position:[%v(%v)]\n", p.Name(), p.jsonPath, p.PositionVerbose(), p.Position())
	}
	return fmt.Sprintf("Name:%-20s Position:[%v(%v)]\n", p.Name(), p.PositionVerbose(), p.Position())
}

func (p *FuzzHTTPRequestParam) Debug() *FuzzHTTPRequestParam {
	fmt.Print(p.String())
	return p
}
