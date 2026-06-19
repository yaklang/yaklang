package yaklib

import (
	"encoding/json"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/xhtml"

	"github.com/yaklang/yaklang/common/domainextractor"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/network"
	"github.com/yaklang/yaklang/common/utils/suspect"
)

// IndexAny 返回字符串s中chars任意字符首次出现的位置的索引，如果字符串中不存在chars，则返回-1
// 参数:
//   - s: 原始字符串
//   - chars: 待匹配的字符集合
//
// 返回值:
//   - 任意字符首次出现的下标，未找到返回 -1
//
// Example:
// ```
// // l 在 "Hello world" 第三个字符首次命中 chars
// idx = str.IndexAny("Hello world", "world")
// println(idx)   // OUT: 2
// assert idx == 2, "IndexAny should locate first matching char"
// // chars 中字符都不出现时返回 -1
// assert str.IndexAny("Hello World", "Yak") == -1, "IndexAny should return -1 when no char matches"
// ```
func IndexAny(s string, chars string) int {
	return strings.IndexAny(s, chars)
}

// StartsWith / HasPrefix 判断字符串s是否以prefix开头
// 参数:
//   - s: 原始字符串
//   - prefix: 要判断的前缀
//
// 返回值:
//   - 是否以该前缀开头
//
// Example:
// ```
// ok = str.StartsWith("Hello Yak", "Hello")
// println(ok)   // OUT: true
// assert ok == true, "StartsWith should match prefix"
// assert str.StartsWith("Hello Yak", "Yak") == false, "StartsWith should reject non-prefix"
// ```
func StartsWith(s string, prefix string) bool {
	return strings.HasPrefix(s, prefix)
}

// EndsWith / HasSuffix 判断字符串s是否以suffix结尾
// 参数:
//   - s: 原始字符串
//   - suffix: 要判断的后缀
//
// 返回值:
//   - 是否以该后缀结尾
//
// Example:
// ```
// ok = str.EndsWith("Hello Yak", "Yak")
// println(ok)   // OUT: true
// assert ok == true, "EndsWith should match suffix"
// assert str.EndsWith("Hello Yak", "Hello") == false, "EndsWith should reject non-suffix"
// ```
func EndsWith(s string, suffix string) bool {
	return strings.HasSuffix(s, suffix)
}

// Title 返回字符串s的标题化版本，即所有单词的首字母都是大写的
// 参数:
//   - s: 原始字符串
//
// 返回值:
//   - 每个单词首字母大写后的字符串
//
// Example:
// ```
// result = str.Title("hello yak")
// println(result)   // OUT: Hello Yak
// assert result == "Hello Yak", "Title should capitalize each word"
// ```
func Title(s string) string {
	return strings.Title(s)
}

// Join 将i中的元素用d连接，如果传入的参数不是字符串，会自动将其转为字符串，再将其用d连接。如果连接失败，则会返回i的字符串形式。
// 参数:
//   - i: 要连接的切片或可迭代对象
//   - d: 连接用的分隔符
//
// 返回值:
//   - 连接后的字符串
//
// Example:
// ```
// result = str.Join(["hello", "yak"], " ")
// println(result)   // OUT: hello yak
// assert result == "hello yak", "Join should join string slice with sep"
// assert str.Join([1, 2, 3], " ") == "1 2 3", "Join should stringify non-string elements"
// ```
func Join(i interface{}, d interface{}) (defaultResult string) {
	s := utils.InterfaceToString(d)
	defaultResult = utils.InterfaceToString(i)
	defer func() {
		recover()
	}()
	defaultResult = strings.Join(funk.Map(i, func(element interface{}) string {
		return utils.InterfaceToString(element)
	}).([]string), s)
	return
}

// TrimLeft 返回将字符串s左侧所有包含cutset字符串中的字符都去掉的字符串
// 参数:
//   - s: 原始字符串
//   - cutset: 要从左侧去除的字符集合
//
// 返回值:
//   - 去除左侧字符后的字符串
//
// Example:
// ```
// result = str.TrimLeft("Hello Yak", "H")
// println(result)   // OUT: ello Yak
// assert result == "ello Yak", "TrimLeft should trim leading chars in cutset"
// ```
func TrimLeft(s string, cutset string) string {
	return strings.TrimLeft(s, cutset)
}

