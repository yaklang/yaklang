package lowhttp

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/h2non/filetype"
	"github.com/yaklang/yaklang/common/log"
	utils "github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding"
)

var charsetRegexp = regexp.MustCompile(`(?i)charset\s*=\s*"?\s*([^\s;\n\r"]+)`)
var metaCharsetRegexp = regexp.MustCompile(`(?i)meta[^<>]*?charset\s*=\s*['"]?\s*([^\s;\n\r'"]+)`)
var mimeCharsetRegexp = regexp.MustCompile(`(?i)content-type:\s*[^\n]*charset\s*=\s*['"]?\s*([^\s;\n\r'"]+)`)
var contentTypeRegexp = regexp.MustCompile(`(?i)content-type:\s*([^\r\n]*)`)
var contentEncodingRegexp = regexp.MustCompile(`(?i)content-encoding:\s*\w*\r?\n`)

//var contentLengthRegexpCase = regexp.MustCompile(`(?i)(content-length:\s*\w*\d+\r?\n)`)

func metaCharsetChanger(raw []byte) []byte {
	if len(raw) <= 0 {
		return raw
	}
	// 这里很关键，需要移除匹配到的内容
	var buf = bytes.NewBuffer(nil)
	var slash [][2]int
	var lastEnd = 0
	for _, va := range metaCharsetRegexp.FindAllSubmatchIndex(raw, -1) {
		if len(va) > 3 {
			slash = append(slash, [2]int{lastEnd, va[2]})
			lastEnd = va[3]
		}
	}
	slash = append(slash, [2]int{lastEnd, len(raw)})
	for _, slashIndex := range slash {
		buf.Write(raw[slashIndex[0]:slashIndex[1]])
		if slashIndex[1] < len(raw) {
			buf.WriteString("utf-8")
		}
	}
	return buf.Bytes()
}

func CharsetToUTF8(bodyRaw []byte, mimeType string, originCharset string) ([]byte, string) {
	if len(bodyRaw) <= 0 {
		return bodyRaw, mimeType
	}

	originMT, kv, _ := mime.ParseMediaType(mimeType)
	newKV := make(map[string]string)
	for k, v := range kv {
		newKV[k] = v
	}

	originMTLower := strings.ToLower(originMT)
	var checkingGB18030 bool
	if ret := strings.HasPrefix(originMTLower, "text/"); ret || strings.Contains(originMTLower, "script") {
		newKV["charset"] = "utf-8"
		checkingGB18030 = ret
	}
	fixedMIME := mime.FormatMediaType(originMT, newKV)
	if fixedMIME != "" {
		mimeType = fixedMIME
	}

	var handledChineseEncoding bool
	var parseFromMIME = func() ([]byte, error) {
		if kv != nil && len(kv) > 0 {
			if charsetStr, ok := kv["charset"]; ok && !handledChineseEncoding {
				encodingIns, name := charset.Lookup(strings.ToLower(charsetStr))
				if encodingIns != nil {
					raw, err := encodingIns.NewDecoder().Bytes(bodyRaw)
					if err != nil {
						return nil, utils.Errorf("decode [%s] from mime type failed: %s", name, err)
					}
					if len(raw) > 0 {
						return raw, nil
					}
				}
			}
		}
		return nil, utils.Errorf("cannot detect charset from mime")
	}
	var encodeHandler encoding.Encoding
	switch originCharset {
	case "gbk", "gb18030":
		// 如果无法检测编码，就看看18030是不是符合
		replaced, _ := codec.GB18030ToUtf8(bodyRaw)
		if replaced != nil {
			handledChineseEncoding = true
			bodyRaw = replaced
		}
	default:
		encodeHandler, _ = charsetPrescan(bodyRaw)
		if encodeHandler == nil && checkingGB18030 && !utf8.Valid(bodyRaw) {
			// 如果无法检测编码，就看看18030是不是符合
			replaced, _ := codec.GB18030ToUtf8(bodyRaw)
			if replaced != nil {
				handledChineseEncoding = true
				bodyRaw = replaced
			}
		}
	}

	raw, _ := parseFromMIME()
	if len(raw) > 0 {
		idxs := charsetRegexp.FindStringSubmatchIndex(mimeType)
		if len(idxs) > 3 {
			start, end := idxs[2], idxs[3]
			prefix, suffix := mimeType[:start], mimeType[end:]
			if encodeHandler != nil {
				raw = metaCharsetChanger(raw)
			}
			return raw, fmt.Sprintf("%v%v%v", prefix, "utf-8", suffix)
		}
		return raw, mimeType
	}

	if encodeHandler != nil {
		raw, err := encodeHandler.NewDecoder().Bytes(bodyRaw)
		if err != nil {
			return bodyRaw, mimeType
		}
		if len(raw) <= 0 {
			return bodyRaw, mimeType
		}
		return metaCharsetChanger(raw), mimeType
	}

	return bodyRaw, mimeType
}

