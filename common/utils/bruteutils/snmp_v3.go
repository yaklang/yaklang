package bruteutils

import (
	"github.com/gosnmp/gosnmp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"time"
)

var snmpV3Auth_MD5 = &DefaultServiceAuthInfo{
	ServiceName:      "snmp_v3_MD5",
	DefaultPorts:     "161",
	DefaultUsernames: append([]string{"snmp"}, CommonUsernames...),
	DefaultPasswords: CommonPasswords,
	UnAuthVerify:     nil,
	BrutePass: func(item *BruteItem) *BruteItemResult {
		return v3Brute(item, gosnmp.MD5)
	},
}

var snmpV3Auth_SHA = &DefaultServiceAuthInfo{
	ServiceName:      "snmp_v3_SHA",
	DefaultPorts:     "161",
	DefaultUsernames: append([]string{"snmp"}, CommonUsernames...),
	DefaultPasswords: CommonPasswords,
	UnAuthVerify:     nil,
	BrutePass: func(item *BruteItem) *BruteItemResult {
		return v3Brute(item, gosnmp.SHA)
	},
}

var snmpV3Auth_SHA_512 = &DefaultServiceAuthInfo{
	ServiceName:      "snmp_v3_SHA-512",
	DefaultPorts:     "161",
	DefaultUsernames: append([]string{"snmp"}, CommonUsernames...),
	DefaultPasswords: CommonPasswords,
	UnAuthVerify:     nil,
	BrutePass: func(item *BruteItem) *BruteItemResult {
		return v3Brute(item, gosnmp.SHA512)
	},
}

var snmpV3Auth_SHA_384 = &DefaultServiceAuthInfo{
	ServiceName:      "snmp_v3_SHA-384",
	DefaultPorts:     "161",
	DefaultUsernames: append([]string{"snmp"}, CommonUsernames...),
	DefaultPasswords: CommonPasswords,
	UnAuthVerify:     nil,
	BrutePass: func(item *BruteItem) *BruteItemResult {
		return v3Brute(item, gosnmp.SHA384)
	},
}

var snmpV3Auth_SHA_256 = &DefaultServiceAuthInfo{
	ServiceName:      "snmp_v3_SHA-256",
	DefaultPorts:     "161",
	DefaultUsernames: append([]string{"snmp"}, CommonUsernames...),
	DefaultPasswords: CommonPasswords,
	UnAuthVerify:     nil,
	BrutePass: func(item *BruteItem) *BruteItemResult {
		return v3Brute(item, gosnmp.SHA256)
	},
}

var snmpV3Auth_SHA_224 = &DefaultServiceAuthInfo{
	ServiceName:      "snmp_v3_SHA-224",
	DefaultPorts:     "161",
	DefaultUsernames: append([]string{"snmp"}, CommonUsernames...),
	DefaultPasswords: CommonPasswords,
	UnAuthVerify:     nil,
	BrutePass: func(item *BruteItem) *BruteItemResult {
		return v3Brute(item, gosnmp.SHA224)
	},
}

func v3Brute(item *BruteItem, alg gosnmp.SnmpV3AuthProtocol) *BruteItemResult {
	target := fixToTarget(item.Target, 161)
	result := item.Result()

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
		Version:            gosnmp.Version3,
		Timeout:            time.Duration(2) * time.Second,
		Retries:            3,
		ExponentialTimeout: true,
		MaxOids:            60,
		SecurityModel:      gosnmp.UserSecurityModel,
		MsgFlags:           gosnmp.AuthNoPriv,
		SecurityParameters: &gosnmp.UsmSecurityParameters{
			UserName:                 item.Username,
			AuthenticationProtocol:   alg,
			AuthenticationPassphrase: item.Password,
		},
	}

	// 尝试连接连接失败不再爆破
	err := snmpConfig.Connect()
	if err != nil {
		result.Finished = true
		return result
	}

	oid := []string{"1.3.6.1.2.1.1.1.0"}
	_, err = snmpConfig.Get(oid)
	if err != nil {
		if strings.Contains(err.Error(), "unknown username") {
			result.UserEliminated = true
		}

		log.Errorf("brute failed: %s", err)
		return result
	}

	result.Ok = true
	return result
}
