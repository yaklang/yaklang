package lowhttp

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"strings"
	"unsafe"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

func escapeQuotes(s string) string {
	return quoteEscaper.Replace(s)
}

func IsHeader(headerLine, wantHeader string) bool {
	return strings.HasPrefix(strings.ToLower(headerLine), strings.ToLower(wantHeader)+":")
}

func IsChunkedHeaderLine(line string) bool {
	line = strings.ToLower(line)
	if strings.HasPrefix(line, "transfer-encoding: chunked") {
		return true
	}
	if line == "chunked" {
		return true
	}
	k, v := SplitHTTPHeader(line)
	if k == "transfer-encoding" && v == "chunked" {
		return true
	}
	return false
}

func ReplaceHTTPPacketFirstLine(packet []byte, firstLine string) []byte {
	var isChunked bool
	var header = []string{firstLine}
	_, body := SplitHTTPPacket(packet, nil, nil, func(line string) string {
		if !isChunked {
			isChunked = IsChunkedHeaderLine(line)
		}
		header = append(header, line)
		return line
	})
	return ReplaceHTTPPacketBody([]byte(strings.Join(header, CRLF)+CRLF), body, isChunked)
}

func ReplaceHTTPPacketMethod(packet []byte, newMethod string) []byte {
	var buf bytes.Buffer
	var header []string
	var (
		isChunked      = false
		isPost         = strings.ToUpper(newMethod) == "POST"
		hasContentType = false
	)

	_, body := SplitHTTPPacket(packet,
		func(method string, requestUri string, proto string) error {
			method = newMethod
			buf.WriteString(method + " " + requestUri + " " + proto)
			buf.WriteString(CRLF)

			return nil
		},
		nil,
		func(line string) string {
			if !isChunked {
				isChunked = IsChunkedHeaderLine(line)
			}
			header = append(header, line)
			if IsHeader(line, "Content-Type") {
				hasContentType = true
			}

			return line
		},
	)

	// fix content-type
	if isPost && !hasContentType {
		header = append(header, "Content-Type: application/x-www-form-urlencoded")
	}

	for _, line := range header {
		buf.WriteString(line)
		buf.WriteString(CRLF)
	}
	return ReplaceHTTPPacketBody(buf.Bytes(), body, isChunked)
}

