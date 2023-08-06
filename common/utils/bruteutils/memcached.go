package bruteutils

import (
	"bytes"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"time"
)

var memcachedAuth = &DefaultServiceAuthInfo{
	ServiceName:      "memcached",
	DefaultPorts:     "11211",
	DefaultUsernames: CommonUsernames,
	DefaultPasswords: CommonPasswords,
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		result := i.Result()

		// 66.71.179.114
		target := appendDefaultPort(i.Target, 11211)
		conn, err := netx.DialTCPTimeout(defaultTimeout, target)
		if err != nil {
			res := i.Result()
			res.Finished = true
			return res
		}
		defer conn.Close()

		_, _ = conn.Write([]byte("stats\r\n"))
		outputs, err := utils.ReadConnWithTimeout(conn, 5*time.Second)
		if err != nil {
			log.Errorf("read conn failed: %s", err)
			return result
		}

		if bytes.Contains(outputs, []byte("STAT")) {
			// 未授权登录成功
			result.Ok = true
			result.Username = ""
			result.Password = ""
			return result
		}

		return result
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		r := i.Result()
		r.Finished = true
		return i.Result()
	},
}
