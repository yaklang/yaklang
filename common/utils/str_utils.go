package utils

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/davecgh/go-spew/spew"
	"github.com/gobwas/glob"
	"github.com/h2non/filetype"
	"github.com/h2non/filetype/matchers"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// ExtractStrContext 从字符串raw中提取一组关键字res上下文的内容，上下文的长度是512个字符确定。
// Example:
// ```
// str.ExtractStrContext("hello yak", ["hello"]) // ["hello yak"]
// ```
func ExtractStrContextByKeyword(raw string, res []string) []string {
	var details []string
	for _, keyword := range res {
		if index := strings.Index(raw, keyword); index >= 0 {
			info := ""

			end := index + len(keyword) + 512

			if index <= 512 {
				info += raw[:index]
			} else {
				info += raw[index-512 : index+len(keyword)]
			}

			if end >= len(raw) {
				info += raw[index:]
			} else {
				info += raw[index:end]
			}

			details = RemoveRepeatStringSlice(append(details, EscapeInvalidUTF8Byte([]byte(info))))
		}
	}
	return details
}

var ShrinkString = codec.ShrinkString

func StringBefore(value string, a string) string {
	pos := strings.Index(value, a)
	if pos == -1 {
		return ""
	}
	return value[0:pos]
}

func StringAfter(value string, a string) string {
	pos := strings.Index(value, a)
	if pos == -1 {
		return ""
	}
	adjustedPos := pos + len(a)
	if adjustedPos >= len(value) {
		return ""
	}
	return value[adjustedPos:]
}

// StringSliceContainsAll 判断字符串切片s中是否完全包含elements中的所有元素，对于非字符串的切片，会尝试将其元素转换为字符串再判断是否包含
// Example:
// ```
// str.StringSliceContainsAll(["hello", "yak"], "hello", "yak") // true
// str.StringSliceContainsAll(["hello", "yak"], "hello", "yak", "world") // false
// ```
func StringSliceContainsAll(s []string, elements ...string) bool {
	for _, e := range elements {
		if !StringArrayContains(s, e) {
			return false
		}
	}
	return true
}

// RemoveRepeat 移除字符串切片slc中的重复元素
// Example:
// ```
// str.RemoveRepeat(["hello", "yak", "hello"]) // ["hello", "yak"]
// ```
func RemoveRepeatStringSlice(slc []string) []string {
	if len(slc) < 1024 {
		return RemoveRepeatStringSliceByLoop(slc)
	} else {
		return RemoveRepeatStringSliceByMap(slc)
	}
}

// 元素去重
func RemoveRepeatUintSlice(slc []uint) []uint {
	if len(slc) < 1024 {
		return RemoveRepeatUintSliceByLoop(slc)
	} else {
		return RemoveRepeatUintSliceByMap(slc)
	}
}

// 元素去重
func RemoveRepeatIntSlice(slc []int) []int {
	if len(slc) < 1024 {
		return RemoveRepeatIntSliceByLoop(slc)
	} else {
		return RemoveRepeatIntSliceByMap(slc)
	}
}

func RemoveRepeatStringSliceByLoop(slc []string) []string {
	result := []string{}
	for i := range slc {
		flag := true
		for j := range result {
			if slc[i] == result[j] {
				flag = false
				break
			}
		}
		if flag {
			result = append(result, slc[i])
		}
	}
	return result
}

func RemoveRepeatStringSliceByMap(slc []string) []string {
	result := []string{}
	tempMap := map[string]byte{}
	for _, e := range slc {
		l := len(tempMap)
		tempMap[e] = 0
		if len(tempMap) != l {
			result = append(result, e)
		}
	}
	return result
}

func RemoveRepeatUintSliceByLoop(slc []uint) []uint {
	result := []uint{}
	for i := range slc {
		flag := true
		for j := range result {
			if slc[i] == result[j] {
				flag = false
				break
			}
		}
		if flag {
			result = append(result, slc[i])
		}
	}
	return result
}

func RemoveRepeatUintSliceByMap(slc []uint) []uint {
	result := []uint{}
	tempMap := map[uint]byte{}
	for _, e := range slc {
		l := len(tempMap)
		tempMap[e] = 0
		if len(tempMap) != l {
			result = append(result, e)
		}
	}
	return result
}

func RemoveRepeatIntSliceByLoop(slc []int) []int {
	result := []int{}
	for i := range slc {
		flag := true
		for j := range result {
			if slc[i] == result[j] {
				flag = false
				break
			}
		}
		if flag {
			result = append(result, slc[i])
		}
	}
	return result
}

func RemoveRepeatIntSliceByMap(slc []int) []int {
	result := []int{}
	tempMap := map[int]byte{}
	for _, e := range slc {
		l := len(tempMap)
		tempMap[e] = 0
		if len(tempMap) != l {
			result = append(result, e)
		}
	}
	return result
}

func StringArrayContains(array []string, element string) bool {
	for _, s := range array {
		if element == s {
			return true
		}
	}
	return false
}

func HTTPPacketIsLargerThanMaxContentLength(res interface{}, maxLength int) bool {
	var length int
	switch ret := res.(type) {
	case *http.Response:
		length, _ = strconv.Atoi(ret.Header.Get("Content-Length"))
	case *http.Request:
		length, _ = strconv.Atoi(ret.Header.Get("Content-Length"))
	}
	if length > maxLength && maxLength > 0 {
		log.Infof("allow rsp/req: %p's content-length: %v passed for limit content-length", res, length)
		return true
	}
	return false
}

func StringHasPrefix(s string, prefix []string) bool {
	for _, x := range prefix {
		if strings.HasPrefix(strings.ToLower(s), strings.ToLower(x)) {
			return true
		}
	}
	return false
}

func StringSubStringArrayContains(array []string, element string) bool {
	for _, s := range array {
		if strings.Contains(element, s) {
			return true
		}
	}
	return false
}

func StringGlobContains(pattern string, element string, seps ...rune) bool {
	if !strings.Contains(pattern, "*") && IContains(element, pattern) {
		return true
	}
	if !strings.HasSuffix(pattern, "*") {
		pattern += "*"
	}
	if !strings.HasPrefix(pattern, "*") {
		pattern = "*" + pattern
	}
	rule, err := glob.Compile(pattern, seps...)
	if err != nil {
		return false
	}
	return rule.Match(element)
}

