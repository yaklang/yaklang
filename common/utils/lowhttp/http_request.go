package lowhttp

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	utils "github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httputil"
	"net/textproto"
	url "net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var _contentLengthRE = regexp.MustCompile(`(?i)Content-Length:(\s+)?(\d+)?\r?\n?`)
var _transferEncodingRE = regexp.MustCompile(`(?i)Transfer-Encoding:(\s+)?.*?(chunked).*?\r?\n?`)
var fetchBoundaryRegexp = regexp.MustCompile(`boundary\s?=\s?([^;]+)`)

func HTTPPacketForceChunked(raw []byte) []byte {
	header, body := SplitHTTPHeadersAndBodyFromPacket(raw)
	return ReplaceHTTPPacketBodyEx([]byte(header), body, true, false)
}

func AppendHeaderToHTTPPacket(raw []byte, line string) []byte {
	header, body := SplitHTTPHeadersAndBodyFromPacket(raw)
	header = strings.TrimRight(header, "\r\n") + CRLF + strings.TrimSpace(line) + CRLF + CRLF
	return []byte(header + string(body))
}

var mayNoCLRE = regexp.MustCompile(`^((GET)|(HEAD)|(DELETE)|(OPTIONS)|(CONNECT)) `)

func FixHTTPPacketCRLF(raw []byte, noFixLength bool) []byte {
	// 移除左边空白字符
	raw = TrimLeftHTTPPacket(raw)
	var isMultipart bool
	var haveChunkedHeader bool
	var haveContentLength bool
	var contentTypeGziped bool
	header, body := SplitHTTPHeadersAndBodyFromPacket(raw, func(line string) {
		key, value := SplitHTTPHeader(line)

		var keyLower = strings.ToLower(key)
		var valLower = strings.ToLower(value)

		if !isMultipart && keyLower == "content-type" && strings.HasPrefix(valLower, "multipart/form-data") {
			isMultipart = true
		}

		if !haveContentLength && strings.ToLower(key) == "content-length" {
			haveContentLength = true
		}

		if !haveChunkedHeader && keyLower == "transfer-encoding" && valLower == "chunked" {
			haveChunkedHeader = true
		}

		if !contentTypeGziped && keyLower == "content-encoding" && valLower == "gzip" {
			contentTypeGziped = true
		}
	})

	// applying patch to restore CRLF at body
	// if `raw` has CRLF at body end (by design HTTP smuggle) and `noFixContentLength` is true
	if bytes.HasSuffix(raw, []byte(CRLF+CRLF)) && noFixLength {
		body = append(body, []byte(CRLF+CRLF)...)
	}

	handleChunked := haveChunkedHeader && !haveContentLength
	if handleChunked {
		bodyRaw, _ := ioutil.ReadAll(httputil.NewChunkedReader(bytes.NewBuffer(body)))
		if len(bodyRaw) > 0 {
			body = bodyRaw
		}
	}

	if isMultipart {
		// 修复数据包的 Boundary
		boundary, fixed := FixMultipartBody(body)
		if boundary != "" {
			header = string(ReplaceMIMEType([]byte(header), "multipart/form-data; boundary="+boundary))
			body = fixed
		}
	}

	var headerTop20 string
	if len(raw) > 10 {
		headerTop20 = string(raw[:10])
	} else {
		headerTop20 = string(raw[:])
	}
	if !handleChunked && !noFixLength {
		if len(body) != 0 || !mayNoCLRE.MatchString(headerTop20) {
			// 修复 Content-Length
			fixed := fmt.Sprintf("Content-Length: %v\r\n", len(body))
			if _contentLengthRE.MatchString(header) {
				header = _contentLengthRE.ReplaceAllString(header, fixed)
			} else {
				header = strings.TrimRight(header, CRLF)
				header += CRLF + fixed + CRLF
			}
		} else {
			header = _contentLengthRE.ReplaceAllString(header, "")
		}
	}

	if body != nil && handleChunked {
		body = codec.HTTPChunkedEncode(body)
	}

	return []byte(header + string(body))
}

func FixHTTPRequestOut(raw []byte) []byte {
	return FixHTTPPacketCRLF(raw, false)
}

