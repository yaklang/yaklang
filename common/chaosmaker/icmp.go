package chaosmaker

import (
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx"
	"github.com/yaklang/yaklang/common/suricata/generate"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
)

func init() {
	chaosMap.Store("suricata-icmp", &icmpHandler{
		GenCountPerRule: 5,
	})
}

type icmpHandler struct {
	GenCountPerRule int
}

var _ chaosHandler = (*icmpHandler)(nil)

func (h *icmpHandler) Generator(maker *ChaosMaker, makerRule *rule.Storage, rule *surirule.Rule) chan *pcapx.ChaosTraffic {
	if rule == nil {
		return nil
	}
	if rule.Protocol != "icmp" {
		return nil
	}

	ch := make(chan *pcapx.ChaosTraffic)
	go (&icmpGenerator{
		makerRule: makerRule,
		rule:      rule,
		maker:     maker,
		out:       ch,
	}).generator(h.GenCountPerRule)

	return ch
}

type icmpGenerator struct {
	makerRule *rule.Storage
	rule      *surirule.Rule
	maker     *ChaosMaker
	out       chan *pcapx.ChaosTraffic
}

func (h *icmpGenerator) generator(count int) {
	defer close(h.out)

	surigen, err := generate.New(h.rule)
	if err != nil {
		log.Errorf("new generator failed: " + err.Error())
		return
	}

	for i := 0; i < count; i++ {
		h.toChaosTraffic(surigen.Gen())
	}
}

func (h *icmpGenerator) toChaosTraffic(data []byte) {
	h.out <- &pcapx.ChaosTraffic{
		ICMPIPInboundPayload: data,
	}
}

func (h *icmpHandler) MatchBytes(i interface{}) bool {
	//TODO implement me
	panic("implement me")
}
