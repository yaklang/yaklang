package chaosmaker

import "yaklang/common/suricata"

type ChaosTraffic struct {
	ChaosRule             *ChaosMakerRule
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