func ConvertHTTPRequestToFuzzTag(i []byte) []byte {
	var boundary string // 如果是上传数据包的话，boundary 就不会为空
	var header, body = SplitHTTPHeadersAndBodyFromPacket(i, func(line string) {
		k, v := SplitHTTPHeader(strings.TrimSpace(line))
		switch strings.ToLower(k) {
		case "content-type":
			ctVal, params, _ := mime.ParseMediaType(v)
			if ctVal == "multipart/form-data" && params != nil && len(params) > 0 {
				boundary = params["boundary"]
			}
		}
	})

	if boundary != "" {
		// 上传文件的情况
		reader := multipart.NewReader(bytes.NewBuffer(body), boundary)

		// 修复数据包
		var buf bytes.Buffer
		var fixedBody = multipart.NewWriter(&buf)
		fixedBody.SetBoundary(boundary)
		for {
			part, err := reader.NextRawPart()
			if err != nil {
				break
			}
			w, err := fixedBody.CreatePart(part.Header)
			if err != nil {
				log.Errorf("write part to new part failed: %s", err)
				continue
			}

			body, err := ioutil.ReadAll(part)
			if err != nil {
				log.Errorf("copy multipart-stream failed: %s", err)
			}
			if utf8.Valid(body) {
				w.Write(body)
			} else {
				w.Write([]byte(ToUnquoteFuzzTag(body)))
			}
		}
		fixedBody.Close()
		body = buf.Bytes()
		return ReplaceHTTPPacketBody([]byte(header), body, false)
	}

	if utf8.Valid(body) {
		return i
	}
	body = []byte(ToUnquoteFuzzTag(body))
	return ReplaceHTTPPacketBody([]byte(header), body, false)
}

const printableMin = 32
const printableMax = 126

func ToUnquoteFuzzTag(i []byte) string {
	if utf8.Valid(i) {
		return string(i)
	}

	var buf = bytes.NewBufferString(`{{unquote("`)
	for _, b := range i {
		if b >= printableMin && b <= printableMax {
			switch b {
			case '(':
				buf.WriteString(`\x29`)
			case ')':
				buf.WriteString(`\x28`)
			case '}':
				buf.WriteString(`\x7d`)
			case '{':
				buf.WriteString(`\x7b`)
			case '"':
				buf.WriteString(`\"`)
			default:
				buf.WriteByte(b)
			}
		} else {
			buf.WriteString(fmt.Sprintf(`\x%02x`, b))
		}
	}
	buf.WriteString(`")}}`)
	return buf.String()
}

//func FixHTTPRequestOut(raw []byte) []byte {
//	// 移除左边空白字符
//	raw = TrimLeftHTTPPacket(raw)
//
//	// 修复不合理的 headers
//	if bytes.Contains(raw, []byte("Transfer-Encoding: chunked")) {
//		headers, body := SplitHTTPHeadersAndBodyFromPacket(raw)
//		headersRaw := fixInvalidHTTPHeaders([]byte(headers))
//		raw = append(headersRaw, body...)
//	}
//	raw = AddConnectionClosed(raw)
//
//	//
//	reader := bufio.NewReader(bytes.NewBuffer(raw))
//	firstLineBytes, _, err := reader.ReadLine()
//	if err != nil {
//		return raw
//	}
//
//	var headers = []string{
//		string(firstLineBytes),
//	}
//
//	// 接下来解析各种 Header
//	isChunked := false
//	for {
//		lineBytes, _, err := reader.ReadLine()
//		if err != nil && err != io.EOF {
//			return raw
//		}
//		line := string(lineBytes)
//		line = strings.TrimSpace(line)
//
//		// Header 解析完毕
//		if line == "" {
//			break
//		}
//
//		if strings.HasPrefix(line, "Transfer-Encoding:") && strings.Contains(line, "chunked") {
//			isChunked = true
//		}
//		headers = append(headers, line)
//	}
//	restBody, _ := ioutil.ReadAll(reader)
//
//	// 移除原有的 \r\n
//	if bytes.HasSuffix(restBody, []byte("\r\n\r\n")) {
//		restBody = restBody[:len(restBody)-4]
//	}
//	if bytes.HasSuffix(restBody, []byte("\n\n")) {
//		restBody = restBody[:len(restBody)-2]
//	}
//
//	// 修复 content-length
//	var index = -1
//	emptyBody := bytes.TrimSpace(restBody) == nil
//	if emptyBody {
//		restBody = nil
//	}
//	contentLength := len(restBody)
//	if !isChunked && contentLength > 0 {
//		for i, r := range headers {
//			if strings.HasPrefix(r, "Content-Length:") {
//				index = i
//			}
//		}
//		if index < 0 {
//			headers = append(headers, fmt.Sprintf("Content-Length: %v", contentLength))
//		} else {
//			headers[index] = fmt.Sprintf("Content-Length: %v", contentLength)
//		}
//	}
//
//	var finalRaw []byte
//	// 添加新的结尾分隔符
//	if emptyBody {
//		finalRaw = []byte(strings.Join(headers, CRLF) + CRLF + CRLF)
//	} else {
//		finalRaw = []byte(strings.Join(headers, CRLF) + CRLF + CRLF + string(restBody) + CRLF + CRLF)
//	}
//
//	return finalRaw
//}

