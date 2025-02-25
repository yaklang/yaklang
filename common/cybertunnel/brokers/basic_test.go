package dnslogbrokers

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"testing"
)

func TestDIGPM1(t *testing.T) {
	t.SkipNow()
	domain, token, err := defaultDigPm1433.Require(utils.FloatSecondDuration(5))
	if err != nil {
		panic(err)
	}
	spew.Dump(domain, token)
	netx.LookupFirst(domain, netx.WithTimeout(utils.FloatSecondDuration(3)))
	a, err := defaultDigPm1433.GetResult(token, utils.FloatSecondDuration(5))
	if err != nil {
		panic(err)
	}
	spew.Dump(a)
}

func TestDIGPM2(t *testing.T) {
	t.SkipNow()
	domain, token, err := defaultDigPMBYPASS.Require(utils.FloatSecondDuration(5))
	if err != nil {
		panic(err)
	}
	spew.Dump(domain, token)
	netx.LookupFirst(domain, netx.WithTimeout(utils.FloatSecondDuration(3)))
	a, err := defaultDigPMBYPASS.GetResult(token, utils.FloatSecondDuration(5))
	if err != nil {
		panic(err)
	}
	spew.Dump(a)
}

func TestDNSLOGCN(t *testing.T) {
	t.SkipNow()
	domain, token, err := defaultDNSLogCN.Require(utils.FloatSecondDuration(5))
	if err != nil {
		panic(err)
	}
	spew.Dump(domain, token)
	netx.LookupFirst(domain, netx.WithTimeout(utils.FloatSecondDuration(3)))
	a, err := defaultDNSLogCN.GetResult(token, utils.FloatSecondDuration(5))
	if err != nil {
		panic(err)
	}
	spew.Dump(a)
}
