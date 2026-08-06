package lowhttp

import (
	"bytes"
	"fmt"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/log"
	utils "github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var (
	charsetRegexp         = regexp.MustCompile(`(?i)charset\s*=\s*"?\s*([^\s;\n\r"]+)`)
	contentTypeRegexp     = regexp.MustCompile(`(?i)content-type:\s*([^\r\n]*)`)
	contentEncodingRegexp = regexp.MustCompile(`(?i)content-encoding:\s*\w*\r?\n`)

	isChunkedBytes = []byte("\r\n0\r\n\r\n")
)

var expect100continue = []byte("HTTP/1.1 100 Continue\r\n\r\n")

var (
	textPlainMIMEGlob = []glob.Glob{
		glob.MustCompile(`text/plain`),
	}

	jsonMIMEGlobs = []glob.Glob{
		glob.MustCompile(`application/json`),
		glob.MustCompile(`application/*json*`),
		glob.MustCompile(`text/*json*`),
	}
	jsMIMEGlobs = []glob.Glob{
		glob.MustCompile(`application/*javascript*`),
		glob.MustCompile(`text/*javascript*`),
		glob.MustCompile(`application/*ecmascript*`),
		glob.MustCompile(`text/*ecmascript*`),
		glob.MustCompile(`text/jscript`),
	}
	htmlMIMEGlob = []glob.Glob{
		glob.MustCompile(`text/html`),
		glob.MustCompile(`application/xhtml+xml`),
		glob.MustCompile(`application/html`),
		glob.MustCompile(`text/x-html`),
		glob.MustCompile(`application/xml`),
		glob.MustCompile(`text/xml`),
		glob.MustCompile(`application/xhtml`),
		glob.MustCompile(`application/*html*`),
		glob.MustCompile(`text/*html*`),
	}
)

func IsTextPlainMIMEType(s string) bool {
	if s == "" {
		return false
	}

	if strings.Contains(strings.ToLower(s), "charset=") {
		lake, _, err := mime.ParseMediaType(s)
		if err == nil {
			s = lake
		}
	}

	for _, g := range textPlainMIMEGlob {
		if g.Match(s) {
			return true
		}
	}
	return false
}

func IsJsonMIMEType(s string) bool {
	if s == "" {
		return false
	}
	for _, g := range jsonMIMEGlobs {
		if g.Match(s) {
			return true
		}
	}
	return false
}

func IsJavaScriptMIMEType(s string) bool {
	if s == "" {
		return false
	}
	for _, g := range jsMIMEGlobs {
		if g.Match(s) {
			return true
		}
	}
	return false
}

func IsHtmlOrXmlMIMEType(s string) bool {
	if s == "" {
		return false
	}

	if strings.Contains(strings.ToLower(s), "charset=") {
		lake, _, err := mime.ParseMediaType(s)
		if err == nil {
			s = lake
		}
	}

	for _, g := range htmlMIMEGlob {
		if g.Match(s) {
			return true
		}
	}
	return false
}

// FixHTTPResponse 尝试对传入的响应进行修复，并返回修复后的响应，响应体和错误
//
// 参数:
//   - raw: 原始 HTTP 响应报文字节
//
// 返回值:
//   - rsp: 修复后的完整响应报文
//   - body: 修复后的响应体
//   - err: 错误信息
//
// Example:
// ```
// fixedResponse, body, err = str.FixHTTPResponse(b"HTTP/1.1 200 OK\r\nContent-Type: text/html; charset=gbk\r\n\r\n<html>你好</html>")
// ```
func FixHTTPResponse(raw []byte) (rsp []byte, body []byte, _ error) {
	return fixHTTPResponse(raw, true)
}

// FixHTTPResponsePacket fixes an HTTP response for callers that only need the
// rebuilt packet. The returned packet owns its storage and raw is not modified.
func FixHTTPResponsePacket(raw []byte) (rsp []byte, _ error) {
	rsp, _, err := fixHTTPResponse(raw, false)
	return rsp, err
}

