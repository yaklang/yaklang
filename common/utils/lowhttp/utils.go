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
	"net/url"
	"yaklang/common/filter"
	"yaklang/common/log"
	"yaklang/common/utils"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http/httpguts"
)

const (
	CRLF = "\r\n"
)

var (
	// Add four bytes as specified in RFC
	// Add final block to squelch unexpected EOF error from flate reader
	TAIL = []byte{0, 0, 0xff, 0xff, 0x01, 0x00, 0x00, 0xff, 0xff}
)

// parseResponseLine parses `HTTP/1.1 200 OK` into its ports
func parseResponseLine(line string) (string, int, string, bool) {
	line = strings.TrimSpace(line)

	var proto string
	var code int
	var status string

	// 第一个一定是 HTTP/1.1 先解析出 proto
	s1 := strings.Index(line, " ")
	if s1 < 0 {
		return "", 0, "", false
	}
	proto = line[:s1]

	// 剩余的部分，找最后一个分割
	line = line[s1+1:]
	s2 := strings.LastIndex(line, " ")
	if s2 < 0 {
		code = utils.Atoi(line)
	} else {
		code = utils.Atoi(line[:s2])
	}
	return proto, code, status, code != 0
}

// parseRequestLine parses "GET /foo HTTP/1.1" into its three parts.
func parseRequestLine(line string) (method, requestURI, proto string, ok bool) {
	s1 := strings.Index(line, " ")
	s2 := strings.LastIndex(line[s1+1:], " ")
	if s1 < 0 {
		return
	}

	var httpVersion = "HTTP/1.1"
	if s2 < 0 {
		return line[:s1], line[s1+1:], httpVersion, true
	}
	s2 += s1 + 1
	return line[:s1], line[s1+1 : s2], line[s2+1:], true
}

func validMethod(method string) bool {
	/*
	     Method         = "OPTIONS"                ; Section 9.2
	                    | "GET"                    ; Section 9.3
	                    | "HEAD"                   ; Section 9.4
	                    | "POST"                   ; Section 9.5
	                    | "PUT"                    ; Section 9.6
	                    | "DELETE"                 ; Section 9.7
	                    | "TRACE"                  ; Section 9.8
	                    | "CONNECT"                ; Section 9.9
	                    | extension-method
	   extension-method = token
	     token          = 1*<any CHAR except CTLs or separators>
	*/
	return len(method) > 0 && strings.IndexFunc(method, isNotToken) == -1
}

func isNotToken(r rune) bool {
	return !httpguts.IsTokenRune(r)
}

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
	return bytes.TrimLeft(raw, "\t \n\v\f\n\b\r")
}

func TrimRightHTTPPacket(raw []byte) []byte {
	return bytes.TrimRight(raw, "\t \n\v\f\n\b\r")
}

func TrimSpaceHTTPPacket(raw []byte) []byte {
	return bytes.Trim(raw, "\t \n\v\f\n\b\r")
}

func ExtractURLFromHTTPRequest(r *http.Request, https bool) (*url.URL, error) {
	if r == nil {
		return nil, utils.Error("no request")
	}

	if strings.HasPrefix(r.RequestURI, "http://") || strings.HasPrefix(r.RequestURI, "https://") {
		return url.Parse(r.RequestURI)
	}

	var raw string
	switch https {
	case true:
		raw = "https://"
	default:
		raw = "http://"
	}

	switch strings.ToUpper(r.Method) {
	case "CONNECT":
		return nil, utils.Errorf("ignore connect")
	}

	var host string
	if r.Host != "" {
		host = r.Host
	} else {
		host = r.Header.Get("Host")
	}
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
		raw += r.RequestURI
	} else {
		raw += r.URL.RequestURI()
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
	if utils.IsHttp(target) {
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

func ParseCookie(i string) []*http.Cookie {
	var header = http.Header{}
	header.Add("Cookie", i)
	return (&http.Request{Header: header}).Cookies()
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
	return SplitHTTPHeadersAndBodyFromPacketEx2(raw, mf, nil, hook...)
}

func SplitHTTPHeadersAndBodyFromPacketEx2(
	raw []byte,
	mf func(method string, requestUri string, proto string) error,
	rspHeader func(proto string, code int, codeMsg string) error,
	hook ...func(line string)) (string, []byte) {
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
		if rspHeader != nil {
			proto, code, codeMsg, _ := parseResponseLine(string(firstLineBytes))
			err := rspHeader(proto, code, codeMsg)
			if err != nil {
				log.Errorf("rspHeader error: %s", err)
				return "", nil
			}
		}
	} else {
		// req
		if mf != nil {
			method, requestURI, proto, _ := parseRequestLine(string(firstLineBytes))
			err := mf(method, requestURI, proto)
			if err != nil && err.Error() != "normal abort" {
				log.Errorf("reqHeader error: %s", err)
				return "", nil
			}
		}
	}

	for {
		//lineBytes, _, err := reader.ReadLine()
		lineBytes, err := utils.BufioReadLine(reader)
		if err != nil && err != io.EOF {
			break
		}
		if bytes.TrimSpace(lineBytes) == nil {
			break
		}

		for _, h := range hook {
			h(string(lineBytes))
		}

		headers = append(headers, string(lineBytes))
	}
	headersRaw := strings.Join(headers, CRLF) + CRLF + CRLF
	bodyRaw, _ := ioutil.ReadAll(reader)
	if bodyRaw == nil {
		return headersRaw, nil
	}

	if bytes.HasSuffix(bodyRaw, []byte(CRLF+CRLF)) {
		bodyRaw = bodyRaw[:len(bodyRaw)-4]
	}

	// 单独修复请求中的问题
	if !strings.HasPrefix(headersRaw, "HTTP/") {
		if bytes.HasSuffix(bodyRaw, []byte("\n\n")) {
			bodyRaw = bodyRaw[:len(bodyRaw)-2]
		}
	}

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
