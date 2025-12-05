package netstackvm

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/crypto/cryptobyte"
	"strings"
	"sync"
)

type sniffRawEndpoint struct {
	protocol     tcpip.TransportProtocolNumber
	sniffHandles []func(*stack.PacketBuffer)
	lock         sync.Mutex
}

func (s *sniffRawEndpoint) HandlePacket(buffer *stack.PacketBuffer) {
	s.lock.Lock()
	defer s.lock.Unlock()
	for _, h := range s.sniffHandles {
		h(buffer)
	}
}

func (s *sniffRawEndpoint) appendHandle(handle func(buffer *stack.PacketBuffer)) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.sniffHandles = append(s.sniffHandles, handle)
}

type NetstackSniffer struct {
	vm            *NetStackVirtualMachine
	sniffEndpoint map[tcpip.TransportProtocolNumber]*sniffRawEndpoint
	lock          sync.Mutex
}

func NewNetstackSniffer(vm *NetStackVirtualMachine) *NetstackSniffer {
	return &NetstackSniffer{
		vm:            vm,
		sniffEndpoint: make(map[tcpip.TransportProtocolNumber]*sniffRawEndpoint),
	}
}

func (m *NetstackSniffer) setSniffEndpoint(id tcpip.TransportProtocolNumber, ep *sniffRawEndpoint) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.sniffEndpoint[id] = ep
}

func (m *NetstackSniffer) RegisterSniffHandle(protocol tcpip.TransportProtocolNumber, handle func(*stack.PacketBuffer)) {
	if ep, ok := m.sniffEndpoint[protocol]; ok {
		ep.appendHandle(handle)
	} else {
		ep := &sniffRawEndpoint{
			protocol:     protocol,
			sniffHandles: make([]func(*stack.PacketBuffer), 0),
		}
		ep.appendHandle(handle)
		m.setSniffEndpoint(protocol, ep)
		m.vm.stack.RegisterRawTransportEndpoint(header.IPv4ProtocolNumber, protocol, ep)
	}
}

type TCPKey struct {
	src string
	dst string
}

func (t *TCPKey) Key() string {
	if strings.Compare(t.src, t.dst) > 0 {
		return utils.CalcSha1(t.src, t.dst)
	} else {
		return utils.CalcSha1(t.dst, t.src)
	}
}

func newTCPKey(src, dst string) *TCPKey {
	return &TCPKey{
		src: src,
		dst: dst,
	}
}

type TCPAliveInfo struct {
	connAliveInfo map[string]struct{}
	lock          sync.Mutex
}

type AliveTargetMonitor struct {
	sniffer     *NetstackSniffer
	targetAlive map[string]int
	targetLock  sync.Mutex

	tcpAlive *sync.Map

	domainAlive map[string]string // key -> domain
	domainLock  sync.Mutex
}

func (m *AliveTargetMonitor) addAliveTarget(dstIP string, key string) {
	m.targetLock.Lock()
	defer m.targetLock.Unlock()

	if _, loaded := m.tcpAlive.LoadOrStore(key, struct{}{}); !loaded {
		if count, ok := m.targetAlive[dstIP]; ok {
			m.targetAlive[dstIP] = count + 1
		} else {
			m.targetAlive[dstIP] = 1
		}
	}
}

func (m *AliveTargetMonitor) deleteAliveTarget(dstIP string, key string) {
	m.targetLock.Lock()
	if _, loaded := m.tcpAlive.LoadAndDelete(key); loaded {
		if count, ok := m.targetAlive[dstIP]; ok {
			if count == 1 {
				delete(m.targetAlive, dstIP)
			} else {
				m.targetAlive[dstIP] = count - 1
			}
		}
	}
	m.targetLock.Unlock()

	m.domainLock.Lock()
	delete(m.domainAlive, key)
	m.domainLock.Unlock()
}

func (m *AliveTargetMonitor) GetAliveIP() map[string]int {
	m.targetLock.Lock()
	defer m.targetLock.Unlock()
	res := make(map[string]int)
	for ip, count := range m.targetAlive {
		res[ip] = count
	}
	return res
}

func (m *AliveTargetMonitor) addAliveDomain(domain string, key string) {
	m.domainLock.Lock()
	defer m.domainLock.Unlock()

	m.domainAlive[key] = domain
}