// FixHTTPResponsePacketBorrowed preserves FixHTTPResponsePacket's byte-for-byte
// result, but may return an immutable view of raw when fixing is an exact no-op.
// FixHTTPResponsePacketBorrowed may retain an already-valid response packet,
// including its original Content-Length header position. The generic
// FixHTTPResponsePacket API continues to return an independently owned,
// canonically rebuilt packet.
func FixHTTPResponsePacketBorrowed(raw []byte) (rsp []byte, _ error) {
	rsp, _, err := fixHTTPResponseWithBorrow(raw, false, true)
	return rsp, err
}

func rebuildFixedHTTPResponse(
	headerBytes, body []byte,
	cloneBody, bodyIndependentlyOwned bool,
	borrowPacket []byte,
) ([]byte, []byte, error) {
	if !cloneBody && bodyIndependentlyOwned {
		packet := replaceHTTPPacketBodyExOwned(headerBytes, body, false, true)
		_, packetBody := SplitHTTPHeadersAndBodyFromPacketView(packet)
		return packet, packetBody, nil
	}
	return replaceHTTPPacketBodyExWithBorrow(headerBytes, body, false, true, false, borrowPacket), body, nil
}

type fixedHTTPResponseHeaderState struct {
	isChunked       bool
	contentEncoding string
	contentType     string
	noContentType   bool
}

func (state *fixedHTTPResponseHeaderState) parse(line string) {
	const (
		contentTypePrefix      = "content-type:"
		transferEncodingPrefix = "transfer-encoding:"
		contentEncodingPrefix  = "content-encoding:"
	)

	switch {
	case len(line) >= len(contentTypePrefix) && utils.AsciiEqualFold(line[:len(contentTypePrefix)], contentTypePrefix):
		_, state.contentType = SplitHTTPHeader(line)
		state.noContentType = false
	case len(line) >= len(transferEncodingPrefix) && utils.AsciiEqualFold(line[:len(transferEncodingPrefix)], transferEncodingPrefix):
		lowerLine := strings.ToLower(line)
		if strings.Contains(lowerLine, "chunked") {
			state.isChunked = true
		}
	case len(line) >= len(contentEncodingPrefix) && utils.AsciiEqualFold(line[:len(contentEncodingPrefix)], contentEncodingPrefix):
		// ContentEncodingDecode historically receives the complete lowercase line.
		state.contentEncoding = strings.ToLower(line)
	}
}

func fixHTTPResponse(raw []byte, cloneBody bool) (rsp []byte, body []byte, _ error) {
	return fixHTTPResponseWithBorrow(raw, cloneBody, false)
}

