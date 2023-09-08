package vulinboxagentclient

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/chaosmaker"
	rule2 "github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/pcapx"
	"github.com/yaklang/yaklang/common/suricata/match"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/vulinbox"
	proto "github.com/yaklang/yaklang/common/vulinboxagentproto"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"math/rand"
	"testing"
	"time"
)

func TestMUSSPASSPing(t *testing.T) {
	server, err := vulinbox.NewVulinServer(context.Background(), rand.Intn(55535)+10000)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(server)
	time.Sleep(time.Second * 3)
	_, err = Connect(server)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSuricataRuleSubscribe(t *testing.T) {
	server, err := vulinbox.NewVulinServer(context.Background(), rand.Intn(55535)+10000)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(server)
	time.Sleep(time.Second * 3)
	c, err := Connect(server)
	if err != nil {
		t.Fatal(err)
	}
	var rule = `alert http any any -> any any (msg:"***Linux wget/curl download .sh script***"; flow:established,to_server; content:".sh"; http_uri;  pcre:"/curl|Wget|linux-gnu/Vi"; classtype:trojan-activity; sid:3013002; rev:1; metadata:by al0ne;)`
	rules, _ := surirule.Parse(rule)
	r := rules[0]
	var count int
	c.RegisterDataback("suricata", func(data any) {
		raw, err := codec.DecodeBase64(data.(string))
		if err != nil {
			t.Log(err)
			return
		}
		if match.New(r).Match(raw) {
			spew.Dump(raw)
			count++
		}
	})
	c.Msg().Send(proto.NewSubscribeAction("suricata", []string{rule}))
	maker := chaosmaker.NewChaosMaker()
	maker.FeedRule(rule2.NewRuleFromSuricata(r))
	for raw := range maker.Generate() {
		pcapx.InjectRaw(raw)
	}
	time.Sleep(time.Second * 10)
	t.Log(count)
	if count <= 0 {
		t.Fatal("no match")
	}
}
