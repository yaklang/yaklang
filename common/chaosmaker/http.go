package chaosmaker

import (
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx"
	"github.com/yaklang/yaklang/common/suricata/generate"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
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

func (h *httpHandler) Generator(maker *ChaosMaker, chaosRule *rule.Storage, originRule *surirule.Rule) chan *pcapx.ChaosTraffic {
	if originRule == nil {
		return nil
	}
	if originRule.Protocol != "http" {
		return nil
	}

	ch := make(chan *pcapx.ChaosTraffic)
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
	out        chan *pcapx.ChaosTraffic
}

func (h *httpGenerator) generator(count int) {
	surigen, err := generate.New(h.originRule)
	if err != nil {
		log.Warnf("new generator failed: %v", err)
	}

	for i := 0; i < count; i++ {
		h.toChaosTraffic(surigen.Gen())
	}

	close(h.out)
}

func (h *httpGenerator) toChaosTraffic(raw []byte) {
	if lowhttp.IsResp(raw) {
		h.out <- &pcapx.ChaosTraffic{
			HttpResponse: raw,
		}
	} else {
		h.out <- &pcapx.ChaosTraffic{
			HttpRequest: raw,
		}
	}
}

func (h *httpHandler) MatchBytes(i any) bool {
	//TODO implement me
	panic("implement me")
}
