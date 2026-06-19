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
	"github.com/yaklang/yaklang/common/utils/yakunquote"
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

// QueryUnescape 对 URL 百分号编码的字符串做查询串解码，同时兼容 %uXXXX 形式
// 参数:
//   - s: 待解码的 URL 编码字符串
//
// 返回值:
//   - string: 解码后的字符串
//   - error: 解码失败时返回的错误
//
// Example:
// ```
// // VARS: URL 解码，波浪号自动解包 error
// result = codec.DecodeUrl("a%20b")~
// // STDOUT: 打印可观察输出
// println(result)   // OUT: a b
// // assert: 锁定结论
// assert result == "a b", "DecodeUrl should decode percent-encoding"
// ```
func QueryUnescape(s string) (string, error) {
	return url.QueryUnescape(UrlUnicodeDecode(s))
}

var PathEscape = url.PathEscape

// PathUnescape 对 URL 路径转义的字符串做解码，同时兼容 %uXXXX 形式
// 参数:
//   - s: 待解码的 URL 路径转义字符串
//
// 返回值:
//   - string: 解码后的字符串
//   - error: 解码失败时返回的错误
//
// Example:
// ```
// // VARS: 路径解码，波浪号自动解包 error
// result = codec.UnescapePathUrl("a%20b")~
// // STDOUT: 打印可观察输出
// println(result)   // OUT: a b
// // assert: 锁定结论(与 EscapePathUrl 往返一致)
// assert string(codec.UnescapePathUrl(codec.EscapePathUrl("/api/info"))~) == "/api/info", "path escape/unescape should round-trip"
// ```
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

// QueryEscape 对字符串做 URL 查询串转义，把保留字符(如空格、= 、&)转义为 %xx(空格转为 +)
// 参数:
//   - s: 待转义的字符串
//
// 返回值:
//   - 转义后的查询串字符串
//
// Example:
// ```
// // VARS: 查询串转义
// result = codec.EscapeQueryUrl("a b")
// // STDOUT: 打印可观察输出(空格被转义为 +)
// println(result)   // OUT: a+b
// // assert: 锁定结论
// assert result == "a+b", "EscapeQueryUrl should escape space to plus"
// ```
func QueryEscape(s string) string {
	return url.QueryEscape(s)
}

var (
	EscapeHtmlString   = html.EscapeString
	UnescapeHtmlString = html.UnescapeString
	StrConvUnquote     = strconv.Unquote
	StrConvQuote       = strconv.Quote
)

// StrConvQuoteHex 将字符串转换为带双引号的可打印形式，非字母数字字节统一转义为 \xNN
// 参数:
//   - s: 待转换的字符串
//
// 返回值:
//   - 带双引号、非字母数字字节转义为 \xNN 的字符串
//
// Example:
// ```
// // VARS: 转为可打印形式(EncodeToPrintable / EncodeASCII 同一函数)
// result = codec.EncodeToPrintable("a b")
// // STDOUT: 打印可观察输出(空格被转义为 \x20)
// println(result)   // OUT: "a\x20b"
// // assert: 锁定结论
// assert result == "\"a\\x20b\"", "EncodeToPrintable should hex-escape non-alnum bytes"
// ```
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

// UnescapeString 处理字符串中的转义字符，无需外层引号
// 支持 \" \\ \n \r \t \xNN \uNNNN \UNNNNNNNN 等转义序列；与 codec.StrconvUnquote 不同，本函数不要求输入带引号包裹
// 参数:
//   - s: 含转义序列的字符串(无需外层引号)
//
// 返回值:
//   - string: 解转义后的字符串
//   - error: 解析失败时返回的错误
//
// Example:
// ```
// // VARS: 解转义，波浪号自动解包 error
// result = codec.UnescapeString("a\\nb")~
// // STDOUT: 打印长度(\n 解为单个换行符，总长 3)
// println(len(result))   // OUT: 3
// // assert: 锁定结论(转义序列 \n 被解析为换行)
// assert result == "a\nb", "UnescapeString should unescape \\n to newline"
// ```
func UnescapeString(s string) (string, error) {
	return yakunquote.UnquoteInner(s, 0)
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

// convertSliceInterfaceToBytes 将 []interface{} 转换为 []byte（用于处理 yak VM 返回的数据）
func convertSliceInterfaceToBytes(slice []interface{}) []byte {
	bytes := make([]byte, 0, len(slice))
	for _, item := range slice {
		switch val := item.(type) {
		case byte:
			bytes = append(bytes, val)
		case int:
			if val >= 0 && val <= 255 {
				bytes = append(bytes, byte(val))
			}
		case []byte:
			bytes = append(bytes, val...)
		default:
			// 对于其他类型，尝试使用 AnyToBytes 转换
			if b := AnyToBytes(val); len(b) > 0 {
				bytes = append(bytes, b...)
			}
		}
	}
	return bytes
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
		// 尝试处理 []interface{}（来自 yak VM 的 file.ReadFile 返回值）
		if slice, ok := i.([]interface{}); ok && len(slice) > 0 {
			bytes = convertSliceInterfaceToBytes(slice)
			if len(bytes) > 0 {
				return bytes
			}
		}
		bytes = []byte(fmt.Sprint(i))
	}

	return bytes
}

