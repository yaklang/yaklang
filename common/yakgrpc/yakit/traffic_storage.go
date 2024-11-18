package yakit

import (
	"fmt"
	"github.com/yaklang/yaklang/common/schema"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	uuid "github.com/google/uuid"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils"
)

type TrafficStorageManager struct {
	sync.Mutex

	db *gorm.DB

	// hash to session
	// icmp: hash = srcip + dstip + icmpid + seq
	// arp: hash(device + req-ip + req-mac)
	// dns: hash(id)
	sessions *utils.Cache[*schema.TrafficSession] // map[string]*TrafficSession
}

func getPacketPayload(packet gopacket.Packet) ([]byte, bool) {
	// transport layer existed
	if l := packet.TransportLayer(); l != nil {
		return l.LayerPayload(), true
	}

	if l := packet.NetworkLayer(); l != nil {
		return l.LayerPayload(), true
	}

	if l := packet.LinkLayer(); l != nil {
		return l.LayerPayload(), true
	}

	return packet.Data(), false
}

func NewTrafficStorageManager(db *gorm.DB) *TrafficStorageManager {
	sessionCache := utils.NewTTLCache[*schema.TrafficSession](time.Minute)
	return &TrafficStorageManager{
		db:       db,
		sessions: sessionCache,
	}
}

func (m *TrafficStorageManager) handleLinkLayerTraffic(t *schema.TrafficPacket, packet gopacket.Packet) (string, bool) {
	switch t.LinkLayerType {
	case "arp":
		l := packet.Layer(layers.LayerTypeARP)
		if l == nil {
			return "", false
		}
		arp, ok := l.(*layers.ARP)
		if !ok {
			return "", false
		}
		var hash string
		switch arp.Operation {
		case layers.ARPRequest:
			hash = utils.CalcSha256("arp", arp.SourceProtAddress, arp.SourceHwAddress)
		case layers.ARPReply:
			hash = utils.CalcSha256("arp", arp.DstProtAddress, arp.DstHwAddress)
		}
		if hash == "" {
			return "", false
		}
		return "", true
	case "ethernet":
		l := packet.Layer(layers.LayerTypeEthernet)
		if l == nil {
			return "", false
		}
		ethernet, ok := l.(*layers.Ethernet)
		if !ok {
			return "", false
		}
		t.EthernetEndpointHardwareAddrSrc = ethernet.SrcMAC.String()
		t.EthernetEndpointHardwareAddrDst = ethernet.DstMAC.String()
	}
	return "", false
}

func (m *TrafficStorageManager) handleNetworkLayerTraffic(t *schema.TrafficPacket, packet gopacket.Packet) (string, bool) {
	switch ret := packet.NetworkLayer().(type) {
	case *layers.IPv4:
		t.NetworkEndpointIPSrc = ret.SrcIP.String()
		t.NetworkEndpointIPDst = ret.DstIP.String()
		t.IsIpv4 = true
	case *layers.IPv6:
		t.NetworkEndpointIPSrc = ret.SrcIP.String()
		t.NetworkEndpointIPDst = ret.DstIP.String()
		t.IsIpv6 = true
	}

	switch t.NetworkLayerType {
	case "icmpv4", "icmp", "icmp4":
		l := packet.Layer(layers.LayerTypeICMPv4)
		if l == nil {
			return "", false
		}
		icmp, ok := l.(*layers.ICMPv4)
		if !ok {
			return "", false
		}
		hash := utils.CalcSha256("icmp4", networkFlowHash(packet), icmp.Id)
		return hash, true
	case "igmp":
		return "", true
	case "icmp6", "icmpv6":
		return "", true
	}
	return "", false
}

func networkFlowHash(packet gopacket.Packet) string {
	var extra string
	if nl := packet.NetworkLayer(); nl != nil {
		item := []string{
			nl.NetworkFlow().Src().String(), nl.NetworkFlow().Dst().String(),
		}
		sort.Strings(item)
		extra = strings.Join(item, "-")
	}
	return extra
}

func flowHashCalc(src string, dst string) string {
	item := []string{
		src, dst,
	}
	sort.Strings(item)
	return strings.Join(item, "-")
}

