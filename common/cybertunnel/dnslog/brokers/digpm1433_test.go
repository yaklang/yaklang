package dnslogbrokers

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"testing"
)

func TestDIGPM1(t *testing.T) {
	domain, token, err := defaultDigPm1433.Require(utils.FloatSecondDuration(5))
	if err != nil {
		panic(err)
	}
	spew.Dump(domain, token)
	utils.GetFirstIPByDnsWithCache(domain, utils.FloatSecondDuration(5))
	a, err := defaultDigPm1433.GetResult(token, utils.FloatSecondDuration(5))
	if err != nil {
		panic(err)
	}
	spew.Dump(a)
}