func (m *AliveTargetMonitor) GetAliveDomain() []string {
	m.domainLock.Lock()
	defer m.domainLock.Unlock()
	res := make([]string, 0)
	for _, domain := range m.domainAlive {
		res = append(res, domain)
	}
	return lo.Uniq(res)
}

func StartTargetMonitor() (*AliveTargetMonitor, error) {
	vm, err := NewSystemNetStackVM(WithPcapCapabilities(stack.CapabilityRXChecksumOffload))
	if err != nil {
		return nil, err
	}
	sniffer := NewNetstackSniffer(vm)
	m := &AliveTargetMonitor{
		sniffer:     sniffer,
		targetAlive: make(map[string]int),
		tcpAlive:    &sync.Map{},
		domainAlive: make(map[string]string),
	}

	localIP := make([]string, 0)
	for _, entry := range vm.entries {
		localIP = append(localIP, entry.mainNICIPv4Address.String())
	}

	checkLocalIP := func(ip string) bool {
		for _, local := range localIP {
			if ip == local {
				return true
			}
		}
		return false
	}

	sniffer.RegisterSniffHandle(header.TCPProtocolNumber, func(buffer *stack.PacketBuffer) {
		ipHeader := header.IPv4(buffer.NetworkHeader().Slice())
		if !ipHeader.IsValid(buffer.Size()) {
			return
		}
		srcIP := ipHeader.SourceAddress().String()
		dstIP := ipHeader.DestinationAddress().String()
		targetIP := dstIP
		if checkLocalIP(dstIP) {
			if checkLocalIP(srcIP) {
				return
			} else {
				targetIP = srcIP
			}
		}

		tcpHeader := header.TCP(buffer.TransportHeader().Slice())
		tcpFlags := tcpHeader.Flags()
		srcPort := tcpHeader.SourcePort()
		dstPort := tcpHeader.DestinationPort()

		key := newTCPKey(utils.HostPort(srcIP, srcPort), utils.HostPort(dstIP, dstPort)).Key()

		if tcpFlags&(header.TCPFlagRst|header.TCPFlagFin) != 0 {
			m.deleteAliveTarget(targetIP, key)
		} else if tcpFlags&(header.TCPFlagSyn|header.TCPFlagAck|header.TCPFlagPsh) != 0 {
			m.addAliveTarget(targetIP, key)
		}

		tcpPayload := GetTransportPayload(buffer)
		if serverName := ReadServerName(tcpPayload); serverName != "" {
			m.addAliveDomain(serverName, key)
		}
	})
	return m, nil
}

func GetTransportPayload(buffer *stack.PacketBuffer) []byte {
	allView := buffer.ToView()
	allView.TrimFront(buffer.LinkHeader().View().Size() + buffer.NetworkHeader().View().Size() + buffer.TransportHeader().View().Size())
	return allView.ToSlice()
}

func ReadServerName(data []byte) string {
	s := cryptobyte.String(data)

	var handshake uint8
	if !s.ReadUint8(&handshake) || handshake != 0x16 { // handshake
		return ""
	}

	if !s.Skip(42) { // trim until session id
		return ""
	}

	var session cryptobyte.String
	if !s.ReadUint8LengthPrefixed(&session) { // trim session id and session
		return ""
	}
	var suites cryptobyte.String
	if !s.ReadUint16LengthPrefixed(&suites) { // trim cipher suites
		return ""
	}
	var compression cryptobyte.String
	if !s.ReadUint8LengthPrefixed(&compression) { // trim compression methods
		return ""
	}

	if !s.Skip(2) {
		return ""
	}

	for !s.Empty() {
		var extension uint16
		var extData cryptobyte.String
		if !s.ReadUint16(&extension) ||
			!s.ReadUint16LengthPrefixed(&extData) {
			return ""
		}

		switch extension {
		case uint16(0):
			// RFC 6066, Section 3
			var nameList cryptobyte.String
			if !extData.ReadUint16LengthPrefixed(&nameList) || nameList.Empty() {
				return ""
			}
			for !nameList.Empty() {
				var nameType uint8
				var serverName cryptobyte.String
				if !nameList.ReadUint8(&nameType) ||
					!nameList.ReadUint16LengthPrefixed(&serverName) ||
					serverName.Empty() {
					return ""
				}
				if nameType != 0 {
					continue
				}
				return string(serverName)
			}

		default:
			// Ignore unknown extensions.
			continue
		}
	}

	return ""
}
