package bruteutils

import (
	"context"
	"github.com/yaklang/yaklang/common/vpnbrute"
	"time"
)

var pptp_Auth = &DefaultServiceAuthInfo{
	ServiceName:      "pptp",
	DefaultPorts:     "1723",
	DefaultUsernames: append([]string{"pptp"}),
	DefaultPasswords: append([]string{"pptp"}, CommonPasswords...),
	UnAuthVerify:     nil,
	BrutePass: func(item *BruteItem) *BruteItemResult {
		target := fixToTarget(item.Target, 1723)
		result := item.Result()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		err, b := vpnbrute.PPTPAuth(ctx, target, item.Username, item.Password)
		if err != nil {
			result.Finished = true
			return result
		}
		if b {
			result.Ok = true
		}
		return result
	},
}
