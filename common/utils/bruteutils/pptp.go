package bruteutils

import (
	"context"

	"github.com/yaklang/yaklang/common/vpnbrute"
)

var pptp_Auth = &DefaultServiceAuthInfo{
	ServiceName:      "pptp",
	DefaultPorts:     "1723",
	DefaultUsernames: []string{"pptp"},
	DefaultPasswords: append([]string{"pptp"}, CommonPasswords...),
	UnAuthVerify:     nil,
	BrutePass: func(item *BruteItem) *BruteItemResult {
		target := fixToTarget(item.Target, 1723)
		result := item.Result()
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
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
