package differential

import (
	"fmt"
	"net"

	"github.com/gopacket/gopacket/layers"
)

type Traffic interface {
	Frames() ([][]byte, error)
}

type Case struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Rules       string   `json:"rules"`
	Engines     []Engine `json:"engines"`
	Traffic     Traffic  `json:"-"`
}

func BuiltinCases() []Case {
	const common = "rev:1;"
	return []Case{
		{
			Name:        "content-case-sensitive",
			Description: "content without nocase must not match different ASCII case",
			Rules:       `alert tcp any any -> any any (msg:"case sensitive"; content:"Attack"; sid:9901001; ` + common + `)`,
			Engines:     []Engine{EngineYaklang, EngineSuricata, EngineSnort},
			Traffic:     NewTCPFlow(40001, 80, []byte("attack")),
		},
		{
			Name:        "content-nocase",
			Description: "nocase content must match different ASCII case",
			Rules:       `alert tcp any any -> any any (msg:"case insensitive"; content:"Attack"; nocase; sid:9901002; ` + common + `)`,
			Engines:     []Engine{EngineYaklang, EngineSuricata},
			Traffic:     NewTCPFlow(40002, 80, []byte("attack")),
		},
		{
			Name:        "bidirectional-reverse",
			Description: "a bidirectional rule must match traffic whose service port is on the source side",
			Rules:       `alert tcp any any <> any 8080 (msg:"bidirectional reverse"; content:"reply-marker"; sid:9901003; ` + common + `)`,
			Engines:     []Engine{EngineYaklang, EngineSuricata},
			Traffic:     reversePayloadFlow(40003, 8080, []byte("reply-marker")),
		},
		{
			Name:        "external-net-excludes-home",
			Description: "$EXTERNAL_NET must not match a source inside $HOME_NET",
			Rules:       `alert tcp $EXTERNAL_NET any -> any any (msg:"external net"; content:"marker"; sid:9901004; ` + common + `)`,
			Engines:     []Engine{EngineYaklang, EngineSuricata},
			Traffic:     homeNetFlow(40004, 80, []byte("marker")),
		},
		{
			Name:        "http-port-variable-negative",
			Description: "$HTTP_PORTS must not match an unconfigured destination port",
			Rules:       `alert tcp any any -> any $HTTP_PORTS (msg:"http ports"; content:"marker"; sid:9901005; ` + common + `)`,
			Engines:     []Engine{EngineYaklang, EngineSuricata},
			Traffic:     NewTCPFlow(40005, 81, []byte("marker")),
		},
		{
			Name:        "tcp-flags-negative",
			Description: "flags:S must not match an ACK payload packet",
			Rules:       `alert tcp any any -> any any (msg:"syn only"; flags:S; content:"marker"; sid:9901006; ` + common + `)`,
			Engines:     []Engine{EngineYaklang, EngineSuricata, EngineSnort},
			Traffic:     NewTCPFlow(40006, 80, []byte("marker")),
		},
		{
			Name:        "dsize-negative",
			Description: "dsize:>16 must not match a shorter payload",
			Rules:       `alert tcp any any -> any any (msg:"large payload"; dsize:>16; content:"marker"; sid:9901007; ` + common + `)`,
			Engines:     []Engine{EngineYaklang, EngineSuricata, EngineSnort},
			Traffic:     NewTCPFlow(40007, 80, []byte("marker")),
		},
		{
			Name:        "stream-split-content",
			Description: "only_stream content must match across two in-order TCP segments",
			Rules:       `alert tcp any any -> any any (msg:"split stream"; flow:to_server,established,only_stream; content:"split-marker"; sid:9901008; ` + common + `)`,
			Engines:     []Engine{EngineYaklang, EngineSuricata},
			Traffic:     NewTCPFlow(40008, 80, []byte("split-"), []byte("marker")),
		},
		{
			Name:        "stream-out-of-order",
			Description: "only_stream content must match after out-of-order TCP segments are reassembled",
			Rules:       `alert tcp any any -> any any (msg:"out of order"; flow:to_server,established,only_stream; content:"order-marker"; sid:9901009; ` + common + `)`,
			Engines:     []Engine{EngineYaklang, EngineSuricata},
			Traffic:     outOfOrderFlow(40009, 80, []byte("order-"), []byte("marker")),
		},
		{
			Name:        "noalert-flowbit",
			Description: "a noalert setter must suppress its own alert while enabling a dependent rule",
			Rules: `alert tcp any any -> any any (msg:"setter"; flow:to_server,established,only_stream; content:"set-marker"; flowbits:set,diff_seen; noalert; sid:9901010; rev:1;)
alert tcp any any -> any any (msg:"checker"; flow:to_server,established,only_stream; content:"check-marker"; flowbits:isset,diff_seen; sid:9901011; rev:1;)`,
			Engines: []Engine{EngineYaklang, EngineSuricata},
			Traffic: NewTCPFlow(40010, 80, []byte("set-marker"), []byte("check-marker")),
		},
		{
			Name:        "midstream-established",
			Description: "flow:established behavior when capture starts after the TCP handshake",
			Rules:       `alert tcp any any -> any any (msg:"midstream established"; flow:to_server,established,only_stream; content:"midstream-marker"; sid:9901012; ` + common + `)`,
			Engines:     []Engine{EngineYaklang, EngineSuricata},
			Traffic:     midstreamFlow(40012, 80, []byte("midstream-marker")),
		},
		{
			Name:        "stream-retransmit-count",
			Description: "an exact TCP retransmission must not create a second stream alert",
			Rules:       `alert tcp any any -> any any (msg:"retransmit count"; flow:to_server,established,only_stream; content:"repeat-marker"; sid:9901013; ` + common + `)`,
			Engines:     []Engine{EngineYaklang, EngineSuricata},
			Traffic:     retransmitFlow(40013, 80, []byte("repeat-marker")),
		},
		{
			Name:        "dns-response-first-direction",
			Description: "a response-only DNS capture must not be classified as flow:to_server",
			Rules:       `alert dns any any -> any any (msg:"response first"; flow:to_server; dns.query; content:"response-first.test"; sid:9901014; ` + common + `)`,
			Engines:     []Engine{EngineYaklang, EngineSuricata},
			Traffic:     DNSResponseFirstFlow(),
		},
		{
			Name:        "dns-query-buffer-on-response",
			Description: "dns.query is a request-direction sticky buffer and must not match a DNS response",
			Rules:       `alert dns any any -> any any (msg:"query buffer on response"; dns.query; content:"response-first.test"; sid:9901016; ` + common + `)`,
			Engines:     []Engine{EngineYaklang, EngineSuricata},
			Traffic:     DNSResponseFirstFlow(),
		},
	}
}

