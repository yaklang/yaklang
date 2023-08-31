package yaklib

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/netx"
	"testing"
)

func TestNewCustomDNSLog(t *testing.T) {
	consts.GetGormProjectDatabase()
	cd := NewCustomDNSLog(setScript("Goby DnsLog"))

	domain, _, err := cd.GetSubDomainAndToken()
	if err != nil {
		t.FailNow()
		return
	}
	netx.LookupFirst(domain)

	tokens, err := cd.CheckDNSLogByToken()
	if err != nil {
		return
	}
	spew.Dump(tokens)
}