func StringGlobArrayContains(array []string, element string, seps ...rune) bool {
	for _, r := range array {
		if StringGlobContains(r, element, seps...) {
			return true
		}
	}
	return false
}

func StringArrayIndex(array []string, element string) int {
	for index, s := range array {
		if element == s {
			return index
		}
	}
	return -1
}

func StringOr(s ...string) string {
	for _, i := range s {
		if i != "" {
			return i
		}
	}
	return ""
}

func IntLargerZeroOr(s ...int) int {
	for _, i := range s {
		if i > 0 {
			return i
		}
	}
	return 0
}

func StringArrayFilterEmpty(array []string) []string {
	var ret []string
	for _, a := range array {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		ret = append(ret, a)
	}
	return ret
}

func StringArrayMerge(t ...[]string) []string {
	m := map[string]interface{}{}
	for _, ta := range t {
		for _, i := range ta {
			m[i] = false
		}
	}

	var n []string
	for k := range m {
		n = append(n, k)
	}
	return n
}

func StringSplitAndStrip(raw string, sep string) []string {
	l := []string{}

	for _, v := range strings.Split(raw, sep) {
		s := strings.TrimSpace(v)
		if s != "" {
			l = append(l, s)
		}
	}

	return l
}

func StringToAsciiBytes(s string) []byte {
	t := make([]byte, utf8.RuneCountInString(s))
	i := 0
	for _, r := range s {
		t[i] = byte(r)
		i++
	}
	return t
}

func AsciiBytesToRegexpMatchedRunes(in []byte) []rune {
	result := make([]rune, len(in))
	for i, b := range in {
		result[i] = rune(b)
	}
	return result
}

func AsciiBytesToRegexpMatchedString(in []byte) string {
	return string(AsciiBytesToRegexpMatchedRunes(in))
}

func stripPort(hostport string) string {
	colon := strings.IndexByte(hostport, ':')
	if colon == -1 {
		return hostport
	}
	if i := strings.IndexByte(hostport, ']'); i != -1 {
		return strings.TrimPrefix(hostport[:i], "[")
	}
	return hostport[:colon]
}

func portOnly(hostport string) string {
	colon := strings.IndexByte(hostport, ':')
	if colon == -1 {
		return ""
	}
	if i := strings.Index(hostport, "]:"); i != -1 {
		return hostport[i+len("]:"):]
	}
	if strings.Contains(hostport, "]") {
		return ""
	}
	return hostport[colon+len(":"):]
}

// by json unmarshal
func StringLiteralToAny(s string) any {
	var result any
	err := json.Unmarshal([]byte(s), &result)
	if err != nil {
		// fallback to string
		log.Errorf("json unmarshal error: %v", err)
		result = s
	}
	return result
}

func InterfaceToBytes(i interface{}) (result []byte) {
	return codec.AnyToBytes(i)
}

func InterfaceToJsonString(i interface{}) string {
	if i == nil {
		return ""
	}
	b, err := json.Marshal(i)
	if err != nil {
		log.Errorf("json marshal error: %v", err)
		return ""
	}
	return string(b)
}

func InterfaceToString(i interface{}) string {
	if a, ok := i.(interface{ String() string }); ok {
		return a.String()
	}
	return codec.AnyToString(i)
}

// Reverse the string
func StringReverse(s string) string {
	n := 0
	runeRet := make([]rune, len(s))
	for _, r := range s {
		runeRet[n] = r
		n++
	}
	runeRet = runeRet[0:n]
	for i := 0; i < n/2; i++ {
		runeRet[i], runeRet[n-1-i] = runeRet[n-1-i], runeRet[i]
	}
	return string(runeRet)
}

func InterfaceToQuotedString(i interface{}) string {
	packetRawStr := InterfaceToString(i)
	if packetRawStr != "" {
		if strings.HasPrefix(packetRawStr, `"`) && strings.HasSuffix(packetRawStr, `"`) {
			raw, _ := strconv.Unquote(packetRawStr)
			if raw != "" {
				return packetRawStr
			}
		}
	}
	return strconv.Quote(packetRawStr)
}

func Int64SliceToIntSlice(i []int64) []int {
	result := make([]int, 0)
	for _, v := range i {
		result = append(result, int(v))
	}
	return result
}

func IntSliceToInt64Slice(i []int) []int64 {
	result := make([]int64, len(i))
	for _, v := range i {
		result = append(result, int64(v))
	}
	return result
}

// ToStringSlice 将任意类型的数据转换为字符串切片
// Example:
// ```
// str.ToStringSlice("hello") // ["hello"]
// str.ToStringSlice([1, 2]) // ["1", "2"]
// ```
func InterfaceToStringSlice(i interface{}) (result []string) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("str.ToStringSlice failed: %s", err)
			spew.Dump(i)
			PrintCurrentGoroutineRuntimeStack()
			result = []string{InterfaceToString(i)}
		}
	}()

	if i == nil {
		return []string{}
	}
	switch ret := i.(type) {
	case []string:
		return ret
	default:
		va := reflect.ValueOf(i)
		switch reflect.TypeOf(i).Kind() {
		case reflect.Slice, reflect.Array:
			for i := 0; i < va.Len(); i++ {
				result = append(result, InterfaceToString(va.Index(i).Interface()))
			}
		default:
			result = append(result, InterfaceToString(i))
		}
	}
	return result
}

func InterfaceToBytesSlice(i interface{}) [][]byte {
	var result [][]byte
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("str.ToBytesSlice failed: %s", err)
		}
	}()
	va := reflect.ValueOf(i)
	switch reflect.TypeOf(i).Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < va.Len(); i++ {
			result = append(result, InterfaceToBytes(va.Index(i).Interface()))
		}
	default:
		result = append(result, InterfaceToBytes(i))
	}
	return result
}

func InterfaceToBoolean(i any) bool {
	if i == nil {
		return false
	}
	switch ret := i.(type) {
	case bool:
		return ret
	default:
		a, _ := strconv.ParseBool(InterfaceToString(i))
		return a
	}
}

