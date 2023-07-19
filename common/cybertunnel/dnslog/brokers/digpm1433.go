package dnslogbrokers

import (
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"time"
)

type digpm1433Broker struct {
}

var defaultDigPm1433 = &digpm1433Broker{}

func init() {
	register("dig.pm-1433", &digpm1433Broker{})
}

func (s *digpm1433Broker) Require(du time.Duration, proxy ...string) (string, string, error) {
	packet := `
POST /new_gen HTTP/1.1
Host: dig.pm
Content-Type: application/x-www-form-urlencoded
Origin: https://dig.pm
Referer: https://dig.pm/
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36

domain=ipv6.1433.eu.org.
`
	/*

	 */
	rsp, _, err := lowhttp.SendHTTPRequestWithRawPacketEx(
		true, "", 0, []byte(packet), du,
		false, false,
		proxy...,
	)
	if err != nil {
		return "", "", utils.Errorf("send dig.pm packet failed: %v", err)
	}
	header, body := lowhttp.SplitHTTPPacket(rsp)
	jsonpath.Find(body, `$.`)
}
