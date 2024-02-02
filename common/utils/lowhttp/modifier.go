package lowhttp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"reflect"
	"sort"
	"strings"
	"unsafe"

	"github.com/samber/lo"

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
	k, v := SplitHTTPHeader(line)
	if utils.IContains(k, "transfer-encoding") && utils.IContains(v, "chunked") {
		return true
	}
	return false
}

func replaceAllParams(values map[string]string, p *QueryParams) {
	// clear all values
	shouldRemove := make(map[string]struct{})
	shouldReplace := make(map[string]string)
	for _, item := range p.Items {
		_, ok := values[item.Key]
		if !ok {
			shouldRemove[item.Key] = struct{}{}
		} else {
			shouldReplace[item.Key] = values[item.Key]
		}
	}

	for k := range shouldRemove {
		p.Remove(k)
	}
	var extraItem []*QueryParamItem
	for k, v := range values {
		_, ok := shouldReplace[k]
		if ok {
			p.Set(k, v)
		} else {
			extraItem = append(extraItem, &QueryParamItem{Key: k, Value: v})
		}
	}

	if len(extraItem) > 0 {
		sort.SliceStable(extraItem, func(i, j int) bool {
			return extraItem[i].Key < extraItem[j].Key
		})
		lo.ForEach(extraItem, func(item *QueryParamItem, _ int) {
			p.Set(item.Key, item.Value)
		})
	}
}