func InterfaceToFloat64(i any) float64 {
	if i == nil {
		return 0
	}
	switch ret := i.(type) {
	case float64:
		return ret
	case float32:
		return float64(ret)
	default:
		return float64(InterfaceToInt(i))
	}
}

func InterfaceToInt(i any) int {
	if i == nil {
		return 0
	}
	switch ret := i.(type) {
	case bool:
		if ret {
			return 1
		} else {
			return 0
		}
	case int:
		return ret
	case int64:
		return int(ret)
	case int32:
		return int(ret)
	case int16:
		return int(ret)
	case int8:
		return int(ret)
	case uint:
		return int(ret)
	case uint64:
		return int(ret)
	case uint32:
		return int(ret)
	case uint16:
		return int(ret)
	default:
		return codec.Atoi(InterfaceToString(i))
	}
}

func InterfaceToMap(i interface{}) map[string][]string {
	finalResult := make(map[string][]string)
	res := make(map[string]interface{})
	switch ret := i.(type) {
	case map[string]interface{}:
		res = ret
	case map[string]string:
		for k, v := range ret {
			res[k] = v
		}
	case map[string][]string:
		for k, v := range ret {
			res[k] = v
		}
	default:
		if reflect.TypeOf(ret).Kind() == reflect.Map {
			value := reflect.ValueOf(ret)
			for _, keyValue := range value.MapKeys() {
				finalResult[InterfaceToString(keyValue.Interface())] = []string{InterfaceToString(value.MapIndex(keyValue).Interface())}
			}
			return finalResult
		}
		finalResult["default"] = []string{fmt.Sprintf("%v", i)}
		return finalResult
	}

	funk.ForEach(res, func(k string, v interface{}) {
		finalResult[k] = []string{}
		switch ret := v.(type) {
		case []interface{}:
			finalResult[k] = append(finalResult[k], funk.Map(ret, func(i interface{}) string {
				return InterfaceToString(i)
			}).([]string)...)
		case []string:
			finalResult[k] = append(finalResult[k], ret...)
		default:
			finalResult[k] = append(finalResult[k], InterfaceToString(ret))
		}
	})
	return finalResult
}

// ParseStringUrlToWebsiteRootPath 将字符串 url 解析为其根路径的URL
// Example:
// ```
// str.ParseStringUrlToWebsiteRootPath("https://yaklang.com/abc?a=1") // https://yaklang.com/
// ```
func ParseStringUrlToWebsiteRootPath(url string) (newURL string) {
	ins, _ := ParseStringUrlToUrlInstance(url)
	if ins == nil {
		return url
	}

	ins.Path = "/"
	ins.RawPath = "/"
	ins.RawQuery = ""
	return ins.String()
}

// ParseStringUrlToUrlInstance 将字符串 url 解析为 URL 结构体并返回错误
// Example:
// ```
// str.ParseStringUrlToUrlInstance("https://yaklang.com/abc?a=1")
// ```
func ParseStringUrlToUrlInstance(s string) (*url.URL, error) {
	return url.Parse(s)
}

// AppendDefaultPort returns host:port format.
// If the port is already specified in the host, it will be returned directly.
// wss -> 443
// ws -> 80
// http -> 80
// https -> 443
func AppendDefaultPort(raw string, port int) string {
	parsedHost, parsedPort, _ := ParseStringToHostPort(raw)
	if parsedPort > 0 {
		return HostPort(parsedHost, parsedPort)
	}
	return HostPort(raw, port)
}

func ParseStringToHttpsAndHostname(res string) (bool, string) {
	host, port, _ := ParseStringToHostPort(res)
	if host == "" {
		return false, res
	}

	urlHttps := strings.HasPrefix(res, "https://")
	isUrl := IsHttpOrHttpsUrl(res)

	if port > 0 {
		if port == 443 {
			if isUrl && !urlHttps {
				return false, HostPort(host, port)
			}
			return true, host
		}

		if port == 80 {
			if isUrl && urlHttps {
				return true, HostPort(host, port)
			}
			return false, host
		}

		return urlHttps, HostPort(host, port)
	}
	return urlHttps, host
}

// ParseStringToHostPort 尝试从字符串中解析出host和port，并与错误一起返回
// Example:
// ```
// host, port, err = str.ParseStringToHostPort("127.0.0.1:8888") // host = "127.0.0.1", port = 8888, err = nil
// host, port, err = str.ParseStringToHostPort("https://example.com") // host = "example.com", port = 443, err = nil
// host, port, err = str.ParseStringToHostPort("Hello Yak") // host = "", port = 0, err = error("unknown port for [Hello Yak]")
// ```
func ParseStringToHostPort(raw string) (host string, port int, err error) {
	if strings.Contains(raw, "://") {
		urlObject, _ := url.Parse(raw)
		if urlObject != nil {
			// 处理 URL
			portRaw := urlObject.Port()
			portInt64, err := strconv.ParseInt(portRaw, 10, 32)
			if err != nil || portInt64 <= 0 {
				switch urlObject.Scheme {
				case "http", "ws":
					port = 80
				case "https", "wss":
					port = 443
				}
			} else {
				port = int(portInt64)
			}

			host = urlObject.Hostname()
			err = nil
			return host, port, err
		}
	}
	// 这里需要处理ipv6的情况，如果是ipv6的话，直接返回
	if ip := net.ParseIP(raw); ip != nil {
		return raw, 0, errors.Errorf("unknown port for [%s]", raw)
	}

	host = stripPort(raw)
	portStr := portOnly(raw)
	if len(portStr) <= 0 {
		return host, 0, errors.Errorf("unknown port for [%s]", raw)
	}

	portStr = strings.TrimSpace(portStr)
	portInt64, err := strconv.ParseInt(portStr, 10, 64)
	if err != nil {
		return host, 0, errors.Errorf("%s parse port(%s) failed: %s", raw, portStr, err)
	}

	port = int(portInt64)
	err = nil
	return
}

