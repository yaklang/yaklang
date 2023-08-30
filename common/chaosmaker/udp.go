package chaosmaker

import (
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx"
	"github.com/yaklang/yaklang/common/suricata/generate"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
)

func init() {
	chaosMap.Store("suricata-udp", &udpHandler{})
}

type udpHandler struct {
}

var _ chaosHandler = (*udpHandler)(nil)

func (h *udpHandler) Generator(maker *ChaosMaker, makerRule *rule.Storage, rule *surirule.Rule) chan *pcapx.ChaosTraffic {
	if rule.Protocol != "udp" {
		return nil
	}

	if rule.ContentRuleConfig == nil {
		return nil
	}

	ch := make(chan *pcapx.ChaosTraffic)
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
	out        chan *pcapx.ChaosTraffic
}

func (t *udpGenerator) generator(count int) {
	defer close(t.out)

	surigen, err := generate.New(t.originRule)
	if err != nil {
		log.Errorf("new generator failed: %v", err)
		return
	}
	var toServer = true
	var toClient = true

	if t.originRule.ContentRuleConfig.Flow != nil {
		toServer = t.originRule.ContentRuleConfig.Flow.ToServer
		toClient = t.originRule.ContentRuleConfig.Flow.ToClient
	}

	for i := 0; i < count; i++ {
		raw := surigen.Gen()
		if raw == nil {
			return
		}
		if toServer {
			t.out <- &pcapx.ChaosTraffic{
				UDPIPOutboundPayload: raw,
			}
		} else if toClient {
			t.out <- &pcapx.ChaosTraffic{
				UDPIPInboundPayload: raw,
			}
		}
	}
}
