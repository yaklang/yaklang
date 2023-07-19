package chaosmaker

import (
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/suricata"
)

type ChaosTraffic struct {
	ChaosRule             *rule.Storage
	SuricataRule          *suricata.Rule
	RawTCP                bool
	LocalIP               string
	LinkLayerPayload      []byte
	TCPIPPayload          []byte
	UDPIPInboundPayload   []byte
	UDPIPOutboundPayload  []byte
	ICMPIPInboundPayload  []byte
	ICMPIPOutboundPayload []byte
	HttpRequest           []byte
	HttpResponse          []byte
}