// TrimPrefix 返回将字符串s前缀prefix去掉的字符串
// 参数:
//   - s: 原始字符串
//   - prefix: 要去除的前缀
//
// 返回值:
//   - 去除前缀后的字符串；若不以prefix开头则原样返回
//
// Example:
// ```
// result = str.TrimPrefix("HelloYak", "Hello")
// println(result)   // OUT: Yak
// assert result == "Yak", "TrimPrefix should drop the prefix"
// ```
func TrimPrefix(s string, prefix string) string {
	return strings.TrimPrefix(s, prefix)
}

// TrimRight 返回将字符串s右侧所有包含cutset字符串中的字符都去掉的字符串
// 参数:
//   - s: 原始字符串
//   - cutset: 要从右侧去除的字符集合
//
// 返回值:
//   - 去除右侧字符后的字符串
//
// Example:
// ```
// result = str.TrimRight("Hello Yak", "k")
// println(result)   // OUT: Hello Ya
// assert result == "Hello Ya", "TrimRight should trim trailing chars in cutset"
// ```
func TrimRight(s string, cutset string) string {
	return strings.TrimRight(s, cutset)
}

// TrimSuffix 返回将字符串s后缀suffix去掉的字符串
// 参数:
//   - s: 原始字符串
//   - suffix: 要去除的后缀
//
// 返回值:
//   - 去除后缀后的字符串；若不以suffix结尾则原样返回
//
// Example:
// ```
// result = str.TrimSuffix("HelloYak", "Yak")
// println(result)   // OUT: Hello
// assert result == "Hello", "TrimSuffix should drop the suffix"
// ```
func TrimSuffix(s string, suffix string) string {
	return strings.TrimSuffix(s, suffix)
}

// Trim 返回将字符串s两侧所有包含cutset字符串中的字符都去掉的字符串
// 参数:
//   - s: 原始字符串
//   - cutset: 要从两侧去除的字符集合
//
// 返回值:
//   - 去除两侧字符后的字符串
//
// Example:
// ```
// result = str.Trim("Hello Yak", "Hk")
// println(result)   // OUT: ello Ya
// assert result == "ello Ya", "Trim should trim chars in cutset from both sides"
// ```
func Trim(s string, cutset string) string {
	return strings.Trim(s, cutset)
}

// TrimSpace 返回将字符串s两侧所有的空白字符都去掉的字符串
// 参数:
//   - s: 原始字符串
//
// 返回值:
//   - 去除两侧空白字符后的字符串
//
// Example:
// ```
// result = str.TrimSpace(" \t\n Hello Yak \n\t\r\n")
// println(result)   // OUT: Hello Yak
// assert result == "Hello Yak", "TrimSpace should strip surrounding whitespace"
// ```
func TrimSpace(s string) string {
	return strings.TrimSpace(s)
}

// Split 将字符串s按照sep分割成字符串切片
// 参数:
//   - s: 原始字符串
//   - sep: 分隔符
//
// 返回值:
//   - 分割后的字符串切片
//
// Example:
// ```
// parts = str.Split("Hello Yak", " ")
// println(parts)   // OUT: [Hello Yak]
// assert len(parts) == 2, "Split should produce two parts"
// assert parts[0] == "Hello" && parts[1] == "Yak", "Split should split by separator"
// ```
func Split(s string, sep string) []string {
	return strings.Split(s, sep)
}

// SplitAfter 将字符串s按照sep分割成字符串切片，但是每个元素都会保留sep
// 参数:
//   - s: 原始字符串
//   - sep: 分隔符
//
// 返回值:
//   - 分割后的字符串切片，每个元素保留结尾的分隔符
//
// Example:
// ```
// parts = str.SplitAfter("Hello-Yak", "-")
// println(parts)   // OUT: [Hello- Yak]
// assert len(parts) == 2, "SplitAfter should produce two parts"
// assert parts[0] == "Hello-" && parts[1] == "Yak", "SplitAfter keeps the separator at element tail"
// ```
func SplitAfter(s string, sep string) []string {
	return strings.SplitAfter(s, sep)
}

// SplitAfterN 将字符串s按照sep分割成字符串切片，但是每个元素都会保留sep，最多分为n个元素
// 参数:
//   - s: 原始字符串
//   - sep: 分隔符
//   - n: 最多分割出的元素个数
//
// 返回值:
//   - 最多n个元素的字符串切片，每个元素保留结尾的分隔符
//
// Example:
// ```
// parts = str.SplitAfterN("Hello-Yak-and-World", "-", 2)
// println(parts)   // OUT: [Hello- Yak-and-World]
// assert len(parts) == 2, "SplitAfterN should cap to n parts"
// assert parts[0] == "Hello-" && parts[1] == "Yak-and-World", "SplitAfterN keeps separator and stops at n"
// ```
func SplitAfterN(s string, sep string, n int) []string {
	return strings.SplitAfterN(s, sep, n)
}