func fixHTTPResponseWithBorrow(raw []byte, cloneBody, allowBorrow bool) (rsp []byte, body []byte, _ error) {
	// log.Infof("response raw: \n%v", codec.EncodeBase64(raw))
	raw, _ = bytes.CutPrefix(raw, expect100continue)
	var borrowPacket []byte
	if allowBorrow {
		borrowPacket = raw
	}

	headerState := fixedHTTPResponseHeaderState{noContentType: true}
	var headers string
	if cloneBody {
		headers, body = SplitHTTPHeadersAndBodyFromPacket(raw, headerState.parse)
	} else {
		headers, body = SplitHTTPHeadersAndBodyFromPacketView(raw, headerState.parse)
	}
	if headers == "" {
		return nil, nil, utils.Errorf("error for parsing http response")
	}
	headerBytes := []byte(headers)
	isChunked := headerState.isChunked
	contentEncoding := headerState.contentEncoding
	contentType := headerState.contentType
	noContentTypeSet := headerState.noContentType

	bodyRaw := body
	bodyIndependentlyOwned := false
	if bodyRaw != nil && isChunked {
		// HTTPChunkedDecode may shorten its error preview in-place. Keep the
		// packet-only API's input immutable even for malformed chunked bodies.
		if !cloneBody {
			bodyRaw = bytes.Clone(bodyRaw)
		}
		unchunked, chunkErr := codec.HTTPChunkedDecode(bodyRaw)
		if unchunked != nil {
			bodyRaw = unchunked
			bodyIndependentlyOwned = true
		} else {
			if chunkErr == nil {
				bodyRaw = []byte{}
				bodyIndependentlyOwned = true
			}
		}
	}
	if contentEncoding != "" {
		decodedBodyRaw, fixed := ContentEncodingDecode(contentEncoding, bodyRaw)
		if decodedBodyRaw != nil && fixed {
			// contents get decoded
			headerBytes = RemoveCEHeaders(headerBytes)
			bodyRaw = decodedBodyRaw
			bodyIndependentlyOwned = true
		}
	}

	if len(bodyRaw) == 0 {
		return rebuildFixedHTTPResponse(headerBytes, bodyRaw, cloneBody, bodyIndependentlyOwned, borrowPacket)
	}
	mimeResult, err := codec.MatchMIMEType(bodyRaw)
	if err != nil {
		log.Warnf("match mime type failed: %v", err)
		return rebuildFixedHTTPResponse(headerBytes, bodyRaw, cloneBody, bodyIndependentlyOwned, borrowPacket)
	}

	// 记录原始 contentType
	originContentType := contentType

	var bodyChanged bool
RetryContentType:
	switch {
	case IsTextPlainMIMEType(contentType):
		fallthrough
	case IsJsonMIMEType(contentType):
		fallthrough
	case IsJavaScriptMIMEType(contentType):
		bodyRaw, bodyChanged = mimeResult.TryUTF8Convertor(bodyRaw)
		if bodyChanged {
			if strings.Contains(strings.ToLower(originContentType), "charset=") {
				newContentType := charsetRegexp.ReplaceAllString(originContentType, "charset=utf-8")
				if strings.ToLower(newContentType) != strings.ToLower(originContentType) {
					log.Infof("auto fix content-type via utf convertor auto, from %#v to %#v", originContentType, newContentType)
					headerBytes = ReplaceMIMEType(headerBytes, newContentType)
				}
			}
		}
		return rebuildFixedHTTPResponse(headerBytes, bodyRaw, cloneBody, bodyIndependentlyOwned, borrowPacket)
	case IsHtmlOrXmlMIMEType(contentType):
		// body is not text, but content-type is ...
		// fix content-type header
		if !IsHtmlOrXmlMIMEType(mimeResult.MIMEType) && !IsTextPlainMIMEType(mimeResult.MIMEType) && !mimeResult.IsText && mimeResult.MIMEType != "application/octet-stream" {
			log.Warnf("origin content-type: %v(%v), fix new content-type: %v, reason: the actually body is not text...", contentType, originContentType, mimeResult.MIMEType)
			contentType = mimeResult.MIMEType
			goto RetryContentType
		}

		var after []byte
		var containsUTF8 bool
		after, bodyChanged = mimeResult.TryUTF8Convertor(bodyRaw)
		if bodyChanged {
			containsUTF8 = true
			log.Infof("HtmlOrXmlMIMEType(%#v) auto fix body, origin: len(%v) -> len(%v)", originContentType, len(bodyRaw), len(after))
			bodyRaw = after
		}

		var newContentType string
		origin, params, err := mime.ParseMediaType(contentType)
		if err != nil {
			newContentType = contentType
		} else {
			// 如果服务端返回了 charset，并且转换utf-8成功，就直接覆盖设置 charset=utf-8，否则使用服务端设置的 charset
			if containsUTF8 {
				params["charset"] = "utf-8"
			}
			newContentType = mime.FormatMediaType(origin, params)
		}
		headerBytes = ReplaceMIMEType(headerBytes, newContentType)
		return rebuildFixedHTTPResponse(headerBytes, bodyRaw, cloneBody, bodyIndependentlyOwned, borrowPacket)
	default:
		if mimeResult == nil || mimeResult.MIMEType == "" {
			return rebuildFixedHTTPResponse(headerBytes, bodyRaw, cloneBody, bodyIndependentlyOwned, borrowPacket)
		}

		if contentType == "" && noContentTypeSet {
			contentType = mimeResult.MIMEType
			goto RetryContentType
		}

		if strings.HasPrefix(strings.ToLower(contentType), "text/") {
			bodyRaw, bodyChanged = mimeResult.TryUTF8Convertor(bodyRaw)
			if bodyChanged {
				withoutCharset, _, err := mime.ParseMediaType(contentType)
				if err != nil {
					withoutCharset = contentType
				} else {
					contentType = withoutCharset
				}
				headerBytes = ReplaceMIMEType(headerBytes, mime.FormatMediaType(contentType, map[string]string{"charset": "utf-8"}))
			}
			return rebuildFixedHTTPResponse(headerBytes, bodyRaw, cloneBody, bodyIndependentlyOwned, borrowPacket)
		} else {
			if !mimeResult.IsText {
				headerBytes = ReplaceMIMEType(headerBytes, mimeResult.MIMEType)
				return rebuildFixedHTTPResponse(headerBytes, bodyRaw, cloneBody, bodyIndependentlyOwned, borrowPacket)
			}
			return rebuildFixedHTTPResponse(headerBytes, bodyRaw, cloneBody, bodyIndependentlyOwned, borrowPacket)
		}
	}
	//
	//// 取前几百个字节，来检测到底类型
	//var bodyPrescan []byte
	//if len(bodyRaw) > 200 {
	//	bodyPrescan = bodyRaw[:200]
	//} else {
	//	bodyPrescan = bodyRaw[:]
	//}
	//
	//mediaType, params, _ := mime.ParseMediaType(strings.ToLower(contentType))
	//mediaTypeLower := strings.ToLower(mediaType)
	//originCharSet, _ := params["charset"]
	//ctUTF8 := originCharSet == "utf-8" || originCharSet == "utf8"
	//isFile := false
	//isTextOrScript := strings.Contains(mediaTypeLower, "text/") || strings.Contains(mediaTypeLower, "script") || mediaType == ""
	//overrideContentType := ""
	//if contentType == "" || !filetype.IsMIME(bodyPrescan, contentType) {
	//	typ, err := filetype.Match(bodyPrescan)
	//	if err != nil {
	//		log.Debugf("detect bodyPrescan file-type failed: %v", err)
	//	} else if typ != types.Unknown {
	//		isFile = true
	//		if typ.MIME.Value != "" {
	//			overrideContentType = typ.MIME.Value
	//		}
	//	}
	//}
	//// 修复编码问题
	//if !isFile && !ctUTF8 && isTextOrScript && !utf8.Valid(bodyRaw) {
	//	var encodeHandler encoding.Encoding
	//	// 如果已经有 charset，就直接获取handler，否则尝试从 HTML 中解析
	//	if originCharSet != "" {
	//		encodeHandler, _ = charset.Lookup(strings.ToLower(originCharSet))
	//	} else {
	//		encodeHandler, originCharSet = charsetPrescan(bodyRaw)
	//	}
	//
	//	// 尝试判断是否是 GBK 编码
	//	if encodeHandler == nil && codec.IsGBK(bodyRaw) {
	//		encodeHandler, originCharSet = charset.Lookup("gbk")
	//	}
	//
	//	// 最后尝试使用基于 ICU 实现的算法与数据进行检测，返回置信度最高的编码
	//	if encodeHandler == nil {
	//		matchResult, err := codec.CharDetectBest(bodyRaw)
	//		if err != nil {
	//			log.Debugf("charset detect failed: %v", err)
	//		} else if matchResult != nil {
	//			encodeHandler, originCharSet = charset.Lookup(strings.ToLower(matchResult.Charset))
	//		}
	//	}
	//
	//	// 如果handler存在，就尝试解码
	//	if encodeHandler != nil {
	//		decoded, err := encodeHandler.NewDecoder().Bytes(bodyRaw)
	//		if err == nil && len(decoded) > 0 {
	//			bodyRaw = metaCharsetChanger(decoded)
	//			if params == nil {
	//				params = make(map[string]string)
	//			}
	//			params["charset"] = "utf-8"
	//			overrideContentType = mime.FormatMediaType(mediaTypeLower, params)
	//		}
	//	}
	//}
	//// 如果是文件，应该修复 content-type
	//if overrideContentType != "" {
	//	headerBytes = ReplaceMIMEType(headerBytes, overrideContentType)
	//}
	//
	//return ReplaceHTTPPacketBodyEx(headerBytes, bodyRaw, false, true), bodyRaw, nil
}

