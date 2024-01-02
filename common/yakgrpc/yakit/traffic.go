package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"time"
)

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

type TrafficTCPReassembledFrame struct {
	gorm.Model

	SessionUuid string `gorm:"index"`
	QuotedData  string
	Seq         int64
	Timestamp   int64
	Source      string
	Destination string
}

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

func SaveTrafficSession(db *gorm.DB, session *TrafficSession) error {
	return db.Save(session).Error
}

func SaveTrafficPacket(db *gorm.DB, packet *TrafficPacket) error {
	return db.Save(packet).Error
}

func QueryTrafficTCPReassembled(db *gorm.DB, request *ypb.QueryTrafficTCPReassembledRequest) (*bizhelper.Paginator, []*TrafficTCPReassembledFrame, error) {
	db = db.Model(&TrafficTCPReassembledFrame{})

	if request.GetTimestampNow() > 0 {
		db = db.Where("created_at >= ?", time.Unix(request.GetTimestampNow(), 0))
	}

	if request.GetFromId() > 0 {
		db = db.Where("id > ?", request.GetFromId())
	}

	if request.GetUntilId() > 0 {
		db = db.Where("id <= ?", request.GetUntilId())
	}

	var data []*TrafficTCPReassembledFrame
	p, db := bizhelper.PagingByPagination(db, request.GetPagination(), &data)
	if db.Error != nil {
		return nil, nil, db.Error
	}
	return p, data, nil
}

func QueryTrafficSessionByUUID(db *gorm.DB, uuid string) (*TrafficSession, error) {
	db = db.Model(&TrafficSession{})
	db = db.Where("uuid = ?", uuid)
	var data TrafficSession
	db = db.Find(&data)
	if db.Error != nil {
		return nil, db.Error
	}
	return &data, nil
}

func QueryTrafficSession(db *gorm.DB, request *ypb.QueryTrafficSessionRequest) (*bizhelper.Paginator, []*TrafficSession, error) {
	db = db.Model(&TrafficSession{})

	if request.GetTimestampNow() > 0 {
		db = db.Where("created_at >= ?", time.Unix(request.GetTimestampNow(), 0))
	}

	if request.GetFromId() > 0 {
		db = db.Where("id > ?", request.GetFromId())
	}

	if request.GetUntilId() > 0 {
		db = db.Where("id <= ?", request.GetUntilId())
	}

	var data []*TrafficSession
	p, err := bizhelper.PagingByPagination(db, request.GetPagination(), &data)
	if err.Error != nil {
		return nil, nil, err.Error
	}
	return p, data, nil
}

func QueryTrafficPacket(db *gorm.DB, request *ypb.QueryTrafficPacketRequest) (*bizhelper.Paginator, []*TrafficPacket, error) {
	db = db.Model(&TrafficPacket{})
	var data []*TrafficPacket

	if request.GetTimestampNow() > 0 {
		db = db.Where("created_at >= ?", time.Unix(request.GetTimestampNow(), 0))
	}

	db = db.Where("id > ?", request.GetFromId())

	p, err := bizhelper.Paging(
		db,
		int(request.GetPagination().GetPage()),
		int(request.GetPagination().GetLimit()),
		&data,
	)
	if err.Error != nil {
		return nil, nil, err.Error
	}
	return p, data, nil
}
