package suspect

import (
	"net/http"
	"reflect"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// IsAlpha 尝试将传入的参数转换为字符串，然后判断其是否都由英文字母组成
// Example:
// ```
// str.IsAlpha("abc") // true
// str.IsAlpha("abc123") // false
// ```
func isAlpha(i interface{}) bool {
	return utils.MatchAllOfRegexp(i, `^[a-zA-Z]+$`)
}

// IsDigit 尝试将传入的参数转换为字符串，然后判断其是否都由数字组成
// Example:
// ```
// str.IsDigit("123") // true
// str.IsDigit("abc123") // false
// ```
func isDigit(i interface{}) bool {
	return utils.MatchAllOfRegexp(i, `^[0-9]+$`)
}

// IsAlphaNum / IsAlNum 尝试将传入的参数转换为字符串，然后判断其是否都由英文字母和数字组成
// Example:
// ```
// str.IsAlphaNum("abc123") // true
// str.IsAlphaNum("abc123!") // false
// ```
func isAlphaNum(i interface{}) bool {
	return utils.MatchAllOfRegexp(i, `^[a-zA-Z0-9]+$`)
}

// IsTLSServer 尝试访问传入的host，然后判断其是否为 TLS 服务。第一个参数为 host，后面可以传入零个或多个参数，为代理地址
// Example:
// ```
// str.IsTLSServer("www.yaklang.com:443") // true
// str.IsTLSServer("www.yaklang.com:80") // false
// ```
func isTLSServer(addr string, proxies ...string) bool {
	return netx.IsTLSService(addr, proxies...)
}

// IsHttpURL 尝试将传入的参数转换为字符串，然后猜测其是否为 http(s) 协议的 URL
// Example:
// ```
// str.IsHttpURL("http://www.yaklang.com") // true
// str.IsHttpURL("https://www.yaklang.com") // true
// str.IsHttpURL("www.yaklang.com") // false
// ```
func isHttpURL(i interface{}) bool {
	return IsFullURL(i)
}

// IsUrlPath 尝试将传入的参数转换为字符串，然后猜测其是否为 URL 路径
// Example:
// ```
// str.IsUrlPath("/index.php") // true
// str.IsUrlPath("index.php") // false
// ```
func isUrlPath(i interface{}) bool {
	return IsURLPath(i)
}

// IsHtmlResponse 猜测传入的参数是否为原始 HTTP 响应报文
// Example:
// ```
// str.IsHtmlResponse("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n<html></html>") // true
// resp, _ = str.ParseStringToHTTPResponse("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n<html></html>")
// str.IsHtmlResponse(resp) // true
// ```
func isHtmlResponse(i interface{}) bool {
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
}

// IsServerError 猜测传入的参数是否为服务器错误
// Example:
// ```
// str.IsServerError(`Fatal error: Uncaught Error: Call to undefined function sum() in F:\xampp\htdocs\test.php:7 Stack trace: #0 {main} thrown in <path> on line 7`) // true，这是PHP报错信息
// ```
func isServerError(i interface{}) bool {
	switch ret := i.(type) {
	case string:
		return HaveServerError([]byte(ret))
	case []byte:
		return HaveServerError(ret)
	default:
		return HaveServerError(utils.InterfaceToBytes(ret))
	}
}

// ExtractChineseIDCards 尝试将传入的参数转换为字符串，然后提取字符串中的身份证号
// Example:
// ```
// str.ExtractChineseIDCards("Your ChineseID is: 110101202008027420?") // ["110101202008027420"]
// ```
func extractChineseIDCards(i interface{}) []string {
	switch ret := i.(type) {
	case string:
		return SearchChineseIDCards([]byte(ret))
	case []byte:
		return SearchChineseIDCards(ret)
	default:
		return SearchChineseIDCards(utils.InterfaceToBytes(ret))
	}
}

// IsJsonResponse 尝试将传入的参数转换为字符串，然后猜测传入的参数是否为 JSON 格式的原始 HTTP 响应报文，这是通过判断Content-Type请求头实现的
// Example:
// ```
// str.IsJsonResponse("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{\"code\": 0}") // true
// str.IsJsonResponse("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\nhello") // false
// ```
func isJsonResponse(i interface{}) bool {
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
}

// IsRedirectParam 根据传入的参数名和参数值猜测是否为重定向参数
// Example:
// ```
// str.IsRedirectParam("to","http://www.yaklang.com") // true，因为参数值为完整的 URL
// str.IsRedirectParam("target","/index.php") // true，因为参数值为一个 URL 路径而且参数名为常见的跳转的参数名
// str.IsRedirectParam("id", "1") // false
// ```
func isRedirectParam(key, value string) bool {
	return BeUsedForRedirect(key, value)
}

// IsJSONPParam 根据传入的参数名和参数值猜测是否为 JSONP 参数
// Example:
// ```
// str.IsJSONPParam("callback","jquery1.0.min.js") // true，因为参数名为常见的 JSONP 参数名，且参数值为常见的JS文件名
// str.IsJSONPParam("f","jquery1.0.min.js") // true，因为参数值为常见的 JS 文件名
// str.IsJSONPParam("id","1") // false
// ```
func isJSONPParam(key, value string) bool {
	return IsJSONPParam(key, value)
}

// IsUrlParam 根据传入的参数名和参数值猜测是否为 URL 参数
// Example:
// ```
// str.IsUrlParam("url","http://www.yaklang.com") // true，因为参数名为常见的 URL 参数名，且参数值为完整的URL
// str.IsUrlParam("id","1") // false
// ```
func isURLParam(key, value string) bool {
	return IsGenericURLParam(key, value)
}

// IsXmlParam 根据传入的参数名和参数值猜测是否为 XML 参数
// Example:
// ```
// str.IsXmlParam("xml","<xml></xml>") // true，因为参数名为常见的 XML 参数名，且参数值为 XML 格式的字符串
// str.IsXmlParam("X","<xml></xml>") // true，因为参数值为 XML 格式的字符串
// str.IsXmlParam("id","1") // false
// ```
func isXMLParam(key, value string) bool {
	return IsXMLParam(key, value)
}

// IsSensitiveJson  尝试将传入的参数转换为字符串，然后猜测其是否为敏感的 JSON 数据
// Example:
// ```
// str.IsSensitiveJson(`{"password":"123456"}`) // true
// str.IsSensitiveJson(`{"uid": 10086}`) // true
// str.IsSensitiveJson(`{"id": 1}`) // false
// ```
func isSensitiveJson(i interface{}) bool {
	switch ret := i.(type) {
	case string:
		return IsSensitiveJSON([]byte(ret))
	case []byte:
		return IsSensitiveJSON(ret)
	default:
		return IsSensitiveJSON(utils.InterfaceToBytes(ret))
	}
}

// IsSensitiveTokenField 尝试将传入的参数转换为字符串，然后猜测其是否是 token 字段
// Example:
// ```
// str.IsSensitiveTokenField("token") // true
// str.IsSensitiveTokenField("access_token") // true
// str.IsSensitiveTokenField("id") // false
// ```
func isSensitiveTokenField(i interface{}) bool {
	switch ret := i.(type) {
	case string:
		return IsTokenParam(ret)
	case []byte:
		return IsTokenParam(string(ret))
	default:
		return IsTokenParam(utils.InterfaceToString(ret))
	}
}

// IsPasswordField 尝试将传入的参数转换为字符串，然后猜测其是否是 password 字段
// Example:
// ```
// str.IsPasswordField("password") // true
// str.IsPasswordField("pwd") // true
// str.IsPasswordField("id") // false
// ```
func isPasswordField(i interface{}) bool {
	switch ret := i.(type) {
	case string:
		return IsPasswordKey(ret)
	case []byte:
		return IsPasswordKey(string(ret))
	default:
		return IsPasswordKey(utils.InterfaceToString(ret))
	}
}

// IsUsernameField 尝试将传入的参数转换为字符串，然后猜测其是否是 username 字段
// Example:
// ```
// str.IsUsernameField("username") // true
// str.IsUsernameField("user") // true
// str.IsUsernameField("id") // false
// ```
func isUsernameField(i interface{}) bool {
	switch ret := i.(type) {
	case string:
		return IsUsernameKey(ret)
	case []byte:
		return IsUsernameKey(string(ret))
	default:
		return IsUsernameKey(utils.InterfaceToString(ret))
	}
}

// IsSQLColumnField 尝试将传入的参数转换为字符串，然后猜测其是否是 SQL 查询字段
// Example:
// ```
// str.IsSQLColumnField("sort") // true
// str.IsSQLColumnField("order") // true
// str.IsSQLColumnField("id") // false
// ```
func isSQLColumnField(i interface{}) bool {
	switch ret := i.(type) {
	case string:
		return IsSQLColumnName(ret)
	case []byte:
		return IsSQLColumnName(string(ret))
	default:
		return IsSQLColumnName(utils.InterfaceToString(ret))
	}
}

// IsCaptchaField 尝试将传入的参数转换为字符串，然后猜测其是否是验证码字段
// Example:
// ```
// str.IsCaptchaField("captcha") // true
// str.IsCaptchaField("code_img") // true
// str.IsCaptchaField("id") // false
// ```
func isCaptchaField(i interface{}) bool {
	switch ret := i.(type) {
	case string:
		return IsCaptchaKey(ret)
	case []byte:
		return IsCaptchaKey(string(ret))
	default:
		return IsCaptchaKey(utils.InterfaceToString(ret))
	}
}

// IsBase64Value 尝试将传入的参数转换为字符串，然后猜测其是否是 Base64 编码的数据
// Example:
// ```
// str.IsBase64Value("MTI=") // true
// str.IsBase64Value("123") // false
// ```
func isBase64Value(i interface{}) bool {
	switch ret := i.(type) {
	case string:
		return IsBase64(ret)
	case []byte:
		return IsBase64(string(ret))
	default:
		return IsBase64(utils.InterfaceToString(ret))
	}
}

// IsPlainBase64Value 尝试将传入的参数转换为字符串，然后猜测其是否是 Base64 编码的数据，它相比于 IsBase64Value 多了一层判断，即判断 base64 解码后的数据是否为可见字符串
// Example:
// ```
// str.IsPlainBase64Value("MTI=") // true
// str.IsPlainBase64Value("Aw==") // false
// ```
func isPlainBase64Value(i interface{}) bool {
	switch ret := i.(type) {
	case string:
		return IsBase64Password(ret)
	case []byte:
		return IsBase64Password(string(ret))
	default:
		return IsBase64Password(utils.InterfaceToString(ret))
	}
}

// IsMD5Value 尝试将传入的参数转换为字符串，然后猜测其是否是 MD5 编码的数据
// Example:
// ```
// str.IsMD5Value("202cb962ac59075b964b07152d234b70") // true
// str.IsMD5Value("123") // false
// ```
func isMD5Value(i interface{}) bool {
	switch ret := i.(type) {
	case string:
		return IsMD5Data(ret)
	case []byte:
		return IsMD5Data(string(ret))
	default:
		return IsMD5Data(utils.InterfaceToString(ret))
	}
}

// IsSha256Value 尝试将传入的参数转换为字符串，然后猜测其是否是 SHA256 编码的数据
// Example:
// ```
// str.IsSha256Value("a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3") // true
// str.IsSha256Value("123") // false
// ```
func isSha256Value(i interface{}) bool {
	switch ret := i.(type) {
	case string:
		return IsSHA256Data(ret)
	case []byte:
		return IsSHA256Data(string(ret))
	default:
		return IsSHA256Data(utils.InterfaceToString(ret))
	}
}

// IsXmlRequest 猜测传入的参数是否为请求头是 XML 格式的原始 HTTP 请求报文
// Example:
// ```
// str.IsXmlRequest("POST / HTTP/1.1\r\nContent-Type: application/xml\r\n\r\n<xml></xml>") // true
// str.IsXmlRequest("POST / HTTP/1.1\r\nContent-Type: text/html\r\n\r\n<html></html>") // false
// ```
func isXMLRequest(i interface{}) bool {
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
}

// IsXmlValue 尝试将传入的参数转换为字符串，然后猜测其是否是 XML 格式的数据
// Example:
// ```
// str.IsXmlValue("<xml></xml>") // true
// str.IsXmlValue("<html></html>") // false
// ```
func isXmlValue(i interface{}) bool {
	switch ret := i.(type) {
	case string:
		return IsXMLString(ret)
	case []byte:
		return IsXMLBytes(ret)
	default:
		return false
	}
}

var GuessExports = map[string]interface{}{
	"IsAlpha":               isAlpha,
	"IsDigit":               isDigit,
	"IsAlphaNum":            isAlphaNum,
	"IsAlNum":               isAlphaNum,
	"IsTLSServer":           isTLSServer,
	"IsHttpURL":             IsFullURL,
	"IsUrlPath":             IsURLPath,
	"IsHtmlResponse":        isHtmlResponse,
	"IsServerError":         isServerError,
	"ExtractChineseIDCards": extractChineseIDCards,
	"IsJsonResponse":        isJsonResponse,
	"IsRedirectParam":       isRedirectParam,
	"IsJSONPParam":          isJSONPParam,
	"IsUrlParam":            isURLParam,
	"IsXmlParam":            isXMLParam,
	"IsSensitiveJson":       isSensitiveJson,
	"IsSensitiveTokenField": isSensitiveTokenField,
	"IsPasswordField":       isPasswordField,
	"IsUsernameField":       isUsernameField,
	"IsSQLColumnField":      isSQLColumnField,
	"IsCaptchaField":        isCaptchaField,
	"IsBase64Value":         isBase64Value,
	"IsPlainBase64Value":    isPlainBase64Value,
	"IsMD5Value":            isMD5Value,
	"IsSha256Value":         isSha256Value,
	"IsXmlRequest":          isXMLRequest,
	"IsXmlValue":            isXmlValue,
	"IsAllVisibleASCII":     isAllVisibleASCII,
}
