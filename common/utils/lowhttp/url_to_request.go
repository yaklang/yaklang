package lowhttp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/yaklang/yaklang/common/log"

	"github.com/yaklang/yaklang/common/utils"
)

func NewRequestPacketFromMethod(method string, targetURL string, originRequest []byte, originReqIns *http.Request, https bool, cookies ...*http.Cookie) []byte {
	if !utils.IsHttpOrHttpsUrl(targetURL) {
		urlIns, _ := ExtractURLFromHTTPRequest(originReqIns, https)
		if urlIns != nil {
			nu, _ := utils.UrlJoin(urlIns.String(), targetURL)
			if nu != "" {
				targetURL = nu
			}
		}
	}

	targetURLIns, err := url.Parse(targetURL)
	if err != nil || targetURLIns == nil {
		return nil
	}

	raw := bytes.Clone(originRequest)

	cookieRaw := CookiesToString(cookies)
	if len(cookieRaw) == 0 {
		raw = DeleteHTTPPacketHeader(raw, "Cookie")
	} else {
		raw = ReplaceHTTPPacketHeader(raw, "Cookie", cookieRaw)
	}

	proto := "HTTP/1.1"
	if originReqIns != nil {
		proto = originReqIns.Proto
	}
	//如果能过url解析，那么将按照原URL发送，
	raw = ReplaceHTTPPacketFirstLine(raw, fmt.Sprintf("%s %s %s", method, utils.GetSimpleUri(targetURLIns), proto))
	raw = ReplaceHTTPPacketHost(raw, targetURLIns.Host)

	return raw
}

func UrlToGetRequestPacket(u string, originRequest []byte, originRequestHttps bool, cookies ...*http.Cookie) []byte {
	raw, err := UrlToRequestPacketEx(http.MethodGet, u, originRequest, originRequestHttps, -1, cookies...)
	if err != nil {
		log.Warnf("url to GET request packet error: %v", err)
	}
	return raw
}

func UrlToRequestPacket(method string, u string, originRequest []byte, originRequestHttps bool, cookies ...*http.Cookie) []byte {
	raw, err := UrlToRequestPacketEx(method, u, originRequest, originRequestHttps, -1, cookies...)
	if err != nil {
		log.Warnf("url to request packet error: %v", err)
	}
	return raw
}

func UrlToRequestPacketEx(method string, targetURL string, originRequest []byte, https bool, statusCode int, cookies ...*http.Cookie) ([]byte, error) {
	var raw []byte

	// 303/302
	// 302在规范下也应该保留请求体和请求方法，但是实际上大部分浏览器都会改为GET请求，所以我们就不保留
	is302Or303 := statusCode == http.StatusSeeOther || statusCode == http.StatusFound
	if is302Or303 {
		method = http.MethodGet
	}
	var (
		originReqIns *http.Request
		err          error
	)
	if len(originRequest) > 0 {
		originReqIns, err = ParseBytesToHttpRequest(originRequest)
		if err != nil && err != io.EOF {
			return nil, utils.Wrap(err, "parse bytes to http request error")
		}
		if originReqIns == nil {
			return nil, utils.Error("parse bytes to http request error, empty request")
		}
		if originReqIns.URL != nil {
			if https {
				// fix https externally
				originReqIns.URL.Scheme = "https"
			} else if originReqIns.URL.Scheme == "" {
				originReqIns.URL.Scheme = "http"
			}
		}
		if method == "" {
			method = originReqIns.Method
		}
	}

	raw = NewRequestPacketFromMethod(method, targetURL, originRequest, originReqIns, https, cookies...)
	if is302Or303 {
		raw = ReplaceHTTPPacketBodyFast(raw, nil)
		raw = DeleteHTTPPacketHeader(raw, "Content-Length")
		raw = DeleteHTTPPacketHeader(raw, "Transfer-Encoding")
		raw = DeleteHTTPPacketHeader(raw, "Content-Type")
	}
	if originReqIns != nil && originReqIns.URL != nil {
		raw = ReplaceHTTPPacketHeader(raw, "Referer", originReqIns.URL.String())
	}

	return FixHTTPRequest(raw), nil
}