func transportFlowHash(packet gopacket.Packet) string {
	var extra string
	if nl := packet.NetworkLayer(); nl != nil {
		var src string
		var dst string
		switch ret := nl.(type) {
		case *layers.IPv4:
			src = ret.SrcIP.String()
			dst = ret.DstIP.String()
		case *layers.IPv6:
			src = ret.SrcIP.String()
			dst = ret.DstIP.String()
		}

		if tl := packet.TransportLayer(); tl != nil {
			tcp, ok := tl.(*layers.TCP)
			if !ok {
				return ""
			}
			if src == "" || dst == "" {
				return ""
			}
			return flowHashCalc(utils.HostPort(src, int(tcp.SrcPort)), utils.HostPort(dst, int(tcp.DstPort)))
		}
	}
	return extra
}

func (m *TrafficStorageManager) handleTransportLayerTraffic(t *schema.TrafficPacket, packet gopacket.Packet) (string, bool) {
	switch ret := packet.TransportLayer().(type) {
	case *layers.TCP:
		t.TransportEndpointPortSrc = int(ret.SrcPort)
		t.TransportEndpointPortDst = int(ret.DstPort)
	case *layers.UDP:
		t.TransportEndpointPortSrc = int(ret.SrcPort)
		t.TransportEndpointPortDst = int(ret.DstPort)
	}

	switch t.TransportLayerType {
	case "tcp", "udp", "tcp4", "udp4":
		// skip tcp, because tcp is a stream handled by pdu reassembled
		// skip udp, cannot find udp session
		return "", true
	}
	return "", false
}

func (m *TrafficStorageManager) handleApplicationLayerTraffic(t *schema.TrafficPacket, packet gopacket.Packet) (string, bool) {
	switch t.ApplicationLayerType {
	case "http":
		// skip http, because http is a flow handled by http flow
		return "", true
	case "dns":
		var hash string
		l := packet.Layer(layers.LayerTypeDNS)
		if l == nil {
			return "", false
		}
		dns, ok := l.(*layers.DNS)
		if !ok {
			return "", false
		}
		hash = utils.CalcSha256("dns", transportFlowHash(packet), dns.ID)
		return hash, true
	}
	return "", false
}

func (m *TrafficStorageManager) FetchSession(hash string, packet gopacket.Packet, tpacket *schema.TrafficPacket, typeStr string, noCreate bool) (*schema.TrafficSession, error) {
	if packet == nil {
		return nil, utils.Error("packet is nil")
	}

	if hash == "" {
		return nil, utils.Error("hash is empty")
	}

	if tpacket == nil {
		return nil, utils.Error("traffic_packet is nil")
	}

	session, ok := m.sessions.Get(hash)
	if !ok {
		if noCreate {
			return nil, utils.Errorf("no existed session/flow: %s", hash)
		}
		session = &schema.TrafficSession{
			Uuid:                  uuid.New().String(),
			SessionType:           strings.ToLower(typeStr),
			LinkLayerSrc:          tpacket.EthernetEndpointHardwareAddrSrc,
			LinkLayerDst:          tpacket.EthernetEndpointHardwareAddrDst,
			NetworkSrcIP:          tpacket.NetworkEndpointIPSrc,
			NetworkDstIP:          tpacket.NetworkEndpointIPDst,
			TransportLayerSrcPort: tpacket.TransportEndpointPortSrc,
			TransportLayerDstPort: tpacket.TransportEndpointPortDst,
			IsIpv4:                tpacket.IsIpv4,
			IsIpv6:                tpacket.IsIpv6,
		}
		if ret := packet.Metadata(); ret != nil && ret.InterfaceIndex >= 0 {
			iface, err := pcaputil.GetPcapInterfaceByIndex(ret.InterfaceIndex)
			if err != nil {
				return nil, err
			}
			session.DeviceName = iface.Name
		}
		switch tpacket.LinkLayerType {
		case "ethernet", "arp":
			session.DeviceType = "ethernet"
			session.IsLinkLayerEthernet = true
		}

		switch tpacket.TransportLayerType {
		case "tcp", "tcp4", "udp", "udp4", "icmp", "icmp4", "icmpv4", "igmp", "icmp6", "icmpv6":
			session.IsTcpIpStack = true
		}

		m.sessions.Set(hash, session)
	}
	return session, nil
}

func (m *TrafficStorageManager) CreateTCPReassembledFlow(flow *pcaputil.TrafficFlow) error {
	if flow == nil {
		return utils.Error("flow is nil")
	}
	hash := flowHashCalc(flow.ClientConn.LocalAddr().String(), flow.ClientConn.RemoteAddr().String())
	session := &schema.TrafficSession{
		Uuid:                  uuid.New().String(),
		SessionType:           "tcp",
		IsIpv4:                flow.IsIpv4,
		IsIpv6:                flow.IsIpv6,
		NetworkSrcIP:          flow.ClientConn.LocalIP().String(),
		NetworkDstIP:          flow.ClientConn.RemoteIP().String(),
		IsTcpIpStack:          true,
		TransportLayerSrcPort: flow.ClientConn.LocalPort(),
		TransportLayerDstPort: flow.ClientConn.RemotePort(),
		IsTCPReassembled:      true,
		IsHalfOpen:            flow.IsHalfOpen,
	}
	err := SaveTrafficSession(m.db, session)
	if err != nil {
		return err
	}
	m.sessions.Set(hash, session)
	return nil
}

