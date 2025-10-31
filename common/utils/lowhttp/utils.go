package lowhttp

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"net/http"
	"net/textproto"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	CRLF       = "\r\n"
	DoubleCRLF = "\r\n\r\n"
)

// ExtractURLFromHTTPRequestRaw 从原始 HTTP 请求报文中提取 URL，返回URL结构体与错误
// Example:
// ```
// url, err := str.ExtractURLFromHTTPRequestRaw(b"GET / HTTP/1.1\r\nHost: www.yaklang.com\r\n\r\n", false)
// ```
func ExtractURLFromHTTPRequestRaw(req []byte, isHttps bool) (*url.URL, error) {
	r, err := ParseBytesToHttpRequest(req)
	if err != nil {
		return nil, err
	}
	return ExtractURLFromHTTPRequest(r, isHttps)
}

// ExtractURLStringFromHTTPRequestRaw parse url string
func ExtractURLStringFromHTTPRequest(req any, isHttps bool) (string, error) {
	var r *http.Request
	var err error
	switch ret := req.(type) {
	case []byte:
		r, err = ParseBytesToHttpRequest(ret)
		if err != nil {
			return "", err
		}
	case string:
		r, err = ParseBytesToHttpRequest([]byte(ret))
		if err != nil {
			return "", err
		}
	case *http.Request:
		r = ret
	case http.Request:
		r = &ret
	default:
		return "", utils.Errorf("not support type: %T", req)
	}

	u, err := ExtractURLFromHTTPRequest(r, isHttps)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

var (
	contentLengthRegexp = regexp.MustCompile(`(Content-Length: \d+\r?\n)`)
	hostRegexp          = regexp.MustCompile(`(Host: .*?\r?\n)`)
)

func fixInvalidHTTPHeaders(raw []byte) []byte {
	res := contentLengthRegexp.FindIndex(raw)
	if len(res) >= 2 {
		raw = append(raw[:res[0]], raw[res[1]:]...)
	}

	res = hostRegexp.FindIndex(raw)
	if len(res) < 2 {
		return raw
	}

	raw = append(raw[:res[1]], append([]byte("Transfer-Encoding: chunked\n"), raw[res[1]:]...)...)

	return raw
}

func AddConnectionClosed(raw []byte) []byte {
	if bytes.Contains(raw, []byte("Connection: close")) {
		return raw
	}

	res := contentLengthRegexp.FindIndex(raw)
	if len(res) >= 2 {
		raw = append(raw[:res[0]], raw[res[1]:]...)
	}

	res = hostRegexp.FindIndex(raw)
	if len(res) < 2 {
		return raw
	}

	raw = append(raw[:res[1]], append([]byte("Connection: close\r\n"), raw[res[1]:]...)...)

	if bytes.HasSuffix(raw, []byte("\r\n\r\n")) {
		return raw
	}

	if bytes.HasSuffix(raw, []byte("\r\n")) {
		return append(raw, []byte("\r\n")...)
	}

	return append(raw, []byte("\r\n\r\n")...)
}

func TrimLeftHTTPPacket(raw []byte) []byte {
	return bytes.TrimLeftFunc(raw, unicode.IsSpace)
}

func TrimLeftCRLF(raw []byte) []byte {
	return bytes.TrimLeftFunc(raw, func(r rune) bool {
		return r == '\r' || r == '\n'
	})
}

func TrimRightHTTPPacket(raw []byte) []byte {
	return bytes.TrimRight(raw, "\t \n\v\f\n\b\r")
}

func TrimSpaceHTTPPacket(raw []byte) []byte {
	// return bytes.Trim(raw, "\t \n\v\f\n\b\r")
	return bytes.TrimFunc(raw, unicode.IsSpace)
}

// ExtractURLFromHTTPRequest 从 HTTP 请求结构体中提取 URL，返回URL结构体与错误
// Example:
// ```
// v, err = http.Raw("GET / HTTP/1.1\r\nHost: www.yaklang.com\r\n\r\n")
// url, err = str.ExtractURLFromHTTPRequest(v, false)
// ```
func ExtractURLFromHTTPRequest(r *http.Request, https bool) (*url.URL, error) {
	if r == nil {
		return nil, utils.Error("no request")
	}

	if utils.IsHttpOrHttpsUrl(r.RequestURI) {
		uIns, err := url.Parse(r.RequestURI)
		if err != nil {
			return nil, err
		}
		if https {
			uIns.Scheme = "https" // 强制修正https
		}
		return uIns, nil
	}

	if utils.IsWebsocketUrl(r.RequestURI) {
		return url.Parse(r.RequestURI)
	}

	if r.URL.Scheme != "" {
		switch r.URL.Scheme {
		case "https", "wss", "ssh", "sftp":
			https = true
		default:
			https = false
		}
	}

	if r.URL.Scheme == "" {
		if https {
			r.URL.Scheme = "https"
		} else {
			r.URL.Scheme = "http"
		}
	}

	var raw string
	switch https {
	case true:
		raw = "https://"
	default:
		raw = "http://"
	}

	if strings.ToUpper(r.Method) == "CONNECT" {
		return r.URL, nil
	}

	var host string
	if r.Host != "" {
		host = r.Host
	} else {
		host = r.Header.Get("Host")
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, utils.Errorf("empty hosts")
	}

	if strings.HasSuffix(host, ":443") && strings.HasPrefix(raw, "https://") {
		// 修复 https :443 的不必要情况
		raw += host[:len(host)-4]
	} else {
		raw += host
	}
	noPath := raw
	if r.RequestURI != "" {
		if r.RequestURI != r.URL.String() {
			if !strings.HasPrefix(r.RequestURI, "/") {
				raw += "/"
			}
			raw += r.RequestURI
		} else {
			raw += r.URL.Path
		}
	} else {
		u := r.URL
		if strings.HasPrefix(u.Path, "/") || u.RawQuery != "" {
			raw += u.RequestURI()
		} else {
			raw += u.Path
		}
	}
	uIns, err := url.Parse(raw)
	if err != nil {
		instance, err := url.Parse(utils.RemoveUnprintableCharsWithReplaceItem(noPath))
		if instance == nil {
			return nil, utils.Errorf("nopath [%s] error: %s", noPath, err)
		}
		instance.Path = r.RequestURI
		return instance, nil
	}
	return uIns, nil
}

// ExtractBodyFromHTTPResponseRaw 从原始 HTTP 响应报文中提取 body
// Example:
// ```
// body, err = str.ExtractBodyFromHTTPResponseRaw(b"HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok") // body = b"ok"
// ```
func ExtractBodyFromHTTPResponseRaw(res []byte) ([]byte, error) {
	_, raw := SplitHTTPHeadersAndBodyFromPacket(res)
	return raw, nil
}

// ParseStringToHTTPResponse 将字符串解析为 HTTP 响应
// Example:
// ```
// res, err := str.ParseStringToHTTPResponse("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
// ```
func ParseStringToHTTPResponse(res string) (*http.Response, error) {
	return ParseBytesToHTTPResponse([]byte(res))
}

// MergeUrlFromHTTPRequest 将传入的 target 与 原始 HTTP 请求报文中的 URL 进行合并，并返回合并后的 URL
// Example:
// ```
// url = str.MergeUrlFromHTTPRequest(b"GET /z HTTP/1.1\r\nHost: www.yaklang.com\r\n\r\n", "/a/b", true) // url = "https://www.yaklang.com/z/a/b"
// ```
func MergeUrlFromHTTPRequest(rawRequest []byte, target string, isHttps bool) (newURL string) {
	if utils.IsHttpOrHttpsUrl(target) {
		return target
	}

	urlIns, err := ExtractURLFromHTTPRequestRaw(rawRequest, isHttps)
	if err != nil {
		return ""
	}

	raw, err := utils.UrlJoin(urlIns.String(), target)
	if err != nil {
		log.Errorf("url join failed: %s field: %v", urlIns.String(), target)
		return ""
	}
	return raw
}

func IsMultipartFormDataRequest(req []byte) bool {
	isMultipart := false
	SplitHTTPHeadersAndBodyFromPacket(req, func(line string) {
		if !isMultipart {
			isMultipart = strings.Contains(strings.ToLower(line), "multipart/form-data")
		}
	})
	return isMultipart
}

func SplitHTTPHeader(i string) (string, string) {
	if ret := strings.Index(i, ":"); ret < 0 {
		return i, ""
	} else {
		key := i[:ret]
		value := strings.TrimSpace(i[ret+1:])
		return key, value
	}
}

func SplitKV(i string) (string, string) {
	if ret := strings.Index(i, "="); ret < 0 {
		return i, ""
	} else {
		key := i[:ret]
		value := strings.TrimSpace(i[ret+1:])
		return key, value
	}
}

// ValidCookieValue 判断是否存在不允许的字符,
func ValidCookieValue(value string) bool {
	for i := 0; i < len(value); i++ {
		if !validCookieValueByte(value[i]) {
			return true
		}
	}
	return false
}

func validCookieValueByte(b byte) bool {
	return 0x20 <= b && b < 0x7f && b != '"' && b != ';' && b != '\\'
}

func parseCookieValue(raw string, allowDoubleQuote bool) (string, bool) {
	// Strip the quotes, if present.
	if allowDoubleQuote && len(raw) > 1 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		raw = raw[1 : len(raw)-1]
	}

	return raw, true
}

