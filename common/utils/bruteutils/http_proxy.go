package bruteutils

import (
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/netx"
)

func testHTTPProxy(host string, username string, password string) bool {
	var proxy string
	if username != "" || password != "" {
		proxy = fmt.Sprintf("http://%s:%s@%s", username, password, host)
	} else {
		proxy = fmt.Sprintf("http://%s", host)
	}

	conn, err := netx.DialTCPTimeoutForceProxy(15*time.Second, "https://example.com", proxy)
	if err != nil && conn != nil {
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
