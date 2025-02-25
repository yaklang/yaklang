package yaklib

import (
	"net/url"
	"regexp"

	"github.com/yaklang/yaklang/common/utils"
)

var (
	// RE_HOSTNAME = regexp.MustCompile(`\b(?:[0-9A-Za-z][0-9A-Za-z-]{0,62})(?:\.(?:[0-9A-Za-z][0-9A-Za-z-]{0,62}))*(\.?|\b)`)
	RE_IPV4      = regexp.MustCompile(`(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)`)
	RE_IPV6      = regexp.MustCompile(`((([0-9A-Fa-f]{1,4}:){7}([0-9A-Fa-f]{1,4}|:))|(([0-9A-Fa-f]{1,4}:){6}(:[0-9A-Fa-f]{1,4}|((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3})|:))|(([0-9A-Fa-f]{1,4}:){5}(((:[0-9A-Fa-f]{1,4}){1,2})|:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3})|:))|(([0-9A-Fa-f]{1,4}:){4}(((:[0-9A-Fa-f]{1,4}){1,3})|((:[0-9A-Fa-f]{1,4})?:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9A-Fa-f]{1,4}:){3}(((:[0-9A-Fa-f]{1,4}){1,4})|((:[0-9A-Fa-f]{1,4}){0,2}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9A-Fa-f]{1,4}:){2}(((:[0-9A-Fa-f]{1,4}){1,5})|((:[0-9A-Fa-f]{1,4}){0,3}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9A-Fa-f]{1,4}:){1}(((:[0-9A-Fa-f]{1,4}){1,6})|((:[0-9A-Fa-f]{1,4}){0,4}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(:(((:[0-9A-Fa-f]{1,4}){1,7})|((:[0-9A-Fa-f]{1,4}){0,5}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:)))(%.+)?`)
	RE_MAC       = regexp.MustCompile(`((?:(?:[A-Fa-f0-9]{4}\.){2}[A-Fa-f0-9]{4})|(?:(?:[A-Fa-f0-9]{2}-){5}[A-Fa-f0-9]{2})|(?:(?:[A-Fa-f0-9]{2}:){5}[A-Fa-f0-9]{2}))`)
	RE_HOSTPORT  = regexp.MustCompile(`(((?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?))|(((([0-9A-Fa-f]{1,4}:){7}([0-9A-Fa-f]{1,4}|:))|(([0-9A-Fa-f]{1,4}:){6}(:[0-9A-Fa-f]{1,4}|((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3})|:))|(([0-9A-Fa-f]{1,4}:){5}(((:[0-9A-Fa-f]{1,4}){1,2})|:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3})|:))|(([0-9A-Fa-f]{1,4}:){4}(((:[0-9A-Fa-f]{1,4}){1,3})|((:[0-9A-Fa-f]{1,4})?:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9A-Fa-f]{1,4}:){3}(((:[0-9A-Fa-f]{1,4}){1,4})|((:[0-9A-Fa-f]{1,4}){0,2}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9A-Fa-f]{1,4}:){2}(((:[0-9A-Fa-f]{1,4}){1,5})|((:[0-9A-Fa-f]{1,4}){0,3}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9A-Fa-f]{1,4}:){1}(((:[0-9A-Fa-f]{1,4}){1,6})|((:[0-9A-Fa-f]{1,4}){0,4}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(:(((:[0-9A-Fa-f]{1,4}){1,7})|((:[0-9A-Fa-f]{1,4}){0,5}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:)))(%.+)?)):(\b(?:[1-9][0-9]*)\b)`)
	RE_URL       = regexp.MustCompile(`[A-Za-z]+(\+[A-Za-z+]+)?://\S+`)
	RE_PATH      = regexp.MustCompile(`(?:/[A-Za-z0-9$.+!*'(){},~:;=@#%_\-]*)+`)
	RE_PATHPARAM = regexp.MustCompile(`(?:/[A-Za-z0-9$.+!*'(){},~:;=@#%_\-]*)+(?:\?[A-Za-z0-9$.+!*'|(){},~@#%&/=:;_?\-\[\]<>]*)?`)
	RE_EMAIL     = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9_.+-=:]+@\b(?:[0-9A-Za-z][0-9A-Za-z-]{0,62})(?:\.(?:[0-9A-Za-z][0-9A-Za-z-]{0,62}))*(\.?|\b)`)
	RE_TTY       = regexp.MustCompile(`(?:/dev/(pts|tty([pq])?)(\w+)?/?(?:[0-9]+))`)
)

// ExtractIPv4 提取字符串中所有的 IPv4 地址
// Example:
// ```
// re.ExtractIPv4("hello your local ip is 127.0.0.1, your public ip is 1.1.1.1") // ["127.0.0.1", "1.1.1.1"]
// ```
func RegexpMatchIPv4(i interface{}) []string {
	var res []string
	for _, group := range RE_IPV4.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		res = append(res, group[0])
	}
	return res
}

