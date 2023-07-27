package codec

import (
	"bytes"
	"encoding/binary"
	"github.com/asaskevich/govalidator"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

type AutoDecodeResult struct {
	Type        string
	TypeVerbose string
	Origin      string
	Result      string
}

var _jsonUnicodeEncoding = regexp.MustCompile(`(?i)\\u[\dabcdef]{4}`)
var base64Regexp = regexp.MustCompile(`(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=|[A-Za-z0-9+/]{4})`)

func AutoDecode(i interface{}) []*AutoDecodeResult {
	rawBytes := interfaceToBytes(i)
	rawStr := string(rawBytes)

	var results []*AutoDecodeResult
	var origin = rawStr
	for i := 0; i < 100; i++ {
		if r, _ := regexp.MatchString(`%[\da-fA-F]{2}`, rawStr); r {
			rawStr, _ = url.QueryUnescape(rawStr)
			if rawStr != "" {
				results = append(results, &AutoDecodeResult{
					Type:        "UrlDecode",
					TypeVerbose: "URL解码",
					Origin:      origin,
					Result:      rawStr,
				})
				origin = rawStr
				continue
			}
		}

		if r := _jsonUnicodeEncoding.MatchString(rawStr); r {
			rawStr = _jsonUnicodeEncoding.ReplaceAllStringFunc(rawStr, func(s string) string {
				number, err := DecodeHex(strings.TrimLeft(s, "\\u"))
				if err != nil {
					return s
				}
				return string(rune(binary.BigEndian.Uint16(number)))
			})
			if rawStr != "" {
				results = append(results, &AutoDecodeResult{
					Type:        "Json Unicode Decode",
					TypeVerbose: "Json Unicode 解码",
					Origin:      origin,
					Result:      rawStr,
				})
				origin = rawStr
				continue
			}
		}

		if govalidator.IsBase64(rawStr) {
			// 解码解到 BASE64
			rawStr = base64Regexp.ReplaceAllStringFunc(rawStr, func(s string) string {
				result, err := DecodeBase64(s)
				if err != nil {
					return s
				}
				for _, ch := range []rune(string(result)) {
					if !strconv.IsPrint(ch) {
						return s
					}
				}
				return EscapeInvalidUTF8Byte(result)
			})
			if rawStr != "" && rawStr != origin {
				results = append(results, &AutoDecodeResult{
					Type:        "Base64 Decode",
					TypeVerbose: "Base64 解码",
					Origin:      origin,
					Result:      rawStr,
				})
				origin = rawStr
				continue
			}
		}

		decodedByBas64, err := DecodeBase64Url(rawStr)
		if len(decodedByBas64) > 0 && err == nil {
			if utf8.Valid(decodedByBas64) {
				results = append(results, &AutoDecodeResult{
					Type:        "Base64 Decode",
					TypeVerbose: "Base64 解码",
					Origin:      origin,
					Result:      string(decodedByBas64),
				})
				origin = rawStr
				rawStr = string(decodedByBas64)
				continue
			}

			decoded, err := GB18030ToUtf8(decodedByBas64)
			if err == nil && len(decoded) > 0 {
				results = append(results, &AutoDecodeResult{
					Type:        "Base64 Decode",
					TypeVerbose: "Base64 解码（UTF8-Invalid）",
					Origin:      origin,
					Result:      EscapeInvalidUTF8Byte(decodedByBas64),
				})
				origin = rawStr
				results = append(results, &AutoDecodeResult{
					Type:        "GB(K/18030) Decode",
					TypeVerbose: "GB(K/18030) 解码",
					Origin:      origin,
					Result:      EscapeInvalidUTF8Byte(decoded),
				})
				origin = rawStr
				rawStr = string(decoded)
				continue
			}
		}

		// base64
		if strings.Count(rawStr, ".") > 1 {
			var blocks = strings.Split(rawStr, ".")
			var buf bytes.Buffer
			var failed = false
			for index, i := range blocks {
				base64Decoded, _ := DecodeBase64(i)
				if len(base64Decoded) <= 0 {
					failed = true
					break
				}
				if govalidator.IsPrintableASCII(string(base64Decoded)) {
					buf.WriteString(EscapeInvalidUTF8Byte(base64Decoded))
				} else {
					buf.WriteString(i)
				}
				if index != len(blocks)-1 {
					buf.WriteByte('.')
				}
			}
			if failed {
				continue
			}
			rawStr = buf.String()
			if rawStr != "" && rawStr != origin {
				results = append(results, &AutoDecodeResult{
					Type:        "jwt",
					TypeVerbose: "JWT",
					Origin:      origin,
					Result:      rawStr,
				})
				origin = rawStr
				continue
			}
		}

		if rawStr == origin {
			break
		}
	}

	if len(results) <= 0 {
		return []*AutoDecodeResult{
			{
				Type:        "No",
				TypeVerbose: "无编码",
				Origin:      rawStr,
				Result:      rawStr,
			},
		}
	}

	return results
}
