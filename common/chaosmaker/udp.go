package chaosmaker

import (
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/generate"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
)

func init() {
	chaosMap.Store("suricata-udp", &udpHandler{})
}

type udpHandler struct {
}

var _ chaosHandler = (*udpHandler)(nil)

func (h *udpHandler) Generator(maker *ChaosMaker, chaosRule *rule.Storage, rule *surirule.Rule) chan []byte {
	if rule.Protocol != "udp" {
		return nil
	}

	if rule.ContentRuleConfig == nil {
		return nil
	}

	ch := make(chan []byte)
	go (&udpGenerator{
		originRule: rule,
		out:        ch,
	}).generator(5)

	return ch
}

func (h *udpHandler) MatchBytes(i interface{}) bool {
	//TODO implement me
	panic("implement me")
}

type udpGenerator struct {
	originRule *surirule.Rule
	out        chan []byte
}

func (t *udpGenerator) generator(count int) {
	defer func() {
		if e := recover(); e != nil {
			log.Errorf("udp generator panic: %v", utils.ErrorStack(e))
		}
	}()
	defer close(t.out)

	surigen, err := generate.New(t.originRule)
	if err != nil {
		log.Errorf("new generator failed: %v", err)
		return
	}

	for i := 0; i < count; i++ {
		raw := surigen.Gen()
		if raw == nil {
			return
		}
		t.out <- raw
	}
}
