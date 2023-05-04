package utils

import (
	"net/url"
	"strings"
)

func ParseStringToUrlParams(i interface{}) string {
	var paramStr string
	switch ret := i.(type) {
	case string:
		paramStr = ret
	case []byte:
		paramStr = string(ret)
	case []rune:
		paramStr = string(ret)
	default:
		params := InterfaceToMap(i)
		if params == nil || len(params) <= 0 {
			return ""
		}
		paramStr = url.Values(params).Encode()
	}
	return paramStr
}

func UrlJoinParams(i string, params ...interface{}) string {
	var paramStrs []string
	for _, p := range params {
		if ret := ParseStringToUrlParams(p); ret != "" {
			paramStrs = append(paramStrs, ret)
		}
	}
	if len(paramStrs) <= 0 {
		return i
	}

	if i == "" {
		return strings.Join(paramStrs, "&")
	}

	u, err := url.Parse(i)
	if err != nil || u.Scheme == "" {
		return i + "&" + strings.Join(paramStrs, "&")
	}

	if u.RawQuery == "" {
		if u.Path == "" {
			return i + "/?" + strings.Join(paramStrs, "&")
		}
		return i + "?" + strings.Join(paramStrs, "&")
	}

	u.RawQuery += "&" + strings.Join(paramStrs, "&")
	return u.String()
}
