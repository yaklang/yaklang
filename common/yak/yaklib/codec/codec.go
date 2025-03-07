package codec

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"html"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/samber/lo"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding"

	"github.com/DataDog/mmh3"
	"github.com/pkg/errors"
	"github.com/saintfish/chardet"
	"github.com/yaklang/yaklang/common/gmsm/sm3"
	"github.com/yaklang/yaklang/common/log"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func ForceQueryUnescape(s string) string {
	val, err := url.QueryUnescape(UrlUnicodeDecode(s))
	if err != nil {
		return s
	}
	return val
}

func QueryUnescape(s string) (string, error) {
	return url.QueryUnescape(UrlUnicodeDecode(s))
}

var PathEscape = url.PathEscape

func PathUnescape(s string) (string, error) {
	return url.PathUnescape(UrlUnicodeDecode(s))
}

var urlUnicodeRegexp = regexp.MustCompile(`%u[0-9a-fA-F]{4}`)

func UrlUnicodeDecode(s string) string {
	return urlUnicodeRegexp.ReplaceAllStringFunc(s, func(s string) string {
		raw, err := hex.DecodeString(s[2:])
		if err != nil {
			return s
		}
		return string(raw)
	})
}

func QueryEscape(s string) string {
	return url.QueryEscape(s)
}

var (
	EscapeHtmlString   = html.EscapeString
	UnescapeHtmlString = html.UnescapeString
	StrConvUnquote     = strconv.Unquote
	StrConvQuote       = strconv.Quote
)

func StrConvQuoteHex(s string) string {
	raw := []byte(s)
	var buf bytes.Buffer
	buf.WriteString("\"")
	for _, b := range raw {
		switch true {
		case b >= 'a' && b <= 'z':
			fallthrough
		case b >= 'A' && b <= 'Z':
			fallthrough
		case b >= '0' && b <= '9':
			buf.WriteByte(b)
		default:
			buf.WriteString(fmt.Sprintf(`\x%02x`, b))
		}
	}
	buf.WriteString("\"")
	return buf.String()
}

func StrConvUnquoteForce(s string) []byte {
	raw, err := StrConvUnquote(s)
	if err != nil {
		return []byte(s)
	}
	return []byte(raw)
}

var DoubleEncodeUrl = func(i interface{}) string {
	return url.QueryEscape(EncodeUrlCode(i))
}

var DoubleDecodeUrl = func(i string) (string, error) {
	raw, err := url.QueryUnescape(i)
	if err != nil {
		return "", err
	}

	return url.QueryUnescape(raw)
}

func interfaceToBytes(i interface{}) []byte {
	var bytes []byte

	switch ret := i.(type) {
	case string:
		bytes = []byte(ret)
	case []byte:
		bytes = ret
	case io.Reader:
		bytes, _ = ioutil.ReadAll(ret)
	default:
		bytes = []byte(fmt.Sprint(i))
	}

	return bytes
}

func EncodeToHex(i interface{}) string {
	raw := interfaceToBytes(i)
	return hex.EncodeToString(raw)
}

func DecodeHex(i string) ([]byte, error) {
	if strings.HasPrefix(i, "0x") {
		i = i[2:]
	}
	return hex.DecodeString(i)
}

func EncodeBase64(i interface{}) string {
	return base64.StdEncoding.EncodeToString(interfaceToBytes(i))
}

func EncodeBase32(i interface{}) string {
	return base32.StdEncoding.EncodeToString(interfaceToBytes(i))
}

func EncodeBase64Url(i interface{}) string {
	org := base64.StdEncoding.EncodeToString(interfaceToBytes(i))
	org = strings.TrimRight(org, "=")
	org = strings.ReplaceAll(org, "+", "-")
	org = strings.ReplaceAll(org, "/", "_")
	return org
}

func DecodeBase64Url(i interface{}) ([]byte, error) {
	org := string(interfaceToBytes(i))
	org = strings.ReplaceAll(org, "-", "+")
	org = strings.ReplaceAll(org, "_", "/")
	return DecodeBase64(org)
}

func Base32Padding(i string) string {
	padding := 8 - len(i)%8
	if padding <= 0 || padding == 8 {
		return i
	}
	return i + strings.Repeat("=", padding)
}

func Base64Padding(i string) string {
	padding := 4 - len(i)%4
	if padding <= 0 || padding == 4 {
		return i
	}
	return i + strings.Repeat("=", padding)
}