// SplitN 将字符串s按照sep分割成字符串切片，最多分为n个元素
// 参数:
//   - s: 原始字符串
//   - sep: 分隔符
//   - n: 最多分割出的元素个数
//
// 返回值:
//   - 最多n个元素的字符串切片
//
// Example:
// ```
// parts = str.SplitN("Hello-Yak-and-World", "-", 2)
// println(parts)   // OUT: [Hello Yak-and-World]
// assert len(parts) == 2, "SplitN should cap to n parts"
// assert parts[0] == "Hello" && parts[1] == "Yak-and-World", "SplitN should stop splitting at n"
// ```
func SplitN(s string, sep string, n int) []string {
	return strings.SplitN(s, sep, n)
}

// ToLower 返回字符串s的小写形式
// 参数:
//   - s: 原始字符串
//
// 返回值:
//   - 全部转为小写后的字符串
//
// Example:
// ```
// result = str.ToLower("HELLO YAK")
// println(result)   // OUT: hello yak
// assert result == "hello yak", "ToLower should lowercase all letters"
// ```
func ToLower(s string) string {
	return strings.ToLower(s)
}

// ToUpper 返回字符串s的大写形式
// 参数:
//   - s: 原始字符串
//
// 返回值:
//   - 全部转为大写后的字符串
//
// Example:
// ```
// result = str.ToUpper("hello yak")
// println(result)   // OUT: HELLO YAK
// assert result == "HELLO YAK", "ToUpper should uppercase all letters"
// ```
func ToUpper(s string) string {
	return strings.ToUpper(s)
}

// Repeat 返回将字符串s重复count次的字符串
// 参数:
//   - s: 原始字符串
//   - count: 重复次数
//
// 返回值:
//   - 重复拼接后的字符串
//
// Example:
// ```
// result = str.Repeat("hello", 3)
// println(result)   // OUT: hellohellohello
// assert result == "hellohellohello", "Repeat should repeat the string count times"
// ```
func Repeat(s string, count int) string {
	return strings.Repeat(s, count)
}

// ToTitle 返回字符串s的标题化版本，其中所有Unicode字母都会被转换为其大写
// 参数:
//   - s: 原始字符串
//
// 返回值:
//   - 全部字母转为大写（Unicode title）后的字符串
//
// Example:
// ```
// result = str.ToTitle("hello yak")
// println(result)   // OUT: HELLO YAK
// assert result == "HELLO YAK", "ToTitle should upper-case all letters"
// ```
func ToTitle(s string) string {
	return strings.ToTitle(s)
}

// Contains 判断字符串s是否包含substr
// 参数:
//   - s: 原始字符串
//   - substr: 要查找的子串
//
// 返回值:
//   - 是否包含该子串（区分大小写）
//
// Example:
// ```
// ok = str.Contains("hello yakit", "yak")
// println(ok)   // OUT: true
// assert ok == true, "Contains should detect substring"
// assert str.Contains("hello yakit", "Yak") == false, "Contains is case sensitive"
// ```
func Contains(s string, substr string) bool {
	return strings.Contains(s, substr)
}

// ReplaceAll 返回将字符串s中所有old字符串替换为new字符串的字符串
// 参数:
//   - s: 原始字符串
//   - old: 要被替换的子串
//   - new: 替换成的子串
//
// 返回值:
//   - 替换全部匹配后的字符串
//
// Example:
// ```
// result = str.ReplaceAll("hello yak", "yak", "yakit")
// println(result)   // OUT: hello yakit
// assert result == "hello yakit", "ReplaceAll should replace all occurrences"
// ```
func ReplaceAll(s string, old string, new string) string {
	return strings.ReplaceAll(s, old, new)
}

// Replace 返回将字符串s中前n个old字符串替换为new字符串的字符串
// 参数:
//   - s: 原始字符串
//   - old: 要被替换的子串
//   - new: 替换成的子串
//   - n: 最多替换的次数（-1 表示全部）
//
// 返回值:
//   - 替换后的字符串
//
// Example:
// ```
// result = str.Replace("hello yak", "l", "L", 1)
// println(result)   // OUT: heLlo yak
// assert result == "heLlo yak", "Replace should replace only the first n occurrences"
// ```
func Replace(s string, old string, new string, n int) string {
	return strings.Replace(s, old, new, n)
}

