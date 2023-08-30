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
	chaosMap.Store("suricata-tcp", &tcpHandler{
		GenCountPerRule: 5,
	})
}

type tcpHandler struct {
	GenCountPerRule int
}

var _ chaosHandler = (*tcpHandler)(nil)

func (t *tcpHandler) Generator(maker *ChaosMaker, makerRule *rule.Storage, rule *surirule.Rule) chan *pcapx.ChaosTraffic {
	if rule == nil {
		return nil
	}

	if rule.Protocol != "tcp" {
		return nil
	}

	count := t.GenCountPerRule
	if rule.ContentRuleConfig != nil && rule.ContentRuleConfig.Thresholding != nil {
		count = utils.Max(t.GenCountPerRule, rule.ContentRuleConfig.Thresholding.Count)
	}

	ch := make(chan *pcapx.ChaosTraffic)
	go (&tcpGenerator{
		chaosRule:  makerRule,
		originRule: rule,
		maker:      maker,
		out:        ch,
	}).generator(count)

	return ch
}

type tcpGenerator struct {
	chaosRule  *rule.Storage
	originRule *surirule.Rule
	maker      *ChaosMaker
	out        chan *pcapx.ChaosTraffic
}

func (t *tcpGenerator) generator(count int) {
	defer close(t.out)

	surigen, err := generate.New(t.originRule)
	if err != nil {
		log.Errorf("new generator failed: %v", err)
		return
	}

	for i := 0; i < count; i++ {
		t.toChaosTraffic(surigen.Gen())
	}
}

func (t *tcpGenerator) toChaosTraffic(raw []byte) {
	t.out <- &pcapx.ChaosTraffic{
		TCPIPPayload: raw,
	}
}

func (t *tcpHandler) MatchBytes(i interface{}) bool {
	//TODO implement me
	panic("implement me")
}