// EncodeToHex 将输入数据编码为十六进制(Hex)字符串
// 参数:
//   - i: 待编码的数据，可为 string、[]byte 等
//
// 返回值:
//   - 十六进制编码后的字符串
//
// Example:
// ```
// // VARS: 把编码结果赋值给变量
// result = codec.EncodeToHex("abc")
// // STDOUT: 打印可观察输出
// println(result)   // OUT: 616263
// // assert: 锁定结论(与 DecodeHex 往返一致)
// assert result == "616263", "EncodeToHex should hex-encode bytes"
// assert string(codec.DecodeHex(result)~) == "abc", "hex encode/decode should round-trip"
// ```
func EncodeToHex(i interface{}) string {
	raw := interfaceToBytes(i)
	return hex.EncodeToString(raw)
}

// DecodeHex 将十六进制(Hex)字符串解码为原始字节，支持可选的 0x 前缀
// 参数:
//   - i: 待解码的十六进制字符串
//
// 返回值:
//   - []byte: 解码后的原始字节
//   - error: 解码失败时返回的错误
//
// Example:
// ```
// // VARS: 波浪号自动解包 error，得到 []byte
// result = codec.DecodeHex("616263")~
// // STDOUT: 转字符串后打印
// println(string(result))   // OUT: abc
// // assert: 锁定结论
// assert string(result) == "abc", "DecodeHex should decode hex back to bytes"
// ```
func DecodeHex(i string) ([]byte, error) {
	if strings.HasPrefix(i, "0x") {
		i = i[2:]
	}
	return hex.DecodeString(i)
}

// EncodeBase64 将输入数据编码为标准 Base64 字符串
// 参数:
//   - i: 待编码的数据，可为 string、[]byte 等
//
// 返回值:
//   - 标准 Base64 编码后的字符串
//
// Example:
// ```
// // VARS: 把编码结果赋值给变量
// result = codec.EncodeBase64("abc")
// // STDOUT: 打印可观察输出
// println(result)   // OUT: YWJj
// // assert: 锁定结论(与 DecodeBase64 往返一致)
// assert result == "YWJj", "EncodeBase64 should base64-encode bytes"
// assert string(codec.DecodeBase64(result)~) == "abc", "base64 encode/decode should round-trip"
// ```
func EncodeBase64(i interface{}) string {
	return base64.StdEncoding.EncodeToString(interfaceToBytes(i))
}

// EncodeBase32 将输入数据编码为标准 Base32 字符串
// 参数:
//   - i: 待编码的数据，可为 string、[]byte 等
//
// 返回值:
//   - 标准 Base32 编码后的字符串
//
// Example:
// ```
// // VARS: 把编码结果赋值给变量
// result = codec.EncodeBase32("abc")
// // STDOUT: 打印可观察输出
// println(result)   // OUT: MFRGG===
// // assert: 锁定结论(与 DecodeBase32 往返一致)
// assert string(codec.DecodeBase32(result)~) == "abc", "base32 encode/decode should round-trip"
// ```
func EncodeBase32(i interface{}) string {
	return base32.StdEncoding.EncodeToString(interfaceToBytes(i))
}

// EncodeBase64Url 将输入数据编码为 URL 安全的 Base64 字符串(用 - _ 替换 + /，并去掉末尾的 =)
// 参数:
//   - i: 待编码的数据，可为 string、[]byte 等
//
// 返回值:
//   - URL 安全的 Base64 编码字符串
//
// Example:
// ```
// // VARS: 对含 + / 的字节做 URL 安全编码
// result = codec.EncodeBase64Url("\xFB\xFF")
// // STDOUT: 打印可观察输出
// println(result)   // OUT: -_8
// // assert: 锁定结论(与 DecodeBase64Url 往返一致)
// assert string(codec.DecodeBase64Url(codec.EncodeBase64Url("abc"))~) == "abc", "base64url encode/decode should round-trip"
// ```
func EncodeBase64Url(i interface{}) string {
	org := base64.StdEncoding.EncodeToString(interfaceToBytes(i))
	org = strings.TrimRight(org, "=")
	org = strings.ReplaceAll(org, "+", "-")
	org = strings.ReplaceAll(org, "/", "_")
	return org
}

// DecodeBase64Url 将 URL 安全的 Base64 字符串解码为原始字节
// 参数:
//   - i: 待解码的 URL 安全 Base64 字符串
//
// 返回值:
//   - []byte: 解码后的原始字节
//   - error: 解码失败时返回的错误
//
// Example:
// ```
// // VARS: URL 安全 Base64 解码，波浪号自动解包 error
// result = codec.DecodeBase64Url(codec.EncodeBase64Url("abc"))~
// // STDOUT: 转字符串后打印
// println(string(result))   // OUT: abc
// // assert: 锁定结论
// assert string(result) == "abc", "base64url decode should recover origin"
// ```
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

