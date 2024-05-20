package schema

import "github.com/jinzhu/gorm"

type TrafficPacket struct {
	gorm.Model

	SessionUuid string `gorm:"index"`

	LinkLayerType        string
	NetworkLayerType     string
	TransportLayerType   string
	ApplicationLayerType string
	Payload              string

	// QuotedRaw contains the raw bytes of the packet, quoted such that it can be
	// caution: QuotedRaw is (maybe) not an utf8-valid string
	// quoted-used for save to database
	QuotedRaw string

	EthernetEndpointHardwareAddrSrc string
	EthernetEndpointHardwareAddrDst string
	IsIpv4                          bool
	IsIpv6                          bool
	NetworkEndpointIPSrc            string
	NetworkEndpointIPDst            string
	TransportEndpointPortSrc        int
	TransportEndpointPortDst        int
}