// UrlJoin 将 字符串 origin 和 字符串数组 paths 拼接成一个新的 URL 字符串，并返回错误
// Example:
// ```
// newURL, err = str.UrlJoin("https://yaklang.com", "asd", "qwe") // newURL = "https://yaklang.com/asd/qwe", err = nil
// newURL, err = str.UrlJoin("https://yaklang.com/zxc", "/asd", "qwe") // newURL = "https://yaklang.com/asd/qwe", err = nil
// ```
func UrlJoin(origin string, paths ...string) (newURL string, err error) {
	/*
		https://baidu.com/abc   a?key=value
		https://baidu.com/abc/a?key=value => [X] https://baidu.com/abc/a%xxkey=value

		[X] https://baidu.com/a?key=value
	*/
	u, err := url.Parse(origin)
	if err != nil {
		return "", errors.Errorf("origin:[%s] is not a valid url: %s", origin, err)
	}
	if len(paths) == 1 && strings.HasPrefix(paths[0], "//") {
		// 处理 //baidu.com 这种情况
		paths[0] = fmt.Sprintf("%s:%s", u.Scheme, paths[0])
	}

	var pathBuf bytes.Buffer
	for index, p := range paths {
		if index == 0 {
			pathBuf.WriteString(p)
			continue
		}
		if strings.HasPrefix(p, "?") {
			pathBuf.WriteString(p)
		} else {
			pathBuf.WriteString("/" + p)
		}
	}

	pathRaw := pathBuf.String()
	var uri *url.URL
	if strings.HasPrefix(pathRaw, "/") {
		uri, err = url.ParseRequestURI(pathRaw)
		if err != nil {
			u.Path = pathRaw
			return u.String(), nil
		}
	} else {
		// 处理 URL 的情况
		inputAsUrl, _ := url.Parse(pathRaw)
		if inputAsUrl != nil && inputAsUrl.Scheme != "" {
			return pathRaw, nil
		}

		// 不是 URL，并且 pathRaw 开头一定不是 /
		// 那么就看 u.Path 结尾是不是 /
		// r := path.Join(u.Path, pathRaw)
		if !strings.HasSuffix(u.Path, "/") && path.Ext(u.Path) == "" {
			u.Path += "/"
		}

		// 移除 ./
	PATHCLEAN:
		for {
			switch true {
			case strings.HasPrefix(pathRaw, "./"):
				pathRaw = pathRaw[2:]
				continue
			case strings.HasPrefix(pathRaw, "../"):
				pathRaw = pathRaw[3:]
				u.Path = path.Join(u.Path, "..")
				if !strings.HasSuffix(u.Path, "/") && path.Ext(u.Path) == "" {
					u.Path += "/"
				}
				continue
			default:
				break PATHCLEAN
			}
		}

		var reqUri string
		if path.Ext(u.Path) == "" {
			reqUri = u.Path + pathRaw
		} else {
			reqUri = path.Join(path.Dir(u.Path), pathRaw)
		}
		uri, err = url.ParseRequestURI(reqUri)
		if err != nil {
			u.Path = reqUri
			return u.String(), nil
		}
	}
	u.RawPath = uri.RawPath
	u.Path = uri.Path
	u.RawQuery = uri.RawQuery
	return u.String(), nil
}

func ParseLines(raw string) chan string {
	outC := make(chan string)
	go func() {
		defer close(outC)

		for _, l := range strings.Split(raw, "\n") {
			hl := strings.TrimSpace(l)
			if hl == "" {
				continue
			}
			outC <- hl
		}
	}()
	return outC
}

func CopyBytes(rsp []byte) []byte {
	b := make([]byte, len(rsp))
	copy(b, rsp)
	return b
}

func CopyMapInterface(i map[string]interface{}) map[string]interface{} {
	if i == nil {
		return make(map[string]interface{})
	}
	m := make(map[string]interface{})
	for k, v := range i {
		m[k] = v
	}
	return m
}

func CopyMapShallow[K comparable, V any](originalMap map[K]V) map[K]V {
	copiedMap := make(map[K]V)
	for key, value := range originalMap {
		copiedMap[key] = value
	}
	return copiedMap
}

func CopySlice[T any](i []T) []T {
	if i == nil {
		return make([]T, 0)
	}
	result := make([]T, len(i))
	copy(result, i)
	return result
}

func ByteCountDecimal(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(b)/float64(div), "kMGTPE"[exp])
}

func ByteCountBinary(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// 每个单词首字母大写
func InitialCapitalizationEachWords(str string) string {
	if len(str) < 1 {
		return ""
	}
	words := strings.Split(str, " ")
	result := []string{}
	for _, w := range words {
		w = strings.ToUpper(w)[:1] + w[1:]
		result = append(result, w)
	}
	return strings.Join(result, " ")
}

func SliceGroup(origin []string, groupSize int) [][]string {
	var result [][]string

	var count int
	var buffer []string
	for _, i := range origin {
		count++
		buffer = append(buffer, i)

		if count >= groupSize {
			count = 0

			result = append(result, buffer)
			buffer = nil
		}
	}

	if len(buffer) > 0 {
		result = append(result, buffer)
	}
	return result
}

func ToNsServer(server string) string {
	// 如果 server 只是一个 IP 则需要把端口加上
	ip := net.ParseIP(server)
	if ip != nil {
		server = ip.String() + ":53"
		return server
	}

	// 这里肯定不是 IP/IP6
	// 所以我们检测是否包含端口，如果不包含端口，则添加端口
	if strings.Contains(server, ":") {
		return server
	}

	for strings.HasSuffix(server, ".") {
		server = server[:len(server)-1]
	}
	server += ":53"
	return server
}

// RandStringBytes 返回在大小写字母表中随机挑选 n 个字符组成的字符串
// Example:
// ```
// str.RandStr(10)
// ```
func RandStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = LetterChar[rand.Intn(len(LetterChar))]
	}
	return string(b)
}

func RandNumberStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = NumberChar[rand.Intn(len(NumberChar))]
	}
	return string(b)
}

func RandAlphaNumStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = AlphaNumChar[rand.Intn(len(AlphaNumChar))]
	}
	return string(b)
}

func RandBytes(n int) []byte {
	return codec.RandBytes(n)
}