// DecodeBase64 将标准 Base64 字符串解码为原始字节
// 参数:
//   - i: 待解码的标准 Base64 字符串
//
// 返回值:
//   - []byte: 解码后的原始字节
//   - error: 解码失败时返回的错误
//
// Example:
// ```
// // VARS: 波浪号自动解包 error，得到 []byte
// result = codec.DecodeBase64("YWJj")~
// // STDOUT: 转字符串后打印
// println(string(result))   // OUT: abc
// // assert: 锁定结论
// assert string(result) == "abc", "DecodeBase64 should decode base64 back to bytes"
// ```
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

// DecodeBase32 将标准 Base32 字符串解码为原始字节
// 参数:
//   - i: 待解码的标准 Base32 字符串
//
// 返回值:
//   - []byte: 解码后的原始字节
//   - error: 解码失败时返回的错误
//
// Example:
// ```
// // VARS: 波浪号自动解包 error，得到 []byte
// result = codec.DecodeBase32("MFRGG===")~
// // STDOUT: 转字符串后打印
// println(string(result))   // OUT: abc
// // assert: 锁定结论
// assert string(result) == "abc", "DecodeBase32 should decode base32 back to bytes"
// ```
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

// Md5 计算输入数据的 MD5 摘要并返回十六进制字符串
// 参数:
//   - i: 待计算摘要的数据，可为 string、[]byte 等
//
// 返回值:
//   - 32 位十六进制 MD5 摘要字符串
//
// Example:
// ```
// // VARS: 计算 MD5 摘要
// result = codec.Md5("abc")
// // STDOUT: 打印可观察输出
// println(result)   // OUT: 900150983cd24fb0d6963f7d28e17f72
// // assert: 锁定结论(已知摘要 + 固定长度)
// assert result == "900150983cd24fb0d6963f7d28e17f72", "Md5 should match known digest"
// assert len(result) == 32, "Md5 hex length should be 32"
// ```
func Md5(i interface{}) string {
	raw := md5.Sum(interfaceToBytes(i))
	return EncodeToHex(raw[:])
}

// Sha1 计算输入数据的 SHA-1 摘要并返回十六进制字符串
// 参数:
//   - i: 待计算摘要的数据，可为 string、[]byte 等
//
// 返回值:
//   - 40 位十六进制 SHA-1 摘要字符串
//
// Example:
// ```
// // VARS: 计算 SHA-1 摘要
// result = codec.Sha1("abc")
// // STDOUT: 打印可观察输出
// println(result)   // OUT: a9993e364706816aba3e25717850c26c9cd0d89d
// // assert: 锁定结论(已知摘要 + 固定长度)
// assert result == "a9993e364706816aba3e25717850c26c9cd0d89d", "Sha1 should match known digest"
// assert len(result) == 40, "Sha1 hex length should be 40"
// ```
func Sha1(i interface{}) string {
	raw := sha1.Sum(interfaceToBytes(i))
	return EncodeToHex(raw[:])
}

// Sha256 计算输入数据的 SHA-256 摘要并返回十六进制字符串
// 参数:
//   - i: 待计算摘要的数据，可为 string、[]byte 等
//
// 返回值:
//   - 64 位十六进制 SHA-256 摘要字符串
//
// Example:
// ```
// // VARS: 计算 SHA-256 摘要
// result = codec.Sha256("abc")
// // STDOUT: 打印可观察输出
// println(result)   // OUT: ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad
// // assert: 锁定结论(已知摘要 + 固定长度)
// assert result == "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", "Sha256 should match known digest"
// assert len(result) == 64, "Sha256 hex length should be 64"
// ```
func Sha256(i interface{}) string {
	raw := sha256.Sum256(interfaceToBytes(i))
	return EncodeToHex(raw[:])
}

// HmacSha1 使用给定密钥计算数据的 HMAC-SHA1 消息认证码，返回字节切片
// 参数:
//   - key: 密钥，可为 string、[]byte 等
//   - data: 待认证的数据，可为 string、[]byte 等
//
// 返回值:
//   - HMAC-SHA1 结果字节切片(20 字节，转 hex 后长度 40)
//
// Example:
// ```
// // VARS: 计算 HMAC-SHA1 并转 hex
// result = codec.EncodeToHex(codec.HmacSha1("secret_key", "Important Message"))
// // STDOUT: 打印长度
// println(len(result))   // OUT: 40
// // assert: 锁定结论(hex 长度固定为 40)
// assert len(result) == 40, "HmacSha1 hex length should be 40"
// ```
func HmacSha1(key, data interface{}) []byte {
	kBytes, dataBytes := interfaceToBytes(key), interfaceToBytes(data)
	h := hmac.New(sha1.New, kBytes)
	h.Write(dataBytes)
	return h.Sum(nil)
}