func DecodeBase64(i string) ([]byte, error) {
	i = strings.TrimSpace(i)
	if strings.Index(i, "%") >= 0 {
		i = ForceQueryUnescape(i)
	}

	if strings.Contains(i, "-") {
		i = strings.ReplaceAll(i, "-", "+")
	}
	if strings.Contains(i, "_") {
		i = strings.ReplaceAll(i, "_", "/")
	}

	padding := 4 - len(i)%4
	if padding <= 0 || padding == 4 {
		return base64.StdEncoding.DecodeString(i)
	}
	return base64.StdEncoding.DecodeString(i + strings.Repeat("=", padding))
}

func DecodeBase32(i string) ([]byte, error) {
	i = strings.TrimSpace(i)
	i = strings.ReplaceAll(i, "%3d", "=")
	i = strings.ReplaceAll(i, "%3D", "=")

	padding := 8 - len(i)%8
	if padding <= 0 || padding == 8 {
		return base32.StdEncoding.DecodeString(i)
	}
	return base32.StdEncoding.DecodeString(i + strings.Repeat("=", padding))
}

func Md5(i interface{}) string {
	raw := md5.Sum(interfaceToBytes(i))
	return EncodeToHex(raw[:])
}

func Sha1(i interface{}) string {
	raw := sha1.Sum(interfaceToBytes(i))
	return EncodeToHex(raw[:])
}

func Sha256(i interface{}) string {
	raw := sha256.Sum256(interfaceToBytes(i))
	return EncodeToHex(raw[:])
}

func HmacSha1(key, data interface{}) []byte {
	kBytes, dataBytes := interfaceToBytes(key), interfaceToBytes(data)
	h := hmac.New(sha1.New, kBytes)
	h.Write(dataBytes)
	return h.Sum(nil)
}

func HmacSha256(key, data interface{}) []byte {
	kBytes, dataBytes := interfaceToBytes(key), interfaceToBytes(data)
	h := hmac.New(sha256.New, kBytes)
	h.Write(dataBytes)
	return h.Sum(nil)
}

func HmacSha512(key, data interface{}) []byte {
	kBytes, dataBytes := interfaceToBytes(key), interfaceToBytes(data)
	h := hmac.New(sha512.New, kBytes)
	h.Write(dataBytes)
	return h.Sum(nil)
}

func HmacSM3(key, data interface{}) []byte {
	kBytes, dataBytes := interfaceToBytes(key), interfaceToBytes(data)
	h := hmac.New(sm3.New, kBytes)
	h.Write(dataBytes)
	return h.Sum(nil)
}

func HmacMD5(key, data interface{}) []byte {
	kBytes, dataBytes := interfaceToBytes(key), interfaceToBytes(data)
	h := hmac.New(md5.New, kBytes)
	h.Write(dataBytes)
	return h.Sum(nil)
}

func Sha224(i interface{}) string {
	raw := sha256.Sum224(interfaceToBytes(i))
	return EncodeToHex(raw[:])
}

func Sha512(i interface{}) string {
	raw := sha512.Sum512(interfaceToBytes(i))
	return EncodeToHex(raw[:])
}

func Sha384(i interface{}) string {
	raw := sha512.Sum384(interfaceToBytes(i))
	return EncodeToHex(raw[:])
}

func MMH3Hash32(i interface{}) int64 {
	return int64(mmh3.Hash32(interfaceToBytes(i)))
}

func MMH3Hash128(i interface{}) string {
	raw := mmh3.Hash128(interfaceToBytes(i))
	return EncodeToHex(raw.Bytes())
}

func MMH3Hash128x64(i interface{}) string {
	raw := mmh3.Hash128x64(interfaceToBytes(i))
	return EncodeToHex(raw)
}

func EncodeHtmlEntityHex(i interface{}) string {
	raw := interfaceToBytes(i)
	scanner := bufio.NewScanner(bytes.NewBuffer(raw))
	scanner.Split(bufio.ScanBytes)

	var buf string
	for scanner.Scan() {
		if len(scanner.Bytes()) <= 0 {
			continue
		}
		buf += fmt.Sprintf("&#x%x;", scanner.Bytes()[0])
	}
	return buf
}

var namedHtmlEntity = strings.NewReplacer(
	`&`, "&amp;",
	`'`, "&apos;",
	`<`, "&lt;",
	`>`, "&gt;",
	`"`, "&quot;",
)

var decHtmlEntity = strings.NewReplacer(
	`&`, "&#38;",
	`'`, "&#39;",
	`<`, "&#60;",
	`>`, "&#62;",
	`"`, "&#34;",
)

var hexHtmlEntity = strings.NewReplacer(
	`&`, "&#x26;",
	`'`, "&#x27;",
	`<`, "&#x3c;",
	`>`, "&#x3e;",
	`"`, "&#x22;",
)