func ParseStringToHttpRequest(raw string) (*http.Request, error) {
	return ParseBytesToHttpRequest([]byte(raw))
}

var contentTypeChineseCharset = regexp.MustCompile(`(?i)charset\s*=\s*['"]?(.*?)(gb[^'^"^\s]+)['"]?`)          // 2 gkxxxx
var charsetInMeta = regexp.MustCompile(`(?i)<\s*meta.*?(charset|content)\s*=\s*['"]?(.*?)(gb[^'^"^\s]+)['"]?`) // 3 gbxxx

func ParseUrlToHttpRequestRaw(method string, i interface{}) (bool, []byte, error) {
	urlStr := utils.InterfaceToString(i)
	req, err := http.NewRequest(strings.ToUpper(method), urlStr, http.NoBody)
	if err != nil {
		return false, nil, err
	}
	req.Header.Set("User-Agent", consts.DefaultUserAgent)
	bytes, err := utils.HttpDumpWithBody(req, true)
	return strings.HasPrefix(strings.ToLower(urlStr), "https://"), bytes, err
}

func CopyRequest(r *http.Request) *http.Request {
	if r == nil {
		return nil
	}
	raw, err := utils.HttpDumpWithBody(r, true)
	if err != nil {
		log.Warnf("copy request && Dump failed: %s", err)
	}
	if raw == nil {
		return nil
	}
	result, err := ParseBytesToHttpRequest(raw)
	if err != nil {
		log.Warnf("copy request && ParseBytesToHttpRequest failed: %s", err)
	}
	return result
}