const (
	passwordSepcialChars = ",.<>?;:[]{}~!@#$%^&*()_+-="
	AllSepcialChars      = ",./<>?;':\"[]{}`~!@#$%^&*()_+-=\\|"
	LittleChar           = "abcdefghijklmnopqrstuvwxyz"
	BigChar              = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	NumberChar           = "1234567890"
	LetterChar           = LittleChar + BigChar
	AlphaNumChar         = LittleChar + BigChar + NumberChar
	PasswordChar         = passwordSepcialChars + LittleChar + BigChar + NumberChar
)

// IsStrongPassword 判断字符串是否为强密码，强密码的定义为：长度大于8，同时包含特殊字符、小写字母、大写字母、数字
// Example:
// ```
// str.IsStrongPassword("12345678") // false
// str.IsStrongPassword("12345678a") // false
// str.IsStrongPassword("12345678aA") // false
// str.IsStrongPassword("12345678aA!") // true
// ```
func IsStrongPassword(s string) bool {
	if len(s) <= 8 {
		return false
	}

	var haveSpecial, haveLittleChar, haveBigChar, haveNumber bool
	for _, c := range s {
		ch := string(c)
		if strings.Contains(passwordSepcialChars, ch) {
			haveSpecial = true
		}

		if strings.Contains(LittleChar, ch) {
			haveLittleChar = true
		}

		if strings.Contains(BigChar, ch) {
			haveBigChar = true
		}

		if strings.Contains(NumberChar, ch) {
			haveNumber = true
		}
	}

	return haveSpecial && haveLittleChar && haveBigChar && haveNumber
}

// RandSecret 返回在所有可见ascii字符表中随机挑选 n 个字符组成的密码字符串，这个密码经过str.IsStrongPassword验证，即为强密码
// Example:
// ```
// str.RandSecret(10)
// ```
func RandSecret(n int) string {
	if n <= 8 {
		n = 12
	}

	for {
		b := make([]byte, n)
		for i := range b {
			b[i] = PasswordChar[rand.Intn(len(PasswordChar))]
		}

		result := IsStrongPassword(string(b))
		if result {
			return string(b)
		}
	}
}

func RandSample(n int, material ...string) string {
	b := make([]rune, n)

	base := LittleChar + BigChar + NumberChar
	if ret := strings.Join(material, ""); ret != "" {
		base = ret
	}
	for i := range b {
		b[i] = rune(base[rand.Intn(len([]rune(base)))])
	}
	return string(b)
}

func RandSampleInRange(minLen, maxLen int, material ...string) string {
	if minLen > maxLen {
		// 如果最小长度大于最大长度，则交换它们
		minLen, maxLen = maxLen, minLen
	}

	// 计算随机长度。rand.Intn(n) 生成 [0, n) 范围内的随机整数
	// 因此，rand.Intn(maxLen - minLen + 1) 生成的是 [0, maxLen - minLen + 1) 范围内的随机数
	// 加上 minLen 后，生成的随机长度就在 [minLen, maxLen] 范围内
	randomLength := minLen + rand.Intn(maxLen-minLen+1)

	// 调用 RandSample 生成并返回随机字符串
	return RandSample(randomLength, material...)
}

func RandChoice(a ...string) string {
	if len(a) > 0 {
		return a[rand.Intn(len(a))]
	}
	return ""
}

func ExtractRawPath(target string) string {
	var rawPath string
	if noSchemaTarget := strings.TrimPrefix(
		strings.TrimPrefix(target, "http://"), "https://",
	); noSchemaTarget != "" && strings.Contains(noSchemaTarget, "/") {
		rawPath = noSchemaTarget[strings.Index(noSchemaTarget, "/"):]
	}
	return rawPath
}

// ParseStringToUrls 尝试从给定的字符串(ip,域名)中解析出 URL 列表，补全协议和端口
// Example:
// ```
// str.ParseStringToUrls("yaklang.com:443", "https://yaklang.io") // [https://yaklang.com, https://yaklang.io]
// ```
func ParseStringToUrls(targets ...string) []string {
	var urls []string
	for _, target := range targets {
		target = strings.TrimSpace(target)
		_t := strings.ToLower(target)
		if strings.HasPrefix(_t, "https://") || strings.HasPrefix(_t, "http://") {
			urls = append(urls, target)
			continue
		}

		rawHost, port, err := ParseStringToHostPort(target)
		if err != nil {
			urls = append(urls, fmt.Sprintf("https://%v", target))
			urls = append(urls, fmt.Sprintf("http://%v", target))
			continue
		}

		rawPath := ExtractRawPath(target)

		if port == 80 {
			urls = append(urls, fmt.Sprintf("http://%v", rawHost)+rawPath)
			continue
		}

		if port == 443 {
			urls = append(urls, fmt.Sprintf("https://%v", rawHost)+rawPath)
			continue
		}

		urls = append(urls, fmt.Sprintf("https://%v:%v", rawHost, port)+rawPath)
		urls = append(urls, fmt.Sprintf("http://%v:%v", rawHost, port)+rawPath)
	}

	return urls
}

type blockParser struct {
	scanner *bufio.Scanner
}

func NewBlockParser(reader io.Reader) *blockParser {
	s := bufio.NewScanner(reader)
	s.Split(bufio.ScanWords)
	return &blockParser{scanner: s}
}

func (b *blockParser) NextStringBlock() string {
	b.scanner.Scan()
	return b.scanner.Text()
}

func (b *blockParser) NextBytesBlock() []byte {
	b.scanner.Scan()
	return b.scanner.Bytes()
}

func (b *blockParser) Next() bool {
	return b.scanner.Scan()
}

func (b *blockParser) GetString() string {
	return b.scanner.Text()
}

func (b *blockParser) GetBytes() []byte {
	return b.scanner.Bytes()
}

func (b *blockParser) GetScanner() *bufio.Scanner {
	return b.scanner
}

func DumpHostFileWithTextAndFiles(raw string, divider string, files ...string) (string, error) {
	l := PrettifyListFromStringSplited(raw, divider)
	return DumpFileWithTextAndFiles(
		strings.Join(ParseStringToHosts(strings.Join(l, ",")), divider),
		divider, files...)
}

