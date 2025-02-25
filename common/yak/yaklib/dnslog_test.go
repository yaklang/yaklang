package yaklib_test

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
)

func init() {
	yaklib.SetEngineInterface(yak.NewScriptEngine(1000))
}

func TestNewCustomDNSLog(t *testing.T) {
	consts.GetGormProjectDatabase()
	cd := yaklib.NewCustomDNSLog(yaklib.WithDNSLog_SetScript("Goby DnsLog"))

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