func ParseBytesToHttpRequest(raw []byte) (*http.Request, error) {
	raw = FixHTTPPacketCRLF(raw, false)

	req, readErr := ReadHTTPRequest(bufio.NewReader(bytes.NewReader(raw)))
	if readErr != nil {
		log.Debugf("read [standard] httpRequest failed: %s", readErr)
	}
	if req != nil {
		return req, nil
	}

	reader := textproto.NewReader(bufio.NewReader(bytes.NewBuffer(raw)))
	firstLine, err := reader.ReadLine()
	if err != nil {
		return nil, utils.Errorf("textproto readfirstline failed: %s", err)
	}

	var ok bool
	// 解析 GET / HTTP/1.1
	req = new(http.Request)
	line := string(TrimSpaceHTTPPacket([]byte(firstLine)))

	// 修复这个小问题
	req.Method, req.RequestURI, req.Proto, ok = parseRequestLine(line)
	if !ok {
		return nil, utils.Errorf("malformed HTTP request header:（origin:%v）line:%v", strconv.Quote(firstLine), strconv.Quote(line))
	}
	if req.ProtoMajor, req.ProtoMinor, ok = http.ParseHTTPVersion(req.Proto); !ok && !strings.HasPrefix(req.Proto, "HTTP/2") {
		log.Debugf("malformed HTTP version: %v", req.Proto)
	}

	if req.Method != "CONNECT" {
		if !strings.HasPrefix(req.RequestURI, "/") {
			req.RequestURI = "/" + req.RequestURI
		}
	} else {
		if utils.IsHttpOrHttpsUrl(req.RequestURI) {
			targetUri, _ := url.Parse(req.RequestURI)
			if targetUri != nil {
				req.URL = targetUri
			}
		}
	}

	req.Header = make(http.Header)

	// 接下来解析各种 Header
	var hostInHeader string
	for {
		line, err := reader.ReadLine()
		if err != nil && err != io.EOF {
			return nil, utils.Errorf("readline for parsing http.Request.Headers failed: %s", err.Error())
		}

		// Header 解析完毕
		if line == "" {
			break
		}

		key, value := SplitHTTPHeader(line)
		if value == "" {
			req.Header[key] = []string{" "}
		} else {
			req.Header.Add(key, value)
		}

		if strings.ToLower(key) == "host" && value != "" {
			hostInHeader = value
		}
	}

	// 处理一下 Request.URL 的问题
	rawUrl := req.RequestURI
	justAuthority := req.Method == "CONNECT" && !strings.HasPrefix(rawUrl, "/")
	if justAuthority {
		rawUrl = "http://" + rawUrl
	}
	if req.URL, err = url.ParseRequestURI(rawUrl); err != nil {
		//log.Errorf("parse request uri[%v] failed: %s", rawUrl, err)
		req.URL, _ = url.ParseRequestURI(utils.RemoveUnprintableCharsWithReplaceItem(rawUrl))
		if req.URL == nil {
			req.URL, _ = url.ParseRequestURI("/")
			if req.URL != nil {
				req.URL.RawPath = rawUrl
			} else {
				req.URL = &url.URL{
					Path: rawUrl,
				}
			}
		}
	}

	if justAuthority {
		req.URL.Scheme = ""
	}

	// RFC 7230, section 5.3: Must treat
	//	GET /index.html HTTP/1.1
	//	Host: www.google.com
	// and
	//	GET http://www.google.com/index.html HTTP/1.1
	//	Host: doesntmatter
	// the same. In the second case, any Host line is ignored.
	req.Host = req.URL.Host
	if req.Host == "" {
		req.Host = req.Header.Get("Host")
	}
	if req.Host == "" && hostInHeader != "" {
		req.Host = hostInHeader
	}

	// 接下来应该处理 Body 的问题了
	rawBody, err := ioutil.ReadAll(reader.R)
	if err != nil {
		return nil, utils.Errorf("read last all body failed: %s", err)
	}
	req.Body = ioutil.NopCloser(bytes.NewBuffer(rawBody))

	// 1. Chunked 分块传输
	chunked := strings.Contains(strings.Join(req.Header.Values("Transfer-Encoding"), "|"), "chunked")
	if chunked {
		return req, nil
	}

	// 普通 Content-Length
	cl := len(rawBody)
	req.Header.Set("Content-Length", fmt.Sprint(cl))
	return req, nil
}

func ReadHTTPRequest(reader *bufio.Reader) (*http.Request, error) {
	return ReadHTTPRequestEx(reader, false)
}

func ReadHTTPResponseEx(reader *bufio.Reader, loadbody bool) (*http.Response, []byte, error) {
	var buf bytes.Buffer
	rsp, err := http.ReadResponse(bufio.NewReader(io.TeeReader(reader, &buf)), nil)
	if err != nil {
		return nil, buf.Bytes(), err
	}

	if loadbody {
		var finalBody, _ = ioutil.ReadAll(rsp.Body)
		rsp.Body = ioutil.NopCloser(bytes.NewBuffer(finalBody))
	}

	var cache = make(map[string]string)
	// 这里用来恢复 Req 的大小写
	SplitHTTPHeadersAndBodyFromPacket(buf.Bytes(), func(line string) {
		if index := strings.Index(line, ":"); index > 0 {
			key := line[:index]
			ckey := textproto.CanonicalMIMEHeaderKey(key)
			_, ok := commonHeader[ckey]
			// 大小写发生了变化，并且不是常见公共头，则说明需要恢复一下
			if ckey != key && !ok {
				cache[ckey] = key
			}
		}
	})

	for ckey, key := range cache {
		values, ok := rsp.Header[ckey]
		if ok {
			rsp.Header[key] = values
			delete(rsp.Header, ckey)
		}
	}

	return rsp, buf.Bytes(), nil
}