// readCookies parses all "Cookie" values from the header h and
// returns the successfully parsed Cookies.
//
// if filter isn't empty, only cookies of that name are returned
// From Go Src
func readCookies(h http.Header, filter string) []*http.Cookie {
	lines := h["Cookie"]
	if len(lines) == 0 {
		return []*http.Cookie{}
	}

	cookies := make([]*http.Cookie, 0, len(lines)+strings.Count(lines[0], ";"))
	for _, line := range lines {
		line = textproto.TrimString(line)

		var part string
		for len(line) > 0 { // continue since we have rest
			part, line, _ = strings.Cut(line, ";")
			part = textproto.TrimString(part)
			if part == "" {
				continue
			}
			name, val, _ := strings.Cut(part, "=")
			if strings.TrimSpace(name) == "" {
				continue
			}
			if filter != "" && filter != name {
				continue
			}

			// 去掉友好显示的tag
			if strings.HasPrefix(name, "{{urlescape(") ||
				strings.HasPrefix(val, "{{urlescape(") {
				name = strings.TrimPrefix(name, "{{urlescape(")
				name = strings.TrimSuffix(name, ")}}")
				val = strings.TrimPrefix(val, "{{urlescape(")
				val = strings.TrimSuffix(val, ")}}")
			}
			if !strings.ContainsAny(val, " ,") {
				// 只去双引号，不判断是否合法
				val, _ = parseCookieValue(val, true)
			}

			//if strings.Contains(val, "%") {
			//	valUnesc, err := url.QueryUnescape(val)
			//	if err == nil {
			//		val = valUnesc
			//	}
			//}
			cookies = append(cookies, &http.Cookie{Name: name, Value: val})
		}
	}
	return cookies
}