func ReplaceMIMEType(headerBytes []byte, mimeType string) []byte {
	if mimeType == "" {
		return headerBytes
	}

	idxs := contentTypeRegexp.FindSubmatchIndex(headerBytes)
	if len(idxs) > 3 {
		buf := bytes.NewBuffer(nil)
		buf.Write(headerBytes[:idxs[2]])
		buf.WriteString(mimeType)
		buf.Write(headerBytes[idxs[3]:])
		return buf.Bytes()
	} else {
		return AppendHeaderToHTTPPacket(headerBytes, "Content-Type: "+mimeType)
	}
}

func RemoveCEHeaders(headerBytes []byte) []byte {
	return contentEncodingRegexp.ReplaceAll(headerBytes, []byte{})
}

// ReplaceBody 将原始 HTTP 请求报文中的 body 替换为指定的 body，并指定是否为 chunked，返回新的 HTTP 请求报文
// 参数:
//   - raw: 原始 HTTP 报文字节数组
//   - body: 替换后的报文主体内容
//   - chunk: 是否使用 chunked 传输编码
//
// 返回值:
//   - 修改后的 HTTP 报文字节数组
//
// Example:
// ```
// poc.ReplaceBody(`POST / HTTP/1.1
// Host: example.com
// Content-Length: 11
//
// hello world`, "hello yak", false)
// ```
func ReplaceHTTPPacketBody(raw []byte, body []byte, chunk bool) (newHTTPRequest []byte) {
	return ReplaceHTTPPacketBodyEx(raw, body, chunk, false)
}