func DumpFileWithTextAndFiles(raw string, divider string, files ...string) (string, error) {
	// 构建 targets
	targets := strings.Join(ParseStringToLines(raw), divider)
	fp, err := ioutil.TempFile("", "tmpfile-*.txt")
	if err != nil {
		return "", err
	}
	fp.WriteString(targets + divider)
	defer func() {
		fp.Close()
	}()
	for _, f := range files {
		raw, _ := ioutil.ReadFile(f)
		if raw == nil {
			continue
		}
		targetsFromFile := strings.Join(ParseStringToLines(string(raw)), divider)
		targetsFromFile += divider
		fp.WriteString(targetsFromFile)
	}
	return fp.Name(), nil
}

// ParseStringToLines 将字符串按换行符(\n)分割成字符串数组，并去除BOM头和空行
// Example:
// ```
// str.ParseStringToLines("Hello World\nHello Yak") // ["Hello World", "Hello Yak"]
// ```
func ParseStringToLines(raw string) []string {
	var lines []string

	reader := bufio.NewReader(bytes.NewBufferString(raw))
	for {
		line, err := BufioReadLine(reader)
		if err != nil {
			break
		}
		if line := strings.TrimSpace(string(line)); line == "" {
			continue
		} else {
			lines = append(lines, RemoveBOMForString(line))
		}
	}
	return lines
}

func ParseStringToRawLines(raw string) []string {
	var lines []string
	scanner := bufio.NewScanner(bytes.NewBufferString(raw))
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		lines = append(lines, RemoveBOMForString(scanner.Text()))
	}
	return lines
}

func Format(raw string, data map[string]string) string {
	for k, v := range data {
		raw = strings.ReplaceAll(raw, "$"+k, v)
	}
	return raw
}

func ReplaceLastSubString(s, sub, new string) string {
	return strings.Replace(s, sub, new, strings.LastIndex(s, sub))
}

// SimplifyUtf8 simplify utf8 bytes to utf8 bytes
func SimplifyUtf8(raw []byte) ([]byte, error) {
	res, err := utf8ToUnicode(raw)
	if err != nil {
		return nil, err
	}
	return unicodeToUtf8(res), nil
}
func utf8ToUnicode(raw []byte) ([]uint32, error) {
	reader := bytes.NewReader(raw)
	var res []uint32
	addBinaryBits := func(res *uint32, b byte, l byte) {
		mask := uint32(1<<l - 1)
		*res = *res<<l | uint32(b)&mask
	}
	for i := 0; i < len(raw); {
		b, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		switch {
		case b>>7 == 0b0:
			res = append(res, uint32(b))
			i += 1
		case b>>5 == 0b110:
			b1, err := reader.ReadByte()
			if err != nil {
				return nil, err
			}
			var ch uint32
			addBinaryBits(&ch, b, 5)
			addBinaryBits(&ch, b1, 6)
			res = append(res, ch)
			i += 2
		case b>>4 == 0b1110:
			b1, err := reader.ReadByte()
			if err != nil {
				return nil, err
			}
			b2, err := reader.ReadByte()
			if err != nil {
				return nil, err
			}
			var ch uint32
			addBinaryBits(&ch, b, 4)
			addBinaryBits(&ch, b1, 6)
			addBinaryBits(&ch, b2, 6)
			res = append(res, ch)
			i += 3
		case b>>3 == 0b11110:
			b1, err := reader.ReadByte()
			if err != nil {
				return nil, err
			}
			b2, err := reader.ReadByte()
			if err != nil {
				return nil, err
			}
			b3, err := reader.ReadByte()
			if err != nil {
				return nil, err
			}
			var ch uint32
			addBinaryBits(&ch, b, 4)
			addBinaryBits(&ch, b1, 6)
			addBinaryBits(&ch, b2, 6)
			addBinaryBits(&ch, b3, 6)
			res = append(res, ch)
			i += 4
		default:
			return nil, errors.New("utf data format is invalid")
		}
	}
	return res, nil
}

func unicodeToUtf8(str []uint32) []byte {
	var res []byte
	for _, ch := range str {
		if ch < 0x80 {
			res = append(res, byte(ch))
		} else if ch < 0x800 {
			res = append(res, byte(0xc0|ch>>6), byte(0x80|ch&0x3f))
		} else if ch < 0x10000 {
			res = append(res, byte(0xe0|ch>>12), byte(0x80|ch>>6&0x3f), byte(0x80|ch&0x3f))
		} else {
			res = append(res, byte(0xf0|ch>>18), byte(0x80|ch>>12&0x3f), byte(0x80|ch>>6&0x3f), byte(0x80|ch&0x3f))
		}
	}
	return res
}

// Utf8EncodeBySpecificLength force encode unicode bytes to utf8 bytes by specific encode length
func Utf8EncodeBySpecificLength(str []byte, l int) []byte {
	unicodeList, err := utf8ToUnicode(str)
	if err != nil {
		log.Errorf("utf8ToUnicode failed: %s", err)
		return str
	}
	if l == 0 {
		return str
	}
	getChByteLength := func(ch uint32) int {
		if ch < 0x80 {
			return 1
		} else if ch < 0x800 {
			return 2
		} else if ch < 0x10000 {
			return 3
		} else {
			return 4
		}
	}
	encodeBySpecificLength := func(ch uint32, l int) []byte {
		switch l {
		case 1:
			return []byte{byte(ch)}
		case 2:
			buf := bytes.Buffer{}
			buf.WriteByte(0xc0 | byte(ch>>6))
			buf.WriteByte(0x80 | byte(ch&0x3f))
			return buf.Bytes()
		case 3:
			buf := bytes.Buffer{}
			buf.WriteByte(0xe0 | byte(ch>>12))
			buf.WriteByte(0x80 | byte((ch>>6)&0x3f))
			buf.WriteByte(0x80 | byte(ch&0x3f))
			return buf.Bytes()
		case 4:
			buf := bytes.Buffer{}
			buf.WriteByte(0xf0 | byte(ch>>18))
			buf.WriteByte(0x80 | byte((ch>>12)&0x3f))
			buf.WriteByte(0x80 | byte((ch>>6)&0x3f))
			buf.WriteByte(0x80 | byte(ch&0x3f))
			return buf.Bytes()
		default:
			return str
		}
	}
	var res []byte
	for _, u := range unicodeList {
		var maxL = getChByteLength(u)
		for maxL < l {
			maxL = l
		}
		res = append(res, encodeBySpecificLength(u, maxL)...)
	}
	return res
}