// BuildRedirectRequest 用于生成重定向请求包
func BuildRedirectRequest(targetUrl string, originRequest []byte, originRequestIsHttps bool, statusCode int) ([]byte, error) {
	// 重写方法
	rewriteMethod := statusCode == http.StatusSeeOther || statusCode == http.StatusFound || statusCode == http.StatusMovedPermanently
	method := http.MethodGet
	if !rewriteMethod {
		method = GetHTTPRequestMethod(originRequest)
	}

	// 解析原始请求
	originReqIns, err := ParseBytesToHttpRequest(originRequest)
	if err != nil && err != io.EOF {
		raw := NewRequestPacketFromMethod(method, targetUrl, nil, originReqIns, false)
		return FixHTTPRequest(raw), nil
	}
	copyedUrl := *originReqIns.URL
	if originRequestIsHttps {
		copyedUrl.Scheme = "https"
	} else {
		copyedUrl.Scheme = "http"
	}
	// 判断同源，如果同源，则允许保留Authorization头
	isSameOrigin, err := isSameOrigin(copyedUrl.String(), targetUrl)
	if err != nil {
		log.Errorf("is same origin error: %v", err)
		isSameOrigin = false
	}

	// 生成重定向请求包
	raw := NewRequestPacketFromMethod(method, targetUrl, nil, originReqIns, false)
	raw = ReplaceHTTPPacketBodyFast(raw, nil)
	allowHeaders := []string{"User-Agent", "Accept", "Accept-Encoding", "Accept-Language"}
	if isSameOrigin {
		allowHeaders = append(allowHeaders, "Authorization")
	}
	for _, header := range allowHeaders {
		headerValue := GetHTTPPacketHeader(originRequest, header)
		if headerValue != "" {
			raw = ReplaceHTTPPacketHeader(raw, header, headerValue)
		}
	}

	if originReqIns != nil && originReqIns.URL != nil {
		raw = ReplaceHTTPPacketHeader(raw, "Referer", originReqIns.URL.String())
	}

	return FixHTTPRequest(raw), nil
}

func isSameOrigin(originUrl, targetUrl string) (bool, error) {
	// 同源判定：scheme、host、port 一致
	// - 端口：空端口按默认端口归一化（http=80，https=443）
	// - 相对/协议相对 URL：先基于 originUrl 解析为绝对 URL 再比较
	originUrlIns, err := url.Parse(originUrl)
	if err != nil {
		return false, err
	}
	targetUrlIns, err := url.Parse(targetUrl)
	if err != nil {
		return false, err
	}
	if originUrlIns.Scheme != targetUrlIns.Scheme {
		return false, nil
	}

	// scheme/host 比较（大小写不敏感）
	if !strings.EqualFold(originUrlIns.Scheme, targetUrlIns.Scheme) {
		return false, nil
	}
	if !strings.EqualFold(originUrlIns.Hostname(), targetUrlIns.Hostname()) {
		return false, nil
	}

	// 端口归一化：仅对 http/https 赋默认端口，其它保持原样
	originPort := originUrlIns.Port()
	targetPort := targetUrlIns.Port()
	if originPort == "" {
		if strings.EqualFold(originUrlIns.Scheme, "https") {
			originPort = "443"
		} else if strings.EqualFold(originUrlIns.Scheme, "http") {
			originPort = "80"
		}
	}
	if targetPort == "" {
		if strings.EqualFold(targetUrlIns.Scheme, "https") {
			targetPort = "443"
		} else if strings.EqualFold(targetUrlIns.Scheme, "http") {
			targetPort = "80"
		}
	}

	if originPort != targetPort {
		return false, nil
	}
	return true, nil
}