func GetOverrideContentType(bodyPrescan []byte, contentType string) (overrideContentType string, originCharset string) {
	defer func() {
		if err := recover(); err != nil {
		}
	}()
	if strings.Contains(strings.ToLower(contentType), "charset") {
		if _, params, _ := mime.ParseMediaType(strings.ToLower(contentType)); params != nil {
			var ok bool
			originCharset, ok = params["charset"]
			_ = ok
			_ = originCharset
		}
	}
	if bodyPrescan != nil && contentType != "" && !filetype.IsMIME(bodyPrescan, contentType) {
		actuallyMIME, err := filetype.Match(bodyPrescan)
		if err != nil {
			log.Debugf("detect bodyPrescan type failed: %v", err)
		}

		if actuallyMIME.MIME.Value != "" {
			log.Debugf("really content-type met: %s, origin: %v", actuallyMIME.MIME.Value, contentType)
			overrideContentType = actuallyMIME.MIME.Value
		}
	}
	return overrideContentType, originCharset
}

// FixHTTPResponse 尝试对传入的响应进行修复，并返回修复后的响应，响应体和错误
// Example:
// ```
// fixedResponse, body, err = str.FixHTTPResponse(b"HTTP/1.1 200 OK\r\nContent-Type: text/html; charset=gbk\r\n\r\n<html>你好</html>")
// ```
func FixHTTPResponse(raw []byte) (rsp []byte, body []byte, _ error) {
	// log.Infof("response raw: \n%v", codec.EncodeBase64(raw))

	var isChunked = false
	// 这两个用来处理编码特殊情况
	var contentEncoding string
	var contentType string
	headers, body := SplitHTTPHeadersAndBodyFromPacket(raw, func(line string) {
		if strings.HasPrefix(strings.ToLower(line), "content-type:") {
			_, contentType = SplitHTTPHeader(line)
		}
		// 判断内容
		line = strings.ToLower(line)
		if strings.HasPrefix(line, "transfer-encoding:") && utils.IContains(line, "chunked") {
			isChunked = true
		}
		if strings.HasPrefix(line, "content-encoding:") {
			contentEncoding = line
		}
	})
	if headers == "" {
		return nil, nil, utils.Errorf("error for parsing http response")
	}
	headerBytes := []byte(headers)

	bodyRaw := body
	if bodyRaw != nil && isChunked {
		unchunked, _ := codec.HTTPChunkedDecode(bodyRaw)
		if unchunked != nil {
			bodyRaw = unchunked
		}
	}
	if contentEncoding != "" {
		decodedBodyRaw, fixed := ContentEncodingDecode(contentEncoding, bodyRaw)
		if decodedBodyRaw != nil && fixed {
			// contents get decoded
			headerBytes = RemoveCEHeaders(headerBytes)
			bodyRaw = decodedBodyRaw
		}
	}

	// 如果 bodyRaw 是图片的话，则不处理，如何判断是图片？
	var skipped = false
	if len(bodyRaw) > 0 {
		if utils.IsImage(bodyRaw) {
			skipped = true
		}
	}

	// 取前几百个字节，来检测到底类型
	var bodyPrescan []byte
	if len(bodyRaw) > 200 {
		bodyPrescan = bodyRaw[:200]
	} else {
		bodyPrescan = bodyRaw[:]
	}
	var overrideContentType, originCharset = GetOverrideContentType(bodyPrescan, contentType)
	/*originCharset is lower!!!*/
	_ = originCharset

	// 都解开了，来处理编码问题
	if bodyRaw != nil && !skipped {
		var mimeType string
		_, params, _ := mime.ParseMediaType(contentType)
		var ctUTF8 = false
		if raw, ok := params["charset"]; ok {
			raw = strings.ToLower(raw)
			ctUTF8 = raw == "utf-8" || raw == "utf8"
		}

		if overrideContentType == "" {
			// 如果类型一致，不需要替换，那么还是只处理 content-type 和编码问题
			bodyRaw, mimeType = CharsetToUTF8(bodyRaw, contentType, originCharset)
			if mimeType != contentType {
				headerBytes = ReplaceMIMEType(headerBytes, mimeType)
			}
			// 是 Js，但是不包含 UTF8，按理说应该给他加成 UTF8
			if utils.IContains(mimeType, "javascript") && !ctUTF8 && len(bodyRaw) > 0 {
				// 这个顺序千万不要弄错了喔，一定要先判断是不是 UTF8，再去判断中文编码
				if !codec.IsUtf8(bodyRaw) {
					if codec.IsGBK(bodyRaw) {
						decoded, err := codec.GbkToUtf8(bodyRaw)
						if err == nil && len(decoded) > 0 {
							bodyRaw = decoded
						}
					} else {
						matchResult, _ := codec.CharDetectBest(bodyRaw)
						if matchResult != nil {
							switch strings.ToLower(matchResult.Charset) {
							case "gbk", "gb2312", "gb-2312", "gb18030", "windows-1252", "gb-18030", "windows1252":
								decoded, err := codec.GB18030ToUtf8(bodyRaw)
								if err == nil && len(decoded) > 0 {
									bodyRaw = decoded
								}
							}
						}
					}
				}

			}
		} else {
			log.Infof("replace content-type to: %s", overrideContentType)
			headerBytes = ReplaceMIMEType(headerBytes, overrideContentType)
		}
	}

	return ReplaceHTTPPacketBodyEx(headerBytes, bodyRaw, false, true), bodyRaw, nil
}