// HmacSha256 使用给定密钥计算数据的 HMAC-SHA256 消息认证码，返回字节切片
// 参数:
//   - key: 密钥，可为 string、[]byte 等
//   - data: 待认证的数据，可为 string、[]byte 等
//
// 返回值:
//   - HMAC-SHA256 结果字节切片(32 字节，转 hex 后长度 64)
//
// Example:
// ```
// // VARS: 计算 HMAC-SHA256 并转 hex
// result = codec.EncodeToHex(codec.HmacSha256("secret_key", "Important Message"))
// // STDOUT: 打印长度
// println(len(result))   // OUT: 64
// // assert: 锁定结论(相同输入结果稳定可复现)
// assert len(result) == 64, "HmacSha256 hex length should be 64"
// assert result == codec.EncodeToHex(codec.HmacSha256("secret_key", "Important Message")), "HmacSha256 should be deterministic"
// ```
func HmacSha256(key, data interface{}) []byte {
	kBytes, dataBytes := interfaceToBytes(key), interfaceToBytes(data)
	h := hmac.New(sha256.New, kBytes)
	h.Write(dataBytes)
	return h.Sum(nil)
}

// HmacSha512 使用给定密钥计算数据的 HMAC-SHA512 消息认证码，返回字节切片
// 参数:
//   - key: 密钥，可为 string、[]byte 等
//   - data: 待认证的数据，可为 string、[]byte 等
//
// 返回值:
//   - HMAC-SHA512 结果字节切片(64 字节，转 hex 后长度 128)
//
// Example:
// ```
// // VARS: 计算 HMAC-SHA512 并转 hex
// result = codec.EncodeToHex(codec.HmacSha512("secret_key", "Important Message"))
// // STDOUT: 打印长度
// println(len(result))   // OUT: 128
// // assert: 锁定结论(hex 长度固定为 128)
// assert len(result) == 128, "HmacSha512 hex length should be 128"
// ```
func HmacSha512(key, data interface{}) []byte {
	kBytes, dataBytes := interfaceToBytes(key), interfaceToBytes(data)
	h := hmac.New(sha512.New, kBytes)
	h.Write(dataBytes)
	return h.Sum(nil)
}

// HmacSM3 使用给定密钥计算数据的国密 HMAC-SM3 消息认证码，返回字节切片
// 参数:
//   - key: 密钥，可为 string、[]byte 等
//   - data: 待认证的数据，可为 string、[]byte 等
//
// 返回值:
//   - HMAC-SM3 结果字节切片(32 字节，转 hex 后长度 64)
//
// Example:
// ```
// // VARS: 计算 HMAC-SM3 并转 hex
// result = codec.EncodeToHex(codec.HmacSM3("secret_key", "Important Message"))
// // STDOUT: 打印长度
// println(len(result))   // OUT: 64
// // assert: 锁定结论(hex 长度固定为 64)
// assert len(result) == 64, "HmacSM3 hex length should be 64"
// ```
func HmacSM3(key, data interface{}) []byte {
	kBytes, dataBytes := interfaceToBytes(key), interfaceToBytes(data)
	h := hmac.New(sm3.New, kBytes)
	h.Write(dataBytes)
	return h.Sum(nil)
}

// HmacMD5 使用给定密钥计算数据的 HMAC-MD5 消息认证码，返回字节切片
// 参数:
//   - key: 密钥，可为 string、[]byte 等
//   - data: 待认证的数据，可为 string、[]byte 等
//
// 返回值:
//   - HMAC-MD5 结果字节切片(16 字节，转 hex 后长度 32)
//
// Example:
// ```
// // VARS: 计算 HMAC-MD5 并转 hex
// result = codec.EncodeToHex(codec.HmacMD5("secret_key", "Important Message"))
// // STDOUT: 打印长度
// println(len(result))   // OUT: 32
// // assert: 锁定结论(hex 长度固定为 32)
// assert len(result) == 32, "HmacMD5 hex length should be 32"
// ```
func HmacMD5(key, data interface{}) []byte {
	kBytes, dataBytes := interfaceToBytes(key), interfaceToBytes(data)
	h := hmac.New(md5.New, kBytes)
	h.Write(dataBytes)
	return h.Sum(nil)
}

// Sha224 计算输入数据的 SHA-224 摘要并返回十六进制字符串
// 参数:
//   - i: 待计算摘要的数据，可为 string、[]byte 等
//
// 返回值:
//   - 56 位十六进制 SHA-224 摘要字符串
//
// Example:
// ```
// // VARS: 计算 SHA-224 摘要
// result = codec.Sha224("abc")
// // STDOUT: 打印可观察输出
// println(result)   // OUT: 23097d223405d8228642a477bda255b32aadbce4bda0b3f7e36c9da7
// // assert: 锁定结论(已知摘要 + 固定长度)
// assert result == "23097d223405d8228642a477bda255b32aadbce4bda0b3f7e36c9da7", "Sha224 should match known digest"
// assert len(result) == 56, "Sha224 hex length should be 56"
// ```
func Sha224(i interface{}) string {
	raw := sha256.Sum224(interfaceToBytes(i))
	return EncodeToHex(raw[:])
}