func SetHTTPPacketUrl(packet []byte, rawURL string) []byte {
	var buf bytes.Buffer
	var header []string
	isChunked := false
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return packet
	}

	_, body := SplitHTTPPacket(packet,
		func(method string, requestUri string, proto string) error {
			buf.WriteString(method + " " + parsed.RequestURI() + " " + proto)
			buf.WriteString(CRLF)
			return nil
		},
		nil,
		func(line string) string {
			if !isChunked {
				isChunked = IsChunkedHeaderLine(line)
			}
			if IsHeader(line, "Host") {
				line = fmt.Sprintf("Host: %s", parsed.Host)
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

// ReplaceHTTPPacketFirstLine 是一个辅助，用于改变请求报文，修改第一行（即请求方法，请求路径，协议版本）
// Example:
// ```
// poc.ReplaceHTTPPacketFirstLine(`GET / HTTP/1.1
// Host: Example.com
// `, "GET /test HTTP/1.1")) // 向 example.com 发起请求，修改请求报文的第一行，请求/test路径
// ```
func ReplaceHTTPPacketFirstLine(packet []byte, firstLine string) []byte {
	var isChunked bool
	header := []string{firstLine}
	_, body := SplitHTTPPacket(packet, nil, nil, func(line string) string {
		if !isChunked {
			isChunked = IsChunkedHeaderLine(line)
		}
		header = append(header, line)
		return line
	})
	return ReplaceHTTPPacketBody([]byte(strings.Join(header, CRLF)+CRLF), body, isChunked)
}

// ReplaceHTTPPacketMethod 是一个辅助函数，用于改变请求报文，修改请求方法
// Example:
// ```
// poc.ReplaceHTTPPacketMethod(poc.BasicRequest(), "OPTIONS") // 修改请求方法为OPTIONS
// ```
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

// ReplaceHTTPPacketPath 是一个辅助函数，用于改变请求报文，修改请求路径
// Example:
// ```
// poc.ReplaceHTTPPacketPath(poc.BasicRequest(), "/get") // 修改请求路径为/get
// ```
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

// AppendHTTPPacketPath 是一个辅助函数，用于改变请求报文，在现有请求路径后添加请求路径
// Example:
// ```
// poc.AppendHTTPPacketPath(`GET /docs HTTP/1.1
// Host: yaklang.com
// `, "/api/poc")) // 向 example.com 发起请求，实际上请求路径改为/docs/api/poc
// ```
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

func handleHTTPPacketQueryParam(packet []byte, noAutoEncode bool, callback func(*QueryParams)) []byte {
	var isChunked bool
	var buf bytes.Buffer
	var header []string

	_, body := SplitHTTPPacket(packet,
		func(method string, requestUri string, proto string) error {
			defer func() {
				buf.WriteString(method + " " + requestUri + " " + proto)
				buf.WriteString(CRLF)
			}()

			urlIns := ForceStringToUrl(requestUri)
			u := NewQueryParams(urlIns.RawQuery).DisableAutoEncode(noAutoEncode)
			callback(u)
			urlIns.RawQuery = u.Encode()
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

// ReplaceAllHTTPPacketQueryParams 是一个辅助函数，用于改变请求报文，修改所有 GET 请求参数，如果不存在则会增加，其接收一个 map[string]string 类型的参数，其中 key 为请求参数名，value 为请求参数值
// Example:
// ```
// poc.ReplaceAllHTTPPacketQueryParams(poc.BasicRequest(), {"a":"b", "c":"d"}) // 添加GET请求参数a，值为b，添加GET请求参数c，值为d
// ```
func ReplaceAllHTTPPacketQueryParams(packet []byte, values map[string]string) []byte {
	return handleHTTPPacketQueryParam(packet, false, func(p *QueryParams) {
		replaceAllParams(values, p)
	})
}

// ReplaceAllHTTPPacketQueryParamsWithoutEscape 是一个辅助函数，用于改变请求报文，修改所有 GET 请求参数，如果不存在则会增加，其接收一个 map[string]string 类型的参数，其中 key 为请求参数名，value 为请求参数值
// 与 poc.ReplaceAllHTTPPacketQueryParams 类似，但是不会将参数值进行转义
// Example:
// ```
// poc.ReplaceAllHTTPPacketQueryParamsWithoutEscape(poc.BasicRequest(), {"a":"b", "c":"d"}) // 添加GET请求参数a，值为b，添加GET请求参数c，值为d
// ```
func ReplaceAllHTTPPacketQueryParamsWithoutEscape(packet []byte, values map[string]string) []byte {
	return handleHTTPPacketQueryParam(packet, true, func(p *QueryParams) {
		replaceAllParams(values, p)
	})
}

// ReplaceHTTPPacketQueryParam 是一个辅助函数，用于改变请求报文，修改GET请求参数，如果不存在则会增加
// Example:
// ```
// _, raw, _ = poc.ParseUrlToHTTPRequestRaw("GET", "https://pie.dev/get")
// poc.ReplaceHTTPPacketQueryParam(raw, "a", "b") // 添加GET请求参数a，值为b
// ```
func ReplaceHTTPPacketQueryParam(packet []byte, key, value string) []byte {
	return handleHTTPPacketQueryParam(packet, false, func(q *QueryParams) {
		q.Set(key, value)
	})
}

func ReplaceHTTPPacketQueryParamWithoutEncoding(packet []byte, key, value string) []byte {
	return handleHTTPPacketQueryParam(packet, true, func(q *QueryParams) {
		q.Set(key, value)
	})
}

func ReplaceHTTPPacketQueryParamRaw(packet []byte, rawQuery string) []byte {
	var isChunked bool
	var buf bytes.Buffer
	var header []string

	_, body := SplitHTTPPacket(packet,
		func(method string, requestUri string, proto string) error {
			defer func() {
				buf.WriteString(method + " " + requestUri + " " + proto)
				buf.WriteString(CRLF)
			}()

			urlIns := ForceStringToUrl(requestUri)
			urlIns.RawQuery = rawQuery
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

// AppendHTTPPacketQueryParam 是一个辅助函数，用于改变请求报文，添加GET请求参数
// Example:
// ```
// poc.AppendHTTPPacketQueryParam(poc.BasicRequest(), "a", "b") // 添加GET请求参数a，值为b
// ```
func AppendHTTPPacketQueryParam(packet []byte, key, value string) []byte {
	return handleHTTPPacketQueryParam(packet, false, func(q *QueryParams) {
		q.Add(key, value)
	})
}

func AppendAllHTTPPacketQueryParam(packet []byte, Params map[string][]string) []byte {
	for key, values := range Params {
		for _, value := range values {
			if value == "" {
				continue
			}
			packet = handleHTTPPacketQueryParam(packet, false, func(q *QueryParams) {
				q.Add(key, value)
			})
		}
	}
	return packet
}

// DeleteHTTPPacketQueryParam 是一个辅助函数，用于改变请求报文，删除GET请求参数
// Example:
// ```
// poc.DeleteHTTPPacketQueryParam(`GET /get?a=b&c=d HTTP/1.1
// Content-Type: application/json
// Host: pie.dev
//
// `, "a") // 删除GET请求参数a
// ```
func DeleteHTTPPacketQueryParam(packet []byte, key string) []byte {
	return handleHTTPPacketQueryParam(packet, false, func(q *QueryParams) {
		q.Del(key)
	})
}

func handleHTTPPacketPostParam(packet []byte, noAutoEncode bool, callback func(*QueryParams)) []byte {
	var isChunked bool

	headersRaw, bodyRaw := SplitHTTPPacket(packet, nil, nil)
	bodyString := utils.UnsafeBytesToString(bodyRaw)
	u := NewQueryParams(bodyString).DisableAutoEncode(noAutoEncode)
	callback(u)
	newBody := u.Encode()

	return ReplaceHTTPPacketBody(utils.UnsafeStringToBytes(headersRaw), utils.UnsafeStringToBytes(newBody), isChunked)
}

// ReplaceAllHTTPPacketPostParams 是一个辅助函数，用于改变请求报文，修改所有 POST 请求参数，如果不存在则会增加，其接收一个 map[string]string 类型的参数，其中 key 为 POST 请求参数名，value 为 POST 请求参数值
// Example:
// ```
// _, raw, _ = poc.ParseUrlToHTTPRequestRaw("POST", "https://pie.dev/post")
// poc.ReplaceAllHTTPPacketPostParams(raw, {"a":"b", "c":"d"}) // 添加POST请求参数a，值为b，POST请求参数c，值为d
// ```
func ReplaceAllHTTPPacketPostParams(packet []byte, values map[string]string) []byte {
	return handleHTTPPacketPostParam(packet, false, func(p *QueryParams) {
		replaceAllParams(values, p)
	})
}

// ReplaceAllHTTPPacketPostParamsWithoutEscape 是一个辅助函数，用于改变请求报文，修改所有 POST 请求参数，如果不存在则会增加，其接收一个 map[string]string 类型的参数，其中 key 为 POST 请求参数名，value 为 POST 请求参数值
// 与 poc.ReplaceAllHTTPPacketPostParams 类似，但是不会将参数值进行转义
//
// Example:
// ```
// _, raw, _ = poc.ParseUrlToHTTPRequestRaw("POST", "https://pie.dev/post")
// poc.ReplaceAllHTTPPacketPostParamsWithoutEscape(raw, {"a":"b", "c":"d"}) // 添加POST请求参数a，值为b，POST请求参数c，值为d
// ```
func ReplaceAllHTTPPacketPostParamsWithoutEscape(packet []byte, values map[string]string) []byte {
	return handleHTTPPacketPostParam(packet, true, func(p *QueryParams) {
		replaceAllParams(values, p)
	})
}

// ReplaceHTTPPacketPostParam 是一个辅助函数，用于改变请求报文，修改POST请求参数，如果不存在则会增加
// Example:
// ```
// _, raw, _ = poc.ParseUrlToHTTPRequestRaw("POST", "https://pie.dev/post")
// poc.ReplaceHTTPPacketPostParam(raw, "a", "b") // 添加POST请求参数a，值为b
// ```
func ReplaceHTTPPacketPostParam(packet []byte, key, value string) []byte {
	return handleHTTPPacketPostParam(packet, false, func(p *QueryParams) {
		p.Set(key, value)
	})
}

// AppendHTTPPacketPostParam 是一个辅助函数，用于改变请求报文，添加POST请求参数
// Example:
// ```
// poc.AppendHTTPPacketPostParam(poc.BasicRequest(), "a", "b") // 向 pie.dev 发起请求，添加POST请求参数a，值为b
// ```
func AppendHTTPPacketPostParam(packet []byte, key, value string) []byte {
	return handleHTTPPacketPostParam(packet, false, func(p *QueryParams) {
		p.Add(key, value)
	})
}

// DeleteHTTPPacketPostParam 是一个辅助函数，用于改变请求报文，删除POST请求参数
// Example:
// ```
// poc.DeleteHTTPPacketPostParam(`POST /post HTTP/1.1
// Content-Type: application/json
// Content-Length: 7
// Host: pie.dev
//
// a=b&c=d`, "a") // 删除POST请求参数a
// ```
func DeleteHTTPPacketPostParam(packet []byte, key string) []byte {
	return handleHTTPPacketPostParam(packet, false, func(p *QueryParams) {
		p.Del(key)
	})
}

// ReplaceHTTPPacketHeader 是一个辅助函数，用于改变请求报文，修改请求头，如果不存在则会增加
// Example:
// ```
// poc.ReplaceHTTPPacketHeader(poc.BasicRequest(),"AAA", "BBB") // 修改AAA请求头的值为BBB，这里没有AAA请求头，所以会增加该请求头
// ```
func ReplaceHTTPPacketHeader(packet []byte, headerKey string, headerValue any) []byte {
	var firstLine string
	var header []string
	var handled bool
	isChunked := IsChunkedHeaderLine(headerKey + ": " + utils.InterfaceToString(headerValue))
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

// ReplaceAllHTTPPacketHeaders 是一个辅助函数，用于改变请求报文，修改所有请求头
// Example:
// ```
// poc.ReplaceAllHTTPPacketHeaders(poc.BasicRequest(), {"AAA": "BBB"}) // 修改所有请求头，这里没有AAA请求头，所以会增加该请求头
// ```
func ReplaceAllHTTPPacketHeaders(packet []byte, headers map[string]string) []byte {
	var firstLine string
	isChunked := false
	host := ""
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
		if IsHeader(line, "Host") {
			_, host = SplitHTTPHeader(line)
		}
		return line
	})
	var buf bytes.Buffer
	buf.WriteString(firstLine)
	buf.WriteString(CRLF)
	if _, ok := headers["Host"]; !ok {
		headers["Host"] = host
	}
	for key, value := range headers {
		buf.WriteString(fmt.Sprintf("%s: %s", key, value))
		buf.WriteString(CRLF)
	}
	return ReplaceHTTPPacketBody(buf.Bytes(), body, isChunked)
}

// ReplaceHTTPPacketHost 是一个辅助函数，用于改变请求报文，修改Host请求头，如果不存在则会增加，实际上是ReplaceHTTPPacketHeader("Host", host)的简写
// Example:
// ```
// _, raw, _ = poc.ParseUrlToHTTPRequestRaw("GET", "https://yaklang.com")
// poc.ReplaceHTTPPacketHost(raw, "www.yaklang.com") // 修改Host请求头的值为 www.yaklang.com
// ```
func ReplaceHTTPPacketHost(packet []byte, host string) []byte {
	return ReplaceHTTPPacketHeader(packet, "Host", host)
}

// ReplaceHTTPPacketBasicAuth 是一个辅助函数，用于改变请求报文，修改Authorization请求头为基础认证的密文，如果不存在则会增加，实际上是ReplaceHTTPPacketHeader("Authorization", codec.EncodeBase64(username + ":" + password))的简写
// Example:
// ```
// _, raw, _ = poc.ParseUrlToHTTPRequestRaw("GET", "https://pie.dev/basic-auth/admin/password")
// poc.ReplaceHTTPPacketBasicAuth(raw, "admin", "password") // 修改Authorization请求头
// ```
func ReplaceHTTPPacketBasicAuth(packet []byte, username, password string) []byte {
	return ReplaceHTTPPacketHeader(packet, "Authorization", "Basic "+codec.EncodeBase64(username+":"+password))
}

// AppendHTTPPacketHeader 是一个辅助函数，用于改变请求报文，添加请求头
// Example:
// ```
// poc.AppendHTTPPacketHeader(poc.BasicRequest(), "AAA", "BBB") // 添加AAA请求头的值为BBB
// ```
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

// DeleteHTTPPacketHeader 是一个辅助函数，用于改变请求报文，删除请求头
// Example:
// ```
// poc.DeleteHTTPPacketHeader(`GET /get HTTP/1.1
// Content-Type: application/json
// AAA: BBB
// Host: pie.dev
//
// `, "AAA") // 删除AAA请求头
// ```
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

// ReplaceHTTPPacketCookie 是一个辅助函数，用于改变请求报文，修改Cookie请求头中的值，如果不存在则会增加
// Example:
// ```
// poc.ReplaceHTTPPacketCookie(poc.BasicRequest(), p"aaa", "bbb") // 修改cookie值，由于这里没有aaa的cookie值，所以会增加
// ```
func ReplaceHTTPPacketCookie(packet []byte, key string, value any) []byte {
	var isReq bool
	var isRsp bool
	handled := false
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
			cookie := make([]*http.Cookie, len(existed))
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
	data := ReplaceHTTPPacketBody([]byte(header), body, isChunked)
	if handled {
		return data
	}
	return AppendHTTPPacketCookie(data, key, value)
}

// AppendHTTPPacketCookie 是一个辅助函数，用于改变请求报文，添加Cookie请求头中的值
// Example:
// ```
// poc.AppendHTTPPacketCookie(poc.BasicRequest(), "aaa", "bbb") // 添加cookie键值对aaa:bbb
// ```
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

// DeleteHTTPPacketCookie 是一个辅助函数，用于改变请求报文，删除Cookie中的值
// Example:
// ```
// poc.DeleteHTTPPacketCookie(`GET /get HTTP/1.1
// Content-Type: application/json
// Cookie: aaa=bbb; ccc=ddd
// Host: pie.dev
//
// `, "aaa") // 删除Cookie中的aaa
// ```
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
					line = fmt.Sprintf("Content-Type: %s", multipartWriter.FormDataContentType())
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
		header = append(header, fmt.Sprintf("Content-Type: %s", multipartWriter.FormDataContentType()))
	}

	for _, line := range header {
		buf.WriteString(line)
		buf.WriteString(CRLF)
	}
	return ReplaceHTTPPacketBody(buf.Bytes(), body, isChunked)
}

// AppendHTTPPacketFormEncoded 是一个辅助函数，用于改变请求报文，添加请求体中的表单
// Example:
// ```
// poc.AppendHTTPPacketFormEncoded(`POST /post HTTP/1.1
// Host: pie.dev
// Content-Type: multipart/form-data; boundary=------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm
// Content-Length: 203
//
// --------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm
// Content-Disposition: form-data; name="aaa"
//
// bbb
// --------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm--`, "ccc", "ddd") // 添加POST请求表单，其中ccc为键，ddd为值
// ```
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

// AppendHTTPPacketUploadFile 是一个辅助函数，用于改变请求报文，添加请求体中的上传的文件，其中第一个参数为原始请求报文，第二个参数为表单名，第三个参数为文件名，第四个参数为文件内容，第五个参数是可选参数，为文件类型(Content-Type)
// Example:
// ```
// _, raw, _ = poc.ParseUrlToHTTPRequestRaw("POST", "https://pie.dev/post")
// poc.AppendHTTPPacketUploadFile(raw, "file", "phpinfo.php", "<?php phpinfo(); ?>", "image/jpeg")) // 添加POST请求表单，其文件名为phpinfo.php，内容为<?php phpinfo(); ?>，文件类型为image/jpeg
// ```
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
			contentDisposition := fmt.Sprintf(`form-data; name="%s"`, escapeQuotes(fieldName))
			if fileName != "" {
				contentDisposition += fmt.Sprintf(`;filename="%s"`, escapeQuotes(fileName))
			}
			h.Set("Content-Disposition", contentDisposition)

			guessContentType := "application/octet-stream"
			if hasContentType {
				guessContentType = contentType[0]
			}

			switch r := fileContent.(type) {
			case string:
				content = unsafe.Slice(unsafe.StringData(r), len(r))
			case []byte:
				content = r
			case io.Reader:
				r.Read(content)
			}
			if !hasContentType {
				guessContentType = http.DetectContentType(content)
			}

			if guessContentType != "" {
				h.Set("Content-Type", guessContentType)
			}

			partWriter, err := multipartWriter.CreatePart(h)
			if err == nil {
				partWriter.Write(content)
			}
		}
		return true
	})
}

// DeleteHTTPPacketForm 是一个辅助函数，用于改变请求报文，删除POST请求表单
// Example:
// ```
// poc.DeleteHTTPPacketForm(`POST /post HTTP/1.1
// Host: pie.dev
// Content-Type: multipart/form-data; boundary=------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm
// Content-Length: 308
//
// --------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm
// Content-Disposition: form-data; name="aaa"
//
// bbb
// --------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm
// Content-Disposition: form-data; name="ccc"
//
// ddd
// --------------------------OFHnlKtUimimGcXvRSxgCZlIMAyDkuqsxeppbIFm--`, "aaa") // 删除POST请求表单aaa
// ```
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

func GetParamsFromBody(contentType string, body []byte) (params map[string]string, useRaw bool, err error) {
	var contentTypeParams map[string]string
	_, contentTypeParams, err = mime.ParseMediaType(contentType)
	if err != nil {
		return
	}
	params = make(map[string]string)
	// 这是为了处理复杂json/xml的情况
	handleUnmarshalValues := func(v any) ([]string, []string) {
		var keys, values []string
		ref := reflect.ValueOf(v)
		switch ref.Kind() {
		case reflect.Array, reflect.Slice:
			arrayLen := ref.Len()
			if arrayLen > 0 {
				return []string{""}, []string{utils.InterfaceToString(ref.Index(arrayLen - 1).Interface())}
			}
		case reflect.Map:
			refKeys := ref.MapKeys()
			if len(refKeys) > 0 {
				for _, refKeys := range refKeys {
					keys = append(keys, utils.InterfaceToString(refKeys.Interface()))
					values = append(values, utils.InterfaceToString(ref.MapIndex(refKeys).Interface()))
				}
				return keys, values
			}
		case reflect.Float32, reflect.Float64:
			floatV, ok := v.(float64)
			if ok && floatV == float64(int(floatV)) {
				v = int(floatV)
			}
		}
		return []string{""}, []string{utils.InterfaceToString(v)}
	}
	handleUnmarshalResults := func(tempMap map[string]any) map[string]string {
		params := make(map[string]string, len(tempMap))
		for k, v := range tempMap {
			extraKeys, extraValues := handleUnmarshalValues(v)
			for i, key := range extraKeys {
				if key == "" {
					params[k] = extraValues[i]
					continue
				}
				params[fmt.Sprintf("%s[%s]", k, key)] = extraValues[i]
			}
		}
		return params
	}

	// try json
	var tempMap map[string]any
	if len(params) == 0 {
		err = json.Unmarshal(body, &tempMap)
		if err == nil {
			params = handleUnmarshalResults(tempMap)
		}
	}

	// try xml
	if len(params) == 0 {
		tempMap = utils.XmlLoads(body)
		if len(tempMap) > 0 {
			params = handleUnmarshalResults(tempMap)
		}
	}
	// try post form
	if len(params) == 0 {
		boundary, ok := contentTypeParams["boundary"]
		if ok {
			mr := multipart.NewReader(bytes.NewReader(body), boundary)
			for {
				part, err := mr.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					return nil, false, err
				}

				// 检查part是否为表单字段
				if part.FormName() != "" {
					content, err := io.ReadAll(part)
					if err != nil && err != io.EOF {
						return nil, false, err
					}
					params[part.FormName()] = utils.UnsafeBytesToString(content)
				}
			}
		}
	}
	// try post values
	if len(params) == 0 {
		var values url.Values
		values, err = url.ParseQuery(utils.UnsafeBytesToString(body))
		if err == nil {
			for k, v := range values {
				if len(v) == 0 {
					continue
				}
				params[k] = v[len(v)-1]
			}
		}
	}

	if len(params) == 0 {
		// 这个flag位用于标记是否调用者直接使用原始的body, 这用于默认情况
		useRaw = true
	}

	if len(params) > 0 {
		err = nil
	}
	return
}

func AppendHTTPPacketHeaderIfNotExist(packet []byte, headerKey string, headerValue any) []byte {
	var firstLine string
	var header []string
	var exist bool
	isChunked := IsChunkedHeaderLine(headerKey + ": " + utils.InterfaceToString(headerValue))
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
			exist = true
		}
		header = append(header, line)
		return line
	})
	if !exist {
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

// GetHTTPPacketCookieValues 是一个辅助函数，用于获取请求报文中Cookie值，其返回值为[]string，这是因为Cookie可能存在多个相同键名的值
// Example:
// ```
// poc.GetHTTPPacketCookieValues(`GET /get HTTP/1.1
// Content-Type: application/json
// Cookie: a=b; a=c
// Host: pie.dev
//
// `, "a") // 获取键名为a的Cookie值，这里会返回["b", "c"]
// ```
func GetHTTPPacketCookieValues(packet []byte, key string) (cookieValues []string) {
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

// GetHTTPPacketCookieFirst 是一个辅助函数，用于获取请求报文中Cookie值，其返回值为string
// Example:
// ```
// poc.GetHTTPPacketCookieFirst(`GET /get HTTP/1.1
// Content-Type: application/json
// Cookie: a=b; c=d
// Host: pie.dev
//
// `, "a") // 获取键名为a的Cookie值，这里会返回"b"
// ```
func GetHTTPPacketCookieFirst(packet []byte, key string) (cookieValue string) {
	ret := GetHTTPPacketCookieValues(packet, key)
	if len(ret) > 0 {
		return ret[0]
	}
	return ""
}

// GetUrlFromHTTPRequest 是一个辅助函数，用于获取请求报文中的URL，其返回值为string
// Example:
// ```
// poc.GetUrlFromHTTPRequest("https", `GET /get HTTP/1.1
// Content-Type: application/json
// Host: pie.dev
//
// `) // 获取URL，这里会返回"https://pie.dev/get"
func GetUrlFromHTTPRequest(scheme string, packet []byte) (url string) {
	if scheme == "" {
		scheme = "http"
	}
	u, err := ExtractURLFromHTTPRequestRaw(packet, strings.HasPrefix(strings.ToLower(scheme), "https"))
	if err != nil {
		return ""
	}
	u.Scheme = scheme
	return u.String()
}

// GetHTTPPacketCookie 是一个辅助函数，用于获取请求报文中Cookie值，其返回值为string
// Example:
// ```
// poc.GetHTTPPacketCookie(`GET /get HTTP/1.1
// Content-Type: application/json
// Cookie: a=b; c=d
// Host: pie.dev
//
// `, "a") // 获取键名为a的Cookie值，这里会返回"b"
// ```
func GetHTTPPacketCookie(packet []byte, key string) (cookieValue string) {
	return GetHTTPPacketCookieFirst(packet, key)
}

// GetHTTPPacketContentType 是一个辅助函数，用于获取请求报文中的Content-Type请求头，其返回值为string
// Example:
// ```
// poc.GetHTTPPacketContentType(`POST /post HTTP/1.1
// Content-Type: application/json
// COntent-Length: 7
// Host: pie.dev
//
// a=b&c=d`) // 获取Content-Type请求头
// ```
func GetHTTPPacketContentType(packet []byte) (contentType string) {
	var val string
	fetched := false
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

// GetHTTPPacketCookies 是一个辅助函数，用于获取请求报文中所有Cookie值，其返回值为map[string]string
// Example:
// ```
// poc.GetHTTPPacketCookies(`GET /get HTTP/1.1
// Content-Type: application/json
// Cookie: a=b; c=d
// Host: pie.dev
//
// `) // 获取所有Cookie值，这里会返回{"a":"b", "c":"d"}
// ```
func GetHTTPPacketCookies(packet []byte) (cookies map[string]string) {
	val := make(map[string]string)
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

// GetHTTPPacketCookiesFull 是一个辅助函数，用于获取请求报文中所有Cookie值，其返回值为map[string][]string，这是因为Cookie可能存在多个相同键名的值
// Example:
// ```
// poc.GetHTTPPacketCookiesFull(`GET /get HTTP/1.1
// Content-Type: application/json
// Cookie: a=b; a=c; c=d
// Host: pie.dev
//
// `) // 获取所有Cookie值，这里会返回{"a":["b", "c"], "c":["d"]}
// ```
func GetHTTPPacketCookiesFull(packet []byte) (cookies map[string][]string) {
	val := make(map[string][]string)
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

// GetHTTPPacketHeaders 是一个辅助函数，用于获取请求报文中所有请求头，其返回值为map[string]string
// Example:
// ```
// poc.GetHTTPPacketCookiesFull(`GET /get HTTP/1.1
// Content-Type: application/json
// Cookie: a=b; a=c; c=d
// Host: pie.dev
//
// `) // 获取所有请求头，这里会返回{"Content-Type": "application/json", "Cookie": "a=b; a=c; c=d", "Host": "pie.dev"}
// ```
func GetHTTPPacketHeaders(packet []byte) (headers map[string]string) {
	val := make(map[string]string)
	SplitHTTPPacket(packet, nil, nil, func(line string) string {
		if k, v := SplitHTTPHeader(line); k != "" {
			val[k] = v
		}
		return line
	})
	return val
}

// GetHTTPPacketHeadersFull 是一个辅助函数，用于获取请求报文中所有请求头，其返回值为map[string][]string，这是因为请求头可能存在多个相同键名的值
// Example:
// ```
// poc.GetHTTPPacketHeadersFull(`GET /get HTTP/1.1
// Content-Type: application/json
// Cookie: a=b; a=c; c=d
// Cookie: e=f
// Host: pie.dev
//
// `) // 获取所有请求头，这里会返回{"Content-Type": ["application/json"], "Cookie": []"a=b; a=c; c=d", "e=f"], "Host": ["pie.dev"]}
// ```
func GetHTTPPacketHeadersFull(packet []byte) (headers map[string][]string) {
	val := make(map[string][]string)
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

// GetHTTPPacketHeaders 是一个辅助函数，用于获取请求报文中指定的请求头，其返回值为string
// Example:
// ```
// poc.GetHTTPPacketCookiesFull(`GET /get HTTP/1.1
// Content-Type: application/json
// Cookie: a=b; a=c; c=d
// Host: pie.dev
//
// `) // 获取Content-Type请求头，这里会返回"application/json"
// ```
func GetHTTPPacketHeader(packet []byte, key string) (header string) {
	ret := GetHTTPPacketHeaders(packet)
	if ret == nil {
		return ""
	}

	fuzzResult := make(map[string]string)
	for headerKey, value := range ret {
		if key == headerKey {
			return value
		}
		if strings.ToLower(key) == strings.ToLower(headerKey) {
			fuzzResult[key] = value
		}
	}
	if len(fuzzResult) > 0 {
		return fuzzResult[key]
	}
	return ""
}

// GetHTTPPacketQueryParam 是一个辅助函数，用于获取请求报文中指定的GET请求参数，其返回值为string
// Example:
// ```
// poc.GetHTTPPacketQueryParam(`GET /get?a=b&c=d HTTP/1.1
// Content-Type: application/json
// Host: pie.dev
//
// `, "a") // 获取GET请求参数a的值
// ```
func GetHTTPRequestQueryParam(packet []byte, key string) (paramValue string) {
	vals := GetHTTPRequestQueryParamFull(packet, key)
	if len(vals) > 0 {
		return vals[0]
	}
	return ""
}

// GetHTTPPacketBody 是一个辅助函数，用于获取请求报文中的请求体，其返回值为bytes
// Example:
// ```
// poc.GetHTTPPacketBody(`POST /post HTTP/1.1
// Content-Type: application/json
// COntent-Length: 7
// Host: pie.dev
//
// a=b&c=d`) // 获取请求头，这里为b"a=b&c=d"
// ```
func GetHTTPPacketBody(packet []byte) (body []byte) {
	_, body = SplitHTTPHeadersAndBodyFromPacket(packet)
	return body
}

// GetHTTPPacketPostParam 是一个辅助函数，用于获取请求报文中指定的POST请求参数，其返回值为string
// Example:
// ```
// poc.GetHTTPPacketPostParam(`POST /post HTTP/1.1
// Content-Type: application/json
// COntent-Length: 7
// Host: pie.dev
//
// a=b&c=d`, "a") // 获取POST请求参数a的值
// ```
func GetHTTPRequestPostParam(packet []byte, key string) (paramValue string) {
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

// GetAllHTTPPacketPostParams 是一个辅助函数，用于获取请求报文中的所有POST请求参数，其返回值为map[string]string，其中键为参数名，值为参数值
// Example:
// ```
// poc.GetAllHTTPPacketPostParams(`POST /post HTTP/1.1
// Content-Type: application/json
// COntent-Length: 7
// Host: pie.dev
//
// a=b&c=d`) // 获取所有POST请求参数
// ```
func GetAllHTTPRequestPostParams(packet []byte) (params map[string]string) {
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

// GetAllHTTPPacketQueryParams 是一个辅助函数，用于获取请求报文中的所有GET请求参数，其返回值为map[string]string，其中键为参数名，值为参数值
// Example:
// ```
// poc.GetAllHTTPPacketQueryParams(`GET /get?a=b&c=d HTTP/1.1
// Content-Type: application/json
// Host: pie.dev
//
// `) // 获取所有GET请求参数
// ```
func GetAllHTTPRequestQueryParams(packet []byte) (params map[string]string) {
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

// GetStatusCodeFromResponse 是一个辅助函数，用于获取响应报文中的状态码，其返回值为int
// Example:
// ```
// poc.GetStatusCodeFromResponse(`HTTP/1.1 200 OK
// Content-Length: 5
//
// hello`) // 获取响应报文中的状态码，这里会返回200
// ```
func GetStatusCodeFromResponse(packet []byte) (statusCode int) {
	SplitHTTPPacket(packet, nil, func(proto string, code int, codeMsg string) error {
		statusCode = code
		return nil
	})
	return statusCode
}

// GetHTTPRequestPathWithoutQuery 是一个辅助函数，用于获取响应报文中的路径，返回值是 string，不包含 query
// Example:
// ```
// poc.GetHTTPRequestPathWithoutQuery("GET /a/bc.html?a=1 HTTP/1.1\r\nHost: www.example.com\r\n\r\n") // /a/bc.html
// ```
func GetHTTPRequestPathWithoutQuery(packet []byte) (path string) {
	return strings.Split(GetHTTPRequestPath(packet), "?")[0]
}

// GetHTTPRequestPath 是一个辅助函数，用于获取响应报文中的路径，返回值是 string，包含 query
// Example:
// ```
// poc.GetHTTPRequestPath("GET /a/bc.html?a=1 HTTP/1.1\r\nHost: www.example.com\r\n\r\n") // /a/bc.html?a=1
// ```
func GetHTTPRequestPath(packet []byte) (path string) {
	SplitHTTPPacket(packet, func(method string, requestUri string, proto string) error {
		path = requestUri
		return io.EOF
	}, nil)
	return path
}

// GetHTTPRequestMethod 是一个辅助函数，用于获取请求报文中的请求方法，其返回值为string
// Example:
// ```
// poc.GetHTTPRequestMethod(`GET /get HTTP/1.1
// Content-Type: application/json
// Cookie: a=b; a=c; c=d
// Host: pie.dev
//
// `) // 获取请求方法，这里会返回"GET"
// ```
func GetHTTPRequestMethod(packet []byte) (method string) {
	SplitHTTPPacket(packet, func(m string, _ string, _ string) error {
		method = m
		return utils.Error("normal")
	}, nil)
	return method
}

// GetHTTPPacketFirstLine 是一个辅助函数，用于获取 HTTP 报文中第一行的值，其返回值为string，string，string
// 在请求报文中，其三个返回值分别为：请求方法，请求URI，协议版本
// 在响应报文中，其三个返回值分别为：协议版本，状态码，状态码描述
// Example:
// ```
// poc.GetHTTPPacketFirstLine(`GET /get HTTP/1.1
// Content-Type: application/json
// Cookie: a=b; a=c; c=d
// Host: pie.dev
//
// `) // 获取请求方法，请求URI，协议版本，这里会返回"GET", "/get", "HTTP/1.1"
// ```
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
		proto, code, codeMsg, _ := utils.ParseHTTPResponseLine(string(firstLineBytes))
		return proto, fmt.Sprint(code), codeMsg
	} else {
		// request
		method, requestURI, proto, _ := utils.ParseHTTPRequestLine(string(firstLineBytes))
		return method, requestURI, proto
	}
}

// ReplaceHTTPPacketBody 是一个辅助函数，用于改变请求报文，修改请求体内容，第一个参数为修改后的请求体内容，第二个参数为是否分块传输
// Example:
// ```
// poc.ReplaceHTTPPacketBody(poc.BasicRequest(), "a=b") // 修改请求体内容为a=b
// ```
func ReplaceHTTPPacketBodyFast(packet []byte, body []byte) []byte {
	var isChunked bool
	SplitHTTPHeadersAndBodyFromPacket(packet, func(line string) {
		if !isChunked {
			isChunked = IsChunkedHeaderLine(line)
		}
	})
	return ReplaceHTTPPacketBody(packet, body, isChunked)
}