// ParseCookie parse 请求包中的 Cookie 字符串
func ParseCookie(key, raw string) []*http.Cookie {
	var cookies []*http.Cookie
	if strings.ToLower(key) == "cookie" {
		header := http.Header{}
		header.Add("Cookie", raw)
		cookies = readCookies(header, "")
	} else if strings.ToLower(key) == "set-cookie" {
		header := http.Header{}
		header.Add("Set-Cookie", raw)
		resp := http.Response{Header: header}
		cookies = resp.Cookies()
	}
	return cookies
}

func MergeCookies(cookies ...*http.Cookie) string {
	req := &http.Request{Header: make(http.Header)}
	f := filter.NewFilter()
	defer f.Close()
	for _, c := range cookies {
		if f.Exist(c.String()) {
			continue
		}
		f.Insert(c.String())
		f.Insert(c.String() + ";")
		req.AddCookie(c)
	}
	return req.Header.Get("Cookie")
}

// func SplitContentTypesFromAcceptHeader(acceptHeader string) []string {
// 	var contentTypes []string

// 	parts := strings.Split(acceptHeader, ",")
// 	for _, part := range parts {
// 		contentType := strings.TrimSpace(part)
// 		if idx := strings.Index(contentType, ";"); idx != -1 {
// 			contentType = strings.TrimSpace(contentType[:idx])
// 		}
// 		if contentType != "" {
// 			contentTypes = append(contentTypes, contentType)
// 		}
// 	}
// 	return contentTypes
// }

