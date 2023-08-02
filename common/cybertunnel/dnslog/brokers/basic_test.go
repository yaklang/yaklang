package dnslogbrokers

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakdns"
	"testing"
)

func TestDIGPM1(t *testing.T) {
	domain, token, err := defaultDigPm1433.Require(utils.FloatSecondDuration(5))
	if err != nil {
		panic(err)
	}
	spew.Dump(domain, token)
	yakdns.LookupFirst(domain, yakdns.WithTimeout(utils.FloatSecondDuration(3)))
	a, err := defaultDigPm1433.GetResult(token, utils.FloatSecondDuration(5))
	if err != nil {
		panic(err)
	}
	spew.Dump(a)
}

func TestDIGPM2(t *testing.T) {
	domain, token, err := defaultDigPMBYPASS.Require(utils.FloatSecondDuration(5))
	if err != nil {
		panic(err)
	}
	spew.Dump(domain, token)
	yakdns.LookupFirst(domain, yakdns.WithTimeout(utils.FloatSecondDuration(3)))
	a, err := defaultDigPMBYPASS.GetResult(token, utils.FloatSecondDuration(5))
	if err != nil {
		panic(err)
	}
	spew.Dump(a)
}

func TestDNSLOGCN(t *testing.T) {
	domain, token, err := defaultDNSLogCN.Require(utils.FloatSecondDuration(5))
	if err != nil {
		panic(err)
	}
	spew.Dump(domain, token)
	yakdns.LookupFirst(domain, yakdns.WithTimeout(utils.FloatSecondDuration(3)))
	a, err := defaultDNSLogCN.GetResult(token, utils.FloatSecondDuration(5))
	if err != nil {
		panic(err)
	}
	spew.Dump(a)
}