// NewReader 返回一个从字符串s读取数据的*Reader
// 参数:
//   - s: 作为数据源的字符串
//
// 返回值:
//   - 可读取该字符串内容的 *strings.Reader
//
// Example:
// ```
// r = str.NewReader("hello yak")
// // Len 返回尚未读取的字节数
// println(r.Len())   // OUT: 9
// assert r.Len() == 9, "NewReader should expose remaining length"
// ```
func NewReader(s string) *strings.Reader {
	return strings.NewReader(s)
}

// Index 返回字符串s中substr第一次出现的位置的索引，如果字符串中不存在substr，则返回-1
// 参数:
//   - s: 原始字符串
//   - substr: 要查找的子串
//
// 返回值:
//   - 首次出现的下标，未找到返回 -1
//
// Example:
// ```
// idx = str.Index("hello yak", "yak")
// println(idx)   // OUT: 6
// assert idx == 6, "Index should return the first match position"
// assert str.Index("hello world", "yak") == -1, "Index should return -1 when not found"
// ```
func Index(s string, substr string) int {
	return strings.Index(s, substr)
}

// Count 返回字符串s中substr出现的次数
// 参数:
//   - s: 原始字符串
//   - substr: 要统计的子串
//
// 返回值:
//   - 不重叠出现的次数
//
// Example:
// ```
// count = str.Count("hello yak", "l")
// println(count)   // OUT: 2
// assert count == 2, "Count should count non-overlapping occurrences"
// ```
func Count(s string, substr string) int {
	return strings.Count(s, substr)
}

// Compare 按照ascii码表顺序逐个比较字符串a和b中的每个字符，如果a==b，则返回0，如果a<b，则返回-1，如果a>b，则返回1
// 参数:
//   - a: 第一个字符串
//   - b: 第二个字符串
//
// 返回值:
//   - 比较结果：相等为0，a<b为-1，a>b为1
//
// Example:
// ```
// result = str.Compare("hello yak", "hello yak")
// println(result)   // OUT: 0
// assert result == 0, "Compare should return 0 for equal strings"
// assert str.Compare("hello yak", "hello") == 1, "Compare should return 1 when a > b"
// assert str.Compare("hello", "hello yak") == -1, "Compare should return -1 when a < b"
// ```
func Compare(a string, b string) int {
	return strings.Compare(a, b)
}

// ContainsAny 判断字符串s是否包含chars中的任意字符
// 参数:
//   - s: 原始字符串
//   - chars: 待匹配的字符集合
//
// 返回值:
//   - 是否包含其中任意一个字符
//
// Example:
// ```
// ok = str.ContainsAny("hello yak", "ly")
// println(ok)   // OUT: true
// assert ok == true, "ContainsAny should match any char present"
// assert str.ContainsAny("hello yak", "m") == false, "ContainsAny should fail when none present"
// ```
func ContainsAny(s string, chars string) bool {
	return strings.ContainsAny(s, chars)
}

// EqualFold 判断字符串s和t是否相等，忽略大小写
// 参数:
//   - s: 第一个字符串
//   - t: 第二个字符串
//
// 返回值:
//   - 忽略大小写后是否相等
//
// Example:
// ```
// ok = str.EqualFold("hello Yak", "HELLO YAK")
// println(ok)   // OUT: true
// assert ok == true, "EqualFold should ignore case"
// ```
func EqualFold(s string, t string) bool {
	unicode.IsSpace('a')
	return strings.EqualFold(s, t)
}

// Fields 返回将字符串s按照空白字符（'\t', '\n', '\v', '\f', '\r', ' ', 0x85, 0xA0）分割的字符串切片
// 参数:
//   - s: 原始字符串
//
// 返回值:
//   - 按连续空白分割后的非空字段切片
//
// Example:
// ```
// fields = str.Fields("hello world\nhello yak\tand\vyakit")
// println(fields)   // OUT: [hello world hello yak and yakit]
// assert len(fields) == 6, "Fields should split on any whitespace run"
// assert fields[0] == "hello" && fields[5] == "yakit", "Fields should keep tokens in order"
// ```
func Fields(s string) []string {
	return strings.Fields(s)
}

// IndexByte 返回字符串s中第一个等于c的字符的索引，如果字符串中不存在c，则返回-1
// 参数:
//   - s: 原始字符串
//   - c: 要查找的字节
//
// 返回值:
//   - 首次出现的下标，未找到返回 -1
//
// Example:
// ```
// idx = str.IndexByte("hello yak", 'y')
// println(idx)   // OUT: 6
// assert idx == 6, "IndexByte should return the first byte position"
// assert str.IndexByte("hello yak", 'm') == -1, "IndexByte should return -1 when not found"
// ```
func IndexByte(s string, c byte) int {
	return strings.IndexByte(s, c)
}

