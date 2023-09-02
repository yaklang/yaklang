package lowhttp

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"

	"github.com/yaklang/yaklang/common/utils"
)

func NewRequestPacketFromMethod(method string, u string, originRequest []byte, originRequestHttps bool, cookies ...*http.Cookie) []byte {
	originReqIns, err := ParseBytesToHttpRequest(originRequest)
	parseReqOk := err == nil

	reqIns, err := http.NewRequest(method, u, http.NoBody)
	if err != nil {
		return nil
	}
	reqIns.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; rv:68.0) Gecko/20100101 Firefox/68.0")
	if parseReqOk {
		if len(cookies) > 0 {
			cookies = append(cookies, originReqIns.Cookies()...)
		} else {
			cookies = originReqIns.Cookies()
		}

		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil
		}

		jar.SetCookies(reqIns.URL, cookies)
		reqIns.Header.Set("Cookie", CookiesToString(jar.Cookies(reqIns.URL)))
	}

	reqRaw, err := utils.HttpDumpWithBody(reqIns, true)
	if err != nil {
		return nil
	}
	return FixHTTPRequest(reqRaw)
}

func UrlToGetRequestPacket(u string, originRequest []byte, originRequestHttps bool, cookies ...*http.Cookie) []byte {
	return urlToRequestPacket("GET", u, originRequest, originRequestHttps, -1, cookies...)
}

// 提取响应码以处理307和302的问题
func UrlToGetRequestPacketWithResponse(u string, originRequest, originResponse []byte, originRequestHttps bool, cookies ...*http.Cookie) []byte {
	res, err := ParseBytesToHTTPResponse(originResponse)
	statusCode := -1
	if err == nil {
		statusCode = res.StatusCode
	}
	return urlToRequestPacket("GET", u, originRequest, originRequestHttps, statusCode, cookies...)
}

func UrlToRequestPacket(method string, u string, originRequest []byte, originRequestHttps bool, cookies ...*http.Cookie) []byte {
	return urlToRequestPacket(method, u, originRequest, originRequestHttps, -1, cookies...)
}

func urlToRequestPacket(method string, u string, originRequest []byte, https bool, responseStatusCode int, cookies ...*http.Cookie) []byte {
	// 无原始请求或者303状态码，直接构造请求
	if originRequest == nil || responseStatusCode == http.StatusSeeOther {
		return NewRequestPacketFromMethod(method, u, originRequest, https, cookies...)
	}

	originReqIns, err := ParseBytesToHttpRequest(originRequest)
	if err != nil {
		return nil
	}

	// originUrl, err := ExtractURLFromHTTPRequestRaw(originRequest, https)
	// if err != nil {
	// 	return nil
	// }
	// originHost, _, _ := utils.ParseStringToHostPort(originUrl.String())
	// currentHost, _, _ := utils.ParseStringToHostPort(u)
	// inSameOrigin := originHost != "" && (strings.Contains(currentHost, originHost) || strings.Contains(originHost, currentHost))
	// sameMethod := strings.ToUpper(reqIns.Method) == strings.ToUpper(method)

	// 307状态码保留请求体和请求方法，改变url，添加cookie
	// 302状态码在规范下也应该保留请求体和请求方法，但是实际上大部分浏览器都会改为GET请求，所以我们就不保留
	if responseStatusCode == http.StatusTemporaryRedirect {
		originReqIns.URL, _ = url.Parse(u)
		if originReqIns.URL == nil {
			return nil
		}
		originReqIns.Host = originReqIns.URL.Host
		originReqIns.RequestURI = originReqIns.URL.RequestURI()

		if len(cookies) > 0 {
			jar, err := cookiejar.New(nil)
			if err != nil {
				return nil
			}
			jar.SetCookies(originReqIns.URL, append(originReqIns.Cookies(), cookies...))
			originReqIns.Header.Set("Cookie", CookiesToString(jar.Cookies(originReqIns.URL)))
		}

		raw, err := utils.HttpDumpWithBody(originReqIns, true)
		if err != nil {
			return nil
		}

		return raw
	}

	return NewRequestPacketFromMethod(method, u, originRequest, https, cookies...)
}
