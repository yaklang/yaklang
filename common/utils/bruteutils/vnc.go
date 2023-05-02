package bruteutils

import (
	"github.com/mitchellh/go-vnc"
	"net"
	"github.com/yaklang/yaklang/common/utils"
)

// https://weakpass.com/generate
var vncAuth = &DefaultServiceAuthInfo{
	ServiceName:      "vnc",
	DefaultPorts:     "5900",
	DefaultUsernames: append([]string{"vnc"}, CommonUsernames...),
	DefaultPasswords: CommonPasswords,
	UnAuthVerify:     nil,
	BrutePass: func(item *BruteItem) *BruteItemResult {
		target := fixToTarget(item.Target, 5900)
		result := item.Result()
		result.OnlyNeedPassword = true

		_, port, _ := utils.ParseStringToHostPort(target)
		if port <= 0 {
			result.Finished = true
			return result
		}

		con, err := net.Dial("tcp", item.Target)
		if err != nil {
			result.Finished = true
			return result
		}

		client, err := vnc.Client(con, &vnc.ClientConfig{
			Auth: []vnc.ClientAuth{
				&vnc.PasswordAuth{
					Password: item.Password,
				},
				new(vnc.ClientAuthNone),
			},
		})
		if err != nil {
			return result
		}
		defer client.Close()

		result.Ok = true
		return result
	},
}