// LastIndex 返回字符串s中substr最后一次出现的位置的索引，如果字符串中不存在substr，则返回-1
// 参数:
//   - s: 原始字符串
//   - substr: 要查找的子串
//
// 返回值:
//   - 最后一次出现的下标，未找到返回 -1
//
// Example:
// ```
// idx = str.LastIndex("hello yak", "l")
// println(idx)   // OUT: 3
// assert idx == 3, "LastIndex should return the last match position"
// assert str.LastIndex("hello yak", "m") == -1, "LastIndex should return -1 when not found"
// ```
func LastIndex(s string, substr string) int {
	return strings.LastIndex(s, substr)
}

// LastIndexAny 返回字符串s中chars任意字符最后一次出现的位置的索引，如果字符串中不存在chars，则返回-1
// 参数:
//   - s: 原始字符串
//   - chars: 待匹配的字符集合
//
// 返回值:
//   - 任意字符最后一次出现的下标，未找到返回 -1
//
// Example:
// ```
// idx = str.LastIndexAny("hello yak", "ly")
// println(idx)   // OUT: 6
// assert idx == 6, "LastIndexAny should return last position of any char"
// assert str.LastIndexAny("hello yak", "m") == -1, "LastIndexAny should return -1 when none present"
// ```
func LastIndexAny(s string, chars string) int {
	return strings.LastIndexAny(s, chars)
}

// LastIndexByte 返回字符串s中最后一个等于c的字符的索引，如果字符串中不存在c，则返回-1
// 参数:
//   - s: 原始字符串
//   - c: 要查找的字节
//
// 返回值:
//   - 最后一次出现的下标，未找到返回 -1
//
// Example:
// ```
// idx = str.LastIndexByte("hello yak", 'l')
// println(idx)   // OUT: 3
// assert idx == 3, "LastIndexByte should return last byte position"
// assert str.LastIndexByte("hello yak", 'm') == -1, "LastIndexByte should return -1 when not found"
// ```
func LastIndexByte(s string, c byte) int {
	return strings.LastIndexByte(s, c)
}

// ToValidUTF8 返回将字符串s中无效的UTF-8编码替换为replacement的字符串
// 参数:
//   - s: 原始字符串
//   - replacement: 无效 UTF-8 字节序列的替换串
//
// 返回值:
//   - 修正后的合法 UTF-8 字符串
//
// Example:
// ```
// result = str.ToValidUTF8("hello yak", "?")
// println(result)   // OUT: hello yak
// assert result == "hello yak", "ToValidUTF8 should keep valid input unchanged"
// ```
func ToValidUTF8(s string, replacement string) string {
	return strings.ToValidUTF8(s, replacement)
}

// ExtractJson 尝试提取字符串中的 JSON 并进行修复, 返回中的元素都是Object
// 参数:
//   - i: 任意可转为字符串的输入
//
// 返回值:
//   - 提取并修复后的 JSON 对象字符串数组
//
// Example:
// ```
// objs = str.ExtractJson(`{"hello": "yak"}`)
// println(len(objs))   // OUT: 1
// assert len(objs) == 1, "ExtractJson should extract one json object"
// assert len(str.ExtractJson("hello yak")) == 0, "ExtractJson should return empty when no json present"
// ```
func extractValidJson(i interface{}) []string {
	return jsonextractor.ExtractObjectsOnly(utils.InterfaceToString(i))
}

// ExtractJsonWithRaw 尝试提取字符串中的 JSON 并返回，第一个返回值返回经过修复后的JSON字符串数组，第二个返回值返回原始JSON字符串数组(如果修复失败)
// 参数:
//   - i: 任意可转为字符串的输入
//
// 返回值:
//   - 修复后的 JSON 字符串数组
//   - 修复失败时的原始 JSON 字符串数组
//
// Example:
// ```
// fixed, raw = str.ExtractJsonWithRaw(`{"hello": "yak"}`)
// println(len(fixed))   // OUT: 1
// assert len(fixed) == 1, "ExtractJsonWithRaw should extract one json object"
// ```
func extractJsonEx(i interface{}) ([]string, []string) {
	return jsonextractor.ExtractJSONWithRaw(utils.InterfaceToString(i))
}

