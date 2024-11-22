package codec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	"golang.org/x/exp/maps"

	"github.com/asaskevich/govalidator"
)

type AutoDecodeResult struct {
	Type        string
	TypeVerbose string
	Origin      string
	Result      string
}

var (
	unicodeRegexp    = regexp.MustCompile(`(\\u[\da-fA-F]{4})|(\\U[\da-fA-F]{8})`)
	base64Regexp     = regexp.MustCompile(`(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=|[A-Za-z0-9+/]{4})`)
	urlRegexp        = regexp.MustCompile(`%[\da-fA-F]{2}`)
	htmlEntityRegexp = regexp.MustCompile(`(&[a-zA-Z]+;)|(&#[a-fA-F0-9]{2};)`)
	hexRegexp        = regexp.MustCompile(`^(0x)?([0-9A-Fa-f]{2}\s?)+[0-9A-Fa-f]{2}$`)
	base32Regexp     = regexp.MustCompile(`^([A-Z2-7]{8})*([A-Z2-7]{8}|[A-Z2-7]{2}([A-Z2-7]{6})*|[A-Z2-7]{4}([A-Z2-7]{4})*|[A-Z2-7]{5}([A-Z2-7]{3})*|[A-Z2-7]{7})(=){0,6}$`)
)

var encodeMap = map[string]func(string) string{
	"UrlDecode": func(s string) string {
		return EncodeUrlCode(s)
	},
	"Html Entity Decode": html.EscapeString,
	"Hex Decode": func(s string) string {
		return EncodeToHex(s)
	},
	"Json Unicode Decode": func(s string) string {
		buf := bytes.Buffer{}
		err := json.NewEncoder(&buf).Encode(s)
		if err != nil {
			log.Errorf("json encode error: %v", err)
			return ""
		}
		return buf.String()
	},
	"Base64 Decode": func(s string) string {
		return EncodeBase64(s)
	},
	"Base32 Decode": func(s string) string {
		return EncodeBase32(s)
	},
	"Base64Url Decode": func(s string) string {
		return EncodeBase64Url(s)
	},
	"Base64Utf8 Decode": func(s string) string {
		byt, err := Utf8ToGB18030([]byte(s))
		if err != nil {
			return ""
		}
		return string(byt)
	},
	"jwt": func(s string) string {
		blocks := strings.Split(s, ".")
		encodeBlocks := []string{}
		for _, i := range blocks {
			encodeBlocks = append(encodeBlocks, EncodeBase64(i))
		}
		return strings.Join(encodeBlocks, ".")
	},
}

func EncodeByType(t string, i interface{}) string {
	if f, ok := encodeMap[t]; ok {
		return f(AnyToString(i))
	}
	return ""
}

