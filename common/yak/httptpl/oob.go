package httptpl

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"time"
)

func RequireOOBAddr(timeout ...float64) (string, string, error) {
	t := 3 * time.Second
	if len(timeout) > 0 {
		t = utils.FloatSecondDuration(timeout[0])
	}

	for i := 0; i < 5; i++ {
		domain, token, err := yakit.NewDNSLogDomainWithContext(utils.TimeoutContext(t))
		if err != nil {
			log.Warnf("get dnslog domain failed: %s", err)
			continue
		}

		if domain != "" && token != "" {
			return domain, token, nil
		}
	}
	return "", "", utils.Errorf("get dnslog domain failed")
}

func CheckingDNSLogOOB(token string, timeout ...float64) bool {
	logs, err := yakit.CheckDNSLogByToken(token, timeout...)
	if err != nil {
		log.Error("checking oob-dnslog by token: " + token + " failed: " + err.Error())
		return false
	}
	if len(logs) > 0 {
		return true
	}
	return false
}
