package chaosmaker

import (
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/data/protocol"
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

func (t *tcpHandler) Generator(maker *ChaosMaker, chaosRule *rule.Storage, originRule *surirule.Rule) chan []byte {
	if originRule == nil {
		return nil
	}

	if originRule.Protocol != protocol.TCP {
		return nil
	}

	count := t.GenCountPerRule
	if originRule.ContentRuleConfig != nil && originRule.ContentRuleConfig.Thresholding != nil {
		count = utils.Max(t.GenCountPerRule, originRule.ContentRuleConfig.Thresholding.Count)
	}

	ch := make(chan []byte)
	go (&tcpGenerator{
		chaosRule:  chaosRule,
		originRule: originRule,
		maker:      maker,
		out:        ch,
	}).generator(count)

	return ch
}

type tcpGenerator struct {
	chaosRule  *rule.Storage
	originRule *surirule.Rule
	maker      *ChaosMaker
	out        chan []byte
}

func (t *tcpGenerator) generator(count int) {
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
		//todo: generate handshake flow
	}
}

func (t *tcpHandler) MatchBytes(i interface{}) bool {
	//TODO implement me
	panic("implement me")
}
