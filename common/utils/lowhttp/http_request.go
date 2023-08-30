package lowhttp

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httputil"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	utils "github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
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

func FixHTTPPacketCRLF(raw []byte, noFixLength bool) []byte {
	// 移除左边空白字符
	raw = TrimLeftHTTPPacket(raw)
	var isMultipart bool
	var haveChunkedHeader bool
	var haveContentLength bool
	var isRequest bool
	var isResponse bool
	var contentLengthIsNotRecommanded bool

	var plrand = fmt.Sprintf("[[REPLACE_CONTENT_LENGTH:%v]]", utils.RandStringBytes(20))
	var plrandHandled = false
	header, body := SplitHTTPPacket(
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
		func(line string) string {
			key, value := SplitHTTPHeader(line)
			var keyLower = strings.ToLower(key)
			var valLower = strings.ToLower(value)
			if !isMultipart && keyLower == "content-type" && strings.HasPrefix(valLower, "multipart/form-data") {
				isMultipart = true
			}
			if !haveContentLength && strings.ToLower(key) == "content-length" {
				haveContentLength = true
				if noFixLength {
					return line
				}
				return fmt.Sprintf(`%v: %v`, key, plrand)
			}
			if !haveChunkedHeader && keyLower == "transfer-encoding" && valLower == "chunked" {
				haveChunkedHeader = true
			}
			return line
		},
	)

	// cl te existed at the same time, handle smuggle!
	var smuggleCase = isRequest && haveContentLength && haveChunkedHeader
	_ = smuggleCase

	// applying patch to restore CRLF at body
	// if `raw` has CRLF at body end (by design HTTP smuggle) and `noFixContentLength` is true
	if bytes.HasSuffix(raw, []byte(CRLF+CRLF)) && noFixLength && len(body) > 0 {
		body = append(body, []byte(CRLF+CRLF)...)
	}

	_ = isResponse
	handleChunked := haveChunkedHeader && !haveContentLength
	var restBody []byte
	if handleChunked {
		// chunked body is very complex
		// if multiRequest: extract and remove body suffix
		var bodyDecode []byte
		bodyDecode, restBody = codec.HTTPChunkedDecodeWithRestBytes(body)
		if len(bodyDecode) > 0 {
			readLen := len(body) - len(restBody)
			body = body[:readLen]
		}
	}

	/* boundary fix */
	if isRequest && isMultipart {
		boundary, fixed := FixMultipartBody(body)
		if boundary != "" {
			header = string(ReplaceMIMEType([]byte(header), "multipart/form-data; boundary="+boundary))
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

func FixHTTPRequestOut(raw []byte) []byte {
	return FixHTTPPacketCRLF(raw, false)
}

func DeletePacketEncoding(raw []byte) []byte {
	var encoding string
	var isChunked bool
	var buf bytes.Buffer
	_, body := SplitHTTPPacket(raw, func(method string, requestUri string, proto string) error {
		buf.WriteString(method + " " + requestUri + " " + proto + CRLF)
		return nil
	}, func(proto string, code int, codeMsg string) error {
		buf.WriteString(proto + " " + strconv.Itoa(code) + " " + codeMsg + CRLF)
		return nil
	}, func(line string) string {
		k, v := SplitHTTPHeader(line)
		ret := strings.ToLower(k)
		if ret == "content-encoding" {
			encoding = v
			return ""
		} else if ret == "transfer-encoding" && strings.ToLower(v) == "chunked" {
			isChunked = true
			return ""
		}
		buf.WriteString(line + CRLF)
		return line
	})
	buf.WriteString(CRLF)

	if isChunked {
		decBody, _ := codec.HTTPChunkedDecode(body)
		if len(decBody) > 0 {
			body = decBody
		}
	}

	decResult, fixed := ContentEncodingDecode(encoding, body)
	if fixed && len(decResult) > 0 {
		body = decResult
	}
	return ReplaceHTTPPacketBody(buf.Bytes(), body, false)
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
	return nil, readErr
}

func ReadHTTPRequest(reader *bufio.Reader) (*http.Request, error) {
	req, err := utils.ReadHTTPRequestFromBufioReader(reader)
	if err != nil {
		return nil, err
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
