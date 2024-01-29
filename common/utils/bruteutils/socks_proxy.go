package bruteutils

import (
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/netx"
)

func testSocksProxy(scheme string, host string, username string, password string) bool {
	var proxy string
	if username != "" || password != "" {
		proxy = fmt.Sprintf("%s://%s:%s@%s", scheme, username, password, host)
	} else {
		proxy = fmt.Sprintf("%s://%s", scheme, host)
	}

	conn, err := netx.DialTCPTimeoutForceProxy(15*time.Second, "https://example.com", proxy)
	if err != nil && conn != nil {
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
