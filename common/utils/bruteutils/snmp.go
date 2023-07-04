package bruteutils

import (
	"github.com/yaklang/yaklang/common/utils"
)

var snmpAuth = &DefaultServiceAuthInfo{
	ServiceName:      "snmp",
	DefaultPorts:     "161",
	DefaultUsernames: append([]string{"snmp"}, CommonUsernames...),
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

		return result
	},
}
