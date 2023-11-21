package utils

import (
	"bufio"
	"bytes"
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
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/davecgh/go-spew/spew"
	"github.com/gobwas/glob"
	"github.com/pkg/errors"
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

func StringGlobArrayContains(array []string, element string, seps ...rune) bool {
	for _, r := range array {
		if !strings.Contains(r, "*") {
			if IContains(element, r) {
				return true
			}
			continue
		}
		if !strings.HasSuffix(r, "*") {
			r += "*"
		}
		if !strings.HasPrefix(r, "*") {
			r = "*" + r
		}
		rule, err := glob.Compile(r, seps...)
		if err != nil {
			continue
		}
		if rule.Match(element) {
			return true
		}
		continue
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
	for k, _ := range m {
		n = append(n, k)
	}
	return n
}

func StringSplitAndStrip(raw string, sep string) []string {
	var l = []string{}

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

func InterfaceToBytes(i interface{}) (result []byte) {
	return codec.AnyToBytes(i)
}

func InterfaceToString(i interface{}) string {
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
	var result = make([]int, 0)
	for _, v := range i {
		result = append(result, int(v))
	}
	return result
}

func IntSliceToInt64Slice(i []int) []int64 {
	var result = make([]int64, len(i))
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

	va := reflect.ValueOf(i)
	switch reflect.TypeOf(i).Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < va.Len(); i++ {
			result = append(result, InterfaceToString(va.Index(i).Interface()))
		}
	default:
		result = append(result, InterfaceToString(i))
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
	var finalResult = make(map[string][]string)
	var res = make(map[string]interface{})
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

	var urlHttps = strings.HasPrefix(res, "https://")
	var isUrl = IsHttpOrHttpsUrl(res)

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
		if !strings.HasSuffix(u.Path, "/") {
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
				if !strings.HasSuffix(u.Path, "/") {
					u.Path += "/"
				}
				continue
			default:
				break PATHCLEAN
			}
		}

		reqUri := u.Path + pathRaw
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
	var (
		result [][]string
	)

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

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// RandStringBytes 返回在大小写字母表中随机挑选 n 个字符组成的字符串
// Example:
// ```
// str.RandStr(10)
// ```
func RandStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
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

const (
	passwordSepcialChars = ",.<>?;:[]{}~!@#$%^&*()_+-="
	AllSepcialChars      = ",./<>?;':\"[]{}`~!@#$%^&*()_+-=\\|"
	LittleChar           = "abcdefghijklmnopqrstuvwxyz"
	BigChar              = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	NumberChar           = "1234567890"
)

var (
	passwordBase = passwordSepcialChars + LittleChar + BigChar + NumberChar
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
			b[i] = passwordBase[rand.Intn(len(passwordBase))]
		}

		result := IsStrongPassword(string(b))
		if result {
			return string(b)
		}
	}
}

func RandSample(n int, material ...string) string {
	b := make([]rune, n)

	var base = LittleChar + BigChar + NumberChar
	if ret := strings.Join(material, ""); ret != "" {
		base = ret
	}
	for i := range b {
		b[i] = rune(base[rand.Intn(len([]rune(base)))])
	}
	return string(b)
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

		var rawPath = ExtractRawPath(target)

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