// Sha512 计算输入数据的 SHA-512 摘要并返回十六进制字符串
// 参数:
//   - i: 待计算摘要的数据，可为 string、[]byte 等
//
// 返回值:
//   - 128 位十六进制 SHA-512 摘要字符串
//
// Example:
// ```
// // VARS: 计算 SHA-512 摘要
// result = codec.Sha512("abc")
// // STDOUT: 打印长度
// println(len(result))   // OUT: 128
// // assert: 锁定结论(已知摘要 + 固定长度)
// assert result == "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f", "Sha512 should match known digest"
// assert len(result) == 128, "Sha512 hex length should be 128"
// ```
func Sha512(i interface{}) string {
	raw := sha512.Sum512(interfaceToBytes(i))
	return EncodeToHex(raw[:])
}

// Sha384 计算输入数据的 SHA-384 摘要并返回十六进制字符串
// 参数:
//   - i: 待计算摘要的数据，可为 string、[]byte 等
//
// 返回值:
//   - 96 位十六进制 SHA-384 摘要字符串
//
// Example:
// ```
// // VARS: 计算 SHA-384 摘要
// result = codec.Sha384("abc")
// // STDOUT: 打印长度
// println(len(result))   // OUT: 96
// // assert: 锁定结论(已知摘要 + 固定长度)
// assert result == "cb00753f45a35e8bb5a03d699ac65007272c32ab0eded1631a8b605a43ff5bed8086072ba1e7cc2358baeca134c825a7", "Sha384 should match known digest"
// assert len(result) == 96, "Sha384 hex length should be 96"
// ```
func Sha384(i interface{}) string {
	raw := sha512.Sum384(interfaceToBytes(i))
	return EncodeToHex(raw[:])
}

// MMH3Hash32 计算输入数据的 MurmurHash3 32 位非加密快速哈希，返回数值
// 参数:
//   - i: 待哈希的数据，可为 string、[]byte 等
//
// 返回值:
//   - MurmurHash3 32 位哈希值(int64)
//
// Example:
// ```
// // VARS: 计算 MMH3 32 位哈希
// result = codec.MMH3Hash32("abc")
// // STDOUT: 打印可观察输出
// println(result)   // OUT: 3017643002
// // assert: 锁定结论(确定性哈希)
// assert result == 3017643002, "MMH3Hash32 should match known value"
// ```
func MMH3Hash32(i interface{}) int64 {
	return int64(mmh3.Hash32(interfaceToBytes(i)))
}

// MMH3Hash128 计算输入数据的 MurmurHash3 128 位哈希并返回十六进制字符串
// 参数:
//   - i: 待哈希的数据，可为 string、[]byte 等
//
// 返回值:
//   - 32 位十六进制 MurmurHash3 128 位哈希字符串
//
// Example:
// ```
// // VARS: 计算 MMH3 128 位哈希
// result = codec.MMH3Hash128("abc")
// // STDOUT: 打印可观察输出
// println(result)   // OUT: 6778ad3f3f3f96b4522dca264174a23b
// // assert: 锁定结论(确定性哈希 + 固定长度)
// assert result == "6778ad3f3f3f96b4522dca264174a23b", "MMH3Hash128 should match known value"
// assert len(result) == 32, "MMH3Hash128 hex length should be 32"
// ```
func MMH3Hash128(i interface{}) string {
	raw := mmh3.Hash128(interfaceToBytes(i))
	return EncodeToHex(raw.Bytes())
}

// MMH3Hash128x64 计算输入数据的 MurmurHash3 128 位(x64 变体)哈希并返回十六进制字符串
// 参数:
//   - i: 待哈希的数据，可为 string、[]byte 等
//
// 返回值:
//   - 32 位十六进制 MurmurHash3 128 位(x64)哈希字符串
//
// Example:
// ```
// // VARS: 计算 MMH3 128 位(x64) 哈希
// result = codec.MMH3Hash128x64("abc")
// // STDOUT: 打印可观察输出
// println(result)   // OUT: 6778ad3f3f3f96b4522dca264174a23b
// // assert: 锁定结论(确定性哈希 + 固定长度)
// assert result == "6778ad3f3f3f96b4522dca264174a23b", "MMH3Hash128x64 should match known value"
// assert len(result) == 32, "MMH3Hash128x64 hex length should be 32"
// ```
func MMH3Hash128x64(i interface{}) string {
	raw := mmh3.Hash128x64(interfaceToBytes(i))
	return EncodeToHex(raw)
}

// EncodeHtmlEntityHex 将输入数据的每个字节编码为十六进制 HTML 实体(如 < 编码为 &#x3c;)，常用于 XSS 构造
// 参数:
//   - i: 待编码的数据，可为 string、[]byte 等
//
// 返回值:
//   - 十六进制 HTML 实体字符串
//
// Example:
// ```
// // VARS: 把特殊字符编码为十六进制 HTML 实体
// result = codec.EncodeHtmlHex("<b>")
// // STDOUT: 打印可观察输出
// println(result)   // OUT: &#x3c;&#x62;&#x3e;
// // assert: 锁定结论(可用 DecodeHtml 还原)
// assert string(codec.DecodeHtml(result)~) == "<b>", "EncodeHtmlHex should be decodable back"
// ```
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

