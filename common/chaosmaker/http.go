package chaosmaker

import (
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx"
	"github.com/yaklang/yaklang/common/suricata/data/protocol"
	"github.com/yaklang/yaklang/common/suricata/generate"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
	"net"
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

		var flow [][]byte
		var err error
		if len(raw) <= 1500 {
			flow, err = pcapx.CompleteTCPFlow(raw)
		} else {
			// 分片, 如果需要的话
			pk := gopacket.NewPacket(raw, layers.LayerTypeEthernet, gopacket.Default)
			if pk == nil {
				continue
			}
			nw := pk.NetworkLayer()
			if nw == nil {
				continue
			}
			tcp := pk.TransportLayer()
			if tcp == nil {
				continue
			}
			payload := tcp.LayerPayload()
			flow, err = pcapx.CreateTCPFlowFromPayload(
				net.JoinHostPort(nw.NetworkFlow().Src().String(), tcp.TransportFlow().Src().String()),
				net.JoinHostPort(nw.NetworkFlow().Dst().String(), tcp.TransportFlow().Dst().String()),
				payload,
			)
		}
		if err != nil {
			log.Errorf("complete tcp flow failed: %v", err)
			continue
		}
		for _, packet := range flow {
			h.out <- packet
		}
	}
}

func (h *httpHandler) MatchBytes(i any) bool {
	//TODO implement me
	panic("implement me")
}
