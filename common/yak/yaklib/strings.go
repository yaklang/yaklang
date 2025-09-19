package yaklib

import (
	"encoding/json"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

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
// Example:
// ```
// str.IndexAny("Hello world", "world") // 2，因为l在第三个字符中首次出现
// str.IndexAny("Hello World", "Yak") // -1
// ```
func IndexAny(s string, chars string) int {
	return strings.IndexAny(s, chars)
}

// StartsWith / HasPrefix 判断字符串s是否以prefix开头
// Example:
// ```
// str.StartsWith("Hello Yak", "Hello") // true
// str.StartsWith("Hello Yak", "Yak") // false
// ```
func StartsWith(s string, prefix string) bool {
	return strings.HasPrefix(s, prefix)
}

// EndsWith / HasSuffix 判断字符串s是否以suffix结尾
// Example:
// ```
// str.EndsWith("Hello Yak", "Yak") // true
// str.EndsWith("Hello Yak", "Hello") // false
// ```
func EndsWith(s string, suffix string) bool {
	return strings.HasSuffix(s, suffix)
}

// Title 返回字符串s的标题化版本，即所有单词的首字母都是大写的
// Example:
// ```
// str.Title("hello yak") // Hello Yak
// ```
func Title(s string) string {
	return strings.Title(s)
}

// Join 将i中的元素用d连接，如果传入的参数不是字符串，会自动将其转为字符串，再将其用d连接。如果连接失败，则会返回i的字符串形式。
// Example:
// ```
// str.Join([]string{"hello", "yak"}, " ") // hello yak
// str.Join([]int{1, 2, 3}, " ") // 1 2 3
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
// Example:
// ```
// str.TrimLeft("Hello Yak", "H") // ello Yak
// str.TrimLeft("HelloYak", "Hello") // Yak
// ```
func TrimLeft(s string, cutset string) string {
	return strings.TrimLeft(s, cutset)
}

// TrimPrefix 返回将字符串s前缀prefix去掉的字符串
// Example:
// ```
// str.TrimPrefix("Hello Yak", "Hello") //  Yak
// str.TrimPrefix("HelloYak", "Hello") // Yak
// ```
func TrimPrefix(s string, prefix string) string {
	return strings.TrimPrefix(s, prefix)
}

// TrimRight 返回将字符串s右侧所有包含cutset字符串中的字符都去掉的字符串
// Example:
// ```
// str.TrimRight("Hello Yak", "k") // Hello Ya
// str.TrimRight("HelloYak", "Yak") // Hello
// ```
func TrimRight(s string, cutset string) string {
	return strings.TrimRight(s, cutset)
}

// TrimSuffix 返回将字符串s后缀suffix去掉的字符串
// Example:
// ```
// str.TrimSuffix("Hello Yak", "ak") // Hello Y
// str.TrimSuffix("HelloYak", "Yak") // Hello
// ```
func TrimSuffix(s string, suffix string) string {
	return strings.TrimSuffix(s, suffix)
}

// Trim 返回将字符串s两侧所有包含cutset字符串中的字符都去掉的字符串
// Example:
// ```
// str.Trim("Hello Yak", "Hk") // ello Ya
// str.Trim("HelloYakHello", "Hello") // Yak
// ```
func Trim(s string, cutset string) string {
	return strings.Trim(s, cutset)
}

// TrimSpace 返回将字符串s两侧所有的空白字符都去掉的字符串
// Example:
// ```
// str.TrimSpace(" \t\n Hello Yak \n\t\r\n") // Hello Yak
// ```
func TrimSpace(s string) string {
	return strings.TrimSpace(s)
}

// Split 将字符串s按照sep分割成字符串切片
// Example:
// ```
// str.Split("Hello Yak", " ") // [Hello", "Yak"]
// ```
func Split(s string, sep string) []string {
	return strings.Split(s, sep)
}

// SplitAfter 将字符串s按照sep分割成字符串切片，但是每个元素都会保留sep
// Example:
// ```
// str.SplitAfter("Hello-Yak", "-") // [Hello-", "Yak"]
// ```
func SplitAfter(s string, sep string) []string {
	return strings.SplitAfter(s, sep)
}

// SplitAfterN 将字符串s按照sep分割成字符串切片，但是每个元素都会保留sep，最多分为n个元素
// Example:
// ```
// str.SplitAfterN("Hello-Yak-and-World", "-", 2) // [Hello-", "Yak-and-World"]
// ```
func SplitAfterN(s string, sep string, n int) []string {
	return strings.SplitAfterN(s, sep, n)
}

// SplitN 将字符串s按照sep分割成字符串切片，最多分为n个元素
// Example:
// ```
// str.SplitN("Hello-Yak-and-World", "-", 2) // [Hello", "Yak-and-World"]
// ```
func SplitN(s string, sep string, n int) []string {
	return strings.SplitN(s, sep, n)
}

// ToLower 返回字符串s的小写形式
// Example:
// ```
// str.ToLower("HELLO YAK") // hello yak
// ```
func ToLower(s string) string {
	return strings.ToLower(s)
}

// ToUpper 返回字符串s的大写形式
// Example:
// ```
// str.ToUpper("hello yak") // HELLO YAK
// ```
func ToUpper(s string) string {
	return strings.ToUpper(s)
}

// Repeat 返回将字符串s重复count次的字符串
// Example:
// ```
// str.Repeat("hello", 3) // hellohellohello
// ```
func Repeat(s string, count int) string {
	return strings.Repeat(s, count)
}

// ToTitle 返回字符串s的标题化版本，其中所有Unicode字母都会被转换为其大写
// Example:
// ```
// str.ToTitle("hello yak") // HELLO YAK
// ```
func ToTitle(s string) string {
	return strings.ToTitle(s)
}

// Contains 判断字符串s是否包含substr
// Example:
// ```
// str.Contains("hello yakit", "yak") // true
// ```
func Contains(s string, substr string) bool {
	return strings.Contains(s, substr)
}

// ReplaceAll 返回将字符串s中所有old字符串替换为new字符串的字符串
// Example:
// ```
// str.ReplaceAll("hello yak", "yak", "yakit") // hello yakit
// ```
func ReplaceAll(s string, old string, new string) string {
	return strings.ReplaceAll(s, old, new)
}

// Replace 返回将字符串s中前n个old字符串替换为new字符串的字符串
// Example:
// ```
// str.Replace("hello yak", "l", "L", 1) // heLlo yak
// ```
func Replace(s string, old string, new string, n int) string {
	return strings.Replace(s, old, new, n)
}

// NewReader 返回一个从字符串s读取数据的*Reader
// Example:
// ```
// r = str.NewReader("hello yak")
// buf = make([]byte, 256)
// _, err = r.Read(buf)
// die(err)
// println(sprintf("%s", buf)) // hello yak
// ```
func NewReader(s string) *strings.Reader {
	return strings.NewReader(s)
}

// Index 返回字符串s中substr第一次出现的位置的索引，如果字符串中不存在substr，则返回-1
// Example:
// ```
// str.Index("hello yak", "yak") // 6
// str.Index("hello world", "yak") // -1
// ```
func Index(s string, substr string) int {
	return strings.Index(s, substr)
}

// Count 返回字符串s中substr出现的次数
// Example:
// ```
// str.Count("hello yak", "l") // 2
// ```
func Count(s string, substr string) int {
	return strings.Count(s, substr)
}

// Compare 按照ascii码表顺序逐个比较字符串a和b中的每个字符，如果a==b，则返回0，如果a<b，则返回-1，如果a>b，则返回1
// Example:
// ```
// str.Compare("hello yak", "hello yak") // 0
// str.Compare("hello yak", "hello") // 1
// str.Compare("hello", "hello yak") // -1
// ```
func Compare(a string, b string) int {
	return strings.Compare(a, b)
}

// ContainsAny 判断字符串s是否包含chars中的任意字符
// Example:
// ```
// str.ContainsAny("hello yak", "ly") // true
// str.ContainsAny("hello yak", "m") // false
// ```
func ContainsAny(s string, chars string) bool {
	return strings.ContainsAny(s, chars)
}

// EqualFold 判断字符串s和t是否相等，忽略大小写
// Example:
// ```
// str.EqualFold("hello Yak", "HELLO YAK") // true
// ```
func EqualFold(s string, t string) bool {
	unicode.IsSpace('a')
	return strings.EqualFold(s, t)
}

// Fields 返回将字符串s按照空白字符（'\t', '\n', '\v', '\f', '\r', ' ', 0x85, 0xA0）分割的字符串切片
// Example:
// ```
// str.Fields("hello world\nhello yak\tand\vyakit") // [hello", "world", "hello", "yak", "and", "yakit"]
// ```
func Fields(s string) []string {
	return strings.Fields(s)
}

// IndexByte 返回字符串s中第一个等于c的字符的索引，如果字符串中不存在c，则返回-1
// Example:
// ```
// str.IndexByte("hello yak", 'y') // 6
// str.IndexByte("hello yak", 'm') // -1
// ```
func IndexByte(s string, c byte) int {
	return strings.IndexByte(s, c)
}

// LastIndex 返回字符串s中substr最后一次出现的位置的索引，如果字符串中不存在substr，则返回-1
// Example:
// ```
// str.LastIndex("hello yak", "l") // 3
// str.LastIndex("hello yak", "m") // -1
// ```
func LastIndex(s string, substr string) int {
	return strings.LastIndex(s, substr)
}

// LastIndexAny 返回字符串s中chars任意字符最后一次出现的位置的索引，如果字符串中不存在chars，则返回-1
// Example:
// ```
// str.LastIndexAny("hello yak", "ly") // 6
// str.LastIndexAny("hello yak", "m") // -1
// ```
func LastIndexAny(s string, chars string) int {
	return strings.LastIndexAny(s, chars)
}

// LastIndexByte 返回字符串s中最后一个等于c的字符的索引，如果字符串中不存在c，则返回-1
// Example:
// ```
// str.LastIndexByte("hello yak", 'l') // 3
// str.LastIndexByte("hello yak", 'm') // -1
// ```
func LastIndexByte(s string, c byte) int {
	return strings.LastIndexByte(s, c)
}

// ToValidUTF8 返回将字符串s中无效的UTF-8编码替换为replacement的字符串
// Example:
// ```
//
// str.ToValidUTF8("hello yak", "?") // hello yak
// ```
func ToValidUTF8(s string, replacement string) string {
	return strings.ToValidUTF8(s, replacement)
}

// ExtractJson 尝试提取字符串中的 JSON 并进行修复, 返回中的元素都是Object
// Example:
// ```
// str.ExtractJson("hello yak") // []
// str.ExtractJson(`{"hello": "yak"}`) // [{"hello": "yak"}]
// ```
func extractValidJson(i interface{}) []string {
	return jsonextractor.ExtractObjectsOnly(utils.InterfaceToString(i))
}

// ExtractJsonWithRaw 尝试提取字符串中的 JSON 并返回，第一个返回值返回经过修复后的JSON字符串数组，第二个返回值返回原始JSON字符串数组(如果修复失败)
// Example:
// ```
// str.ExtractJsonWithRaw("hello yak") // [], []
// str.ExtractJsonWithRaw(`{"hello": "yak"}`) // [{"hello": "yak"}], []
// ```
func extractJsonEx(i interface{}) ([]string, []string) {
	return jsonextractor.ExtractJSONWithRaw(utils.InterfaceToString(i))
}

// ExtractDomain 尝试提取字符串中的域名并返回，后续可以接收一个 tryDecode 参数，如果传入 true，则会尝试对输入的文本进行解码(双重URL编码，URL编码，unicode编码，quoted编码)
// Example:
// ```
// str.ExtractDomain("hello yak") // []
// str.ExtractDomain("hello yaklang.com or yaklang.io") // ["yaklang.com", "yaklang.io"]
// str.ExtractDomain(`{"message:"%79%61%6b%6c%61%6e%67.com"}`, true) // ["yaklang.com"]
// ```
func extractDomain(i any, tryDecode ...bool) []string {
	return domainextractor.ExtractDomains(utils.InterfaceToString(i), tryDecode...)
}

// ExtractRootDomain 尝试提取字符串中的根域名并返回
// Example:
// ```
// str.ExtractRootDomain("hello yak") // []
// str.ExtractRootDomain("hello www.yaklang.com or www.yaklang.io") // ["yaklang.com", "yaklang.io"]
// ```
func extractRootDomain(i interface{}) []string {
	return domainextractor.ExtractRootDomains(utils.InterfaceToString(i))
}

// ExtractTitle 尝试将传入的字符串进行HTML解析并提取其中的标题(title标签)返回
// Example:
// ```
// str.ExtractTitle("hello yak") // ""
// str.ExtractTitle("<title>hello yak</title>") // "hello yak"
// ```
func extractTitle(i interface{}) string {
	return utils.ExtractTitleFromHTMLTitle(utils.InterfaceToString(i), "")
}

// PathJoin 将传入的文件路径进行拼接并返回
// Example:
// ```
// str.PathJoin("/var", "www", "html") // in *unix: "/var/www/html"    in Windows: \var\www\html
// ```
func pathJoin(elem ...string) (newPath string) {
	return filepath.Join(elem...)
}

// ToJsonIndentStr 将v转换为格式化的JSON字符串并返回，如果转换失败，则返回空字符串
// Example:
// ```
// str.ToJsonIndentStr({"hello":"yak"}) // {"hello": "yak"}
// ```
func toJsonIndentStr(d interface{}) string {
	raw, err := json.MarshalIndent(d, "", "    ")
	if err != nil {
		return ""
	}
	return string(raw)
}

// RandomUpperAndLower 返回一个随机大小写的字符串
// Example:
// ```
// str.RandomUpperAndLower("target") // TArGeT
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