// EncodeHtmlEntity 将输入数据的每个字节编码为十进制 HTML 实体(如 < 编码为 &#60;)，常用于 XSS 构造
// 参数:
//   - i: 待编码的数据，可为 string、[]byte 等
//
// 返回值:
//   - 十进制 HTML 实体字符串
//
// Example:
// ```
// // VARS: 把特殊字符编码为十进制 HTML 实体
// result = codec.EncodeHtml("<b>")
// // STDOUT: 打印可观察输出
// println(result)   // OUT: &#60;&#98;&#62;
// // assert: 锁定结论(可用 DecodeHtml 还原)
// assert string(codec.DecodeHtml(result)~) == "<b>", "EncodeHtml should be decodable back"
// ```
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

// EncodeUrlCode 对输入数据做激进的百分号(URL)编码，把每个字节都编码成 %xx 形式
// 参数:
//   - i: 待编码的数据，可为 string、[]byte 等
//
// 返回值:
//   - 百分号编码后的字符串
//
// Example:
// ```
// // VARS: 把每个字节都编码成 %xx
// result = codec.EncodeUrl("abc")
// // STDOUT: 打印可观察输出
// println(result)   // OUT: %61%62%63
// // assert: 锁定结论(可用 DecodeUrl 还原)
// assert string(codec.DecodeUrl(result)~) == "abc", "EncodeUrl should be decodable back"
// ```
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

// PKCS5Padding 对数据按指定块大小做 PKCS5/PKCS7 填充，使其长度补齐到块大小的整数倍
// 参数:
//   - ciphertext: 待填充的原始数据
//   - blockSize: 块大小(字节)，如 8 或 16
//
// 返回值:
//   - 填充后的字节切片
//
// Example:
// ```
// // VARS: 把 2 字节数据填充到 16 字节块
// result = codec.PKCS5Padding([]byte("hi"), 16)
// // STDOUT: 打印长度
// println(len(result))   // OUT: 16
// // assert: 锁定结论(可用 PKCS5UnPadding 去填充)
// assert len(result) == 16, "PKCS5Padding should pad to block size"
// assert string(codec.PKCS5UnPadding(result)) == "hi", "PKCS5 pad/unpad should round-trip"
// ```
func PKCS5Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padText...)
}

// PKCS5UnPadding 去除数据末尾的 PKCS5/PKCS7 填充，返回原始数据
// 参数:
//   - origData: 带填充的数据
//
// 返回值:
//   - 去除填充后的字节切片
//
// Example:
// ```
// // VARS: 先填充再去填充
// padded = codec.PKCS5Padding([]byte("hi"), 16)
// result = codec.PKCS5UnPadding(padded)
// // STDOUT: 转字符串后打印
// println(string(result))   // OUT: hi
// // assert: 锁定结论(还原原始数据)
// assert string(result) == "hi", "PKCS5UnPadding should remove padding"
// ```
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

// ZeroPadding 对数据按指定块大小做零字节(0x00)填充，使其长度补齐到块大小的整数倍
// 参数:
//   - origin: 待填充的原始数据
//   - blockSize: 块大小(字节)，如 8 或 16
//
// 返回值:
//   - 填充后的字节切片
//
// Example:
// ```
// // VARS: 把数据零填充到 16 字节块
// result = codec.ZeroPadding([]byte("Test Data"), 16)
// // STDOUT: 打印长度
// println(len(result))   // OUT: 16
// // assert: 锁定结论(可用 ZeroUnPadding 去填充)
// assert len(result) == 16, "ZeroPadding should pad to block size"
// assert string(codec.ZeroUnPadding(result)) == "Test Data", "Zero pad/unpad should round-trip"
// ```
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

// ZeroUnPadding 去除数据末尾的零字节(0x00)填充，返回原始数据
// 参数:
//   - originData: 带零填充的数据
//
// 返回值:
//   - 去除零填充后的字节切片
//
// Example:
// ```
// // VARS: 先零填充再去填充
// padded = codec.ZeroPadding([]byte("Test Data"), 16)
// result = codec.ZeroUnPadding(padded)
// // STDOUT: 转字符串后打印
// println(string(result))   // OUT: Test Data
// // assert: 锁定结论(还原原始数据)
// assert string(result) == "Test Data", "ZeroUnPadding should remove zero padding"
// ```
func ZeroUnPadding(originData []byte) []byte {
	return bytes.TrimRight(originData, "\x00")
}

func HTTPChunkedDecodeWithRestBytes(raw []byte) ([]byte, []byte) {
	return readHTTPChunkedData(raw)
}

func HTTPChunkedDecoderWithRestBytes(raw io.Reader) ([]byte, []byte, io.Reader, error) {
	return readChunkedDataFromReader(raw)
}