func SplitHTTPHeadersAndBodyFromPacketEx(raw []byte, mf func(method string, requestUri string, proto string) error, hook ...func(line string)) (string, []byte) {
	if len(hook) > 0 {
		return SplitHTTPPacket(raw, mf, nil, func(line string) (ret string) {
			ret = line
			defer func() {
				if err := recover(); err != nil {
					utils.PrintCurrentGoroutineRuntimeStack()
				}
				ret = line
			}()
			for _, h := range hook {
				h(line)
			}
			return ret
		})
	}
	return SplitHTTPPacket(raw, mf, nil)
}

func SplitHTTPPacketFast(raw any) (string, []byte) {
	return SplitHTTPPacket(utils.InterfaceToBytes(raw), func(method string, requestUri string, proto string) error {
		return nil
	}, func(proto string, code int, codeMsg string) error {
		return nil
	})
}

// SplitHTTPPacket split http packet to headers and body
// reqFirstLine: method, requestUri, proto: error for empty result
// rspFirstLine: proto, code, codeMsg: error for empty result
// hook: hook func
func SplitHTTPPacket(
	raw []byte,
	reqFirstLine func(method string, requestUri string, proto string) error,
	rspFirstLine func(proto string, code int, codeMsg string) error,
	hook ...func(line string) string,
) (string, []byte) {
	return SplitHTTPPacketEx(raw, reqFirstLine, rspFirstLine, nil, hook...)
}

