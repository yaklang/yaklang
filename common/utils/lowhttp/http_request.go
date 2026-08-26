package lowhttp

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/utils/multipart"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	utils "github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var (
	_transferEncodingRE = regexp.MustCompile(`(?i)Transfer-Encoding:(\s+)?.*?(chunked).*?\r?\n?`)
)

// HTTPPacketForceChunked 将一个HTTP报文的body强制转换为chunked编码
// 参数:
//   - raw: 原始 HTTP 报文字节数组
//
// 返回值:
//   - 转换为 chunked 编码后的 HTTP 报文字节数组
//
// Example:
// ```
// poc.HTTPPacketForceChunked(`POST / HTTP/1.1
// Host: example.com
// Content-Length: 11
//
// hello world`)
// ```
func HTTPPacketForceChunked(raw []byte) []byte {
	header, body := SplitHTTPHeadersAndBodyFromPacket(raw)
	return ReplaceHTTPPacketBodyEx([]byte(header), body, true, false)
}

func HTTPHeaderForceChunked(raw []byte) []byte {
	header, body := SplitHTTPHeadersAndBodyFromPacket(raw)
	newPacket := ReplaceHTTPPacketBodyEx([]byte(header), body, true, false)
	newHeader, _ := SplitHTTPHeadersAndBodyFromPacket(newPacket)
	return []byte(newHeader + string(body))
}

func AppendHeaderToHTTPPacket(raw []byte, line string) []byte {
	header, body := SplitHTTPHeadersAndBodyFromPacket(raw)
	header = strings.TrimRight(header, "\r\n") + CRLF + strings.TrimSpace(line) + CRLF + CRLF
	return []byte(header + string(body))
}

var _contentTypeHeaderRegexp = regexp.MustCompile(`(?i)content-type: ?`)

// FixHTTPPacketCRLF 修复一个HTTP报文的CRLF问题（正常的报文每行末尾为\r\n，但是某些报文可能是有\n），如果noFixLength为true，则不会修复Content-Length，否则会尝试修复Content-Length
// 参数:
//   - raw: 原始 HTTP 报文字节数组
//   - noFixLength: 是否不修复 Content-Length，为 true 时不修复
//
// 返回值:
//   - 修复后的 HTTP 报文字节数组
//
// Example:
// ```
// poc.FixHTTPPacketCRLF(`POST / HTTP/1.1
// Host: example.com
// Content-Length: 11
//
// hello world`, false)
// ```
func FixHTTPPacketCRLF(raw []byte, noFixLength bool) []byte {
	return fixHTTPPacketCRLF(raw, noFixLength, false)
}

// FixHTTPPacketCRLFBorrowed preserves FixHTTPPacketCRLF's byte-for-byte result,
// but may return an immutable view of raw when normalization is an exact no-op.
// Callers must keep raw alive and must not mutate the returned packet.
func FixHTTPPacketCRLFBorrowed(raw []byte, noFixLength bool) []byte {
	return fixHTTPPacketCRLFWithBorrow(raw, noFixLength, false, true)
}

// HTTP header names and the tokens inspected below are ASCII. Preserve the
// legacy Unicode/invalid-UTF-8 behavior by falling back to strings.ToLower for
// non-ASCII input while keeping the normal path allocation-free.
func equalASCIIFoldOrLower(value, lowerTarget string) bool {
	if !isASCIIBytes(value) {
		return strings.ToLower(value) == lowerTarget
	}
	return utils.AsciiEqualFold(value, lowerTarget)
}

func hasPrefixASCIIFoldOrLower(value, lowerPrefix string) bool {
	if !isASCIIBytes(value) {
		return strings.HasPrefix(strings.ToLower(value), lowerPrefix)
	}
	return len(value) >= len(lowerPrefix) && utils.AsciiEqualFold(value[:len(lowerPrefix)], lowerPrefix)
}

func containsASCIIFoldOrLower(value, lowerNeedle string) bool {
	if !isASCIIBytes(value) {
		return strings.Contains(strings.ToLower(value), lowerNeedle)
	}
	for index := 0; index+len(lowerNeedle) <= len(value); index++ {
		if utils.AsciiEqualFold(value[index:index+len(lowerNeedle)], lowerNeedle) {
			return true
		}
	}
	return false
}