func reversePayloadFlow(clientPort, serverPort uint16, payload []byte) TCPFlow {
	flow := NewTCPFlow(clientPort, serverPort)
	const clientSeq = 1001
	const serverSeq = 9001
	flow.Packets = append(flow.Packets[:3],
		TCPPacket{ToServer: false, Seq: serverSeq, Ack: clientSeq, ACK: true, PSH: true, Payload: payload},
		TCPPacket{ToServer: false, Seq: serverSeq + uint32(len(payload)), Ack: clientSeq, ACK: true, FIN: true},
	)
	return flow
}

func homeNetFlow(clientPort, serverPort uint16, payload []byte) TCPFlow {
	flow := NewTCPFlow(clientPort, serverPort, payload)
	flow.ClientIP = net.IPv4(192, 168, 10, 10)
	flow.ServerIP = net.IPv4(198, 51, 100, 20)
	return flow
}

func outOfOrderFlow(clientPort, serverPort uint16, first, second []byte) TCPFlow {
	flow := NewTCPFlow(clientPort, serverPort, first, second)
	if len(flow.Packets) < 6 {
		panic(fmt.Sprintf("unexpected TCP flow packet count %d", len(flow.Packets)))
	}
	flow.Packets[3], flow.Packets[4] = flow.Packets[4], flow.Packets[3]
	return flow
}

func midstreamFlow(clientPort, serverPort uint16, payload []byte) TCPFlow {
	return TCPFlow{
		ClientIP:   net.IPv4(192, 0, 2, 10),
		ServerIP:   net.IPv4(198, 51, 100, 20),
		ClientPort: layers.TCPPort(clientPort),
		ServerPort: layers.TCPPort(serverPort),
		Packets: []TCPPacket{
			{ToServer: true, Seq: 5000, Ack: 7000, ACK: true, PSH: true, Payload: payload},
			{ToServer: true, Seq: 5000 + uint32(len(payload)), Ack: 7000, ACK: true, FIN: true},
		},
	}
}

func retransmitFlow(clientPort, serverPort uint16, payload []byte) TCPFlow {
	flow := NewTCPFlow(clientPort, serverPort, payload)
	if len(flow.Packets) < 5 {
		panic(fmt.Sprintf("unexpected TCP flow packet count %d", len(flow.Packets)))
	}
	retransmit := flow.Packets[3]
	flow.Packets = append(flow.Packets[:4], append([]TCPPacket{retransmit}, flow.Packets[4:]...)...)
	return flow
}
