package chaosmaker

import (
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/data/protocol"
	"github.com/yaklang/yaklang/common/suricata/generate"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
)

func init() {
	chaosMap.Store("suricata-http", &httpHandler{
		GenCountPerRule: 5,
	})
}

type httpHandler struct {
	GenCountPerRule int
}

var _ chaosHandler = (*httpHandler)(nil)

func (h *httpHandler) Generator(maker *ChaosMaker, chaosRule *rule.Storage, originRule *surirule.Rule) chan []byte {
	if originRule == nil {
		return nil
	}
	if originRule.Protocol != protocol.HTTP {
		return nil
	}

	ch := make(chan []byte)
	go (&httpGenerator{
		chaosRule:  chaosRule,
		originRule: originRule,
		maker:      maker,
		out:        ch,
	}).generator(h.GenCountPerRule)

	return ch
}

type httpGenerator struct {
	chaosRule  *rule.Storage
	originRule *surirule.Rule
	maker      *ChaosMaker
	out        chan []byte
}

func (h *httpGenerator) generator(count int) {
	defer close(h.out)

	surigen, err := generate.New(h.originRule)
	if err != nil {
		log.Errorf("new generator failed: %v", err)
	}

	for i := 0; i < count; i++ {
		raw := surigen.Gen()
		if raw == nil {
			return
		}
		h.out <- raw
	}
}

func (h *httpHandler) MatchBytes(i any) bool {
	//TODO implement me
	panic("implement me")
}
