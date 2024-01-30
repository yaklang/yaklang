package bruteutils

import (
	"fmt"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func testHTTPProxy(host string, username string, password string) bool {
	var proxy string
	if username != "" || password != "" {
		proxy = fmt.Sprintf("http://%s:%s@%s", username, password, host)
	} else {
		proxy = fmt.Sprintf("http://%s", host)
	}

	rspInst, err := lowhttp.HTTP(
		lowhttp.WithPacketBytes(lowhttp.BasicRequest()),
		lowhttp.WithProxy(proxy),
		lowhttp.WithConnectTimeoutFloat(15),
		lowhttp.WithTimeoutFloat(10),
	)
	if err == nil && len(rspInst.MultiResponseInstances) > 0 && rspInst.MultiResponseInstances[0].StatusCode == 200 {
		return true
	}

	return false
}

var httpProxyAuth = &DefaultServiceAuthInfo{
	ServiceName:  "http",
	DefaultPorts: "80",
	DefaultUsernames: []string{
		"root", "admin",
	},
	DefaultPasswords: []string{
		"root", "admin",
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		result := i.Result()
		result.Ok = testHTTPProxy(i.Target, i.Username, i.Password)
		return result
	},
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		result := i.Result()
		result.Ok = testHTTPProxy(i.Target, "", "")
		return result
	},
}