// ExtractDomain 尝试提取字符串中的域名并返回，后续可以接收一个 tryDecode 参数，如果传入 true，则会尝试对输入的文本进行解码(双重URL编码，URL编码，unicode编码，quoted编码)
// 参数:
//   - i: 任意可转为字符串的输入
//   - tryDecode: 可选，是否在提取前尝试解码
//
// 返回值:
//   - 提取到的域名数组
//
// Example:
// ```
// domains = str.ExtractDomain("hello yaklang.com or yaklang.io")
// println(len(domains))   // OUT: 2
// assert len(domains) == 2, "ExtractDomain should extract both domains"
// ```
func extractDomain(i any, tryDecode ...bool) []string {
	return domainextractor.ExtractDomains(utils.InterfaceToString(i), tryDecode...)
}

// ExtractRootDomain 尝试提取字符串中的根域名并返回
// 参数:
//   - i: 任意可转为字符串的输入
//
// 返回值:
//   - 提取到的根域名数组
//
// Example:
// ```
// roots = str.ExtractRootDomain("hello www.yaklang.com or www.yaklang.io")
// println(len(roots))   // OUT: 2
// assert len(roots) == 2, "ExtractRootDomain should extract both root domains"
// ```
func extractRootDomain(i interface{}) []string {
	return domainextractor.ExtractRootDomains(utils.InterfaceToString(i))
}

// ExtractTitle 尝试将传入的字符串进行HTML解析并提取其中的标题(title标签)返回
// 参数:
//   - i: 任意可转为字符串的 HTML 输入
//
// 返回值:
//   - 提取到的标题文本，未找到返回空字符串
//
// Example:
// ```
// title = str.ExtractTitle("<title>hello yak</title>")
// println(title)   // OUT: hello yak
// assert title == "hello yak", "ExtractTitle should read the title tag"
// ```
func extractTitle(i interface{}) string {
	return utils.ExtractTitleFromHTMLTitle(utils.InterfaceToString(i), "")
}

// PathJoin 将传入的文件路径进行拼接并返回
// 参数:
//   - elem: 任意数量的路径片段
//
// 返回值:
//   - 用系统分隔符拼接后的路径
//
// Example:
// ```
// // *nix 下使用 / 作为分隔符
// p = str.PathJoin("/var", "www", "html")
// println(p)   // OUT: /var/www/html
// assert p == "/var/www/html", "PathJoin should join path segments"
// ```
func pathJoin(elem ...string) (newPath string) {
	return filepath.Join(elem...)
}

// ToJsonIndentStr 将v转换为格式化的JSON字符串并返回，如果转换失败，则返回空字符串
// 参数:
//   - d: 任意可序列化为 JSON 的对象
//
// 返回值:
//   - 带缩进的 JSON 字符串，失败返回空字符串
//
// Example:
// ```
// s = str.ToJsonIndentStr({"hello": "yak"})
// // 输出为带缩进的多行 JSON，这里打印是否包含关键字段
// println(str.Contains(s, "hello"))   // OUT: true
// assert str.Contains(s, "hello") && str.Contains(s, "yak"), "ToJsonIndentStr should serialize keys and values"
// ```
func toJsonIndentStr(d interface{}) string {
	raw, err := json.MarshalIndent(d, "", "    ")
	if err != nil {
		return ""
	}
	return string(raw)
}

// RandomUpperAndLower 返回一个随机大小写的字符串
// 参数:
//   - s: 原始字符串
//
// 返回值:
//   - 随机切换字母大小写后的字符串（字母内容不变）
//
// Example:
// ```
// out = str.RandomUpperAndLower("target")
// // 仅大小写变化，小写化后应与原串相同
// println(str.ToLower(out))   // OUT: target
// assert str.ToLower(out) == "target", "RandomUpperAndLower should only flip letter case"
// ```
func randomUpperAndLower(s string) string {
	return xhtml.RandomUpperAndLower(s)
}

