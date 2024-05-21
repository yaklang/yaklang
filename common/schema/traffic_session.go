package schema

import "github.com/jinzhu/gorm"

type TrafficSession struct {
	gorm.Model

	Uuid string `gorm:"index"`

	// Traffic SessionType Means a TCP Session / ICMP Request-Response / UDP Request-Response
	// DNS Request-Response
	// HTTP Request-Response
	// we can't treat Proto as any transport layer proto or application layer proto
	// because we can't know the proto of a packet before we parse it
	//
	// just use session type as a hint / verbose to group some frames(packets).
	//
	// 1. tcp (reassembled)
	// 2. udp (try figure out request-response)
	// 3. dns
	// 4. http (flow)
	// 5. icmp (request-response)
	// 6. sni (tls client hello)
	SessionType string `gorm:"index"`

	DeviceName string `gorm:"index"`
	DeviceType string

	// LinkLayer physical layer
	IsLinkLayerEthernet bool
	LinkLayerSrc        string
	LinkLayerDst        string

	// NetworkLayer network layer
	IsIpv4          bool
	IsIpv6          bool
	NetworkSrcIP    string
	NetworkSrcIPInt int64
	NetworkDstIP    string
	NetworkDstIPInt int64

	// TransportLayer transport layer
	IsTcpIpStack          bool
	TransportLayerSrcPort int
	TransportLayerDstPort int

	// TCP State Flags
	// PDU Reassembled
	IsTCPReassembled bool
	// TCP SYN Detected? If so, it's a new TCP Session
	// 'half' means we haven't seen a FIN or RST
	IsHalfOpen bool
	// TCP FIN Detected
	IsClosed bool
	// TCP RST Detected
	IsForceClosed bool

	// TLS ClientHello
	HaveClientHello bool
	SNI             string
}
