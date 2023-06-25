package lowhttp

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
	"net/http"
	"strings"
)

func ReplaceHTTPPacketFirstLine(packet []byte, firstLine string) []byte {
	var header = []string{firstLine}
	_, body := SplitHTTPPacket(packet, nil, nil, func(line string) string {
		header = append(header, line)
		return line
	})
	return ReplaceHTTPPacketBody([]byte(strings.Join(header, CRLF)+CRLF), body, false)
}

func ReplaceHTTPPacketHeader(packet []byte, headerKey string, headerValue any) []byte {
	var firstLine string
	var header []string
	var handled bool
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
	buf.WriteString(strings.Join(header, CRLF))
	return ReplaceHTTPPacketBody(buf.Bytes(), body, false)
}

func AppendHTTPPacketHeader(packet []byte, headerKey string, headerValue any) []byte {
	var firstLine string
	var header []string
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
		header = append(header, line)
		return line
	})
	header = append(header, headerKey+": "+utils.InterfaceToString(headerValue))
	var buf bytes.Buffer
	buf.WriteString(firstLine)
	buf.WriteString(CRLF)
	buf.WriteString(strings.Join(header, CRLF))
	return ReplaceHTTPPacketBody(buf.Bytes(), body, false)
}

func DeleteHTTPPacketHeader(packet []byte, headerKey string) []byte {
	var firstLine string
	var header []string
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
	header, body := SplitHTTPPacket(packet, func(method string, requestUri string, proto string) error {
		isReq = true
		return nil
	}, func(proto string, code int, codeMsg string) error {
		isRsp = true
		return nil
	}, func(line string) string {
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
	var data = ReplaceHTTPPacketBody([]byte(header), body, false)
	if handled {
		return data
	}
	return AppendHTTPPacketCookie(data, key, value)
}

func AppendHTTPPacketCookie(packet []byte, key string, value any) []byte {
	var isReq bool
	var added bool
	var isRsp bool
	header, body := SplitHTTPPacket(packet, func(method string, requestUri string, proto string) error {
		isReq = true
		return nil
	}, func(proto string, code int, codeMsg string) error {
		isRsp = true
		return nil
	}, func(line string) string {
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
	return ReplaceHTTPPacketBody([]byte(header), body, false)
}

func DeleteHTTPPacketCookie(packet []byte, key string) []byte {
	var isReq bool
	var isRsp bool
	header, body := SplitHTTPPacket(packet, func(method string, requestUri string, proto string) error {
		isReq = true
		return nil
	}, func(proto string, code int, codeMsg string) error {
		isRsp = true
		return nil
	}, func(line string) string {
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
	return ReplaceHTTPPacketBody([]byte(header), body, false)
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

func GetStatusCodeFromResponse(packet []byte) int {
	var statusCode int
	SplitHTTPPacket(packet, nil, func(proto string, code int, codeMsg string) error {
		statusCode = code
		return nil
	})
	return statusCode
}

func ReplaceHTTPPacketBodyFast(packet []byte, body []byte) []byte {
	return ReplaceHTTPPacketBody(packet, body, false)
}