func ReplaceHTTPPacketPath(packet []byte, p string) []byte {
	var isChunked bool
	var buf bytes.Buffer
	var header []string

	_, body := SplitHTTPPacket(packet,
		func(method string, requestUri string, proto string) error {
			defer func() {
				buf.WriteString(method + " " + requestUri + " " + proto)
				buf.WriteString(CRLF)
			}()

			// handle requestUri
			u, _ := url.Parse(requestUri)
			if u == nil { // invalid url
				return nil
			}

			if !strings.HasPrefix(p, "/") {
				p = "/" + p
			}
			u.Path = p
			requestUri = u.String()

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

func AppendHTTPPacketPath(packet []byte, p string) []byte {
	var isChunked bool
	var buf bytes.Buffer
	var header []string

	_, body := SplitHTTPPacket(packet,
		func(method string, requestUri string, proto string) error {
			defer func() {
				buf.WriteString(method + " " + requestUri + " " + proto)
				buf.WriteString(CRLF)
			}()

			// handle requestUri
			u, _ := url.Parse(requestUri)
			if u == nil { // invalid url
				return nil
			}

			u.Path = path.Join(u.Path, p)
			requestUri = u.String()

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

func handleHTTPPacketQueryParam(packet []byte, callback func(url.Values)) []byte {
	var isChunked bool
	var buf bytes.Buffer
	var header []string

	_, body := SplitHTTPPacket(packet,
		func(method string, requestUri string, proto string) error {
			defer func() {
				buf.WriteString(method + " " + requestUri + " " + proto)
				buf.WriteString(CRLF)
			}()

			// handle requestUri
			u, _ := url.Parse(requestUri)
			if u == nil { // invalid url
				return nil
			}
			q := u.Query()
			callback(q)
			u.RawQuery = q.Encode()
			requestUri = u.String()

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

func ReplaceAllHTTPPacketQueryParams(packet []byte, values map[string]string) []byte {
	return handleHTTPPacketQueryParam(packet, func(q url.Values) {
		// clear all values
		for k := range q {
			q.Del(k)
		}

		for k, v := range values {

			q.Set(k, v)
		}
	})
}

func ReplaceHTTPPacketQueryParam(packet []byte, key, value string) []byte {
	return handleHTTPPacketQueryParam(packet, func(q url.Values) {
		q.Set(key, value)
	})
}

func AppendHTTPPacketQueryParam(packet []byte, key, value string) []byte {
	return handleHTTPPacketQueryParam(packet, func(q url.Values) {
		q.Add(key, value)
	})
}

func DeleteHTTPPacketQueryParam(packet []byte, key string) []byte {
	return handleHTTPPacketQueryParam(packet, func(q url.Values) {
		q.Del(key)
	})
}

func handleHTTPPacketPostParam(packet []byte, callback func(url.Values)) []byte {
	var isChunked bool

	headersRaw, bodyRaw := SplitHTTPPacket(packet, nil, nil)
	bodyString := unsafe.String(unsafe.SliceData(bodyRaw), len(bodyRaw))
	values, err := url.ParseQuery(bodyString)
	if err == nil {
		callback(values)
		// values.Set(key, value)
		newBody := values.Encode()
		bodyRaw = unsafe.Slice(unsafe.StringData(newBody), len(newBody))
	}

	return ReplaceHTTPPacketBody([]byte(headersRaw), bodyRaw, isChunked)
}

func ReplaceAllHTTPPacketPostParams(packet []byte, values map[string]string) []byte {
	return handleHTTPPacketPostParam(packet, func(q url.Values) {
		// clear all values
		for k := range q {
			q.Del(k)
		}

		for k, v := range values {
			q.Set(k, v)
		}
	})
}

func ReplaceHTTPPacketPostParam(packet []byte, key, value string) []byte {
	return handleHTTPPacketPostParam(packet, func(q url.Values) {
		q.Set(key, value)
	})
}

func AppendHTTPPacketPostParam(packet []byte, key, value string) []byte {
	return handleHTTPPacketPostParam(packet, func(q url.Values) {
		q.Add(key, value)
	})
}

func DeleteHTTPPacketPostParam(packet []byte, key string) []byte {
	return handleHTTPPacketPostParam(packet, func(q url.Values) {
		q.Del(key)
	})
}

func RemoveHTTPPacketHeader(packet []byte, headerKey ...string) []byte {
	var firstLine string
	var header []string
	var isChunked = false
	var removeGzip = false
	var removeChunk = false
	_, body := SplitHTTPPacket(packet, func(method string, requestUri string, proto string) error {
		firstLine = method + " " + requestUri + " " + proto
		return nil
	}, func(proto string, code int, codeMsg string) error {
		if codeMsg == "" {
			firstLine = proto + " " + fmt.Sprint(code)
		} else {
			firstLine = proto + " " + fmt.Sprint(code) + " " + codeMsg
		}
		return nil
	}, func(line string) string {
		if !isChunked {
			isChunked = IsChunkedHeaderLine(line)
		}

		if k, v := SplitHTTPHeader(line); utils.StringArrayContains(headerKey, k) {
			if strings.ToLower(k) == "content-encoding" && strings.ToLower(v) == "gzip" {
				removeGzip = true
			} else if strings.ToLower(k) == "transfer-encoding" && strings.ToLower(v) == "chunked" {
				removeChunk = true
			}
			return line
		}
		header = append(header, line)
		return line
	})

	if removeGzip {
		var bodyDecoded, _ = utils.GzipDeCompress(body)
		if len(bodyDecoded) > 0 {
			body = bodyDecoded
		}
	}

	var buf bytes.Buffer
	buf.WriteString(firstLine)
	buf.WriteString(CRLF)
	for _, line := range header {
		buf.WriteString(line)
		buf.WriteString(CRLF)
	}
	return ReplaceHTTPPacketBody(buf.Bytes(), body, isChunked && !removeChunk)
}

func ReplaceHTTPPacketHeader(packet []byte, headerKey string, headerValue any) []byte {
	var firstLine string
	var header []string
	var handled bool
	var isChunked = IsChunkedHeaderLine(headerKey + ": " + utils.InterfaceToString(headerValue))
	_, body := SplitHTTPPacket(packet, func(method string, requestUri string, proto string) error {
		firstLine = method + " " + requestUri + " " + proto
		return nil
	}, func(proto string, code int, codeMsg string) error {
		if codeMsg == "" {
			firstLine = proto + " " + fmt.Sprint(code)
		} else {
			firstLine = proto + " " + fmt.Sprint(code) + " " + codeMsg
		}
		return nil
	}, func(line string) string {
		if !isChunked {
			isChunked = IsChunkedHeaderLine(line)
		}
		if k, _ := SplitHTTPHeader(line); k == headerKey {
			handled = true
			header = append(header, headerKey+": "+utils.InterfaceToString(headerValue))
			return line
		}
		header = append(header, line)
		return line
	})
	if !handled {
		header = append(header, headerKey+": "+utils.InterfaceToString(headerValue))
	}
	var buf bytes.Buffer
	buf.WriteString(firstLine)
	buf.WriteString(CRLF)
	for _, line := range header {
		buf.WriteString(line)
		buf.WriteString(CRLF)
	}
	return ReplaceHTTPPacketBody(buf.Bytes(), body, isChunked)
}

func ReplaceHTTPPacketHost(packet []byte, host string) []byte {
	return ReplaceHTTPPacketHeader(packet, "Host", host)
}

func ReplaceHTTPPacketBasicAuth(packet []byte, username, password string) []byte {
	return ReplaceHTTPPacketHeader(packet, "Authorization", "Basic "+codec.EncodeBase64(username+":"+password))
}

func AppendHTTPPacketHeader(packet []byte, headerKey string, headerValue any) []byte {
	var firstLine string
	var header []string
	var isChunked bool
	_, body := SplitHTTPPacket(packet, func(method string, requestUri string, proto string) error {
		firstLine = method + " " + requestUri + " " + proto
		return nil
	}, func(proto string, code int, codeMsg string) error {
		if codeMsg == "" {
			firstLine = proto + " " + fmt.Sprint(code)
		} else {
			firstLine = proto + " " + fmt.Sprint(code) + " " + codeMsg
		}
		return nil
	}, func(line string) string {
		if !isChunked {
			isChunked = IsChunkedHeaderLine(line)
		}
		header = append(header, line)
		return line
	})
	header = append(header, headerKey+": "+utils.InterfaceToString(headerValue))
	var buf bytes.Buffer
	buf.WriteString(firstLine)
	buf.WriteString(CRLF)
	buf.WriteString(strings.Join(header, CRLF))
	return ReplaceHTTPPacketBody(buf.Bytes(), body, isChunked)
}

func DeleteHTTPPacketHeader(packet []byte, headerKey string) []byte {
	var firstLine string
	var header []string
	var isChunked bool
	_, body := SplitHTTPPacket(packet, func(method string, requestUri string, proto string) error {
		firstLine = method + " " + requestUri + " " + proto
		return nil
	}, func(proto string, code int, codeMsg string) error {
		if codeMsg == "" {
			firstLine = proto + " " + fmt.Sprint(code)
		} else {
			firstLine = proto + " " + fmt.Sprint(code) + " " + codeMsg
		}
		return nil
	}, func(line string) string {
		if !isChunked {
			isChunked = IsChunkedHeaderLine(line)
		}

		if k, _ := SplitHTTPHeader(line); k == headerKey {
			return ""
		}
		header = append(header, line)
		return line
	})
	var buf bytes.Buffer
	buf.WriteString(firstLine)
	buf.WriteString(CRLF)
	buf.WriteString(strings.Join(header, CRLF))
	return ReplaceHTTPPacketBody(buf.Bytes(), body, false)
}

func ReplaceHTTPPacketCookie(packet []byte, key string, value any) []byte {
	var isReq bool
	var isRsp bool
	var handled = false
	var isChunked bool
	header, body := SplitHTTPPacket(packet, func(method string, requestUri string, proto string) error {
		isReq = true
		return nil
	}, func(proto string, code int, codeMsg string) error {
		isRsp = true
		return nil
	}, func(line string) string {
		if !isChunked {
			isChunked = IsChunkedHeaderLine(line)
		}

		if !isReq && !isRsp {
			return line
		}

		k, cookieRaw := SplitHTTPHeader(line)
		if (strings.ToLower(k) == "cookie" && isReq) || (strings.ToLower(k) == "set-cookie" && isRsp) {
			existed := ParseCookie(cookieRaw)
			if len(existed) <= 0 {
				return line
			}
			var cookie = make([]*http.Cookie, len(existed))
			for index, c := range existed {
				if c.Name == key {
					handled = true
					c.Value = utils.InterfaceToString(value)
				}
				cookie[index] = c
			}
			return k + ": " + CookiesToString(cookie)
		}
		return line
	})
	var data = ReplaceHTTPPacketBody([]byte(header), body, isChunked)
	if handled {
		return data
	}
	return AppendHTTPPacketCookie(data, key, value)
}

func AppendHTTPPacketCookie(packet []byte, key string, value any) []byte {
	var isReq bool
	var added bool
	var isRsp bool
	var isChunked bool
	header, body := SplitHTTPPacket(packet, func(method string, requestUri string, proto string) error {
		isReq = true
		return nil
	}, func(proto string, code int, codeMsg string) error {
		isRsp = true
		return nil
	}, func(line string) string {
		if !isChunked {
			isChunked = IsChunkedHeaderLine(line)
		}

		if !isReq && !isRsp {
			return line
		}

		if added {
			return line
		}

		k, cookieRaw := SplitHTTPHeader(line)
		k = strings.ToLower(k)
		if (k == "cookie" && isReq) || (k == "set-cookie" && isRsp) {
			existed := ParseCookie(cookieRaw)
			existed = append(existed, &http.Cookie{Name: key, Value: utils.InterfaceToString(value)})
			added = true
			return k + ": " + CookiesToString(existed)
		}

		return line
	})
	if !added {
		if isReq {
			header = strings.Trim(header, CRLF) + CRLF + "Cookie: " + CookiesToString([]*http.Cookie{
				{Name: key, Value: utils.InterfaceToString(value)},
			})
		}
		if isRsp {
			header = strings.Trim(header, CRLF) + CRLF + "Set-Cookie: " + CookiesToString([]*http.Cookie{
				{Name: key, Value: utils.InterfaceToString(value)},
			})
		}
	}
	return ReplaceHTTPPacketBody([]byte(header), body, isChunked)
}

func DeleteHTTPPacketCookie(packet []byte, key string) []byte {
	var isReq bool
	var isRsp bool
	var isChunked bool
	header, body := SplitHTTPPacket(packet, func(method string, requestUri string, proto string) error {
		isReq = true
		return nil
	}, func(proto string, code int, codeMsg string) error {
		isRsp = true
		return nil
	}, func(line string) string {
		if !isChunked {
			isChunked = IsChunkedHeaderLine(line)
		}
		if !isReq && !isRsp {
			return line
		}

		k, cookieRaw := SplitHTTPHeader(line)
		k = strings.ToLower(k)

		if (k == "cookie" && isReq) || (k == "set-cookie" && isRsp) {
			existed := ParseCookie(cookieRaw)
			existed = funk.Filter(existed, func(cookie *http.Cookie) bool {
				return cookie.Name != key
			}).([]*http.Cookie)
			return k + ": " + CookiesToString(existed)
		}

		return line
	})
	return ReplaceHTTPPacketBody([]byte(header), body, isChunked)
}

func handleHTTPRequestForm(packet []byte, fixMethod bool, fixContentType bool, callback func(string, *multipart.Reader, *multipart.Writer) bool) []byte {
	var header []string
	var (
		buf             bytes.Buffer
		bodyBuf         bytes.Buffer
		requestMethod                     = ""
		hasContentType                    = false
		isChunked                         = false
		isFormDataPost                    = false
		fixBody                           = false
		multipartWriter *multipart.Writer = multipart.NewWriter(&bodyBuf)
	)
	// not handle response
	if bytes.HasPrefix(packet, []byte("HTTP/")) {
		return packet
	}

	_, body := SplitHTTPPacket(packet,
		func(method string, requestUri string, proto string) error {
			requestMethod = method
			// rewrite method
			if fixMethod {
				method = "POST"
			}

			buf.WriteString(method + " " + requestUri + " " + proto)
			buf.WriteString(CRLF)

			return nil
		},
		nil,
		func(line string) string {
			if !isChunked {
				isChunked = IsChunkedHeaderLine(line)
			}
			if IsHeader(line, "Content-Type") {
				hasContentType = true
				_, v := SplitHTTPHeader(line)
				d, params, _ := mime.ParseMediaType(v)

				if d == "multipart/form-data" {
					isFormDataPost = true
					// try to get boundary
					if boundary, ok := params["boundary"]; ok {
						multipartWriter.SetBoundary(boundary)
					}
				} else if fixContentType {
					// rewrite content-type
					line = multipartWriter.FormDataContentType()
				}
			}
			header = append(header, line)
			return line
		},
	)

	if isFormDataPost {
		// multipart reader
		multipartReader := multipart.NewReader(bytes.NewReader(body), multipartWriter.Boundary())
		// append form
		fixBody = callback(requestMethod, multipartReader, multipartWriter)
	} else {
		// rewrite body
		fixBody = callback(requestMethod, nil, multipartWriter)
	}
	multipartWriter.Close()
	if fixBody {
		body = bodyBuf.Bytes()
	}

	if fixContentType && !hasContentType {
		header = append(header, multipartWriter.FormDataContentType())
	}

	for _, line := range header {
		buf.WriteString(line)
		buf.WriteString(CRLF)
	}
	return ReplaceHTTPPacketBody(buf.Bytes(), body, isChunked)
}

func AppendHTTPPacketFormEncoded(packet []byte, key, value string) []byte {
	return handleHTTPRequestForm(packet, true, true, func(_ string, multipartReader *multipart.Reader, multipartWriter *multipart.Writer) bool {
		if multipartReader != nil {
			// copy part
			for {
				part, err := multipartReader.NextPart()
				if err != nil {
					break
				}
				partWriter, err := multipartWriter.CreatePart(part.Header)

				if err != nil {
					break
				}
				_, err = io.Copy(partWriter, part)
				if err != nil {
					break
				}
			}
		}
		// append form
		if multipartWriter != nil {
			multipartWriter.WriteField(key, value)
		}
		return true
	})
}

func AppendHTTPPacketUploadFile(packet []byte, fieldName, fileName string, fileContent interface{}, contentType ...string) []byte {
	hasContentType := len(contentType) > 0

	return handleHTTPRequestForm(packet, true, true, func(_ string, multipartReader *multipart.Reader, multipartWriter *multipart.Writer) bool {
		if multipartReader != nil {
			// copy part
			for {
				part, err := multipartReader.NextPart()
				if err != nil {
					break
				}
				partWriter, err := multipartWriter.CreatePart(part.Header)

				if err != nil {
					break
				}
				_, err = io.Copy(partWriter, part)
				if err != nil {
					break
				}
			}
		}
		// append upload file
		if multipartWriter != nil {
			var content []byte
			h := make(textproto.MIMEHeader)
			h.Set("Content-Disposition",
				fmt.Sprintf(`form-data; name="%s"; filename="%s"`,
					escapeQuotes(fieldName), escapeQuotes(fileName)))

			guessContentType := "application/octet-stream"
			if hasContentType {
				guessContentType = contentType[0]
			}

			switch r := fileContent.(type) {
			case string:
				content = unsafe.Slice(unsafe.StringData(r), len(r))
				if !hasContentType {
					guessContentType = http.DetectContentType(content)
				}
			case []byte:
				content = r
				if !hasContentType {
					guessContentType = http.DetectContentType(r)
				}
			case io.Reader:
				r.Read(content)
				if !hasContentType {
					guessContentType = http.DetectContentType(content)
				}
			}
			h.Set("Content-Type", guessContentType)

			partWriter, err := multipartWriter.CreatePart(h)
			if err == nil {
				partWriter.Write(content)
			}
		}
		return true
	})
}

func DeleteHTTPPacketForm(packet []byte, key string) []byte {
	return handleHTTPRequestForm(packet, false, false, func(method string, multipartReader *multipart.Reader, multipartWriter *multipart.Writer) bool {
		if strings.ToUpper(method) != "POST" {
			return false
		}

		if multipartReader != nil {
			// copy part
			for {
				part, err := multipartReader.NextPart()
				if err != nil {
					break
				}

				// skip part if key matched
				if part.FormName() == key {
					continue
				}

				partWriter, err := multipartWriter.CreatePart(part.Header)

				if err != nil {
					break
				}
				_, err = io.Copy(partWriter, part)
				if err != nil {
					break
				}
			}
			return true
		}
		return false
	})
}

func GetHTTPPacketCookieValues(packet []byte, key string) []string {
	var val []string
	SplitHTTPPacket(packet, nil, nil, func(line string) string {
		if k, cookieRaw := SplitHTTPHeader(line); k == "Cookie" || k == "cookie" {
			existed := ParseCookie(cookieRaw)
			for _, e := range existed {
				if e.Name == key {
					val = append(val, e.Value)
				}
			}
		}

		if k, cookieRaw := SplitHTTPHeader(line); strings.ToLower(k) == "set-cookie" {
			existed := ParseCookie(cookieRaw)
			for _, e := range existed {
				if e.Name == key {
					val = append(val, e.Value)
				}
			}
		}

		return line
	})
	return val
}

func GetHTTPPacketCookieFirst(packet []byte, key string) string {
	ret := GetHTTPPacketCookieValues(packet, key)
	if len(ret) > 0 {
		return ret[0]
	}
	return ""
}

func GetHTTPPacketCookie(packet []byte, key string) string {
	return GetHTTPPacketCookieFirst(packet, key)
}

func GetHTTPPacketContentType(packet []byte) string {
	var val string
	var fetched = false
	SplitHTTPPacket(packet, nil, nil, func(line string) string {
		if fetched {
			return line
		}
		if k, v := SplitHTTPHeader(line); strings.ToLower(k) == "content-type" {
			fetched = true
			val = v
		}
		return line
	})
	return val
}

func GetHTTPPacketCookies(packet []byte) map[string]string {
	var val = make(map[string]string)
	SplitHTTPPacket(packet, nil, nil, func(line string) string {
		if k, cookieRaw := SplitHTTPHeader(line); k == "Cookie" || k == "cookie" {
			existed := ParseCookie(cookieRaw)
			for _, e := range existed {
				val[e.Name] = e.Value
			}
		}

		if k, cookieRaw := SplitHTTPHeader(line); strings.ToLower(k) == "set-cookie" {
			existed := ParseCookie(cookieRaw)
			for _, e := range existed {
				val[e.Name] = e.Value
			}
		}

		return line
	})
	return val
}

func GetHTTPPacketCookiesFull(packet []byte) map[string][]string {
	var val = make(map[string][]string)
	SplitHTTPPacket(packet, nil, nil, func(line string) string {
		if k, cookieRaw := SplitHTTPHeader(line); k == "Cookie" || k == "cookie" {
			existed := ParseCookie(cookieRaw)
			for _, e := range existed {
				if _, ok := val[e.Name]; !ok {
					val[e.Name] = make([]string, 0)
				}
				val[e.Name] = append(val[e.Name], e.Value)
			}
		}

		if k, cookieRaw := SplitHTTPHeader(line); strings.ToLower(k) == "set-cookie" {
			existed := ParseCookie(cookieRaw)
			for _, e := range existed {
				if _, ok := val[e.Name]; !ok {
					val[e.Name] = make([]string, 0)
				}
				val[e.Name] = append(val[e.Name], e.Value)
			}
		}
		return line
	})
	return val
}

func GetHTTPPacketHeaders(packet []byte) map[string]string {
	var val = make(map[string]string)
	SplitHTTPPacket(packet, nil, nil, func(line string) string {
		if k, v := SplitHTTPHeader(line); k != "" {
			val[k] = v
		}
		return line
	})
	return val
}

func GetHTTPPacketHeadersFull(packet []byte) map[string][]string {
	var val = make(map[string][]string)
	SplitHTTPPacket(packet, nil, nil, func(line string) string {
		if k, v := SplitHTTPHeader(line); k != "" {
			if _, ok := val[k]; !ok {
				val[k] = make([]string, 0)
			}
			val[k] = append(val[k], v)
		}
		return line
	})
	return val
}

func GetHTTPPacketHeader(packet []byte, key string) string {
	raw, ok := GetHTTPPacketHeaders(packet)[key]
	if !ok {
		return ""
	}
	return raw
}

func GetHTTPRequestQueryParam(packet []byte, key string) string {
	vals := GetHTTPRequestQueryParamFull(packet, key)
	if len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func GetHTTPPacketBody(packet []byte) []byte {
	_, body := SplitHTTPHeadersAndBodyFromPacket(packet)
	return body
}

func GetHTTPRequestPostParam(packet []byte, key string) string {
	vals := GetHTTPRequestPostParamFull(packet, key)
	if len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func GetHTTPPacketJSONValue(packet []byte, key string) any {
	return jsonpath.Find(GetHTTPPacketBody(packet), "$."+key)
}

func GetHTTPPacketJSONPath(packet []byte, key string) any {
	return jsonpath.Find(GetHTTPPacketBody(packet), key)
}

func GetHTTPRequestPostParamFull(packet []byte, key string) []string {
	body := GetHTTPPacketBody(packet)
	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return nil
	}
	v, ok := vals[key]
	if ok {
		return v
	}
	return nil
}

func GetHTTPRequestQueryParamFull(packet []byte, key string) []string {
	u, err := ExtractURLFromHTTPRequestRaw(packet, false)
	if err != nil {
		return nil
	}
	val := u.Query()
	vals, ok := val[key]
	if ok {
		return vals
	}
	return []string{}
}

func GetAllHTTPRequestPostParams(packet []byte) map[string]string {
	body := GetHTTPPacketBody(packet)
	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return nil
	}
	ret := make(map[string]string)
	for k, v := range vals {
		ret[k] = v[len(v)-1]
	}
	return ret
}

func GetAllHTTPRequestQueryParams(packet []byte) map[string]string {
	u, err := ExtractURLFromHTTPRequestRaw(packet, false)
	if err != nil {
		return nil
	}
	vals := u.Query()
	ret := make(map[string]string)
	for k, v := range vals {
		ret[k] = v[len(v)-1]
	}
	return ret
}

func GetStatusCodeFromResponse(packet []byte) int {
	var statusCode int
	SplitHTTPPacket(packet, nil, func(proto string, code int, codeMsg string) error {
		statusCode = code
		return nil
	})
	return statusCode
}

func GetHTTPPacketFirstLine(packet []byte) (string, string, string) {
	packet = TrimLeftHTTPPacket(packet)
	reader := bufio.NewReader(bytes.NewBuffer(packet))
	var err error
	firstLineBytes, err := utils.BufioReadLine(reader)
	if err != nil {
		return "", "", ""
	}
	firstLineBytes = TrimSpaceHTTPPacket(firstLineBytes)

	var headers []string
	headers = append(headers, string(firstLineBytes))
	if bytes.HasPrefix(firstLineBytes, []byte("HTTP/")) {
		// response
		proto, code, codeMsg, _ := parseResponseLine(string(firstLineBytes))
		return proto, fmt.Sprint(code), codeMsg
	} else {
		// request
		method, requestURI, proto, _ := parseRequestLine(string(firstLineBytes))
		return method, requestURI, proto
	}
}

func ReplaceHTTPPacketBodyFast(packet []byte, body []byte) []byte {
	var isChunked bool
	SplitHTTPHeadersAndBodyFromPacket(packet, func(line string) {
		if !isChunked {
			isChunked = IsChunkedHeaderLine(line)
		}
	})
	return ReplaceHTTPPacketBody(packet, body, isChunked)
}