func ReplaceHTTPPacketBodyEx(raw []byte, body []byte, chunk bool, forceCL bool) []byte {
	return replaceHTTPPacketBodyEx(raw, body, chunk, forceCL, false)
}

func replaceHTTPPacketBodyExOwned(raw []byte, body []byte, chunk bool, forceCL bool) []byte {
	return replaceHTTPPacketBodyEx(raw, body, chunk, forceCL, true)
}

func replaceHTTPPacketBodyEx(raw []byte, body []byte, chunk bool, forceCL bool, bodyIndependentlyOwned bool) []byte {
	return replaceHTTPPacketBodyExWithBorrow(raw, body, chunk, forceCL, bodyIndependentlyOwned, nil)
}

func replaceHTTPPacketBodyExWithBorrow(
	raw []byte,
	body []byte,
	chunk bool,
	forceCL bool,
	bodyIndependentlyOwned bool,
	borrowPacket []byte,
) []byte {
	isChunked := false
	contentLengthCount := 0
	contentLength := -1
	var firstLine string
	var headers []string
	_, _ = SplitHTTPPacketEx(raw, nil, nil, func(rawLine string) error {
		firstLine = rawLine
		return nil
	}, func(line string) string {
		if utils.IHasPrefix(line, "transfer-encoding:") && utils.IContains(line, "chunked") {
			isChunked = true
			return line
		}

		if utils.IHasPrefix(line, "content-length") {
			key, value := SplitHTTPHeader(line)
			if !strings.EqualFold(strings.TrimSpace(key), "content-length") {
				contentLengthCount = 2
				return line
			}
			contentLengthCount++
			parsed, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || parsed < 0 {
				contentLengthCount = 2
				return line
			}
			contentLength = parsed
			return line
		}
		headers = append(headers, line)
		return line
	})
	if packetCanBorrowValidResponse(
		borrowPacket,
		raw,
		body,
		bodyIndependentlyOwned,
		forceCL,
		isChunked,
		contentLengthCount,
		contentLength,
	) {
		return borrowPacket
	}
	headers = append([]string{firstLine}, headers...)
	var buf bytes.Buffer
	// 空 body
	if body == nil {
		header := strings.Join(headers, CRLF) + CRLF + CRLF
		if packetMatchesBorrowedParts(borrowPacket, header, nil, bodyIndependentlyOwned) {
			return borrowPacket
		}
		buf.WriteString(header)
		return buf.Bytes()
	}

	// 只有包含了Transfer-Encoding: chunked，以及body符合chunked格式，才认为已经是chunked
	if isChunked {
		isChunked = bytes.Contains(body, isChunkedBytes)
	}

	// chunked
	if chunk {
		headers = append(headers, "Transfer-Encoding: chunked")
		if !isChunked {
			body = codec.HTTPChunkedEncode(body)
			bodyIndependentlyOwned = true
		}
	} else if isChunked {
		newBody, err := codec.HTTPChunkedDecode(body)
		if err == nil {
			body = newBody
			bodyIndependentlyOwned = true
		}
	}
	if !chunk && (len(body) > 0 || forceCL) {
		headers = append(headers, fmt.Sprintf("Content-Length: %d", len(body)))
	}
	header := strings.Join(headers, CRLF) + CRLF + CRLF
	if bodyIndependentlyOwned && len(body) > 0 && len(header) <= cap(body)-len(body) {
		bodyLength := len(body)
		packet := body[:bodyLength+len(header)]
		copy(packet[len(header):], packet[:bodyLength])
		copy(packet, header)
		return packet
	}
	if packetMatchesBorrowedParts(borrowPacket, header, body, bodyIndependentlyOwned) {
		return borrowPacket
	}
	buf.WriteString(header)
	buf.Write(body)
	return buf.Bytes()
}

