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

func CheckingDNSLogOOB(token string, timeout ...float64) (string, []byte) {
	DnsLogEvents, err := yakit.CheckDNSLogByToken(token, timeout...)
	if err != nil {
		log.Error("CheckDNSLogByToken failed: ", err)
	}
	HTTPLogEvents, err := yakit.CheckHTTPLogByToken(token, timeout...)
	if err != nil {
		log.Error("CheckHTTPLogByToken failed: ", err)
	}

	var request []byte

	if len(HTTPLogEvents) > 0 {
		request = HTTPLogEvents[len(HTTPLogEvents)-1].Request
	}

	return strings.Join(lo.Uniq(lo.Map(DnsLogEvents, func(item *tpb.DNSLogEvent, index int) string {
		return item.Type
	})), ","), request
}
