package httptpl

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"strings"
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

func CheckingDNSLogOOB(token string, runtimeID string, templateName string, timeout ...float64) (string, []byte) {
	DnsLogEvents, err := yakit.CheckDNSLogByToken(token, yakit.YakitPluginInfo{
		PluginName: templateName,
		RuntimeId:  runtimeID,
	}, timeout...)
	if err != nil {
		log.Error("CheckDNSLogByToken failed: ", err)
	}

	var request []byte
	for _, item := range DnsLogEvents {
		if strings.ToLower(item.Type) == "http" {
			request = item.Raw
			break
		}
	}
	return strings.Join(lo.Uniq(lo.Map(DnsLogEvents, func(item *tpb.DNSLogEvent, index int) string {
		if item.Type == "A" || item.Type == "AAAA" || item.Type == "CNAME" {
			return "dns"
		}
		return item.Type
	})), ","), request
}