var StringsExport = map[string]interface{}{
	// 基础字符串工具
	"IndexAny":       IndexAny,
	"StartsWith":     StartsWith,
	"EndsWith":       EndsWith,
	"Title":          Title,
	"Join":           Join,
	"TrimLeft":       TrimLeft,
	"TrimPrefix":     TrimPrefix,
	"TrimRight":      TrimRight,
	"TrimSuffix":     TrimSuffix,
	"Trim":           Trim,
	"TrimSpace":      TrimSpace,
	"Split":          Split,
	"SplitAfter":     SplitAfter,
	"SplitAfterN":    SplitAfterN,
	"SplitN":         SplitN,
	"ToLower":        ToLower,
	"ToUpper":        ToUpper,
	"HasPrefix":      StartsWith,
	"HasSuffix":      EndsWith,
	"Repeat":         Repeat,
	"ToTitleSpecial": strings.ToTitleSpecial,
	"ToTitle":        ToTitle,
	"Contains":       Contains,
	"ReplaceAll":     ReplaceAll,
	"Replace":        Replace,
	"NewReader":      strings.NewReader,
	"Index":          Index,
	"Count":          Count,
	"Compare":        Compare,
	"ContainsAny":    ContainsAny,
	"EqualFold":      EqualFold,
	"Fields":         Fields,
	"IndexByte":      IndexByte,
	"LastIndex":      LastIndex,
	"LastIndexAny":   LastIndexAny,
	"LastIndexByte":  LastIndexByte,
	"ToLowerSpecial": strings.ToLowerSpecial,
	"ToUpperSpecial": strings.ToUpperSpecial,
	"ToValidUTF8":    ToValidUTF8,
	"Quote":          strconv.Quote,
	"Unquote":        strconv.Unquote,

	// 特有的
	"RandStr":             utils.RandStringBytes,
	"Random":              randomUpperAndLower,
	"RandomUpperAndLower": randomUpperAndLower,
	"RandSecret":          utils.RandSecret,

	// 其他
	"f":                      _sfmt,
	"SplitAndTrim":           utils.PrettifyListFromStringSplited,
	"StringSliceContains":    utils.StringSliceContain,
	"StringSliceContainsAll": utils.StringSliceContainsAll,
	"RemoveRepeat":           utils.RemoveRepeatStringSlice,
	"IsStrongPassword":       utils.IsStrongPassword,
	"ExtractStrContext":      utils.ExtractStrContextByKeyword,

	// 支持 url、host:port 的解析成 Host Port
	"CalcSimilarity":                    utils.CalcSimilarity,
	"CalcTextMaxSubStrStability":        utils.CalcTextSubStringStability,
	"CalcSSDeepStability":               utils.CalcSSDeepStability,
	"CalcSimHashStability":              utils.CalcSimHashStability,
	"CalcSimHash":                       utils.SimHash,
	"CalcSSDeep":                        utils.SSDeepHash,
	"ParseStringToHostPort":             utils.ParseStringToHostPort,
	"IsIPv6":                            utils.IsIPv6,
	"IsIPv4":                            utils.IsIPv4,
	"StringContainsAnyOfSubString":      utils.StringContainsAnyOfSubString,
	"ExtractHost":                       utils.ExtractHost,
	"ExtractHostPort":                   utils.ExtractHostPort,
	"ExtractDomain":                     extractDomain,
	"ExtractRootDomain":                 extractRootDomain,
	"ExtractJson":                       extractValidJson,
	"ExtractJsonWithRaw":                extractJsonEx,
	"ExtractURLFromHTTPRequestRaw":      lowhttp.ExtractURLFromHTTPRequestRaw,
	"ExtractURLFromHTTPRequest":         lowhttp.ExtractURLFromHTTPRequest,
	"ExtractTitle":                      extractTitle,
	"LowerAndTrimSpace":                 utils.StringLowerAndTrimSpace,
	"HostPort":                          utils.HostPort,
	"ParseStringToHTTPRequest":          lowhttp.ParseStringToHttpRequest,
	"SplitHostsToPrivateAndPublic":      utils.SplitHostsToPrivateAndPublic,
	"ParseBytesToHTTPRequest":           lowhttp.ParseBytesToHttpRequest,
	"ParseStringToHTTPResponse":         lowhttp.ParseStringToHTTPResponse,
	"ParseBytesToHTTPResponse":          lowhttp.ParseBytesToHTTPResponse,
	"FixHTTPResponse":                   lowhttp.FixHTTPResponse,
	"ExtractBodyFromHTTPResponseRaw":    lowhttp.ExtractBodyFromHTTPResponseRaw,
	"FixHTTPRequest":                    lowhttp.FixHTTPRequest,
	"SplitHTTPHeadersAndBodyFromPacket": lowhttp.SplitHTTPHeadersAndBodyFromPacket,
	"MergeUrlFromHTTPRequest":           lowhttp.MergeUrlFromHTTPRequest,
	"ReplaceHTTPPacketBody":             lowhttp.ReplaceHTTPPacketBody,

	"ParseStringToHosts":              utils.ParseStringToHosts,
	"ParseStringToPorts":              utils.ParseStringToPorts,
	"ParseStringToUrls":               utils.ParseStringToUrls,
	"ParseStringToUrlsWith3W":         utils.ParseStringToUrlsWith3W,
	"ParseStringToCClassHosts":        network.ParseStringToCClassHosts,
	"ParseStringUrlToWebsiteRootPath": utils.ParseStringUrlToWebsiteRootPath,
	"ParseStringUrlToUrlInstance":     utils.ParseStringUrlToUrlInstance,
	"UrlJoin":                         utils.UrlJoin,
	"IPv4ToCClassNetwork":             utils.GetCClassByIPv4,
	"ParseStringToLines":              utils.ParseStringToLines,
	"PathJoin":                        pathJoin,
	"Grok":                            Grok,
	"JsonToMapList":                   JsonToMapList,
	// "JsonStreamToMapList":             JsonStreamToMapList,
	"JsonToMap":       JsonToMap,
	"ParamsGetOr":     ParamsGetOr,
	"ToJsonIndentStr": toJsonIndentStr,

	"NewFilter":            filter.NoCacheNewFilter,
	"RemoveDuplicatePorts": filter.RemoveDuplicatePorts,
	"FilterPorts":          filter.FilterPorts,

	"RegexpMatch": _strRegexpMatch,

	"MatchAllOfRegexp":    utils.MatchAllOfRegexp,
	"MatchAllOfGlob":      utils.MatchAllOfGlob,
	"MatchAllOfSubString": utils.MatchAllOfSubString,
	"MatchAnyOfRegexp":    utils.MatchAnyOfRegexp,
	"MatchAnyOfGlob":      utils.MatchAnyOfGlob,
	"MatchAnyOfSubString": utils.MatchAnyOfSubString,

	"IntersectString":     funk.IntersectString,
	"Intersect":           funk.IntersectString,
	"Subtract":            funk.SubtractString,
	"ToStringSlice":       utils.InterfaceToStringSlice,
	"VersionGreater":      utils.VersionGreater,
	"VersionGreaterEqual": utils.VersionGreaterEqual,
	"VersionEqual":        utils.VersionEqual,
	"VersionLessEqual":    utils.VersionLessEqual,
	"VersionLess":         utils.VersionLess,
	"VersionCompare":      utils.VersionCompare,
	"Cut":                 strings.Cut,
	"CutPrefix":           strings.CutPrefix,
	"CutSuffix":           strings.CutSuffix,

	"TextReaderSplit": utils.DefaultTextSplitter.SplitReader,
	"TextSplit":       utils.DefaultTextSplitter.Split,

	"ShrinkString":                _shrinkString,
	"AddPrefixLineNumber":         prefixLineNumber,
	"AddPrefixLineNumberToReader": prefixLineNumberReader,
	"RenderTemplate":              RenderTemplate,

	// 流式 JSON 解析配套：UTF-8 安全 reader 与 JSON 字符串解码（配合 jsonstream / jsonextractor 字段流）
	"NewUTF8Reader":       NewUTF8Reader,
	"NewJSONStringReader": NewJSONStringReader,
	"JsonStringDecode":    JsonStringDecode,

	// AI 处理常用：基于 Qwen BPE 词表的 token 计算与编解码（复用 common/ai/ytoken）
	"CalcTokenCount":         CalcTokenCount,
	"CalcOrdinaryTokenCount": CalcOrdinaryTokenCount,
	"EncodeTokens":           EncodeTokens,
	"EncodeOrdinaryTokens":   EncodeOrdinaryTokens,
	"DecodeTokens":           DecodeTokens,
}

func RenderTemplate(i string, m any) string {
	result, err := utils.RenderTemplate(i, m)
	if err != nil {
		log.Warnf("failed to render template: %v", err)
		if result == "" {
			return i
		}
		return result
	}
	return result
}

func prefixLineNumberReader(i any) io.Reader {
	switch ret := i.(type) {
	case io.Reader:
		return utils.PrefixLinesWithLineNumbersReader(ret)
	}
	result := utils.InterfaceToString(i)
	return utils.PrefixLinesWithLineNumbersReader(strings.NewReader(result))
}

func prefixLineNumber(i any) string {
	return utils.PrefixLinesWithLineNumbers(i)
}

// str.ShrinkString 将会把一个字符串压缩成一个设定一个长度下的较短的字符串
// Example:
// ```
// result = str.ShrinkString("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 20)
// println(result)
// /* output: aaaaaaaaaa...aaaaaaaaaa */
// ```
func _shrinkString(i any, size int) string {
	return utils.ShrinkString(i, size)
}

func init() {
	for k, v := range suspect.GuessExports {
		StringsExport[k] = v
	}
}