// todo replace HtmlEntityEncode
func EncodeHtmlEntityEx(i interface{}, encodeType string, fullEncode bool) string {
	raw := AnyToString(i)
	if !fullEncode {
		var res string
		switch encodeType {
		case "dec":
			res = decHtmlEntity.Replace(raw)
		case "hex":
			res = hexHtmlEntity.Replace(raw)
		case "named":
			res = namedHtmlEntity.Replace(raw)
		default:
			res = namedHtmlEntity.Replace(raw) // 默认使用 named
		}
		return res
	}

	namedChar := []string{`&`, `'`, `<`, `>`, `"`}
	formatString := "&#%d;"
	if encodeType == "hex" {
		formatString = "&#x%x;"
	}

	var res string
	for _, char := range raw {
		if encodeType == "named" {
			if lo.Contains(namedChar, string(char)) {
				res += namedHtmlEntity.Replace(string(char))
				continue
			}
		}
		res += fmt.Sprintf(formatString, char)
	}
	return res
}

func EncodeHtmlEntity(i interface{}) string {
	raw := interfaceToBytes(i)
	scanner := bufio.NewScanner(bytes.NewBuffer(raw))
	scanner.Split(bufio.ScanBytes)

	var buf string
	for scanner.Scan() {
		if len(scanner.Bytes()) <= 0 {
			continue
		}
		buf += fmt.Sprintf("&#%d;", scanner.Bytes()[0])
	}
	return buf
}

func EncodeUrlCode(i interface{}) string {
	raw := interfaceToBytes(i)
	scanner := bufio.NewScanner(bytes.NewBuffer(raw))
	scanner.Split(bufio.ScanBytes)

	var buf string
	for scanner.Scan() {
		if len(scanner.Bytes()) <= 0 {
			continue
		}
		payload := fmt.Sprintf("%x", scanner.Bytes()[0])
		if len(payload) == 1 {
			payload = "0" + payload
		}
		buf += "%" + payload
	}
	return buf
}

func PKCS5Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padText...)
}

func PKCS5UnPadding(origData []byte) []byte {
	length := len(origData)
	if length == 0 {
		log.Error("input data is empty")
		return origData
	}

	unpadding := int(origData[length-1])
	if unpadding > length {
		log.Debug("invalid padding")
		return origData
	}

	for i := length - unpadding; i < length; i++ {
		if int(origData[i]) != unpadding {
			log.Debug("invalid padding")
			return origData
		}
	}

	return origData[:(length - unpadding)]
}

func MustPKCS5UnPadding(origData []byte) ([]byte, error) {
	length := len(origData)
	if length == 0 {
		return nil, errors.New("input data is empty")
	}

	unpadding := int(origData[length-1])
	if unpadding > length {
		return nil, errors.New("invalid padding")
	}

	for i := length - unpadding; i < length; i++ {
		if int(origData[i]) != unpadding {
			return nil, errors.New("invalid padding")
		}
	}

	return origData[:(length - unpadding)], nil
}

func ZeroPadding(origin []byte, blockSize int) []byte {
	originLen := len(origin)
	if originLen%blockSize == 0 {
		return origin
	} else {
		out := make([]byte, (originLen/blockSize+1)*blockSize)
		copy(out, origin)
		return out
	}
}

func ZeroUnPadding(originData []byte) []byte {
	return bytes.TrimRight(originData, "\x00")
}

func HTTPChunkedDecodeWithRestBytes(raw []byte) ([]byte, []byte) {
	return readHTTPChunkedData(raw)
}

func HTTPChunkedDecoderWithRestBytes(raw io.Reader) ([]byte, []byte, io.Reader, error) {
	return readChunkedDataFromReader(raw)
}

func HTTPChunkedDecode(raw []byte) ([]byte, error) {
	if ret := string(raw); ret == "" {
		return nil, errors.New("empty input")
	} else if ret == "0\r\n\r\n" {
		return nil, nil
	}

	results, _, rest, err := ReadHTTPChunkedDataWithFixedError(raw)
	_ = rest
	if len(results) > 0 {
		return results, nil
	}
	if len(raw) > 128 {
		raw = append(raw[:128], []byte("...")...)
	}
	return nil, errors.Errorf("parse %v to http chunked failed: %v", strconv.Quote(string(raw)), err)
}

var (
	gb18030encoding      encoding.Encoding
	gb18030encodingMutex = new(sync.Mutex)
)