func IsPlainText(raw []byte) bool {
	typ, _ := filetype.Match(raw)
	if typ == filetype.Unknown {
		return true
	}

	if _, ok := matchers.Application[typ]; ok {
		return false
	}
	if _, ok := matchers.Archive[typ]; ok {
		return false
	}
	if _, ok := matchers.Video[typ]; ok {
		return false
	}
	if _, ok := matchers.Audio[typ]; ok {
		return false
	}
	if _, ok := matchers.Image[typ]; ok {
		return false
	}
	if _, ok := matchers.Document[typ]; ok {
		return false
	}
	if _, ok := matchers.Font[typ]; ok {
		return false
	}

	return true
}

func MIMEGlobRuleCheck(target string, rule string) bool {
	if strings.Contains(rule, "/") && strings.Contains(target, "/") { // 如果两个都包含/，则进行分割匹配
		ruleType := strings.SplitN(rule, "/", 2)
		targetType := strings.SplitN(target, "/", 2)
		for i := 0; i < 2; i++ {
			if strings.Contains(ruleType[i], "*") {
				rule, err := glob.Compile(ruleType[i])
				if err != nil || !rule.Match(targetType[i]) {
					return false // 任意部分匹配失败则 false,包括glob编译失败
				}
			} else {
				if ruleType[i] != targetType[i] {
					return false // 任意部分匹配失败则 false
				}
			}
		}
		return true // 全部通过 true
	}

	if !strings.Contains(target, "/") && !strings.Contains(rule, "/") { // 如果都不包含 /
		if strings.Contains(rule, "*") { // 尝试glob 匹配
			rule, err := glob.Compile(rule)
			if err == nil && rule.Match(target) {
				return true
			}
		} else { // 直接 contains
			if IContains(target, rule) {
				return true
			}
		}
		return false
	}

	if strings.Contains(target, "/") && !strings.Contains(rule, "/") { // 仅rule不包含 /
		targetType := strings.SplitN(target, "/", 2)
		for i := 0; i < 2; i++ {
			if strings.Contains(rule, "*") {
				rule, err := glob.Compile(rule)
				if err != nil {
					continue
				}
				if rule.Match(targetType[i]) {
					return true // 任意部分匹配成功 则true
				}
			} else {
				if rule == targetType[i] {
					return true // 任意部分匹配成功 则true
				}
			}
		}
		return false // 全部失败 则false
	}

	return false // 仅 rule 有 / 则直接返回 false
}

func UnquoteANSICWithQuote(s string, quote rune) (string, error) {
	// 检查是否以单引号开始和结束
	if quote != 0 {
		if len(s) < 2 || s[0] != byte(quote) || s[len(s)-1] != byte(quote) {
			return "", fmt.Errorf("string must begin and end with %c", quote)
		}
		// 去掉首尾的引号
		s = s[1 : len(s)-1]
	}

	var result strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			result.WriteByte(s[i])
			continue
		}

		// 处理转义序列
		if i+1 >= len(s) {
			return "", fmt.Errorf("invalid escape sequence at end of string")
		}

		i++
		switch s[i] {
		case 'a':
			result.WriteByte('\a')
		case 'b':
			result.WriteByte('\b')
		case 'f':
			result.WriteByte('\f')
		case 'n':
			result.WriteByte('\n')
		case 'r':
			result.WriteByte('\r')
		case 't':
			result.WriteByte('\t')
		case 'v':
			result.WriteByte('\v')
		case '\\':
			result.WriteByte('\\')
		case '\'':
			result.WriteByte('\'')
		case '"':
			result.WriteByte('"')
		case '?':
			result.WriteByte('?')
		case 'e', 'E':
			result.WriteByte('\033')
		case 'x':
			// 处理十六进制转义序列 \xHH
			if i+2 >= len(s) {
				return "", fmt.Errorf("invalid hex escape sequence")
			}
			hex := s[i+1 : i+3]
			n, err := strconv.ParseUint(hex, 16, 8)
			if err != nil {
				return "", fmt.Errorf("invalid hex escape sequence: %s", hex)
			}
			result.WriteByte(byte(n))
			i += 2
		case '0', '1', '2', '3', '4', '5', '6', '7':
			// 处理八进制转义序列 \ooo
			end := i + 1
			for end < len(s) && end-i < 3 && s[end] >= '0' && s[end] <= '7' {
				end++
			}
			oct := s[i:end]
			n, err := strconv.ParseUint(oct, 8, 8)
			if err != nil {
				return "", fmt.Errorf("invalid octal escape sequence: %s", oct)
			}
			result.WriteByte(byte(n))
			i = end - 1
		case 'u':
			// 处理Unicode转义序列 \uHHHH
			if i+4 >= len(s) {
				return "", fmt.Errorf("invalid unicode escape sequence")
			}
			hex := s[i+1 : i+5]
			n, err := strconv.ParseUint(hex, 16, 16)
			if err != nil {
				return "", fmt.Errorf("invalid unicode escape sequence: %s", hex)
			}
			v := rune(n)
			if !utf8.ValidRune(v) {
				return "", fmt.Errorf("invalid unicode escape sequence: %s", hex)
			}
			result.WriteRune(v)
			i = i + 4
		case 'U':
			// 处理Unicode转义序列 \UHHHHHHHH
			if i+8 >= len(s) {
				return "", fmt.Errorf("invalid long unicode escape sequence")
			}
			hex := s[i+1 : i+9]
			n, err := strconv.ParseUint(hex, 16, 32)
			if err != nil {
				return "", fmt.Errorf("invalid long unicode escape sequence: %s", hex)
			}
			v := rune(n)
			if !utf8.ValidRune(v) {
				return "", fmt.Errorf("invalid long unicode escape sequence: %s", hex)
			}
			result.WriteRune(v)
			i = i + 8
		default:
			return "", fmt.Errorf("invalid escape sequence: \\%c", s[i])
		}
	}
	return result.String(), nil
}

// UnquoteANSIC 解码ANSI-C风格的引号字符串
func UnquoteANSIC(s string) (string, error) {
	return UnquoteANSICWithQuote(s, '\'')
}

