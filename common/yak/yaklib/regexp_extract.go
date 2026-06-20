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

// ExtractIPv4 提取字符串中所有的 IPv4 地址（导出名为 re.ExtractIPv4）
// 参数:
//   - i: 待提取的输入（任意可转为字符串）
//
// 返回值:
//   - 提取到的 IPv4 地址列表，未匹配返回空切片
//
// Example:
// ```
// ips = re.ExtractIPv4("hello your local ip is 127.0.0.1, your public ip is 1.1.1.1")
// println(ips)   // OUT: [127.0.0.1 1.1.1.1]
// assert len(ips) == 2 && ips[0] == "127.0.0.1", "ExtractIPv4 should find both addresses"
// ```
func RegexpMatchIPv4(i interface{}) []string {
	var res []string
	for _, group := range RE_IPV4.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		res = append(res, group[0])
	}
	return res
}

// ExtractIPv6 提取字符串中所有的 IPv6 地址（导出名为 re.ExtractIPv6）
// 参数:
//   - i: 待提取的输入（任意可转为字符串）
//
// 返回值:
//   - 提取到的 IPv6 地址列表，未匹配返回空切片
//
// Example:
// ```
// ips = re.ExtractIPv6("public ipv6 is 2001:4860:4860::8888 here")
// println(ips[0])   // OUT: 2001:4860:4860::8888
// assert len(ips) >= 1, "ExtractIPv6 should find the ipv6 address"
// ```
func RegexpMatchIPv6(i interface{}) []string {
	var res []string
	for _, group := range RE_IPV6.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		res = append(res, group[0])
	}
	return res
}

// ExtractMac 提取字符串中所有的 MAC 地址（导出名为 re.ExtractMac）
// 参数:
//   - i: 待提取的输入（任意可转为字符串）
//
// 返回值:
//   - 提取到的 MAC 地址列表，未匹配返回空切片
//
// Example:
// ```
// macs = re.ExtractMac("hello your mac is 00:11:22:33:44:55")
// println(macs[0])   // OUT: 00:11:22:33:44:55
// assert len(macs) == 1, "ExtractMac should find one mac address"
// ```
func RegexpMatchMac(i interface{}) []string {
	var res []string
	for _, group := range RE_MAC.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		res = append(res, group[0])
	}
	return res
}

// ExtractIP 提取字符串中所有的 IP 地址（IPv4 与 IPv6）（导出名为 re.ExtractIP）
// 参数:
//   - i: 待提取的输入（任意可转为字符串）
//
// 返回值:
//   - 提取到的 IP 地址列表（先 IPv6 后 IPv4），未匹配返回空切片
//
// Example:
// ```
// ips = re.ExtractIP("local ip is 127.0.0.1 here")
// println(ips)   // OUT: [127.0.0.1]
// assert len(ips) == 1 && ips[0] == "127.0.0.1", "ExtractIP should find the ipv4 address"
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

// ExtractHostPort 提取字符串中所有的 Host:Port（导出名为 re.ExtractHostPort）
// 参数:
//   - i: 待提取的输入（任意可转为字符串）
//
// 返回值:
//   - 提取到的 Host:Port 列表，未匹配返回空切片
//
// Example:
// ```
// hps = re.ExtractHostPort("open: 127.0.0.1:80 and 127.0.0.1:443")
// println(hps)   // OUT: [127.0.0.1:80 127.0.0.1:443]
// assert len(hps) == 2 && hps[0] == "127.0.0.1:80", "ExtractHostPort should find both host:port"
// ```
func RegexpMatchHostPort(i interface{}) []string {
	var res []string
	for _, group := range RE_HOSTPORT.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		res = append(res, group[0])
	}
	return res
}

// ExtractPath 提取字符串中所有的路径与查询字符串（导出名为 re.ExtractPath）
// 参数:
//   - i: 待提取的输入（任意可转为字符串）
//
// 返回值:
//   - 提取到的 路径(?查询) 列表，未匹配返回空切片
//
// Example:
// ```
// paths = re.ExtractPath("visit yaklang.com/docs/api/re?name=anonymous now")
// println(paths[0])   // OUT: /docs/api/re?name=anonymous
// assert len(paths) >= 1, "ExtractPath should extract the path with query"
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
		var query string
		if u.RawQuery != "" {
			query = "?" + u.RawQuery
		}
		if u.ForceQuery {
			query = "?"
		}
		res = append(res, p+query)
	}
	return res
}

// ExtractEmail 提取字符串中所有的 Email 地址（导出名为 re.ExtractEmail）
// 参数:
//   - i: 待提取的输入（任意可转为字符串）
//
// 返回值:
//   - 提取到的 Email 地址列表，未匹配返回空切片
//
// Example:
// ```
// emails = re.ExtractEmail("hello your email is anonymous@yaklang.io")
// println(emails[0])   // OUT: anonymous@yaklang.io
// assert len(emails) == 1, "ExtractEmail should find one email"
// ```
func RegexpMatchEmail(i interface{}) []string {
	var res []string
	for _, group := range RE_EMAIL.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		res = append(res, group[0])
	}
	return res
}

// ExtractTTY 提取字符串中所有的 Linux/Unix 终端设备路径（导出名为 re.ExtractTTY）
// 参数:
//   - i: 待提取的输入（任意可转为字符串）
//
// 返回值:
//   - 提取到的设备文件路径列表，未匹配返回空切片
//
// Example:
// ```
// ttys = re.ExtractTTY("hello your tty is /dev/pts/1")
// println(ttys[0])   // OUT: /dev/pts/1
// assert len(ttys) == 1, "ExtractTTY should find one tty path"
// ```
func RegexpMatchTTY(i interface{}) []string {
	var res []string
	for _, group := range RE_TTY.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		res = append(res, group[0])
	}
	return res
}

// ExtractURL 提取字符串中所有的 URL 地址（导出名为 re.ExtractURL）
// 参数:
//   - i: 待提取的输入（任意可转为字符串）
//
// 返回值:
//   - 提取到的 URL 列表，未匹配返回空切片
//
// Example:
// ```
// urls = re.ExtractURL("Yak site: https://yaklang.com and https://yaklang.io")
// println(urls)   // OUT: [https://yaklang.com https://yaklang.io]
// assert len(urls) == 2, "ExtractURL should find both urls"
// ```
func RegexpMatchURL(i interface{}) []string {
	var res []string
	for _, group := range RE_URL.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		res = append(res, group[0])
	}
	return res
}
