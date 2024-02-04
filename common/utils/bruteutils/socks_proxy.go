package bruteutils

import (
	"bytes"
	"fmt"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func testSocksProxy(scheme string, host string, username string, password string) bool {
	var proxy string
	if username != "" || password != "" {
		proxy = fmt.Sprintf("%s://%s:%s@%s", scheme, username, password, host)
	} else {
		proxy = fmt.Sprintf("%s://%s", scheme, host)
	}

	rspInst, err := lowhttp.HTTP(
		lowhttp.WithPacketBytes(lowhttp.BasicRequest()),
		lowhttp.WithProxy(proxy),
		lowhttp.WithConnectTimeoutFloat(15),
		lowhttp.WithTimeoutFloat(10),
	)
	if err == nil && len(rspInst.MultiResponseInstances) > 0 && rspInst.MultiResponseInstances[0].StatusCode == 200 && bytes.Contains(rspInst.RawPacket, ExampleChallengeContent) {
		return true
	}

	return false
}

func SocksProxyBruteAuthFactory(scheme string) *DefaultServiceAuthInfo {
	return &DefaultServiceAuthInfo{
		ServiceName:  "scheme",
		DefaultPorts: "1080",
		DefaultUsernames: []string{
			"root", "admin",
		},
		DefaultPasswords: []string{
			"root", "admin",
		},
		BrutePass: func(i *BruteItem) *BruteItemResult {
			result := i.Result()
			result.Ok = testSocksProxy(scheme, i.Target, i.Username, i.Password)
			return result
		},
		UnAuthVerify: func(i *BruteItem) *BruteItemResult {
			result := i.Result()
			result.Ok = testSocksProxy(scheme, i.Target, "", "")
			return result
		},
	}
}
