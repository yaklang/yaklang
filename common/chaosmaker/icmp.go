package chaosmaker

import (
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/generate"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
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

func (h *icmpHandler) Generator(maker *ChaosMaker, chaosRule *rule.Storage, originRule *surirule.Rule) chan []byte {
	if originRule == nil {
		return nil
	}
	if originRule.Protocol != "icmp" {
		return nil
	}

	ch := make(chan []byte)
	go (&icmpGenerator{
		makerRule: chaosRule,
		rule:      originRule,
		maker:     maker,
		out:       ch,
	}).generator(h.GenCountPerRule)

	return ch
}

type icmpGenerator struct {
	makerRule *rule.Storage
	rule      *surirule.Rule
	maker     *ChaosMaker
	out       chan []byte
}

func (h *icmpGenerator) generator(count int) {
	defer func() {
		if e := recover(); e != nil {
			log.Errorf("icmp generator panic: %v", utils.ErrorStack(e))
		}
	}()
	defer close(h.out)

	surigen, err := generate.New(h.rule)
	if err != nil {
		log.Errorf("new generator failed: " + err.Error())
		return
	}

	for i := 0; i < count; i++ {
		raw := surigen.Gen()
		if raw == nil {
			return
		}
		h.out <- raw
	}
}

func (h *icmpHandler) MatchBytes(i interface{}) bool {
	//TODO implement me
	panic("implement me")
}