// HTTPChunkedDecode 解码 HTTP Transfer-Encoding: chunked 分块传输数据，还原原始 body
// 参数:
//   - raw: 分块编码后的字节数据
//
// 返回值:
//   - []byte: 解码还原后的原始 body
//   - error: 解码失败时返回的错误
//
// Example:
// ```
// // VARS: 先分块编码再解码，波浪号自动解包 error
// result = codec.DecodeChunked(codec.EncodeChunked("chunked body"))~
// // STDOUT: 转字符串后打印
// println(string(result))   // OUT: chunked body
// // assert: 锁定结论(分块编解码往返一致)
// assert string(result) == "chunked body", "chunked encode/decode should round-trip"
// ```
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

// GB18030ToUtf8 将 GB18030 编码的字节转换为 UTF-8 字节
// 参数:
//   - s: GB18030 编码的字节数据
//
// 返回值:
//   - []byte: 转换后的 UTF-8 字节
//   - error: 转换失败时返回的错误
//
// Example:
// ```
// // VARS: 先转 GB18030 再转回 UTF-8，波浪号自动解包 error
// result = codec.GB18030ToUTF8(codec.UTF8ToGB18030("中文")~)~
// // STDOUT: 转字符串后打印
// println(string(result))   // OUT: 中文
// // assert: 锁定结论(GB18030 与 UTF-8 往返一致)
// assert string(result) == "中文", "GB18030/UTF8 should round-trip"
// ```
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

// HZGB2312ToUtf8 将 HZ-GB2312 编码的字节转换为 UTF-8 字节
// 参数:
//   - s: HZ-GB2312(兼容 GB18030 解码)编码的字节数据
//
// 返回值:
//   - []byte: 转换后的 UTF-8 字节
//   - error: 转换失败时返回的错误
//
// Example:
// ```
// // VARS: GBK 编码的中文再用 HZGB2312ToUTF8 还原，波浪号自动解包 error
// result = codec.HZGB2312ToUTF8(codec.UTF8ToGBK("中文")~)~
// // STDOUT: 转字符串后打印
// println(string(result))   // OUT: 中文
// // assert: 锁定结论(还原为 UTF-8 中文)
// assert string(result) == "中文", "HZGB2312ToUTF8 should recover utf8 chinese"
// ```
func HZGB2312ToUtf8(s []byte) ([]byte, error) {
	return GB18030ToUtf8(s)
}

// GbkToUtf8 将 GBK 编码的字节转换为 UTF-8 字节
// 参数:
//   - s: GBK 编码的字节数据
//
// 返回值:
//   - []byte: 转换后的 UTF-8 字节
//   - error: 转换失败时返回的错误
//
// Example:
// ```
// // VARS: 先转 GBK 再转回 UTF-8，波浪号自动解包 error
// result = codec.GBKToUTF8(codec.UTF8ToGBK("中文")~)~
// // STDOUT: 转字符串后打印
// println(string(result))   // OUT: 中文
// // assert: 锁定结论(GBK 与 UTF-8 往返一致)
// assert string(result) == "中文", "GBK/UTF8 should round-trip"
// ```
func GbkToUtf8(s []byte) ([]byte, error) {
	return GB18030ToUtf8(s)
}

// Utf8ToGbk 将 UTF-8 编码的字节转换为 GBK 编码的字节
// 参数:
//   - s: UTF-8 编码的字节数据
//
// 返回值:
//   - []byte: 转换后的 GBK 字节
//   - error: 转换失败时返回的错误
//
// Example:
// ```
// // VARS: UTF-8 转 GBK 再转回 UTF-8，波浪号自动解包 error
// gbk = codec.UTF8ToGBK("中文")~
// result = codec.GBKToUTF8(gbk)~
// // STDOUT: 转字符串后打印
// println(string(result))   // OUT: 中文
// // assert: 锁定结论(UTF-8 与 GBK 往返一致)
// assert string(result) == "中文", "UTF8/GBK should round-trip"
// ```
func Utf8ToGbk(s []byte) ([]byte, error) {
	reader := transform.NewReader(bytes.NewReader(s), simplifiedchinese.GBK.NewEncoder())
	d, e := ioutil.ReadAll(reader)
	if e != nil {
		return nil, e
	}
	return d, nil
}

// Utf8ToGB18030 将 UTF-8 编码的字节转换为 GB18030 编码的字节
// 参数:
//   - s: UTF-8 编码的字节数据
//
// 返回值:
//   - []byte: 转换后的 GB18030 字节
//   - error: 转换失败时返回的错误
//
// Example:
// ```
// // VARS: UTF-8 转 GB18030 再转回 UTF-8，波浪号自动解包 error
// gb = codec.UTF8ToGB18030("中文")~
// result = codec.GB18030ToUTF8(gb)~
// // STDOUT: 转字符串后打印
// println(string(result))   // OUT: 中文
// // assert: 锁定结论(UTF-8 与 GB18030 往返一致)
// assert string(result) == "中文", "UTF8/GB18030 should round-trip"
// ```
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

