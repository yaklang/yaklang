package chaosmaker

import (
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/generate"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
)

func init() {
	chaosMap.Store("suricata-dns", &dnsHandler{
		GenCountPerRule: 5,
	})
}

type dnsHandler struct {
	GenCountPerRule int
}

var _ chaosHandler = (*dnsHandler)(nil)

func (h *dnsHandler) Generator(maker *ChaosMaker, chaosRule *rule.Storage, originRule *surirule.Rule) chan []byte {
	if originRule == nil {
		return nil
	}

	if originRule.Protocol != "dns" {
		return nil
	}

	count := h.GenCountPerRule
	if originRule.ContentRuleConfig != nil && originRule.ContentRuleConfig.Thresholding != nil {
		count = utils.Max(h.GenCountPerRule, originRule.ContentRuleConfig.Thresholding.Count)
	}

	ch := make(chan []byte)
	go (&dnsGenerator{
		chaosRule:  chaosRule,
		originRule: originRule,
		maker:      maker,
		out:        ch,
	}).generator(count)

	return ch
}

type dnsGenerator struct {
	chaosRule  *rule.Storage
	originRule *surirule.Rule
	maker      *ChaosMaker
	out        chan []byte
}

func (g *dnsGenerator) generator(count int) {
	defer close(g.out)

	surigen, err := generate.New(g.originRule)
	if err != nil {
		log.Errorf("new generator failed: %v", err)
		return
	}

	for i := 0; i < count; i++ {
		raw := surigen.Gen()
		if raw == nil {
			return
		}
		g.out <- raw
	}
}

func (h *dnsHandler) MatchBytes(i any) bool {
	//todo: implement
	return false
}
