package yaklib

import (
	"regexp"
	"yaklang/common/utils"
)

var (
	//RE_HOSTNAME = regexp.MustCompile(`\b(?:[0-9A-Za-z][0-9A-Za-z-]{0,62})(?:\.(?:[0-9A-Za-z][0-9A-Za-z-]{0,62}))*(\.?|\b)`)
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

func RegexpMatchIPv4(i interface{}) []string {
	var res []string
	for _, group := range RE_IPV4.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		res = append(res, group[0])
	}
	return res
}

func RegexpMatchIPv6(i interface{}) []string {
	var res []string
	for _, group := range RE_IPV6.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		res = append(res, group[0])
	}
	return res
}

func RegexpMatchMac(i interface{}) []string {
	var res []string
	for _, group := range RE_MAC.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		res = append(res, group[0])
	}
	return res
}

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

// HOSTPORT
func RegexpMatchHostPort(i interface{}) []string {
	var res []string
	for _, group := range RE_HOSTPORT.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		res = append(res, group[0])
	}
	return res
}

// PATHPARAM
func RegexpMatchPathParam(i interface{}) []string {
	var res []string
	for _, group := range RE_PATHPARAM.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		res = append(res, group[0])
	}
	return res
}

func RegexpMatchEmail(i interface{}) []string {
	var res []string
	for _, group := range RE_EMAIL.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		res = append(res, group[0])
	}
	return res
}

// TTY
func RegexpMatchTTY(i interface{}) []string {
	var res []string
	for _, group := range RE_TTY.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		res = append(res, group[0])
	}
	return res
}

// URL
func RegexpMatchURL(i interface{}) []string {
	var res []string
	for _, group := range RE_URL.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		res = append(res, group[0])
	}
	return res
}
