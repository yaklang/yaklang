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

	"github.com/h2non/filetype"
	"github.com/h2non/filetype/types"
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
	rawStr := string(interfaceToBytes(i))
	origin := rawStr

	var (
		results                []*AutoDecodeResult
		sameMap                = make(map[string]struct{})
		resultMap              = make(map[string]struct{})
		base32Bytes            []byte
		base64Bytes            []byte
		matchedType            types.Type
		alreadyMatchedFileType bool
		jwtBuf                 bytes.Buffer
		mimeResult             *MIMEResult
	)
	addResult := func(result *AutoDecodeResult) {
		results = append(results, result)
		maps.Clear(sameMap)
	}
	checkFileType := func(new string, typ string) bool {
		if alreadyMatchedFileType || matchedType.Extension == "" {
			return false
		}
		alreadyMatchedFileType = true

		return true
	}
	checkNewIsSameToOrigin := func(new string, typ string) bool {
		if new == origin {
			sameMap[typ] = struct{}{}
		}
		return new == origin
	}
	checkRepeatedDecode := func(new string, typ string) bool {
		hash := Sha256(fmt.Sprintf("%s%s", new, typ))
		if _, ok := resultMap[hash]; !ok {
			resultMap[hash] = struct{}{}
			return false
		}
		return true
	}
	isSame := func(new string) bool {
		_, ok := sameMap[new]
		return ok
	}

	tryDecodeEx := func(rawStr string, matchFunc func(string) bool, checkFunc func(string, string) bool, decodeFunc func(string) (decoded, typ, typVerbose string, err error)) bool {
		if matchFunc(rawStr) {
			decoded, typ, typVerbose, err := decodeFunc(rawStr)
			if err != nil {
				return false
			}
			if decoded != "" && checkFunc(decoded, typ) {
				showDecoded := decoded
				if !utf8.ValidString(decoded) {
					showDecoded = EscapeInvalidUTF8Byte([]byte(decoded))
				}
				addResult(&AutoDecodeResult{
					Type:        typ,
					TypeVerbose: typVerbose,
					Origin:      origin,
					Result:      showDecoded,
				})
				origin = decoded
				return true
			}
			return false
		}
		return false
	}
	tryDecode := func(rawStr string, typ, typVerbose string, matchFunc func(string) bool, decodeFunc func(string) (string, error)) bool {
		if isSame(typ) {
			return false
		}
		return tryDecodeEx(rawStr, matchFunc,
			func(decoded, typ string) bool {
				return !checkNewIsSameToOrigin(decoded, typ) && !checkRepeatedDecode(decoded, typ)
			},
			func(rawStr string) (string, string, string, error) {
				decoded, err := decodeFunc(rawStr)
				return decoded, typ, typVerbose, err
			},
		)
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
		rawStr = Base32Padding(rawStr)
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
		rawStr = Base64Padding(rawStr)
		matched := govalidator.IsBase64(rawStr)
		if !matched {
			return false
		}
		decoded, err := DecodeBase64(rawStr)
		if err != nil {
			return false
		}
		// if !utf8.Valid(decoded) {
		// 	return false
		// }
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
		if err != nil {
			return false
		}
		if !mimeResult.NeedCharset || mimeResult.Charset == "" {
			return false
		}

		return true
	}
	charsetDecode := func(rawStr string) (string, error) {
		rawBytes := []byte(rawStr)
		newBytes, ok := mimeResult.TryUTF8Convertor(rawBytes)
		if ok && !bytes.Equal(newBytes, rawBytes) {
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
	fileTypeDetect := func(rawStr string) bool {
		var err error
		matchedType, err = filetype.Match([]byte(rawStr))
		if err != nil || matchedType == types.Unknown {
			return false
		}
		return true
	}
	fileTypeDecode := func(rawStr string) (decoded, typ, typVerbose string, err error) {
		return rawStr, matchedType.Extension, matchedType.MIME.Value, nil
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
		// file type
		if tryDecodeEx(origin, fileTypeDetect, checkFileType, fileTypeDecode) {
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