// Utf8ToHZGB2312 将 UTF-8 编码的字节转换为 HZ-GB2312 编码的字节
// 参数:
//   - s: UTF-8 编码的字节数据
//
// 返回值:
//   - []byte: 转换后的 HZ-GB2312 字节
//   - error: 转换失败时返回的错误
//
// Example:
// ```
// // VARS: UTF-8 转 HZ-GB2312，结果非空
// result = codec.UTF8ToHZGB2312("中文")~
// // STDOUT: 打印是否非空
// println(len(result) > 0)   // OUT: true
// // assert: 锁定结论(转换得到非空字节)
// assert len(result) > 0, "UTF8ToHZGB2312 should produce bytes"
// ```
func Utf8ToHZGB2312(s []byte) ([]byte, error) {
	reader := transform.NewReader(bytes.NewReader(s), simplifiedchinese.HZGB2312.NewEncoder())
	d, e := ioutil.ReadAll(reader)
	if e != nil {
		return nil, e
	}
	return d, nil
}

// GBKSafeString 将字节数据安全转换为可读字符串：已是合法 UTF-8 则原样返回，否则尝试按 GBK 解码为 UTF-8
// 参数:
//   - s: 待转换的字节数据(可能是 UTF-8 或 GBK)
//
// 返回值:
//   - string: 转换后的可读字符串
//   - error: 既非合法 UTF-8 又无法按 GBK 解码时返回的错误
//
// Example:
// ```
// // VARS: 合法 UTF-8 输入原样返回，波浪号自动解包 error
// result = codec.GBKSafe("hello")~
// // STDOUT: 打印可观察输出
// println(result)   // OUT: hello
// // assert: 锁定结论
// assert result == "hello", "GBKSafe should return valid utf8 as-is"
// ```
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

// EscapeInvalidUTF8Byte 将字节数据中的非法 UTF-8 字节与不可见控制字符转义为 \xNN 形式，得到可读字符串
// 参数:
//   - s: 待修复的字节数据(可能含非法 UTF-8 或控制字符)
//
// 返回值:
//   - 修复/转义后的可读 UTF-8 字符串
//
// Example:
// ```
// // VARS: 合法字符串原样返回
// result = codec.FixUTF8("hello")
// // STDOUT: 打印可观察输出
// println(result)   // OUT: hello
// // assert: 锁定结论(合法输入保持不变)
// assert result == "hello", "FixUTF8 should keep valid string unchanged"
// ```
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

// CharDetect 检测输入数据可能的字符集编码，返回所有候选结果(按置信度排序)
// 参数:
//   - raw: 待检测的数据，可为 string、[]byte 等
//
// 返回值:
//   - []chardet.Result: 候选字符集检测结果列表(含 Charset、Confidence 等字段)
//   - error: 检测失败时返回的错误
//
// Example:
// ```
// // VARS: 检测字符集，波浪号自动解包 error
// results = codec.HTMLChardet("hello world, this is plain english text")~
// // STDOUT: 打印是否得到候选结果
// println(len(results) > 0)   // OUT: true
// // assert: 锁定结论(返回非空候选列表)
// assert len(results) > 0, "HTMLChardet should return candidates"
// ```
func CharDetect(raw interface{}) ([]chardet.Result, error) {
	return chardet.NewHtmlDetector().DetectAll(interfaceToBytes(raw))
}

// CharDetectBest 检测输入数据最可能的字符集编码，返回置信度最高的单个结果
// 参数:
//   - raw: 待检测的数据，可为 string、[]byte 等
//
// 返回值:
//   - *chardet.Result: 置信度最高的字符集检测结果(含 Charset、Confidence 等字段)
//   - error: 检测失败时返回的错误
//
// Example:
// ```
// // VARS: 检测最佳字符集，波浪号自动解包 error
// best = codec.HTMLChardetBest("hello world, this is plain english text")~
// // STDOUT: 打印是否检测到结果
// println(best != nil)   // OUT: true
// // assert: 锁定结论(返回非空结果)
// assert best != nil, "HTMLChardetBest should return a result"
// ```
func CharDetectBest(raw interface{}) (*chardet.Result, error) {
	return chardet.NewHtmlDetector().DetectBest(interfaceToBytes(raw))
}

// HTTPChunkedEncode 将原始数据编码为 HTTP Transfer-Encoding: chunked 分块传输格式
// 参数:
//   - raw: 待编码的原始 body 字节
//
// 返回值:
//   - 分块编码后的字节数据
//
// Example:
// ```
// // VARS: 先分块编码再解码，验证往返一致
// encoded = codec.EncodeChunked("chunked body")
// result = codec.DecodeChunked(encoded)~
// // STDOUT: 转字符串后打印
// println(string(result))   // OUT: chunked body
// // assert: 锁定结论(分块编解码往返一致)
// assert string(result) == "chunked body", "EncodeChunked should be decodable back"
// ```
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