func (m *TrafficStorageManager) CloseTCPFlow(flow *pcaputil.TrafficFlow, force bool) error {
	hash := flowHashCalc(flow.ClientConn.LocalAddr().String(), flow.ClientConn.RemoteAddr().String())
	session, ok := m.sessions.Get(hash)
	if !ok {
		return utils.Errorf("no existed session/flow: %s", hash)
	}
	session.IsClosed = true
	if force {
		session.IsForceClosed = true
	}
	return m.db.Save(session).Error
}

func (m *TrafficStorageManager) SaveTCPReassembledFrame(flow *pcaputil.TrafficFlow, frame *pcaputil.TrafficFrame) error {
	hash := flowHashCalc(flow.ClientConn.LocalAddr().String(), flow.ClientConn.RemoteAddr().String())
	session, ok := m.sessions.Get(hash)
	if !ok {
		return utils.Errorf("no existed session/flow: %s", hash)
	}
	storageFrame := &schema.TrafficTCPReassembledFrame{
		SessionUuid: session.Uuid,
		QuotedData:  strconv.Quote(string(frame.Payload)),
		Seq:         int64(frame.Seq),
		Timestamp:   frame.Timestamp.Unix(),
	}
	return m.db.Save(storageFrame).Error
}

func (m *TrafficStorageManager) CreateHTTPFlow(flow *pcaputil.TrafficFlow, req *http.Request, rsp *http.Response) error {
	return nil
}

func (m *TrafficStorageManager) SaveRawPacket(packet gopacket.Packet) error {
	payload, ok := getPacketPayload(packet)
	if !ok {
		payload = nil
	}
	trafficPacket := &schema.TrafficPacket{
		LinkLayerType:        strings.ToLower(pcaputil.LinkLayerName(packet)),
		NetworkLayerType:     strings.ToLower(pcaputil.NetworkLayerName(packet)),
		TransportLayerType:   strings.ToLower(pcaputil.TransportLayerName(packet)),
		ApplicationLayerType: strings.ToLower(pcaputil.ApplicationLayerName(packet)),
		Payload:              strconv.Quote(string(payload)),
		QuotedRaw:            strconv.Quote(string(packet.Data())),
	}

	var hash string
	var sessionType string
	var noCreateFlow bool
	if hash, ok = m.handleLinkLayerTraffic(trafficPacket, packet); ok {
		if hash != "" {
			sessionType = trafficPacket.LinkLayerType
			log.Infof("%v: %v", trafficPacket.LinkLayerType, hash)
		}
	} else if hash, ok = m.handleNetworkLayerTraffic(trafficPacket, packet); ok {
		if hash != "" {
			sessionType = trafficPacket.NetworkLayerType
			log.Infof("%v: %v", trafficPacket.NetworkLayerType, hash)
		}
	} else if hash, ok = m.handleTransportLayerTraffic(trafficPacket, packet); ok {
		// transport layer traffic is a flow
		// not created simple session
		// create via pdu reassembled
		noCreateFlow = true
		if hash != "" {
			sessionType = trafficPacket.TransportLayerType
			log.Infof("%v: %v", trafficPacket.TransportLayerType, hash)
		}
	} else {
		log.Infof("packet: %v-%v-%v-%v", trafficPacket.LinkLayerType, trafficPacket.NetworkLayerType, trafficPacket.TransportLayerType, trafficPacket.ApplicationLayerType)
		fmt.Println(packet.Dump())
	}

	if hash != "" {
		session, err := m.FetchSession(hash, packet, trafficPacket, sessionType, noCreateFlow)
		if err != nil {
			return err
		}
		trafficPacket.SessionUuid = session.Uuid
		err = SaveTrafficSession(m.db, session)
		if err != nil {
			log.Errorf("save traffic session failed: %s", err)
		}
	}

	if err := SaveTrafficPacket(m.db, trafficPacket); err != nil {
		log.Errorf("save traffic packet failed: %s", err)
		return err
	}
	return nil
}