func ReplaceMIMEType(headerBytes []byte, mimeType string) []byte {
	if mimeType == "" {
		return headerBytes
	}

	var idxs = contentTypeRegexp.FindSubmatchIndex(headerBytes)
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
	// 移除左边空白字符
	raw = TrimLeftHTTPPacket(raw)
	reader := bufio.NewReader(bytes.NewBuffer(raw))
	firstLineBytes, err := utils.BufioReadLine(reader)
	if err != nil {
		return raw
	}

	var headers = []string{
		string(firstLineBytes),
	}

	// 接下来解析各种 Header
	for {
		lineBytes, err := utils.BufioReadLine(reader)
		if err != nil && err != io.EOF {
			break
		}
		line := string(lineBytes)
		line = strings.TrimSpace(line)

		// Header 解析完毕
		if line == "" {
			break
		}

		lineLower := strings.ToLower(line)
		// 移除 chunked
		if strings.HasPrefix(lineLower, "transfer-encoding:") && utils.IContains(line, "chunked") {
			continue
		}

		//if strings.HasPrefix(lineLower, "content-encoding:") {
		//	headers = append(headers, fmt.Sprintf("Content-Encoding: %v", "identity"))
		//	continue
		//}

		// 设置 content-length
		if strings.HasPrefix(lineLower, "content-length") {
			continue
		}
		headers = append(headers, line)
	}

	// 空 body
	if body == nil {
		raw := strings.Join(headers, CRLF) + CRLF + CRLF
		return []byte(raw)
	}

	// chunked
	if chunk {
		headers = append(headers, "Transfer-Encoding: chunked")
		body = codec.HTTPChunkedEncode(body)
		buf := bytes.NewBuffer(nil)
		for _, header := range headers {
			buf.WriteString(header)
			buf.WriteString(CRLF)
		}
		buf.WriteString(CRLF)
		buf.Write(body)
		return buf.Bytes()
	}

	if len(body) > 0 || forceCL {
		headers = append(headers, fmt.Sprintf("Content-Length: %d", len(body)))
	}
	var buf = new(bytes.Buffer)
	for _, header := range headers {
		buf.WriteString(header)
		buf.WriteString(CRLF)
	}
	buf.WriteString(CRLF)
	buf.Write(body)
	return buf.Bytes()
}

// ParseBytesToHTTPResponse 将字节数组解析为 HTTP 响应
// Example:
// ```
// res, err := str.ParseBytesToHTTPResponse(b"HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
// ```
func ParseBytesToHTTPResponse(res []byte) (*http.Response, error) {
	if len(res) <= 0 {
		return nil, utils.Errorf("empty http response")
	}
	rsp, err := utils.ReadHTTPResponseFromBytes(res, nil)
	if err != nil {
		return nil, err
	}
	return rsp, nil
}