func SplitHTTPPacketEx(
	raw []byte,
	reqFirstLine func(method string, requestUri string, proto string) error,
	rspFirstLine func(proto string, code int, codeMsg string) error,
	rawFistLine func(string) error,
	hook ...func(line string) string,
) (string, []byte) {
	reader := bufio.NewReader(bytes.NewBuffer(raw))
	firstLineBytes, err := utils.BufioReadLine(reader)
	if err != nil {
		return "", nil
	}
	prefix, firstLineBytes, _ := utils.CutBytesPrefixFunc(firstLineBytes, utils.NotSpaceRune)
	firstLineBytes = TrimSpaceHTTPPacket(firstLineBytes)
	if rawFistLine != nil {
		err := rawFistLine(string(firstLineBytes))
		if err != nil {
			log.Debugf("rawFistLine error: %s", err)
			return "", nil
		}
	}
	var isResp = bytes.HasPrefix(firstLineBytes, []byte("HTTP/")) || bytes.HasPrefix(firstLineBytes, []byte("RTSP/"))
	if isResp {
		// rsp
		if rspFirstLine != nil {
			proto, code, codeMsg, _ := utils.ParseHTTPResponseLine(string(firstLineBytes))
			err := rspFirstLine(proto, code, codeMsg)
			if err != nil {
				log.Debugf("rspHeader error: %s", err)
				return "", nil
			}
		}
	} else {
		// req
		if reqFirstLine != nil {
			method, requestURI, proto, _ := utils.ParseHTTPRequestLine(string(firstLineBytes))
			err := reqFirstLine(method, requestURI, proto)
			if err != nil && err.Error() != "normal abort" {
				log.Debugf("reqHeader error: %s", err)
				return "", nil
			}
		}
	}

	var headers []string
	headers = append(headers, string(firstLineBytes))
	haveCl := false
	err = utils.ScanHTTPHeader(reader, func(rawHeader []byte) {
		if len(rawHeader) == 0 {
			return
		}
		line := string(rawHeader)
		skipHeader := false
		for _, h := range hook {
			hooked := h(line)
			if hooked == "" {
				skipHeader = true
			}
			if skipHeader {
				break
			}
			line = hooked
		}
		if skipHeader {
			return
		}
		k, _ := SplitHTTPHeader(line)
		if strings.ToLower(k) == "content-length" {
			haveCl = true
		}
		headers = append(headers, line)
	}, prefix, isResp)
	headersRaw := strings.Join(headers, CRLF) + CRLF + CRLF
	bodyRaw, _ := ioutil.ReadAll(reader)
	if bodyRaw == nil {
		return headersRaw, nil
	}

	if len(bytes.TrimSpace(bodyRaw)) == 0 && !haveCl {
		bodyRaw = nil
	}

	// 单独修复请求中的问题
	//if !strings.HasPrefix(headersRaw, "HTTP/") {
	//	if bytes.HasSuffix(bodyRaw, []byte("\n\n")) {
	//		bodyRaw = bodyRaw[:len(bodyRaw)-2]
	//	}
	//}

	return headersRaw, bodyRaw
}

// SplitHTTPHeadersAndBodyFromPacket 将传入的 HTTP 报文分割为 headers 和 body，如果传入了hook，则会在每次读取到一行 header 时调用 hook
// Example:
// ```
// headers, body = str.SplitHTTPHeadersAndBodyFromPacket(b"GET / HTTP/1.1\r\nHost: www.yaklang.com\r\n\r\n")
// ```
func SplitHTTPHeadersAndBodyFromPacket(raw []byte, hook ...func(line string)) (headers string, body []byte) {
	return SplitHTTPHeadersAndBodyFromPacketEx(raw, nil, hook...)
}

func RemoveZeroContentLengthHTTPHeader(raw []byte) []byte {
	removeContentLength := false
	chunk := false
	method := ""
	headers, body := SplitHTTPHeadersAndBodyFromPacketEx(raw, func(m string, _, _ string) error {
		method = m
		return nil
	}, func(line string) {
		if ret := strings.Split(line, ":"); len(ret) > 1 {
			key, value := ret[0], strings.TrimSpace(ret[1])
			if utils.AsciiEqualFold(key, "transfer-encoding") && utils.IContains(value, "chunked") {
				chunk = true
			}
		}
	})

	cl := len(body)
	removeContentLength = ShouldSendReqContentLength(method, int64(cl))

	var lines []string
	for line := range utils.ParseLines(headers) {
		if (removeContentLength || chunk) && strings.HasPrefix(strings.ToLower(line), "content-length: ") {
			continue
		}
		lines = append(lines, line)
	}
	return ReplaceHTTPPacketBody([]byte(strings.Join(lines, "\r\n")), body, false)
}

// ShouldSendReqContentLength reports whether the http2.Transport should send
// a "content-length" request header. This logic is basically a copy of the net/http
// transferWriter.shouldSendContentLength.
// The contentLength is the corrected contentLength (so 0 means actually 0, not unknown).
// -1 means unknown.
func ShouldSendReqContentLength(method string, contentLength int64) bool {
	if contentLength > 0 {
		return true
	}
	if contentLength < 0 {
		return false
	}
	// For zero bodies, whether we send a content-length depends on the method.
	// It also kinda doesn't matter for http2 either way, with END_STREAM.
	switch method {
	case "POST", "PUT", "PATCH":
		return true
	default:
		return false
	}
}