func packetCanBorrowValidResponse(
	packet []byte,
	header []byte,
	body []byte,
	bodyIndependentlyOwned bool,
	forceCL bool,
	isChunked bool,
	contentLengthCount int,
	contentLength int,
) bool {
	if bodyIndependentlyOwned || len(packet) == 0 || isChunked {
		return false
	}
	packetHeader, packetBody := SplitHTTPHeadersAndBodyFromPacketView(packet)
	if packetHeader != utils.UnsafeBytesToString(header) ||
		len(packetBody) != len(body) ||
		(len(body) > 0 && &packetBody[0] != &body[0]) {
		return false
	}
	if len(body) > 0 || forceCL {
		return contentLengthCount == 1 && contentLength == len(body)
	}
	return contentLengthCount == 0
}

func packetMatchesBorrowedParts(packet []byte, header string, body []byte, bodyIndependentlyOwned bool) bool {
	if bodyIndependentlyOwned ||
		len(packet) == 0 ||
		len(header) > len(packet) ||
		len(header)+len(body) != len(packet) ||
		header != utils.UnsafeBytesToString(packet[:len(header)]) {
		return false
	}
	return len(body) == 0 || &body[0] == &packet[len(header)]
}

func ReplaceHTTPPacketBodyRaw(raw []byte, body []byte, fixCL bool) []byte {
	// 移除左边空白字符
	var firstLine string
	var headers []string
	var hasChunkHeader bool
	var contentLengthLine = -1
	_, _ = SplitHTTPPacketEx(raw, nil, nil, func(rawLine string) error {
		firstLine = rawLine
		return nil
	}, func(line string) string {
		if utils.IHasPrefix(line, "transfer-encoding:") && utils.IContains(line, "chunked") {
			hasChunkHeader = true
		}
		if utils.IHasPrefix(line, "content-length") {
			contentLengthLine = len(headers)
		}
		headers = append(headers, line)
		return line
	})
	headers = append([]string{firstLine}, headers...)
	var buf bytes.Buffer

	// 空 body
	if body == nil {
		buf.WriteString(strings.Join(headers, CRLF) + CRLF + CRLF)
		return buf.Bytes()
	}

	// fix CL and is CL
	if fixCL && !hasChunkHeader && contentLengthLine > -1 {
		// fix index append first line
		headers[contentLengthLine+1] = fmt.Sprintf("Content-Length: %d", len(body))
	}

	buf.WriteString(strings.Join(headers, CRLF))
	buf.WriteString(CRLF + CRLF)
	buf.Write(body)
	return buf.Bytes()
}

// ParseBytesToHTTPResponse 将字节数组解析为 HTTP 响应
// 参数:
//   - res: 原始 HTTP 响应报文字节数组
//
// 返回值:
//   - 解析得到的 HTTP 响应对象
//   - 错误信息，解析失败时返回非空
//
// Example:
// ```
// res, err := str.ParseBytesToHTTPResponse(b"HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
// ```
func ParseBytesToHTTPResponse(res []byte) (rspInst *http.Response, err error) {
	if len(res) <= 0 {
		return nil, utils.Errorf("empty http response")
	}
	rsp, err := utils.ReadHTTPResponseFromBytes(res, nil)
	if err != nil {
		return nil, err
	}
	return rsp, nil
}
