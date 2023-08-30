package lowhttp

import (
	"bufio"
	"bytes"
	"compress/flate"
	"compress/zlib"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	CRLF       = "\r\n"
	DoubleCRLF = "\r\n\r\n"
)

var (
	// Add four bytes as specified in RFC
	// Add final block to squelch unexpected EOF error from flate reader
	TAIL = []byte{0, 0, 0xff, 0xff, 0x01, 0x00, 0x00, 0xff, 0xff}
)

func ExtractURLFromHTTPRequestRaw(req []byte, isHttps bool) (*url.URL, error) {
	r, err := ParseBytesToHttpRequest(req)
	if err != nil {
		return nil, err
	}
	return ExtractURLFromHTTPRequest(r, isHttps)
}

var (
	contentLengthRegexp = regexp.MustCompile(`(Content-Length: \d+\r?\n)`)
	hostRegexp          = regexp.MustCompile(`(Host: .*?\r?\n)`)
	connectionClosed    = regexp.MustCompile(`(Connection: .*?\r?\n)`)
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

func TrimRightHTTPPacket(raw []byte) []byte {
	return bytes.TrimRight(raw, "\t \n\v\f\n\b\r")
}

func TrimSpaceHTTPPacket(raw []byte) []byte {
	//return bytes.Trim(raw, "\t \n\v\f\n\b\r")
	return bytes.TrimFunc(raw, unicode.IsSpace)
}

func ExtractURLFromHTTPRequest(r *http.Request, https bool) (*url.URL, error) {
	if r == nil {
		return nil, utils.Error("no request")
	}

	if utils.IsHttpOrHttpsUrl(r.RequestURI) {
		return url.Parse(r.RequestURI)
	}

	if strings.HasPrefix(r.RequestURI, "http://") || strings.HasPrefix(r.RequestURI, "https://") {
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
		if !strings.HasPrefix(r.RequestURI, "/") {
			raw += "/"
		}
		raw += r.RequestURI
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

func ExtractBodyFromHTTPResponseRaw(res []byte) ([]byte, error) {
	_, raw := SplitHTTPHeadersAndBodyFromPacket(res)
	return raw, nil
}

func ParseStringToHTTPResponse(res string) (*http.Response, error) {
	return ParseBytesToHTTPResponse([]byte(res))
}

func MergeUrlFromHTTPRequest(rawRequest []byte, target string, isHttps bool) string {
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

			if strings.Contains(val, "%") {
				valUnesc, err := url.QueryUnescape(val)
				if err == nil {
					val = valUnesc
				}
			}
			cookies = append(cookies, &http.Cookie{Name: name, Value: val})
		}
	}
	return cookies
}

func ParseCookie(i string) []*http.Cookie {
	var header = http.Header{}
	header.Add("Cookie", i)
	return readCookies(header, "")
}

func MergeCookies(cookies ...*http.Cookie) string {
	req := &http.Request{Header: make(http.Header)}
	f := filter.NewFilter()
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
	hook ...func(line string) string) (string, []byte) {
	raw = TrimLeftHTTPPacket(raw)
	reader := bufio.NewReader(bytes.NewBuffer(raw))
	var err error
	firstLineBytes, err := utils.BufioReadLine(reader)
	if err != nil {
		return "", nil
	}
	firstLineBytes = TrimSpaceHTTPPacket(firstLineBytes)

	var headers []string
	headers = append(headers, string(firstLineBytes))
	if bytes.HasPrefix(firstLineBytes, []byte("HTTP/")) {
		// rsp
		if rspFirstLine != nil {
			proto, code, codeMsg, _ := utils.ParseHTTPResponseLine(string(firstLineBytes))
			err := rspFirstLine(proto, code, codeMsg)
			if err != nil {
				log.Errorf("rspHeader error: %s", err)
				return "", nil
			}
		}
	} else {
		// req
		if reqFirstLine != nil {
			method, requestURI, proto, _ := utils.ParseHTTPRequestLine(string(firstLineBytes))
			err := reqFirstLine(method, requestURI, proto)
			if err != nil && err.Error() != "normal abort" {
				log.Errorf("reqHeader error: %s", err)
				return "", nil
			}
		}
	}

	var haveCl = false
	for {
		//lineBytes, _, err := reader.ReadLine()
		lineBytes, err := utils.BufioReadLine(reader)
		if err != nil && err != io.EOF {
			break
		}
		if bytes.TrimSpace(lineBytes) == nil {
			break
		}

		var skipHeader = false
		for _, h := range hook {
			hooked := h(string(lineBytes))
			if hooked == "" {
				skipHeader = true
			}
			if skipHeader {
				break
			}
			k, _ := SplitHTTPHeader(hooked)
			switch strings.ToLower(k) {
			case "content-length":
				haveCl = true
			}
			lineBytes = []byte(hooked)
		}
		if skipHeader {
			continue
		}

		headers = append(headers, string(lineBytes))
	}
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

func SplitHTTPHeadersAndBodyFromPacket(raw []byte, hook ...func(line string)) (string, []byte) {
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
			if utils.AsciiEqualFold(key, "transfer-encoding") && strings.Contains(value, "chunked") {
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
	var isTls = false
	if req != nil && req.URL != nil && strings.ToLower(req.URL.Scheme) == "https" {
		isTls = true
	}
	urlRaw, err := ExtractURLFromHTTPRequest(req, isTls)
	if err != nil {
		log.Errorf("extract url(ws) from req failed: %s", err)
		return isTls, ""
	}
	var urlStr = urlRaw.String()
	if strings.HasPrefix(urlStr, "https://") {
		urlStr = fmt.Sprintf("wss://%v", urlStr[8:])
	}
	if strings.HasPrefix(urlStr, "http://") {
		urlStr = fmt.Sprintf("ws://%v", urlStr[7:])
	}
	return isTls, urlStr
}

func ReadHTTPPacketBodySize(raw []byte) (cl int, chunked bool) {
	var haveCL = false
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

func newRnd() *rand.Rand {
	var seed = time.Now().UnixNano()
	var src = rand.NewSource(seed)
	return rand.New(src)
}

var rnd = newRnd()
var rndMu sync.Mutex

// Return capped exponential backoff with jitter
// http://www.awsarchitectureblog.com/2015/03/backoff.html
func jitterBackoff(min, max time.Duration, attempt int) time.Duration {
	base := float64(min)
	capLevel := float64(max)

	temp := math.Min(capLevel, base*math.Exp2(float64(attempt)))
	ri := time.Duration(temp / 2)
	result := randDuration(ri)

	if result < min {
		result = min
	}

	return result
}

func randDuration(center time.Duration) time.Duration {
	rndMu.Lock()
	defer rndMu.Unlock()

	var ri = int64(center)
	if ri <= 0 {
		return 0
	}
	var jitter = rnd.Int63n(ri)
	return time.Duration(math.Abs(float64(ri + jitter)))
}

func isErrorTimeout(err error) bool {
	if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
		return true
	}
	return false
}

func IsPrint(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < ' ' || s[i] > '~' {
			return false
		}
	}
	return true
}

func ToLower(s string) (lower string, ok bool) {
	if !IsPrint(s) {
		return "", false
	}
	return strings.ToLower(s), true
}

func deflate(data []byte) (_ []byte, rerr error) {
	buf := new(bytes.Buffer)
	w, err := flate.NewWriter(buf, flate.BestSpeed)
	defer func() {
		if err := w.Close(); err != nil && rerr != nil {
			rerr = err
		}
	}()
	if err != nil {
		return nil, err
	}
	w.Write(data)
	w.Flush()
	return buf.Bytes(), nil
}

func _inflate(data []byte) (_ []byte, rerr error) {
	r := flate.NewReader(io.MultiReader(bytes.NewReader(data), bytes.NewReader(TAIL)))

	defer func() {
		if err := r.Close(); err != nil && rerr != nil {
			rerr = err
		}
	}()

	newData, err := ioutil.ReadAll(r)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		err = nil
	} else if _, ok := err.(flate.CorruptInputError); ok {
		r = flate.NewReader(bytes.NewReader(data))
		newData, err = ioutil.ReadAll(r)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			err = nil
		}
	}

	return newData, err
}