func AutoDecode(i interface{}) []*AutoDecodeResult {
	rawBytes := interfaceToBytes(i)
	rawStr := string(rawBytes)
	origin := rawStr

	var (
		results     []*AutoDecodeResult
		sameMap     = make(map[string]struct{})
		base32Bytes []byte
		base64Bytes []byte
		jwtBuf      bytes.Buffer
		mimeResult  *MIMEResult
	)
	addResult := func(result *AutoDecodeResult) {
		results = append(results, result)
		maps.Clear(sameMap)
	}
	checkNewIsSameToOrigin := func(new string, typ string) bool {
		if new == origin {
			sameMap[typ] = struct{}{}
		}
		return new == origin
	}
	isSame := func(new string) bool {
		_, ok := sameMap[new]
		return ok
	}
	tryDecode := func(rawStr string, typ, typVerbose string, matchFunc func(string) bool, decodeFunc func(string) (string, error)) bool {
		if !isSame(typ) && matchFunc(rawStr) {
			decoded, err := decodeFunc(rawStr)
			if err != nil {
				return false
			}
			if decoded != "" && !checkNewIsSameToOrigin(decoded, typ) {
				if !utf8.ValidString(decoded) {
					decoded = EscapeInvalidUTF8Byte([]byte(decoded))
				}
				addResult(&AutoDecodeResult{
					Type:        typ,
					TypeVerbose: typVerbose,
					Origin:      origin,
					Result:      decoded,
				})
				origin = decoded
				return true
			}
			return false
		}
		return false
	}
	htmlDecode := func(rawStr string) (string, error) {
		return html.UnescapeString(rawStr), nil
	}
	hexDecode := func(rawStr string) (string, error) {
		if strings.HasPrefix(rawStr, "0x") {
			rawStr = rawStr[2:]
		}
		rawStr = hexRegexp.ReplaceAllStringFunc(rawStr, func(s string) string {
			result, err := DecodeHex(strings.ReplaceAll(s, " ", ""))
			if err != nil {
				return s
			}
			return string(result)
		})
		return rawStr, nil
	}
	unicodeDecode := func(rawStr string) (string, error) {
		return yakunquote.UnquoteInner(rawStr, 0)
	}
	base32Detect := func(rawStr string) bool {
		matched := base32Regexp.MatchString(rawStr)
		if !matched {
			return false
		}
		decoded, err := DecodeBase32(rawStr)
		if err != nil {
			return false
		}
		if !utf8.Valid(decoded) {
			return false
		}
		base32Bytes = decoded
		return true
	}
	base64Detect := func(rawStr string) bool {
		matched := govalidator.IsBase64(rawStr)
		if !matched {
			return false
		}
		decoded, err := DecodeBase64(rawStr)
		if err != nil {
			return false
		}
		if !utf8.Valid(decoded) {
			return false
		}
		base64Bytes = decoded
		return true
	}
	base64Decode := func(rawStr string) (string, error) {
		return string(base64Bytes), nil
	}
	base32Decode := func(rawStr string) (string, error) {
		return string(base32Bytes), nil
	}
	charsetDetect := func(rawStr string) bool {
		var err error
		mimeResult, err = MatchMIMEType(rawStr)
		return err == nil
	}
	charsetDecode := func(rawStr string) (string, error) {
		newBytes, changed := mimeResult.TryUTF8Convertor(rawBytes)
		if changed {
			return string(newBytes), nil
		}
		return "", fmt.Errorf("no changed")
	}
	jwtDetect := func(rawStr string) bool {
		if strings.Count(rawStr, ".") <= 1 {
			return false
		}
		blocks := strings.Split(rawStr, ".")
		failed := false
		for index, i := range blocks {
			base64Decoded, _ := DecodeBase64(i)
			if len(base64Decoded) <= 0 {
				break
			}
			if govalidator.IsPrintableASCII(string(base64Decoded)) {
				jwtBuf.WriteString(EscapeInvalidUTF8Byte(base64Decoded))
			} else {
				jwtBuf.WriteString(i)
			}
			if index != len(blocks)-1 {
				jwtBuf.WriteByte('.')
			}
		}
		return !failed
	}
	jwtDecode := func(rawStr string) (string, error) {
		return jwtBuf.String(), nil
	}

	for i := 0; i < 100; i++ {
		// url
		if tryDecode(origin, "UrlDecode", "URL编码", urlRegexp.MatchString, url.QueryUnescape) {
			continue
		}
		// html entity
		if tryDecode(origin, "HTML Entity Decode", "HTML实体编码", htmlEntityRegexp.MatchString, htmlDecode) {
			continue
		}
		// hex
		if tryDecode(origin, "Hex Decode", "Hex 解码", hexRegexp.MatchString, hexDecode) {
			continue
		}
		// unicode
		if tryDecode(origin, "Unicode Decode", "Unicode 解码", unicodeRegexp.MatchString, unicodeDecode) {
			continue
		}
		// base32
		if tryDecode(origin, "Base32 Decode", "Base32 解码", base32Detect, base32Decode) {
			continue
		}
		// base64
		if tryDecode(origin, "Base64 Decode", "Base64 解码", base64Detect, base64Decode) {
			continue
		}
		// jwt
		if tryDecode(origin, "jwt", "JWT 解码", jwtDetect, jwtDecode) {
			// if jwt decode success, break anymore
			break
		}
		// charset
		if tryDecode(origin, "Charset Decode", "字符集解码", charsetDetect, charsetDecode) {
			continue
		}
		break
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