func GB18030ToUtf8(s []byte) ([]byte, error) {
	if gb18030encoding != nil {
		return gb18030encoding.NewDecoder().Bytes(s)
	}

	gb18030encodingMutex.Lock()
	defer gb18030encodingMutex.Unlock()

	if gb18030encoding != nil {
		return gb18030encoding.NewDecoder().Bytes(s)
	}
	var name string
	gb18030encoding, name = charset.Lookup("gb18030")
	if gb18030encoding == nil {
		return nil, fmt.Errorf("failed to lookup gb18030 encoding: %s", name)
	}
	return gb18030encoding.NewDecoder().Bytes(s)
}

func HZGB2312ToUtf8(s []byte) ([]byte, error) {
	return GB18030ToUtf8(s)
}

func GbkToUtf8(s []byte) ([]byte, error) {
	return GB18030ToUtf8(s)
}

func Utf8ToGbk(s []byte) ([]byte, error) {
	reader := transform.NewReader(bytes.NewReader(s), simplifiedchinese.GBK.NewEncoder())
	d, e := ioutil.ReadAll(reader)
	if e != nil {
		return nil, e
	}
	return d, nil
}

func Utf8ToGB18030(s []byte) ([]byte, error) {
	reader := transform.NewReader(bytes.NewReader(s), simplifiedchinese.GB18030.NewEncoder())
	d, e := ioutil.ReadAll(reader)
	if e != nil {
		return nil, e
	}
	return d, nil
}

func AllASCII(i any) bool {
	for _, c := range AnyToBytes(i) {
		if c > 127 {
			return false
		}
	}
	return true
}

func Utf8ToHZGB2312(s []byte) ([]byte, error) {
	reader := transform.NewReader(bytes.NewReader(s), simplifiedchinese.HZGB2312.NewEncoder())
	d, e := ioutil.ReadAll(reader)
	if e != nil {
		return nil, e
	}
	return d, nil
}

func GBKSafeString(s []byte) (string, error) {
	if utf8.Valid(s) {
		return string(s), nil
	}

	raw, err := GbkToUtf8(s)
	if err != nil {
		return "", errors.Errorf("failed to parse gbk: %s", err)
	}

	if utf8.Valid(raw) {
		return string(raw), nil
	}

	return "", errors.Errorf("invalid utf8: %#v", raw)
}

func EscapeInvalidUTF8Byte(s []byte) string {
	// 这个操作返回的结果和原始字符串是非等价的
	ret := make([]rune, 0, len(s)+20)
	start := 0
	for {
		r, size := utf8.DecodeRune(s[start:])
		if r == utf8.RuneError {
			// 说明是空的
			if size == 0 {
				break
			} else {
				// 不是 rune
				ret = append(ret, []rune(fmt.Sprintf("\\x%02x", s[start]))...)
			}
		} else {
			// 不是换行之类的控制字符
			if unicode.IsControl(r) && !unicode.IsSpace(r) {
				ret = append(ret, []rune(fmt.Sprintf("\\x%02x", r))...)
			} else {
				// 正常字符
				ret = append(ret, r)
			}
		}
		start += size
	}
	return string(ret)
}

func CharDetect(raw interface{}) ([]chardet.Result, error) {
	return chardet.NewHtmlDetector().DetectAll(interfaceToBytes(raw))
}

func CharDetectBest(raw interface{}) (*chardet.Result, error) {
	return chardet.NewHtmlDetector().DetectBest(interfaceToBytes(raw))
}

func HTTPChunkedEncode(raw []byte) []byte {
	var buf bytes.Buffer
	writer := httputil.NewChunkedWriter(&buf)

	maxSplit := len(raw) / 2
	if maxSplit <= 0 {
		maxSplit = 47
	}

	offset := 0
	maxBuffer := 3 + rand.Intn(maxSplit)
	for offset < len(raw) {
		end := offset + maxBuffer
		if end > len(raw) {
			end = len(raw)
		}
		chunk := raw[offset:end]
		writer.Write(chunk)
		offset = end
		maxBuffer = 3 + rand.Intn(maxSplit)
	}

	writer.Close()
	buf.WriteString("\r\n")
	return buf.Bytes()
}

func RandomUpperAndLower(s string) string {
	last := _RandomUpperAndLower(s)
	count := 0
	for last == s && count < 10 {
		last = _RandomUpperAndLower(s)
		count++
	}
	return last
}

func _RandomUpperAndLower(s string) string {
	bs := []byte(s)
	for i := 0; i < len(bs); i++ {
		if bs[i] >= 'a' && bs[i] <= 'z' {
			if rand.Intn(2) == 1 {
				bs[i] -= uint8(uint8('a') - uint8('A'))
			}
		} else if bs[i] >= 'A' && bs[i] <= 'Z' {
			if rand.Intn(2) == 1 {
				bs[i] += uint8(uint8('a') - uint8('A'))
			}
		}
	}
	return string(bs)
}
