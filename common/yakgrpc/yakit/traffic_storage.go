package yakit

import (
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/jinzhu/gorm"
	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type TrafficStorageManager struct {
	sync.Mutex

	db *gorm.DB

	// hash to session
	// icmp: hash = srcip + dstip + icmpid + seq
	// arp: hash(device + req-ip + req-mac)
	// dns: hash(id)
	sessions map[string]*TrafficSession
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
	return &TrafficStorageManager{
		db:       db,
		sessions: make(map[string]*TrafficSession),
	}
}

func (m *TrafficStorageManager) handleLinkLayerTraffic(t *TrafficPacket, packet gopacket.Packet) (string, bool) {
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
	}
	return "", false
}

func (m *TrafficStorageManager) handleNetworkLayerTraffic(t *TrafficPacket, packet gopacket.Packet) (string, bool) {
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
		var hash = utils.CalcSha256("icmp4", networkFlowHash(packet), icmp.Id)
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
			item := []string{
				utils.HostPort(src, int(tcp.SrcPort)),
				utils.HostPort(dst, int(tcp.DstPort)),
			}
			sort.Strings(item)
			return strings.Join(item, "-")
		}
	}
	return extra
}

func (m *TrafficStorageManager) handleTransportLayerTraffic(t *TrafficPacket, packet gopacket.Packet) (string, bool) {
	switch t.TransportLayerType {
	case "tcp", "udp", "tcp4", "udp4":
		// skip tcp, because tcp is a stream handled by pdu reassembled
		// skip udp, cannot find udp session
		return "", true
	}
	return "", false
}

func (m *TrafficStorageManager) handleApplicationLayerTraffic(t *TrafficPacket, packet gopacket.Packet) (string, bool) {
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

func (m *TrafficStorageManager) CreateOrFetchSession(hash string, packet gopacket.Packet, tpacket *TrafficPacket, typeStr string) (*TrafficSession, error) {
	if packet == nil {
		return nil, utils.Error("packet is nil")
	}

	if hash == "" {
		return nil, utils.Error("hash is empty")
	}

	if tpacket == nil {
		return nil, utils.Error("traffic_packet is nil")
	}

	session, ok := m.sessions[hash]
	if !ok {
		session = &TrafficSession{
			Uuid:                  uuid.NewV4().String(),
			SessionType:           strings.ToLower(typeStr),
			LinkLayerSrc:          "",
			LinkLayerDst:          "",
			NetworkLayerSrc:       "",
			NetworkSrcIP:          "",
			NetworkSrcIPInt:       0,
			NetworkLayerDst:       "",
			NetworkDstIP:          "",
			NetworkDstIPInt:       0,
			TransportLayerSrcPort: 0,
			TransportLayerDstPort: 0,
			IsTCPReassembled:      false,
			IsHalfOpen:            false,
			IsClosed:              false,
			IsForceClosed:         false,
			HaveClientHello:       false,
			SNI:                   "",
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

		m.sessions[hash] = session
	}
	return session, nil
}

func (m *TrafficStorageManager) Save(packet gopacket.Packet) error {
	payload, ok := getPacketPayload(packet)
	if !ok {
		payload = nil
	}
	var trafficPacket = &TrafficPacket{
		LinkLayerType:        strings.ToLower(pcaputil.LinkLayerName(packet)),
		NetworkLayerType:     strings.ToLower(pcaputil.NetworkLayerName(packet)),
		TransportLayerType:   strings.ToLower(pcaputil.TransportLayerName(packet)),
		ApplicationLayerType: strings.ToLower(pcaputil.ApplicationLayerName(packet)),
		Payload:              strconv.Quote(string(payload)),
		QuotedRaw:            strconv.Quote(string(packet.Data())),
	}

	var hash string
	if hash, ok = m.handleLinkLayerTraffic(trafficPacket, packet); ok {
		if hash != "" {
			log.Infof("%v: %v", trafficPacket.LinkLayerType, hash)
		}
	} else if hash, ok = m.handleNetworkLayerTraffic(trafficPacket, packet); ok {
		if hash != "" {
			log.Infof("%v: %v", trafficPacket.NetworkLayerType, hash)
		}
	} else if hash, ok = m.handleTransportLayerTraffic(trafficPacket, packet); ok {
		if hash != "" {
			log.Infof("%v: %v", trafficPacket.TransportLayerType, hash)
		}
	} else {
		log.Infof("packet: %v-%v-%v-%v", trafficPacket.LinkLayerType, trafficPacket.NetworkLayerType, trafficPacket.TransportLayerType, trafficPacket.ApplicationLayerType)
		fmt.Println(packet.Dump())
	}

	if hash != "" {

	}

	if err := SaveTrafficPacket(consts.GetGormProjectDatabase(), trafficPacket); err != nil {
		log.Errorf("save traffic packet failed: %s", err)
		return err
	}
	return nil
}