func inflate(data []byte) ([]byte, error) {
	after, err := _inflate(data)
	if err != nil {
		if zr, _ := zlib.NewReader(bytes.NewReader(data)); zr != nil {
			after, err = ioutil.ReadAll(zr)
			if err != nil {
				return data, err
			}
			return after, nil
		}
		return data, err
	}
	return after, nil
}

func IsPermessageDeflate(headers http.Header) bool {

	isDeflate := false
	websocketExtensions, ok := headers["Sec-WebSocket-Extensions"]
	if !ok {
		lowerHeaders := make(map[string][]string, len(headers))
		for k, v := range headers {
			lowerHeaders[strings.ToLower(k)] = v
		}
		websocketExtensions, ok = lowerHeaders["sec-websocket-extensions"]
	}

	websocketExtensionRaw := strings.Join(websocketExtensions, "; ")
	if ok {
		websocketExts := strings.Split(websocketExtensionRaw, ";")
		for _, ext := range websocketExts {
			ext = strings.TrimSpace(ext)
			if ext == "permessage-deflate" || ext == "x-webkit-deflate-frame" {
				isDeflate = true
				break
			}
		}
	}
	return isDeflate
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

// IsResp test if bytesstream is http response
func IsResp(data any) bool {
	switch data := data.(type) {
	case string:
		return strings.HasPrefix(strings.TrimLeftFunc(data, unicode.IsSpace), "HTTP/")
	case []byte:
		return bytes.HasPrefix(bytes.TrimLeftFunc(data, unicode.IsSpace), []byte("HTTP/"))
	case http.Response, *http.Response:
		return true
	}
	return false
}