func isASCIIBytes(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func fixHTTPPacketCRLF(raw []byte, noFixLength bool, cloneBody bool) []byte {
	return fixHTTPPacketCRLFWithBorrow(raw, noFixLength, cloneBody, false)
}

func fixHTTPPacketCRLFWithBorrow(raw []byte, noFixLength bool, cloneBody bool, allowBorrow bool) []byte {
	// 移除左边空白字符
	raw = TrimLeftCRLF(raw)
	if raw == nil || len(raw) == 0 {
		return nil
	}
	var isMultipart bool
	var haveChunkedHeader bool
	var haveContentLength bool
	var isRequest bool
	var isResponse bool
	var contentLengthIsNotRecommanded bool

	plrand := fmt.Sprintf("[[REPLACE_CONTENT_LENGTH:%v]]", utils.RandStringBytes(20))
	plrandHandled := false
	contentTypeRawValue := ""
	header, body := splitHTTPPacketEx(
		raw,
		func(m, u, proto string) error {
			isRequest = true
			contentLengthIsNotRecommanded = utils.ShouldRemoveZeroContentLengthHeader(m)
			return nil
		},
		func(proto string, code int, codeMsg string) error {
			isResponse = true
			return nil
		},
		nil,
		cloneBody,
		func(line string) string {
			key, value := SplitHTTPHeader(line)
			if !isMultipart &&
				equalASCIIFoldOrLower(key, "content-type") &&
				hasPrefixASCIIFoldOrLower(value, "multipart/form-data") {
				if matchResult := _contentTypeHeaderRegexp.FindIndex([]byte(line)); len(matchResult) > 1 {
					end := matchResult[1]
					contentTypeRawValue = line[end:]
				} else {
					contentTypeRawValue = value
				}
				isMultipart = true
			}
			if !haveContentLength && equalASCIIFoldOrLower(key, "content-length") {
				haveContentLength = true
				if noFixLength {
					return line
				}
				return fmt.Sprintf(`%v: %v`, key, plrand)
			}
			if !haveChunkedHeader &&
				equalASCIIFoldOrLower(key, "transfer-encoding") &&
				containsASCIIFoldOrLower(value, "chunked") {
				haveChunkedHeader = true
			}
			return line
		},
	)

	// cl te existed at the same time, handle smuggle!
	smuggleCase := isRequest && haveContentLength && haveChunkedHeader
	_ = smuggleCase

	// applying patch to restore CRLF at body
	// if `raw` has CRLF at body end (by design HTTP smuggle) and `noFixContentLength` is true
	//if bytes.HasSuffix(raw, []byte(CRLF+CRLF)) && noFixLength && len(body) > 0 && !smuggleCase {
	//	body = append(body, []byte(CRLF+CRLF)...)
	//}

	_ = isResponse
	handleChunked := haveChunkedHeader && !haveContentLength
	var restBody []byte
	if handleChunked {
		// chunked body is very complex
		// if multiRequest: extract and remove body suffix
		var bodyDecode []byte
		var fixedBody []byte
		var err error
		bodyDecode, fixedBody, restBody, err = codec.ReadHTTPChunkedDataWithFixedError(body)
		if err != nil {
			restBody = nil
		} else {
			if len(bodyDecode) > 0 {
				if len(restBody) > 0 {
					readLen := len(body) - len(restBody)
					body = body[:readLen]
				} else {
					body = fixedBody
				}
			}
		}

	}

	/* boundary fix */
	if isRequest && isMultipart {
		boundary, fixed := FixMultipartBody(body)
		if boundary != "" {
			newContentType := "multipart/form-data; boundary=" + boundary
			origin, params, err := mime.ParseMediaType(contentTypeRawValue)
			if err == nil {
				params["boundary"] = boundary
				newContentType = mime.FormatMediaType(origin, params)
			}
			if !strings.Contains(contentTypeRawValue, boundary) {
				header = string(ReplaceMIMEType([]byte(header), newContentType))
			}
			body = fixed
		}
	}

	if !noFixLength && !haveChunkedHeader {
		if haveContentLength {
			// have CL
			// only cl && no chunked && fix length
			// fix content-length
			header = strings.Replace(header, plrand, strconv.Itoa(len(body)), 1)
		} else {
			bodyLength := len(body)
			if bodyLength > 0 {
				// no CL
				// fix content-length
				header = strings.TrimRight(header, CRLF)
				header += fmt.Sprintf("\r\nContent-Length: %v\r\n\r\n", bodyLength)
			} else {
				if !contentLengthIsNotRecommanded {
					header = strings.TrimRight(header, CRLF)
					header += "\r\nContent-Length: 0\r\n\r\n"
				}
			}
		}
		plrandHandled = true
	}

	if !plrandHandled && haveContentLength && !noFixLength {
		header = strings.Replace(header, plrand, strconv.Itoa(len(body)), 1)
	}

	if allowBorrow &&
		len(restBody) == 0 &&
		len(header) <= len(raw) &&
		len(header)+len(body) == len(raw) &&
		header == utils.UnsafeBytesToString(raw[:len(header)]) &&
		(len(body) == 0 || &body[0] == &raw[len(header)]) {
		return raw
	}

	var buf bytes.Buffer
	buf.Write([]byte(header))
	if len(body) > 0 {
		buf.Write(body)
	}
	if len(restBody) > 0 {
		buf.Write(restBody)
	}
	return buf.Bytes()
}

func FixHTTPPacketQueryEscape(raw []byte) []byte {
	var isChunked bool
	var buf bytes.Buffer
	var header []string

	_, body := SplitHTTPPacket(raw,
		func(method string, requestUri string, proto string) error {
			defer func() {
				buf.WriteString(method + " " + requestUri + " " + proto)
				buf.WriteString(CRLF)
			}()

			// handle requestUri
			urlIns := ForceStringToUrl(requestUri)
			if urlIns == nil {
				// invalid url, return as is
				return nil
			}

			// Parse and re-encode query parameters to ensure proper escaping
			if urlIns.RawQuery != "" {
				queryParams := ParseQueryParams(urlIns.RawQuery, WithDisableAutoEncode(false))
				urlIns.RawQuery = queryParams.Encode()
			}

			// Reconstruct the request URI
			requestUri = urlIns.String()
			return nil
		},
		nil,
		func(line string) string {
			if !isChunked {
				isChunked = IsChunkedHeaderLine(line)
			}
			header = append(header, line)
			return line
		},
	)

	for _, line := range header {
		buf.WriteString(line)
		buf.WriteString(CRLF)
	}
	return ReplaceHTTPPacketBody(buf.Bytes(), body, isChunked)
}

// FixHTTPRequest 尝试对传入的HTTP请求报文进行修复，并返回修复后的请求
//
// 参数:
//   - raw: 原始 HTTP 请求报文字节
//
// 返回值:
//   - 修复后的 HTTP 请求报文字节
//
// Example:
// ```
// str.FixHTTPRequest(b"GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
// ```
func FixHTTPRequest(raw []byte) []byte {
	return FixHTTPPacketCRLF(raw, false)
}

// FixHTTPRequestBorrowed is the immutable-input variant used by forwarding
// paths that only need a normalized packet for the duration of the call.
func FixHTTPRequestBorrowed(raw []byte) []byte {
	return FixHTTPPacketCRLFBorrowed(raw, false)
}

func DeletePacketEncoding(raw []byte) []byte {
	plain, _ := DeletePacketEncodingWithOwnership(raw)
	return plain
}

// DeletePacketEncodingWithOwnership removes transfer/content encoding and
// reports whether the returned packet is independently owned. A false result
// means the conservative/no-op path returned a view of raw; callers that retain
// it beyond raw's lifetime must still clone it.
func DeletePacketEncodingWithOwnership(raw []byte) (plain []byte, independentlyOwned bool) {
	plain, _, independentlyOwned = _unzipPacketEncodingInternal(
		raw,
		_defaultUnzipPacketEncodingConfig(),
		_withUnzipPacketEncodingMaxDecodedBytes(0),
	)
	return plain, independentlyOwned
}

func ConvertHTTPRequestToFuzzTag(i []byte) []byte {
	var boundary string // 如果是上传数据包的话，boundary 就不会为空
	header, body := SplitHTTPHeadersAndBodyFromPacket(i, func(line string) {
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
		reader := multipart.NewReader(bytes.NewBuffer(body))

		// 修复数据包
		var buf bytes.Buffer
		fixedBody := multipart.NewWriter(&buf)
		fixedBody.SetBoundary(boundary)
		converted := false
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
			if !bodyNeedsUnquote(body) {
				w.Write(body)
			} else {
				converted = true
				w.Write([]byte(ToUnquoteFuzzTagForce(body)))
			}
		}
		fixedBody.Close()
		if !converted {
			return i
		}
		body = buf.Bytes()
		return ReplaceHTTPPacketBody([]byte(header), body, false)
	}

	if !bodyNeedsUnquote(body) {
		return i
	}
	body = []byte(ToUnquoteFuzzTagForce(body))
	return ReplaceHTTPPacketBody([]byte(header), body, false)
}

func bodyNeedsUnquote(body []byte) bool {
	// The editor representation follows the bytes, not the declared MIME.
	// Multipart callers invoke this for each part independently so one invalid
	// file part does not turn otherwise safe text fields into binary chips.
	return !utf8.Valid(body)
}

type fuzzTagBodyCountingWriter struct {
	n int
}

func (w *fuzzTagBodyCountingWriter) Write(p []byte) (int, error) {
	w.n += len(p)
	return len(p), nil
}

// MeasureHTTPRequestFuzzTagBodySize returns the exact body size that
// ConvertHTTPRequestToFuzzTag would expose to a text editor, without allocating
// the expanded binary representation. The second result reports whether any
// body/part requires an unquote tag.
func MeasureHTTPRequestFuzzTagBodySize(i []byte) (int, bool, error) {
	var boundary string
	_, body := SplitHTTPHeadersAndBodyFromPacket(i, func(line string) {
		k, v := SplitHTTPHeader(strings.TrimSpace(line))
		if !strings.EqualFold(k, "content-type") {
			return
		}
		ctVal, params, _ := mime.ParseMediaType(v)
		if ctVal == "multipart/form-data" && params != nil {
			boundary = params["boundary"]
		}
	})

	if boundary == "" {
		binary := bodyNeedsUnquote(body)
		if !binary {
			return len(body), false, nil
		}
		return UnquoteFuzzTagSize(body, true), true, nil
	}

	reader := multipart.NewReader(bytes.NewReader(body))
	counter := &fuzzTagBodyCountingWriter{}
	writer := multipart.NewWriter(counter)
	if err := writer.SetBoundary(boundary); err != nil {
		return 0, false, err
	}
	converted := false
	for {
		part, err := reader.NextRawPart()
		if err != nil {
			if err == io.EOF {
				break
			}
			return 0, false, err
		}
		if _, err := writer.CreatePart(part.Header); err != nil {
			return 0, false, err
		}
		partBody, err := io.ReadAll(part)
		if err != nil {
			return 0, false, err
		}
		binary := bodyNeedsUnquote(partBody)
		converted = converted || binary
		if binary {
			counter.n += UnquoteFuzzTagSize(partBody, true)
		} else {
			counter.n += len(partBody)
		}
	}
	if err := writer.Close(); err != nil {
		return 0, false, err
	}
	if !converted {
		return len(body), false, nil
	}
	return counter.n, true, nil
}

const (
	printableMin = 32
	printableMax = 126
)

func ToUnquoteFuzzTag(i []byte) string {
	return toUnquoteFuzzTag(i, false)
}

// ToUnquoteFuzzTagForce encodes bytes even when they happen to be valid UTF-8.
// It is used when a caller explicitly needs to preserve an existing binary
// editor/Fuzztag representation, such as after saving a HEX edit.
func ToUnquoteFuzzTagForce(i []byte) string {
	return toUnquoteFuzzTag(i, true)
}

func toUnquoteFuzzTag(i []byte, force bool) string {
	if !force && utf8.Valid(i) {
		return string(i)
	}

	buf := bytes.NewBufferString(`{{unquote("`)
	for _, b := range i {
		if b >= printableMin && b <= printableMax {
			switch b {
			case '(':
				buf.WriteString(`\x28`)
			case ')':
				buf.WriteString(`\x29`)
			case '}':
				buf.WriteString(`\x7d`)
			case '{':
				buf.WriteString(`\x7b`)
			case '\\':
				buf.WriteString(`\\`)
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

// UnquoteFuzzTagSize returns len(ToUnquoteFuzzTag(i)), or the forced variant
// when force is true, without allocating the encoded string. The representation
// has 15 bytes of fixed syntax; each source byte contributes 1, 2, or 4 bytes.
func UnquoteFuzzTagSize(i []byte, force bool) int {
	if !force && utf8.Valid(i) {
		return len(i)
	}
	size := len("{{unquote(\"\")}}")
	for _, b := range i {
		switch {
		case b < printableMin || b > printableMax:
			size += 4
		case b == '(' || b == ')' || b == '{' || b == '}':
			size += 4
		case b == '\\' || b == '"':
			size += 2
		default:
			size++
		}
	}
	return size
}

//func FixHTTPRequest(raw []byte) []byte {
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

// ParseStringToHTTPRequest 将字符串解析为 HTTP 请求
//
// 参数:
//   - raw: HTTP 请求报文字符串
//
// 返回值:
//   - 解析出的 HTTP 请求结构体
//   - 错误信息
//
// Example:
// ```
// req, err = str.ParseStringToHTTPRequest("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
// ```
func ParseStringToHttpRequest(raw string) (*http.Request, error) {
	return ParseBytesToHttpRequest([]byte(raw))
}

var (
	contentTypeChineseCharset = regexp.MustCompile(`(?i)charset\s*=\s*['"]?(.*?)(gb[^'^"^\s]+)['"]?`)                      // 2 gkxxxx
	charsetInMeta             = regexp.MustCompile(`(?i)<\s*meta.*?(charset|content)\s*=\s*['"]?(.*?)(gb[^'^"^\s]+)['"]?`) // 3 gbxxx
)

// ParseUrlToHTTPRequestRaw 将URL解析为原始 HTTP 请求报文，返回是否为 HTTPS，原始请求报文与错误
// 参数:
//   - method: 请求方法，如 GET、POST
//   - i: 目标 URL，可为字符串或可转换为字符串的对象
//
// 返回值:
//   - 是否为 HTTPS
//   - 原始 HTTP 请求报文字节数组
//   - 错误信息，解析失败时返回非空
//
// Example:
// ```
// ishttps, raw, err = poc.ParseUrlToHTTPRequestRaw("GET", "https://yaklang.com")
// ```
func ParseUrlToHttpRequestRaw(method string, i interface{}) (isHttps bool, req []byte, err error) {
	urlStr := utils.InterfaceToString(i)
	reqInst, err := http.NewRequest(strings.ToUpper(method), urlStr, http.NoBody)
	if err != nil {
		return false, nil, err
	}
	reqInst.Header.Set("User-Agent", consts.DefaultUserAgent)
	bytes, err := utils.HttpDumpWithBody(reqInst, true)
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

// ParseBytesToHTTPRequest 将字节数组解析为 HTTP 请求
// 参数:
//   - raw: 原始 HTTP 请求报文字节数组
//
// 返回值:
//   - 解析得到的 HTTP 请求对象
//   - 错误信息，解析失败时返回非空
//
// Example:
// ```
// req, err := str.ParseBytesToHTTPRequest(b"GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
// ```
func ParseBytesToHttpRequest(raw []byte) (reqInst *http.Request, err error) {
	fixed := FixHTTPPacketCRLF(raw, false)
	if fixed == nil {
		return nil, io.EOF
	}
	req, readErr := utils.ReadHTTPRequestFromBytes(fixed)
	if readErr != nil {
		log.Errorf("read [standard] httpRequest failed: %s", readErr)
		return nil, readErr
	}
	return req, nil
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

var (
	ReadHTTPRequestFromBytes        = utils.ReadHTTPRequestFromBytes
	ReadHTTPRequestFromBufioReader  = utils.ReadHTTPRequestFromBufioReader
	ReadHTTPResponseFromBytes       = utils.ReadHTTPResponseFromBytes
	ReadHTTPResponseFromBufioReader = utils.ReadHTTPResponseFromBufioReader
)

func ExtractStatusCodeFromResponse(raw []byte) int {
	var statusCode int
	SplitHTTPPacket(raw, nil, func(proto string, code int, codeMsg string) error {
		statusCode = code
		return utils.Error("abort")
	})
	return statusCode
}
