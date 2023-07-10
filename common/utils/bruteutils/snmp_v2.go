package bruteutils

import (
	"github.com/gosnmp/gosnmp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"time"
)

var snmp_v2Auth = &DefaultServiceAuthInfo{
	ServiceName:      "snmp_v2",
	DefaultPorts:     "161",
	DefaultUsernames: append([]string{"snmp"}),
	DefaultPasswords: append([]string{"public"}, CommonPasswords...),
	UnAuthVerify:     nil,
	BrutePass: func(item *BruteItem) *BruteItemResult {
		target := fixToTarget(item.Target, 161)
		result := item.Result()
		result.OnlyNeedPassword = true

		host, port, _ := utils.ParseStringToHostPort(target)
		if port <= 0 {
			result.Finished = true
			return result
		}

		snmpConfig := &gosnmp.GoSNMP{
			Target:             host,
			Port:               uint16(port),
			Transport:          "udp",
			Community:          item.Password,
			Version:            gosnmp.Version2c,
			Timeout:            time.Duration(2) * time.Second,
			Retries:            3,
			ExponentialTimeout: true,
			MaxOids:            60,
		}

		// 尝试连接连接失败不再爆破
		err := snmpConfig.Connect()
		if err != nil {
			result.Finished = true
			return result
		}

		oid := []string{"1.3.6.1.2.1.1.1.0"}
		res, err := snmpConfig.Get(oid)
		if err != nil {
			log.Errorf("brute failed: %s", err)
			return result
		}

		if res.Variables[0].Type != gosnmp.OctetString {
			log.Errorf("brute failed")
			return result
		}
		result.Ok = true
		result.Username = ""
		return result
	},
}