func ExtractWebsocketURLFromHTTPRequest(req *http.Request) (bool, string) {
	isTls := false
	if req != nil && req.URL != nil && strings.ToLower(req.URL.Scheme) == "https" {
		isTls = true
	}
	urlRaw, err := ExtractURLFromHTTPRequest(req, isTls)
	if err != nil {
		log.Errorf("extract url(ws) from req failed: %s", err)
		return isTls, ""
	}

	return isTls, urlRaw.String()
}

func ReadHTTPPacketBodySize(raw []byte) (cl int, chunked bool) {
	haveCL := false
	SplitHTTPHeadersAndBodyFromPacket(raw, func(line string) {
		line = strings.ToLower(strings.TrimSpace(line))
		if !haveCL && strings.HasPrefix(line, "content-type:") {
			haveCL = true
			n := strings.TrimSpace(line[13:])
			cl, _ = strconv.Atoi(n)
		}

		if !chunked && !haveCL && _transferEncodingRE.MatchString(line) {
			chunked = true
		}
	})

	if haveCL {
		return cl, false
	}
	return 0, chunked
}



func FixRequestHostAndPort(r *http.Request) {
	var host string
	if r.Host != "" {
		host = r.Host
	}

	if host == "" {
		host = r.Header.Get("Host")
	}

	if host == "" {
		host = r.URL.Host
	}

	if host != "" && r.URL.Host == "" {
		r.URL.Host = host
	}

	host, port, err := utils.ParseStringToHostPort(r.URL.String())
	if err != nil {
		return
	}
	r.Host = utils.HostPort(host, port)
	r.Header.Set("Host", r.Host)
	r.URL.Host = r.Host
}

// IsResp 判断传入的数据是否为 HTTP 响应报文
// Example:
// ```
// poc.IsResp(b"HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok") // true
// ```
func IsResp(raw any) (isHTTPResponse bool) {
	switch data := raw.(type) {
	case []byte:
		_, err := ParseBytesToHTTPResponse(data)
		return err == nil
	case string:
		_, err := ParseBytesToHTTPResponse([]byte(data))
		return err == nil
	case http.Response, *http.Response:
		return true
	}
	return false
}

func IsRespFast(raw any) (isHTTPResponse bool) {
	first := ""
	switch data := raw.(type) {
	case []byte:
		first, _, _ = GetHTTPPacketFirstLine([]byte(data))
	case string:
		first, _, _ = GetHTTPPacketFirstLine([]byte(data))
	case http.Response, *http.Response:
		return true
	}
	return strings.HasPrefix(first, "HTTP/")
}

func IGetHeader(packet interface{}, headerKey string) []string {
	var headers map[string][]string
	switch data := packet.(type) {
	case []byte:
		headers = GetHTTPPacketHeadersFull(data)
	case string:
		headers = GetHTTPPacketHeadersFull([]byte(data))
	case http.Response:
		return IGetHTTPInsHeader(data.Header, headerKey)
	case *http.Response:
		return IGetHTTPInsHeader(data.Header, headerKey)
	case http.Request:
		return IGetHTTPInsHeader(data.Header, headerKey)
	case *http.Request:
		return IGetHTTPInsHeader(data.Header, headerKey)
	}

	var headerValue []string
	for k, v := range headers {
		if strings.ToLower(k) == strings.ToLower(headerKey) {
			headerValue = append(headerValue, v...)
		}
	}
	return nil
}

func IGetHTTPInsHeader(headers http.Header, headerKey string) []string {
	var headerValue []string
	for k, v := range headers {
		if strings.ToLower(k) == strings.ToLower(headerKey) {
			headerValue = append(headerValue, v...)
		}
	}
	return headerValue
}