func ReadHTTPRequestEx(reader *bufio.Reader, loadbody bool) (*http.Request, error) {
	var buf bytes.Buffer
	req, err := http.ReadRequest(bufio.NewReader(io.TeeReader(reader, &buf)))
	if err != nil {
		return nil, err
	}

	if utils.IsHttpOrHttpsUrl(req.RequestURI) {
		u, _ := url.Parse(req.RequestURI)
		if u != nil {
			req.URL = u
			req.Host = u.Host
			if strings.HasPrefix(u.Path, "/") || u.RawQuery != "" {
				req.RequestURI = u.RequestURI()
			} else {
				req.RequestURI = u.Path
			}
		}
	}

	if loadbody {
		var finalBody, _ = ioutil.ReadAll(req.Body)
		req.Body = ioutil.NopCloser(bytes.NewBuffer(finalBody))
	}

	var cache = make(map[string]string)
	var host = req.Host
	var cachedHeader = make(map[string][]string)
	// 这里用来恢复 Req 的大小写
	SplitHTTPHeadersAndBodyFromPacket(buf.Bytes(), func(line string) {
		key, value := SplitHTTPHeader(line)
		cachedHeader[key] = append(cachedHeader[key], value)

		if strings.ToLower(key) == "host" && value != "" {
			host = value
		}

		ckey := textproto.CanonicalMIMEHeaderKey(key)
		_, ok := commonHeader[ckey]
		// 大小写发生了变化，并且不是常见公共头，则说明需要恢复一下
		if ckey != key && !ok {
			cache[ckey] = key
		}
	})

	for ckey, key := range cache {
		values, ok := req.Header[ckey]
		if ok {
			req.Header[key] = values
			delete(req.Header, ckey)
		}
	}

	for key, values := range cachedHeader {
		req.Header[key] = values
	}
	req.Host = host

	//black magic fix when browser use http proxy the RequestURI is not canonical
	if strings.HasPrefix(req.RequestURI, "http://") || strings.HasPrefix(req.RequestURI, "https://") {
		if req.Header.Get("Host") == "" {
			req.Header.Add("Host", req.URL.Host)
		}
		req.RequestURI = req.URL.RequestURI()
	}

	return req, nil
}

func ReadHTTPPacketSafe(r *bufio.Reader) ([]byte, error) {
	var line []string
	firstLine, err := utils.BufioReadLine(r)
	if err != nil {
		return nil, errors.Wrapf(err, "read httppacket first line")
	}
	line = append(line, string(firstLine))

	for {
		lineBytes, err := utils.BufioReadLine(r)
		if err != nil {
			break
		}
		line = append(line, string(lineBytes))
		if lineBytes == nil {
			break
		}
	}

	var raw []byte
	headers := strings.Join(line, CRLF)
	headerBytes := []byte(headers)
	cl, chunk := ReadHTTPPacketBodySize([]byte(headers))
	if chunk {
		raw, _ = ioutil.ReadAll(httputil.NewChunkedReader(r))
		return ReplaceHTTPPacketBody(headerBytes, raw, false), nil
	} else {
		var body = make([]byte, cl)
		_, err := io.ReadFull(r, body)
		if err != nil {
			return nil, errors.Wrapf(err, "bufio.Reader => io.ReadFull [%v]", cl)
		}
		return ReplaceHTTPPacketBody(headerBytes, body, false), nil
	}
}

func ExtractBoundaryFromBody(raw interface{}) string {
	bodyStr := strings.TrimSpace(utils.InterfaceToString(raw))
	if strings.HasPrefix(bodyStr, "--") && strings.HasSuffix(bodyStr, "--") {
		sc := bufio.NewScanner(bytes.NewBufferString(bodyStr))
		sc.Split(bufio.ScanLines)
		if !sc.Scan() {
			return ""
		}
		prefixWithBoundary := sc.Text()
		if strings.HasPrefix(prefixWithBoundary, "--") {
			return prefixWithBoundary[2:]
		}
		return ""
	}
	return ""
}

var extractStatusRe = regexp.MustCompile(`^HTTP/[\d](\.\d)?\s(\d{3})`)

func ExtractStatusCodeFromResponse(raw []byte) int {
	var m = make([]byte, 20)
	copy(m, raw)

	if ret := extractStatusRe.FindStringSubmatch(strings.Trim(string(m), "\x00")); len(ret) > 2 {
		code, _ := strconv.Atoi(ret[2])
		return code
	}
	return 0
}
