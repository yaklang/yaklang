package bruteutils

import (
	"github.com/gosnmp/gosnmp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"time"
)

func snmpV3BruteFactory(name string) *DefaultServiceAuthInfo {
	alg := gosnmp.MD5
	switch name {
	case "snmpv3_sha":
		alg = gosnmp.SHA
	case "snmpv3_sha-224":
		alg = gosnmp.SHA224
	case "snmpv3_sha-256":
		alg = gosnmp.SHA256
	case "snmpv3_sha-384":
		alg = gosnmp.SHA384
	case "snmpv3_sha-512":
		alg = gosnmp.SHA512
	}
	return &DefaultServiceAuthInfo{
		ServiceName:      name,
		DefaultPorts:     "161",
		DefaultUsernames: append([]string{"snmp"}, CommonUsernames...),
		DefaultPasswords: CommonPasswords,
		UnAuthVerify:     nil,
		BrutePass: func(item *BruteItem) *BruteItemResult {
			return v3Brute(item, alg)
		},
	}
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
	res, err := snmpConfig.Get(oid)
	if err != nil {
		if strings.Contains(err.Error(), "unknown username") {
			result.UserEliminated = true
		}

		log.Errorf("brute failed: %s", err)
		return result
	}

	if res.Variables[0].Type != gosnmp.OctetString {
		log.Errorf("brute failed")
		return result
	}

	result.Ok = true
	return result
}