func UrlToHTTPRequest(text string) ([]byte, error) {
	var r *http.Request
	if !(strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://")) {
		text = "http://" + text
	}
	u := utils.ParseStringToUrl(text)
	if u == nil {
		return nil, errors.New("parse url error")
	}
	r, err := http.NewRequest("GET", text, http.NoBody)
	if err != nil {
		return nil, err
	}

	if u.RawPath == "" && u.Path == "" {
		u.Path = "/"
	}

	if u.RawPath != "" {
		r.RequestURI = u.RawPath
	} else {
		r.RequestURI = u.Path
	}

	if u.RawQuery != "" {
		r.RequestURI += "?" + u.RawQuery
	}

	if u.RawFragment != "" {
		r.RequestURI += "#" + u.RawFragment
	} else if u.Fragment != "" {
		r.RequestURI += "#" + url.PathEscape(u.Fragment)
	} else if strings.HasSuffix(text, "#") {
		r.RequestURI += "#"
	}

	raw := simpleDumpHTTPRequest(r)

	raw = FixHTTPRequest(raw)
	return raw, nil
}

func simpleDumpHTTPRequest(r *http.Request) []byte {
	var buf bytes.Buffer
	buf.WriteString(r.Method)
	buf.WriteString(" ")
	buf.WriteString(r.RequestURI)

	buf.WriteString(" ")
	r.Proto = fmt.Sprint("HTTP/", r.ProtoMajor, ".", r.ProtoMinor)
	buf.WriteString(r.Proto)
	buf.WriteString(CRLF)

	// handle host
	buf.WriteString("Host: ")
	if r.Host != "" {
		buf.WriteString(r.Host)
	} else if r.URL.Host != "" {
		buf.WriteString(r.URL.Host)
	}

	buf.WriteString(CRLF)
	//Accept-Encoding: gzip, deflate, br
	//Accept: */*
	//Accept-Language: en-US;q=0.9,en;q=0.8
	//User-Agent: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36
	//Connection: close
	//Cache-Control: max-age=0
	buf.WriteString("Accept-Encoding: gzip, deflate, br\r\n")
	buf.WriteString("Accept: */*\r\n")
	buf.WriteString("Accept-Language: en-US;q=0.9,en;q=0.8\r\n")
	buf.WriteString("User-Agent: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36\r\n")
	buf.WriteString("Cache-Control: max-age=0\r\n")
	buf.WriteString("\r\n")
	return buf.Bytes()
}

// FixHttpURL fill the scheme and simplify the host
// Example: FixHttpURL("example.com") => "http://example.com" FixHttpURL("https://example.com:443/abc") => "https://example.com/abc"
func FixHttpURL(u string) (string, error) {
	// fix url scheme by port if not set scheme
	u = FixURLScheme(u)
	urlPath := utils.ExtractRawPath(u)
	host, port, _ := utils.ParseStringToHostPort(u)
	if host == "" {
		return "", errors.New("empty host")
	}
	isHttps := false
	if strings.HasPrefix(u, "https://") {
		isHttps = true
	}
	// fix port by scheme
	if port == 0 {
		if isHttps {
			port = 443
		} else {
			port = 80
		}
	}
	if isHttps {
		if port == 443 {
			return fmt.Sprintf("https://%s%s", host, urlPath), nil
		}
		return fmt.Sprintf("https://%s:%d%s", host, port, urlPath), nil
	}
	if port == 80 {
		return fmt.Sprintf("http://%s%s", host, urlPath), nil
	}
	return fmt.Sprintf("http://%s:%d%s", host, port, urlPath), nil
}
func FixURLScheme(u string) string {
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	ins := utils.ParseStringToUrl(u)
	if port := ins.Port(); port == "443" {
		ins.Scheme = "https"
		ins.Host = ins.Hostname()
	} else if port == "80" {
		ins.Scheme = "http"
		ins.Host = ins.Hostname()
	} else {
		ins.Scheme = "http"
	}
	return ins.String()
}
