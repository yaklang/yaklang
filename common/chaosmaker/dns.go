package chaosmaker

import (
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx"
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

func (h *dnsHandler) Generator(maker *ChaosMaker, makerRule *rule.Storage, rule *surirule.Rule) chan *pcapx.ChaosTraffic {
	if rule == nil {
		return nil
	}

	if rule.Protocol != "dns" {
		return nil
	}

	count := h.GenCountPerRule
	if rule.ContentRuleConfig != nil && rule.ContentRuleConfig.Thresholding != nil {
		count = utils.Max(h.GenCountPerRule, rule.ContentRuleConfig.Thresholding.Count)
	}

	ch := make(chan *pcapx.ChaosTraffic)
	go (&dnsGenerator{
		chaosRule:  makerRule,
		originRule: rule,
		maker:      maker,
		out:        ch,
	}).generator(count)

	return ch
}

type dnsGenerator struct {
	chaosRule  *rule.Storage
	originRule *surirule.Rule
	maker      *ChaosMaker
	out        chan *pcapx.ChaosTraffic
}

func (g *dnsGenerator) generator(count int) {
	surigen, err := generate.New(g.originRule)
	if err != nil {
		log.Warnf("new generator failed: %v", err)
	}

	for i := 0; i < count; i++ {
		g.toChaosTraffic(surigen.Gen())
	}

	close(g.out)
}

func (g *dnsGenerator) toChaosTraffic(raw []byte) {
	g.out <- &pcapx.ChaosTraffic{
		UDPIPInboundPayload: raw,
	}
}

func (h *dnsHandler) MatchBytes(i any) bool {
	//todo: implement
	return false
}