func QuoteCSV(s string) string {
	if strings.ContainsAny(s, ",\"\n") {
		s = strings.ReplaceAll(s, `"`, `""`)
		return fmt.Sprintf(`"%s"`, s)
	}
	return s
}

func UnquoteCSV(s string) string {
	if strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"") {
		s = strings.ReplaceAll(s, `""`, `"`)
	}
	return s
}

func RuneIndex(s, sub []rune) int {
	n := len(sub)
	switch {
	case n == 0:
		return 0
	case n == 1:
		return strings.IndexRune(string(s), sub[0])
	case n == len(s):
		if string(sub) == string(s) {
			return 0
		}
		return -1
	case n > len(s):
		return -1
	default:
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			if s[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func CutBytesPrefixFunc(raw []byte, handle func(rune) bool) ([]byte, []byte, bool) {
	index := bytes.IndexFunc(raw, handle)
	if index < 0 {
		return nil, raw, false
	}
	return raw[:index], raw[index:], true
}

func NotSpaceRune(r rune) bool {
	return !unicode.IsSpace(r)
}

// PrefixLines 为文本的每一行添加指定前缀
// 对所有输入（包括单行和多行）都添加前缀
// Example:
// ```
// str.PrefixLines("hello", "> ") // "> hello"
// str.PrefixLines("line1\nline2", "> ") // "> line1\n> line2"
// ```
func PrefixLines(input interface{}, prefix string) string {
	var content string

	switch v := input.(type) {
	case string:
		content = v
	case io.Reader:
		data, err := io.ReadAll(v)
		if err != nil {
			log.Errorf("failed to read from io.Reader: %v", err)
			return ""
		}
		content = string(data)
	default:
		content = InterfaceToString(input)
	}

	lines := strings.Split(content, "\n")
	var result []string

	for _, line := range lines {
		result = append(result, prefix+line)
	}

	return strings.Join(result, "\n")
}

// PrefixLinesWithLineNumbers 为文本的每一行添加行号前缀
// 对所有输入（包括单行和多行）都添加行号前缀
// Example:
// ```
// str.PrefixLinesWithLineNumbers("hello") // "1 | hello"
// str.PrefixLinesWithLineNumbers("line1\nline2") // "1 | line1\n2 | line2"
// ```
func PrefixLinesWithLineNumbers(input interface{}) string {
	var content string

	switch v := input.(type) {
	case string:
		content = v
	case io.Reader:
		data, err := io.ReadAll(v)
		if err != nil {
			log.Errorf("failed to read from io.Reader: %v", err)
			return ""
		}
		content = string(data)
	default:
		content = InterfaceToString(input)
	}

	lines := strings.Split(content, "\n")
	var result []string

	for i, line := range lines {
		result = append(result, fmt.Sprintf("%d | %s", i+1, line))
	}

	return strings.Join(result, "\n")
}

// PrefixLinesReader 为文本的每一行添加指定前缀，输入和输出都是 io.Reader
// 对所有输入（包括单行和多行）都添加前缀，适合处理大文件或流式数据
// Example:
// ```
// reader := strings.NewReader("line1\nline2")
// prefixedReader := str.PrefixLinesReader(reader, "> ")
// // 读取 prefixedReader 会得到 "> line1\n> line2"
// ```
func PrefixLinesReader(input io.Reader, prefix string) io.Reader {
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()

		// 读取所有内容
		data, err := io.ReadAll(input)
		if err != nil {
			log.Errorf("PrefixLinesReader read error: %v", err)
			return
		}

		content := string(data)

		// 为所有行添加前缀
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			if i > 0 {
				pw.Write([]byte("\n"))
			}
			pw.Write([]byte(prefix + line))
		}
	}()

	return pr
}

// PrefixLinesReaderSimple 简化版本的 PrefixLinesReader，总是添加前缀
// 适合已知是多行文本的场景，使用流式处理
func PrefixLinesReaderSimple(input io.Reader, prefix string) io.Reader {
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()

		scanner := bufio.NewScanner(input)
		isFirst := true

		for scanner.Scan() {
			line := scanner.Text()
			if !isFirst {
				pw.Write([]byte("\n"))
			}
			pw.Write([]byte(prefix + line))
			isFirst = false
		}

		if err := scanner.Err(); err != nil {
			log.Errorf("PrefixLinesReaderSimple scanner error: %v", err)
		}
	}()

	return pr
}

// PrefixLinesWithLineNumbersReader 为文本的每一行添加行号前缀，输入和输出都是 io.Reader
// 对所有输入（包括单行和多行）都添加行号前缀，适合处理大文件或流式数据
// Example:
// ```
// reader := strings.NewReader("line1\nline2")
// numberedReader := str.PrefixLinesWithLineNumbersReader(reader)
// // 读取 numberedReader 会得到 "1 | line1\n2 | line2"
// ```
func PrefixLinesWithLineNumbersReader(input io.Reader) io.Reader {
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()

		// 读取所有内容
		data, err := io.ReadAll(input)
		if err != nil {
			log.Errorf("PrefixLinesWithLineNumbersReader read error: %v", err)
			return
		}

		content := string(data)

		// 为所有行添加行号前缀
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			if i > 0 {
				pw.Write([]byte("\n"))
			}
			pw.Write([]byte(fmt.Sprintf("%d | %s", i+1, line)))
		}
	}()

	return pr
}

// PrefixLinesWithLineNumbersReaderSimple 简化版本，总是添加行号前缀，使用流式处理
func PrefixLinesWithLineNumbersReaderSimple(input io.Reader) io.Reader {
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()

		scanner := bufio.NewScanner(input)
		lineNumber := 1
		isFirst := true

		for scanner.Scan() {
			line := scanner.Text()
			if !isFirst {
				pw.Write([]byte("\n"))
			}
			pw.Write([]byte(fmt.Sprintf("%d | %s", lineNumber, line)))
			lineNumber++
			isFirst = false
		}

		if err := scanner.Err(); err != nil {
			log.Errorf("PrefixLinesWithLineNumbersReaderSimple scanner error: %v", err)
		}
	}()

	return pr
}