// ExtractIPv6 提取字符串中所有的 IPv6 地址
// Example:
// ```
// re.ExtractIPv6("hello your local ipv6 ip is fe80::1, your public ipv6 ip is 2001:4860:4860::8888") // ["fe80::1", "2001:4860:4860::8888"]
// ```
func RegexpMatchIPv6(i interface{}) []string {
	var res []string
	for _, group := range RE_IPV6.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		res = append(res, group[0])
	}
	return res
}

// ExtractMac 提取字符串中所有的 MAC 地址
// Example:
// ```
// re.ExtractMac("hello your mac is 00:00:00:00:00:00") // ["00:00:00:00:00:00"]
// ```
func RegexpMatchMac(i interface{}) []string {
	var res []string
	for _, group := range RE_MAC.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		res = append(res, group[0])
	}
	return res
}

// ExtractIP 提取字符串中所有的 IP 地址
// Example:
// ```
// re.ExtractIP("hello your local ip is 127.0.0.1, your local ipv6 ip is fe80::1") // ["127.0.0.1", "fe80::1"]
// ```
func RegexpMatchIP(i interface{}) []string {
	var res []string
	for _, group := range RE_IPV6.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		res = append(res, group[0])
	}

	for _, group := range RE_IPV4.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		res = append(res, group[0])
	}
	return res
}

// ExtractHostPort 提取字符串中所有的 Host:Port
// Example:
// ```
// re.ExtractHostPort("Open Host:Port\n127.0.0.1:80\n127.0.0.1:443") // ["127.0.0.1:80", "127.0.0.1:443"]
// ```
func RegexpMatchHostPort(i interface{}) []string {
	var res []string
	for _, group := range RE_HOSTPORT.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		res = append(res, group[0])
	}
	return res
}

// ExtractPath 提取URL中的路径和查询字符串
// Example:
// ```
// re.ExtractPath("visit this website: yaklang.com/docs/api/re?name=anonymous") // ["/docs/api/re?name=anonymous"]
// ```
func RegexpMatchPathParam(i interface{}) []string {
	var res []string
	var allIndexs [][]int
	inputData := utils.InterfaceToBytes(i)
	urlIndexs := RE_URL.FindAllSubmatchIndex(inputData, -1)
	pathIndexs := RE_PATHPARAM.FindAllSubmatchIndex(inputData, -1)
	allIndexs = append(allIndexs, urlIndexs...)
	// 如果pathIndex的范围和urlIndex的范围重叠，则不添加到allIndexs中, 否则添加到allIndexs中
	for _, pathIndex := range pathIndexs {
		inUrlMatch := false
		for _, urlIndex := range urlIndexs {
			if pathIndex[0] >= urlIndex[0] && pathIndex[1] <= urlIndex[1] {
				inUrlMatch = true
				break
			}
		}
		if !inUrlMatch {
			allIndexs = append(allIndexs, pathIndex)
		}
	}

	for _, index := range allIndexs {
		submatch := string(inputData[index[0]:index[1]])
		u, err := url.Parse(submatch)
		if err != nil {
			continue
		}
		p := u.RawPath
		if p == "" {
			p = u.Path
		}
		query := "?" + u.RawQuery
		if u.ForceQuery {
			query = "?"
		}
		res = append(res, p+query)
	}
	return res
}

// ExtractEmail 提取字符串中所有的 Email 地址
// Example:
// ```
// re.ExtractEmail("hello your email is anonymous@yaklang.io") // ["anonymous@yaklang.io"]
// ```
func RegexpMatchEmail(i interface{}) []string {
	var res []string
	for _, group := range RE_EMAIL.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		res = append(res, group[0])
	}
	return res
}

// ExtractTTY 提取字符串中所有的Linux/Unix系统中的设备文件路径
// Example:
// ```
// re.ExtractTTY("hello your tty is /dev/pts/1") // ["/dev/pts/1"]
// ```
func RegexpMatchTTY(i interface{}) []string {
	var res []string
	for _, group := range RE_TTY.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		res = append(res, group[0])
	}
	return res
}

// ExtractURL 提取字符串中所有的 URL 地址
// Example:
// ```
// re.ExtractURL("Yak official website: https://yaklang.com and https://yaklang.io") // ["https://yaklang.com", "https://yaklang.io"]
// ```
func RegexpMatchURL(i interface{}) []string {
	var res []string
	for _, group := range RE_URL.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		res = append(res, group[0])
	}
	return res
}
