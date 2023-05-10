package utils

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
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
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ReneKroon/ttlcache"
	"github.com/davecgh/go-spew/spew"
	"github.com/gobwas/glob"
	"github.com/miekg/dns"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/retryabledns"
	"github.com/projectdiscovery/retryabledns/hostsfile"
)

func ExtractStrContextByKeyword(raw []byte, res []string) []string {
	rawStrContent := string(raw)
	var details []string
	for _, keyword := range res {
		if index := strings.Index(rawStrContent, keyword); index > 0 {
			info := ""

			end := index + len(keyword) + 512

			if index <= 512 {
				info += rawStrContent[:index]
			} else {
				info += rawStrContent[index-512 : index+len(keyword)]
			}

			if end >= len(rawStrContent) {
				info += rawStrContent[index:]
			} else {
				info += rawStrContent[index:end]
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

func StringSliceContainsAll(o []string, elements ...string) bool {
	for _, e := range elements {
		if !StringArrayContains(o, e) {
			return false
		}
	}
	return true
}

// 元素去重
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

func StringGlobArrayContains(array []string, element string) bool {
	var globs []glob.Glob
	var strList []string
	for _, r := range array {
		rule, err := glob.Compile(r)
		if err != nil {
			strList = append(strList, r)
			continue
		}
		globs = append(globs, rule)
	}

	for _, s := range array {
		if element == s {
			return true
		}
	}

	for _, g := range globs {
		if g.Match(element) {
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

func BytesStripUntil(raw []byte, s byte) []byte {
	for end := len(raw); end > 0; end-- {
		c_index := end - 1
		if raw[c_index] == s {
			return raw[:c_index]
		}
	}
	return []byte{}
}

func ExtractRawHeadersFromRequest(req *http.Request) string {
	header := []string{}

	header = append(header, fmt.Sprintf("%s %s %s", req.Method, req.URL.Path, req.Proto))

	for headerName, headerValues := range req.Header {
		for _, value := range headerValues {
			header = append(header, fmt.Sprintf("%s: %s", headerName, value))
		}
	}

	header = append(header, "")
	return strings.Join(header, "\r\n")
}

func ExtractRawHeadersFromResponse(rsp *http.Response) string {
	header := []string{}

	header = append(header, fmt.Sprintf("%s %s", rsp.Proto, rsp.Status))

	for headerName, headerValues := range rsp.Header {
		for _, value := range headerValues {
			header = append(header, fmt.Sprintf("%s: %s", headerName, value))
		}
	}

	header = append(header, "")
	return strings.Join(header, "\r\n")
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

var (
	dnsTtlCache = ttlcache.NewCache()
)

func GetFirstIPByDnsWithCache(domain string, timeout time.Duration, dnsServers ...string) string {
	data, ok := dnsTtlCache.Get(domain)
	if ok {
		if raw, _ := data.(string); raw != "" && !IsLoopback(raw) {
			dnsTtlCache.SetWithTTL(domain, raw, 30*time.Second)
			return raw
		}
	}

	var count = 0
	for {
		count++
		r := GetFirstIPFromHostWithTimeout(timeout, domain, dnsServers)
		if r == "" {
			if count >= 5 {
				return ""
			}
			continue
		}
		dnsTtlCache.SetWithTTL(domain, r, 30*time.Second)
		return r
	}
}

func InterfaceToBytes(i interface{}) (result []byte) {
	var bytes []byte
	defer func() {
		if err := recover(); err != nil {
			result = []byte(fmt.Sprintf("%v", i))
		}
	}()

	if i == nil {
		return []byte{}
	}

	switch s := i.(type) {
	case nil:
		return []byte{}
	case string:
		bytes = []byte(s)
	case []byte:
		bytes = s[0:]
	case bool:
		bytes = []byte(strconv.FormatBool(s))
	case float64:
		return []byte(strconv.FormatFloat(s, 'f', -1, 64))
	case float32:
		return []byte(strconv.FormatFloat(float64(s), 'f', -1, 32))
	case int:
		return []byte(strconv.Itoa(s))
	case int64:
		return []byte(strconv.FormatInt(s, 10))
	case int32:
		return []byte(strconv.Itoa(int(s)))
	case int16:
		return []byte(strconv.FormatInt(int64(s), 10))
	case int8:
		return []byte(strconv.FormatInt(int64(s), 10))
	case uint:
		return []byte(strconv.FormatUint(uint64(s), 10))
	case uint64:
		return []byte(strconv.FormatUint(s, 10))
	case uint32:
		return []byte(strconv.FormatUint(uint64(s), 10))
	case uint16:
		return []byte(strconv.FormatUint(uint64(s), 10))
	case uint8:
		return []byte(strconv.FormatUint(uint64(s), 10))
	case fmt.Stringer:
		return []byte(s.String())
	case error:
		return []byte(s.Error())
	//case io.Reader:
	//	if ret != nil && ret.Read != nil {
	//		bytes, _ = ioutil.ReadAll(ret)
	//		return bytes
	//	}
	//	return []byte(fmt.Sprintf("%v", i))
	default:
		bytes = []byte(fmt.Sprintf("%v", i))
	}

	return bytes
}

func InterfaceToString(i interface{}) string {
	return string(InterfaceToBytes(i))
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
	var result = make([]int, len(i))
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

func InterfaceToStringSlice(i interface{}) []string {
	var result []string
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("str.ToStringSlice failed: %s", err)
			spew.Dump(i)
			PrintCurrentGoroutineRuntimeStack()
		}
	}()

	if i == nil {
		return nil
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
			spew.Dump(ret)
			value := reflect.ValueOf(ret)
			for _, keyValue := range value.MapKeys() {
				finalResult[keyValue.String()] = []string{InterfaceToString(value.MapIndex(keyValue).Interface())}
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

func ParseStringUrlToWebsiteRootPath(s string) string {
	ins, _ := ParseStringUrlToUrlInstance(s)
	if ins == nil {
		return s
	}

	ins.Path = "/"
	ins.RawPath = "/"
	ins.RawQuery = ""
	return ins.String()
}

func ParseStringUrlToUrlInstance(s string) (*url.URL, error) {
	return url.Parse(s)
}

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
		return "", 0, errors.Errorf("unknown port for [%s]", raw)
	}

	portStr = strings.TrimSpace(portStr)
	portInt64, err := strconv.ParseInt(portStr, 10, 32)
	if err != nil {
		return "", 0, errors.Errorf("%s parse port(%s) failed: %s", raw, portStr, err)
	}

	port = int(portInt64)
	err = nil
	return
}

/*
https://baidu.com/abc   a?key=value
https://baidu.com/abc/a?key=value => [X] https://baidu.com/abc/a%xxkey=value

[X] https://baidu.com/a?key=value
*/
func UrlJoin(origin string, paths ...string) (string, error) {
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

func Atoi(i string) int {
	raw, _ := strconv.Atoi(i)
	return raw
}

func Atof(i string) float64 {
	raw, _ := strconv.ParseFloat(i, 64)
	return raw
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

func DomainToIP(domain string, timeout time.Duration) []string {
	if net.ParseIP(FixForParseIP(domain)) == nil {
		// 不是 IP 尝试 dns 查询下
		ctx, _ := context.WithTimeout(context.Background(), timeout)
		rets, err := net.DefaultResolver.LookupIPAddr(ctx, domain)
		if err != nil {
			return nil
		}

		var addrs []string
		for _, i := range rets {
			addrs = append(addrs, i.String())
		}
		return addrs
	} else {
		return []string{domain}
	}
}

var (
	DefaultDNSClient = dns.Client{
		Timeout:      5 * time.Second,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
	RetryableDNSClient = &retryabledns.Client{
		TCPFallback: false,
	}
	DefaultDNSConn   = dns.Dial
	DefaultDNSServer = []string{
		"223.5.5.5",       // ali
		"119.29.29.29",    // tencent
		"180.76.76.76",    // baidu
		"114.114.114.114", // dianxin
		"1.1.1.1",         // cf
		"8.8.8.8",
	}
)

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

var hostsRecords map[string][]string

func init() {
	raw, _ := hostsfile.ParseDefault()
	if raw != nil {
		hostsRecords = raw
	}
}

func GetIPFromHostWithContextAndDNSServers(
	timeout time.Duration, domain string, DNSServers []string, cb func(domain string) bool,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	DNSClient := DefaultDNSClient
	DNSClient.Timeout = timeout
	DNSClient.DialTimeout = timeout
	DNSClient.ReadTimeout = timeout
	DNSClient.WriteTimeout = timeout
	if strings.TrimSpace(domain) == "" {
		return Error("empty domain input")
	}
	//ctx, cancel := context.WithCancel(ctx)
	//defer cancel()
	if cb == nil {
		return Errorf("callback cannot be empty")
	}

	if records, ok := hostsRecords[domain]; ok {
		for _, r := range records {
			if cb(r) {
				continue
			} else {
				return nil
			}
		}
	}

	nativeDNS := func() bool {
		defer func() {
			if err := recover(); err != nil {
				log.Error(err)
			}
		}()

		if domain == "" {
			return false
		}

		ips, err := net.DefaultResolver.LookupIPAddr(ctx, strings.TrimRight(domain, "."))
		if err != nil {
			log.Errorf("default resolver parse failed: %s", err)
			return false
		}
		for _, i := range ips {
			if cb(i.String()) {
				continue
			} else {
				return true
			}
		}
		if len(ips) <= 0 {
			return false
		}
		return true
	}

	if DNSServers == nil {
		DNSServers = DefaultDNSServer
	}

	var (
		errs []error
	)

	haveResult := NewBool(false)
	callback := func(domain string) bool {
		haveResult.Set()
		return cb(domain)
	}

	if !strings.HasSuffix(domain, ".") {
		domain = domain + "."
	}
Main:
	for _, qType := range []uint16{dns.TypeA, dns.TypeAAAA} {
		var typeRaw = "A/AAAA"
		switch qType {
		case dns.TypeAAAA:
			typeRaw = "AAAA"
		case dns.TypeA:
			typeRaw = "A"
		}
		for _, server := range DNSServers {
			switch typeRaw {
			case "A":
				data, err := RetryableDNSClient.A(domain)
				if err != nil {
					errs = append(errs, Errorf("DNS(%v)[%v] Err: %v", server, typeRaw, err))
					continue
				}
				for _, record := range data.A {
					if callback(record) {
						continue
					} else {
						break Main
					}
				}
			case "AAAA":
				data, err := RetryableDNSClient.AAAA(domain)
				if err != nil {
					errs = append(errs, Errorf("DNS(%v)[%v] Err: %v", server, typeRaw, err))
					continue
				}
				for _, record := range data.AAAA {
					if callback(record) {
						continue
					} else {
						break Main
					}
				}
			}

			server = ToNsServer(server)
			m := dns.Msg{}
			m.SetQuestion(domain, qType)
			rsp, _, err := DNSClient.Exchange(&m, server)
			if err != nil {
				//log.Errorf("query dns failed: %s", err)
				errs = append(errs, Errorf("DNS(%v)[%v] Err: %v", server, typeRaw, err))
				continue
			}

			for _, ans := range rsp.Answer {
				switch record := ans.(type) {
				case *dns.A:
					if record.A.String() != "" {
						if callback(record.A.String()) {
							continue
						} else {
							break Main
						}
					}
				case *dns.AAAA:
					if record.AAAA.String() != "" {
						if callback(record.AAAA.String()) {
							continue
						} else {
							break Main
						}
					}
				}
			}
		}
	}

	if !haveResult.IsSet() {
		if nativeDNS() {
			return nil
		}
		return Errorf("empty results: QueryErrors: \n"+
			"%v"+
			"\n", spew.Sdump(errs))
	}

	return nil
}

func GetFirstIPFromHostWithContextE(timeout time.Duration, domain string, DNSServers []string) (string, error) {
	var Result string
	err := GetIPFromHostWithContextAndDNSServers(timeout, domain, DNSServers, func(domain string) bool {
		Result = domain
		return false
	})
	return Result, err
}

func GetIPsFromHostWithContextE(timeout time.Duration, domain string, DNSServers []string) ([]string, error) {
	var results []string
	err := GetIPFromHostWithContextAndDNSServers(timeout, domain, DNSServers, func(domain string) bool {
		results = append(results, domain)
		return true
	})
	return results, err
}

func GetIPsFromHostWithTimeoutE(timeout time.Duration, domain string, dnsServers []string) ([]string, error) {
	return GetIPsFromHostWithContextE(timeout, domain, dnsServers)
}

func GetFirstIPFromHostWithTimeoutE(timeout time.Duration, domain string, dnsServres []string) (string, error) {
	return GetFirstIPFromHostWithContextE(timeout, domain, dnsServres)
}

func GetFirstIPFromHostWithTimeout(timeout time.Duration, domain string, dnsServres []string) string {
	s, _ := GetFirstIPFromHostWithTimeoutE(timeout, domain, dnsServres)
	return s
}

func GetIPsFromHostWithTimeout(timeout time.Duration, domain string, dnsServers []string) []string {
	r, _ := GetIPsFromHostWithTimeoutE(timeout, domain, dnsServers)
	return r
}

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

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

func ParseStringToLines(raw string) []string {
	var lines []string

	scanner := bufio.NewScanner(bytes.NewBufferString(raw))
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line == "" {
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

func ParseStringToCClassHosts(targets string) string {
	var target []string
	var cclassMap = new(sync.Map)
	for _, r := range ParseStringToHosts(targets) {
		if IsIPv4(r) {
			netStr, err := IPv4ToCClassNetwork(r)
			if err != nil {
				target = append(target, r)
				continue
			}
			cclassMap.Store(netStr, nil)
			continue
		}

		if IsIPv6(r) {
			target = append(target, r)
			continue
		}

		ip := GetFirstIPFromHostWithTimeout(5*time.Second, r, nil)
		if ip != "" && IsIPv4(ip) {
			netStr, err := IPv4ToCClassNetwork(ip)
			if err != nil {
				target = append(target, r)
				continue
			}
			cclassMap.Store(netStr, nil)
			continue
		} else {
			target = append(target, r)
		}
	}
	cclassMap.Range(func(key, value interface{}) bool {
		s := key.(string)
		target = append(target, s)
		return true
	})
	return strings.Join(target, ",")
}
func Format(raw string, data map[string]string) string {
	for k, v := range data {
		raw = strings.ReplaceAll(raw, "$"+k, v)
	}
	return raw
}
