package suspect

import (
	"fmt"
	"net/http"
	"reflect"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/utils/lowhttp"
)

var GuessExports = map[string]interface{}{
	"IsAlpha": func(i interface{}) bool {
		return utils.MatchAllOfRegexp(i, `[a-zA-Z]+`)
	},
	"IsDigit": func(i interface{}) bool {
		return utils.MatchAllOfRegexp(i, `[0-9]+`)
	},
	"IsAlphaNum": func(i interface{}) bool {
		return utils.MatchAllOfRegexp(i, `[a-zA-Z0-9]+`)
	},
	"IsAlNum": func(i interface{}) bool {
		return utils.MatchAllOfRegexp(i, `[a-zA-Z0-9]+`)
	},
	"IsTLSServer": utils.IsTLSService,
	"IsHttpURL":   IsFullURL,
	"IsUrlPath":   IsURLPath,
	"IsHtmlResponse": func(i interface{}) bool {
		switch ret := i.(type) {
		case string:
			rsp, err := lowhttp.ParseBytesToHTTPResponse([]byte(ret))
			if err != nil {
				log.Error(err)
				return false
			}
			return IsHTMLResponse(rsp)
		case []byte:
			rsp, err := lowhttp.ParseBytesToHTTPResponse(ret)
			if err != nil {
				log.Error(err)
				return false
			}
			return IsHTMLResponse(rsp)
		case *http.Response:
			return IsHTMLResponse(ret)
		default:
			log.Errorf("need []byte/string/*http.Response but got %s", reflect.TypeOf(ret))
			return false
		}
	},
	"IsServerError": func(i interface{}) bool {
		switch ret := i.(type) {
		case string:
			return HaveServerError([]byte(ret))
		case []byte:
			return HaveServerError(ret)
		default:
			return HaveServerError([]byte(fmt.Sprint(ret)))
		}
	},
	"ExtractChineseIDCards": func(i interface{}) []string {
		switch ret := i.(type) {
		case string:
			return SearchChineseIDCards([]byte(ret))
		case []byte:
			return SearchChineseIDCards(ret)
		default:
			return SearchChineseIDCards([]byte(fmt.Sprint(ret)))
		}
	},
	"IsJsonResponse": func(i interface{}) bool {
		switch ret := i.(type) {
		case string:
			rsp, err := lowhttp.ParseBytesToHTTPResponse([]byte(ret))
			if err != nil {
				log.Error(err)
				return false
			}
			return IsJsonResponse(rsp)
		case []byte:
			rsp, err := lowhttp.ParseBytesToHTTPResponse(ret)
			if err != nil {
				log.Error(err)
				return false
			}
			return IsJsonResponse(rsp)
		case *http.Response:
			return IsJsonResponse(ret)
		default:
			log.Errorf("need []byte/string/*http.Response but got %s", reflect.TypeOf(ret))
			return false
		}
	},
	"IsRedirectParam":       BeUsedForRedirect,
	"IsJSONPParam":          IsJSONPParam,
	"IsUrlParam":            IsGenericURLParam,
	"IsXmlParam":            IsXMLParam,
	"IsSensitiveJson":       IsSensitiveJSON,
	"IsSensitiveTokenField": IsTokenParam,
	"IsPasswordField":       IsPasswordKey,
	"IsUsernameField":       IsUsernameKey,
	"IsSQLColumnField":      IsSQLColumnName,
	"IsCaptchaField":        IsCaptchaKey,
	"IsBase64Value":         IsBase64,
	"IsPlainBase64Value":    IsBase64Password,
	"IsMD5Value":            IsMD5Data,
	"IsSha256Value":         IsSHA256Data,
	"IsXmlRequest": func(i interface{}) bool {
		switch ret := i.(type) {
		case []byte:
			return IsXMLRequest(ret)
		case string:
			return IsXMLRequest([]byte(ret))
		case *http.Request:
			raw, _ := utils.HttpDumpWithBody(i, true)
			return IsXMLRequest(raw)
		default:
			return false
		}
	},
	"IsXmlValue": func(i interface{}) bool {
		switch ret := i.(type) {
		case string:
			return IsXMLString(ret)
		case []byte:
			return IsXMLBytes(ret)
		}
		return false
	},
}
