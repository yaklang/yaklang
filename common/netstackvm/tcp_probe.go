package netstackvm

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/adapters/gonet"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/link/channel"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/tcp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/waiter"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
)

const tcpProbeStepTimeout = 3 * time.Second

const (
	// TCPSegmentSent is a segment the probe's stack transmitted.
	TCPSegmentSent = "sent"
	// TCPSegmentReceived is a segment the probe accepted from the peer.
	TCPSegmentReceived = "received"
)

// TCPSegment is one TCP segment observed on the probe's own stack, described
// from the connection's point of view (local is this endpoint, remote is the peer).
type TCPSegment struct {
	LocalIP    net.IP
	LocalPort  uint16
	RemoteIP   net.IP
	RemotePort uint16
	Seq        uint32
	Ack        uint32
	Flags      uint8
	SYN        bool
	ACK        bool
	FIN        bool
	RST        bool
	PSH        bool
	URG        bool
	PayloadLen int
	Direction  string
}

func (s TCPSegment) String() string {
	return fmt.Sprintf("%s local=%s:%d remote=%s:%d seq=%d ack=%d flags=0x%02x syn=%t ackf=%t fin=%t rst=%t psh=%t urg=%t payload=%d",
		s.Direction, s.LocalIP, s.LocalPort, s.RemoteIP, s.RemotePort, s.Seq, s.Ack, s.Flags, s.SYN, s.ACK, s.FIN, s.RST, s.PSH, s.URG, s.PayloadLen)
}

type probePhase int

const (
	phaseInit probePhase = iota
	phaseSynSent
	phaseSynAckSeen
	phaseEstablished
	phaseFinSent
	phasePeerClosed
	phaseClosed
)

func (p probePhase) String() string {
	switch p {
	case phaseInit:
		return "init"
	case phaseSynSent:
		return "syn-sent"
	case phaseSynAckSeen:
		return "syn-ack-held"
	case phaseEstablished:
		return "established"
	case phaseFinSent:
		return "fin-sent"
	case phasePeerClosed:
		return "peer-closed"
	case phaseClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// TCPProbe is one caller-driven TCP connection. Each method moves gVisor's
// existing handshake or close by exactly one step and returns the segment
// that step sent or received. Calling a step before the previous one has
// succeeded returns an error and does not mark the connection established
// or cleanly closed.
type TCPProbe struct {
	transport tcpProbeTransport
	retry     SYNRetryPolicy
	attempts  int
	probeStep chan struct{}
	halfOpen  bool

	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.Mutex
	phase  probePhase
	closed bool

	ep   tcpip.Endpoint
	wq   waiter.Queue
	conn *gonet.TCPConn

	link     *channel.Endpoint
	peerLink *channel.Endpoint
	remote   tcpip.FullAddress

	handshakeDone bool
	cleanClose    bool

	syn     TCPSegment
	synAck  TCPSegment
	fin     TCPSegment
	peerFin TCPSegment

	heldSynAck  []byte
	heldPeerFin [][]byte

	clientStash [][]byte
	peerStash   [][]byte

	bridgePaused bool
	bridgeCancel context.CancelFunc
	bridgeWG     sync.WaitGroup
}

// StartTCPProbe binds a gVisor TCP endpoint on vm toward hostport.
// peer is the channel-backed virtual machine that owns the listening stack.
// The handshake does not start until ProbeSYN.
func (vm *NetStackVirtualMachineEntry) StartTCPProbe(ctx context.Context, hostport string, peer *NetStackVirtualMachineEntry, options ...TCPProbeOption) (*TCPProbe, error) {
	if vm == nil || vm.stack == nil {
		return nil, fmt.Errorf("StartTCPProbe: virtual machine has no stack")
	}
	if peer == nil || peer == vm {
		return nil, fmt.Errorf("StartTCPProbe: need a distinct peer virtual machine")
	}
	if vm.link == nil || peer.link == nil {
		return nil, fmt.Errorf("StartTCPProbe: both virtual machines must be channel-backed")
	}
	if !vm.dhcpSuccess.IsSet() {
		return nil, fmt.Errorf("StartTCPProbe: gVisor path is not ready")
	}
	host, port, err := utils.ParseStringToHostPort(hostport)
	if err != nil {
		return nil, err
	}
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("StartTCPProbe: invalid port %d", port)
	}
	if !utils.IsIPv4(host) {
		host = netx.LookupFirst(host)
	}
	if !utils.IsIPv4(host) {
		return nil, fmt.Errorf("StartTCPProbe: need an ipv4 target, got %q", host)
	}
	localIP := vm.mainNICIPv4Address.To4()
	if localIP == nil {
		return nil, fmt.Errorf("StartTCPProbe: virtual machine has no ipv4 address")
	}
	remoteIP := net.ParseIP(host).To4()
	if remoteIP == nil {
		return nil, fmt.Errorf("StartTCPProbe: invalid ipv4 %q", host)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)

	p := &TCPProbe{
		ctx:      ctx,
		cancel:   cancel,
		link:     vm.link,
		peerLink: peer.link,
		remote: tcpip.FullAddress{
			NIC:  vm.MainNICID(),
			Addr: tcpip.AddrFrom4([4]byte(remoteIP)),
			Port: uint16(port),
		},
	}
	p.retry = SYNRetryPolicy{1, tcpProbeStepTimeout, tcpProbeStepTimeout, tcpProbeStepTimeout}
	for _, option := range options {
		option(&p.retry)
	}
	if err := p.retry.validate(); err != nil {
		cancel()
		return nil, err
	}
	p.probeStep = make(chan struct{}, 1)
	p.transport = &channelProbeTransport{p: p}
	ep, tcpErr := vm.stack.NewEndpoint(tcp.ProtocolNumber, header.IPv4ProtocolNumber, &p.wq)
	if tcpErr != nil {
		cancel()
		return nil, fmt.Errorf("StartTCPProbe: new endpoint: %s", tcpErr)
	}
	p.ep = ep
	local := tcpip.FullAddress{
		NIC:  vm.MainNICID(),
		Addr: tcpip.AddrFrom4([4]byte(localIP)),
	}
	if tcpErr = ep.Bind(local); tcpErr != nil {
		ep.Close()
		cancel()
		return nil, fmt.Errorf("StartTCPProbe: bind: %s", tcpErr)
	}
	return p, nil
}

// Established reports whether ProbeACK has completed.
func (p *TCPProbe) Established() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.handshakeDone
}

// Closed reports whether SendFinalACK has completed a clean four-way close.
func (p *TCPProbe) Closed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cleanClose
}

// Conn returns the established socket. It fails before ProbeACK and after
// SendFinalACK.
func (p *TCPProbe) Conn() (net.Conn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cleanClose || p.phase == phaseClosed {
		return nil, fmt.Errorf("tcp probe connection is closed")
	}
	if !p.handshakeDone || p.conn == nil {
		return nil, fmt.Errorf("tcp handshake is not established")
	}
	return p.conn, nil
}

// Write sends payload on the established connection.
func (p *TCPProbe) Write(b []byte) (int, error) {
	conn, err := p.Conn()
	if err != nil {
		return 0, err
	}
	return conn.Write(b)
}

// Close aborts the endpoint. It is not a clean four-way close.
func (p *TCPProbe) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	cancel := p.cancel
	ep := p.ep
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	p.pauseBridge()
	if p.transport != nil {
		return p.transport.close()
	}
	if ep != nil {
		ep.Close()
	}
	return nil
}

// ProbeSYN starts the gVisor active open and returns the SYN it transmitted.
func (p *TCPProbe) ProbeSYN() (TCPSegment, error) {
	return p.ProbeSYNContext(p.ctx)
}

// ReceiveSYNACK is the compatibility form using the probe lifetime context.
// Use ReceiveSYNACKContext to bound an individual operation.
func (p *TCPProbe) ReceiveSYNACK() (TCPSegment, error) {
	return p.ReceiveSYNACKContext(p.ctx)
}

// ProbeACK delivers the held SYN-ACK and returns the ACK gVisor sends.
// Payload can flow only after this returns successfully.
func (p *TCPProbe) ProbeACK() (TCPSegment, error) {
	if p.halfOpen {
		return TCPSegment{}, ErrHalfOpenACK
	}
	p.mu.Lock()
	if err := p.beginStep("ProbeACK", phaseSynAckSeen); err != nil {
		p.mu.Unlock()
		return TCPSegment{}, err
	}
	raw := append([]byte(nil), p.heldSynAck...)
	syn := p.syn
	synAck := p.synAck
	ep := p.ep
	p.mu.Unlock()
	if len(raw) == 0 {
		return TCPSegment{}, fmt.Errorf("ProbeACK: no SYN-ACK held")
	}
	injectIPv4(p.link, raw)

	ctx, cancel := p.stepCtx()
	defer cancel()
	for {
		seg, pkt, err := p.readTCP(ctx, p.link, &p.clientStash, false, "ACK")
		if err != nil {
			return TCPSegment{}, fmt.Errorf("ProbeACK: %w", err)
		}
		if seg.RST {
			return TCPSegment{}, fmt.Errorf("ProbeACK: peer reset: %s", seg)
		}
		if seg.SYN && !seg.ACK {
			continue
		}
		if seg.ACK && !seg.SYN && !seg.FIN && seg.Seq == syn.Seq+1 && seg.Ack == synAck.Seq+1 {
			if _, tcpErr := ep.GetRemoteAddress(); tcpErr != nil {
				return TCPSegment{}, fmt.Errorf("ProbeACK: stack is not established: %s", tcpErr)
			}
			injectIPv4(p.peerLink, pkt)
			p.mu.Lock()
			p.conn = gonet.NewTCPConn(&p.wq, ep)
			p.handshakeDone = true
			p.phase = phaseEstablished
			p.mu.Unlock()
			p.startBridge()
			return seg, nil
		}
		return TCPSegment{}, fmt.Errorf("ProbeACK: unexpected segment %s", seg)
	}
}

// SendFIN shuts down the write side and returns the FIN gVisor transmits.
func (p *TCPProbe) SendFIN() (TCPSegment, error) {
	p.mu.Lock()
	if err := p.beginStep("SendFIN", phaseEstablished); err != nil {
		p.mu.Unlock()
		return TCPSegment{}, err
	}
	ep := p.ep
	p.mu.Unlock()

	// Stop the bridge before shutdown so it cannot forward the FIN before
	// this call has recorded it.
	p.pauseBridge()
	if tcpErr := ep.Shutdown(tcpip.ShutdownWrite); tcpErr != nil {
		return TCPSegment{}, fmt.Errorf("SendFIN: shutdown write: %s", tcpErr)
	}

	ctx, cancel := p.stepCtx()
	defer cancel()
	for {
		seg, raw, err := p.readTCP(ctx, p.link, &p.clientStash, false, "FIN")
		if err != nil {
			return TCPSegment{}, fmt.Errorf("SendFIN: %w", err)
		}
		if !seg.FIN {
			injectIPv4(p.peerLink, raw)
			continue
		}
		injectIPv4(p.peerLink, raw)
		p.mu.Lock()
		p.fin = seg
		p.phase = phaseFinSent
		p.mu.Unlock()
		return seg, nil
	}
}

// ReceivePeerClose waits until the peer has acknowledged our FIN and sent
// its own FIN. ack is the segment that acknowledges our FIN. fin is the
// segment that carries the peer FIN. They are the same segment when the peer
// piggybacks both. The peer FIN is held until SendFinalACK.
func (p *TCPProbe) ReceivePeerClose() (ack TCPSegment, fin TCPSegment, err error) {
	p.mu.Lock()
	if err = p.beginStep("ReceivePeerClose", phaseFinSent); err != nil {
		p.mu.Unlock()
		return TCPSegment{}, TCPSegment{}, err
	}
	wantAck := p.fin.Seq + 1
	p.mu.Unlock()

	ctx, cancel := p.stepCtx()
	defer cancel()
	var (
		ackSeg  TCPSegment
		finSeg  TCPSegment
		haveAck bool
		haveFin bool
		held    [][]byte
	)
	for !(haveAck && haveFin) {
		seg, raw, readErr := p.readTCP(ctx, p.peerLink, &p.peerStash, true, "peer close")
		if readErr != nil {
			return TCPSegment{}, TCPSegment{}, fmt.Errorf("ReceivePeerClose: %w", readErr)
		}
		if seg.RST {
			return TCPSegment{}, TCPSegment{}, fmt.Errorf("ReceivePeerClose: reset: %s", seg)
		}
		coversOurFIN := seg.ACK && seg.Ack == wantAck
		if seg.FIN {
			finSeg = seg
			haveFin = true
			held = append(held, append([]byte(nil), raw...))
			if coversOurFIN {
				ackSeg = seg
				haveAck = true
			}
			continue
		}
		injectIPv4(p.link, raw)
		if coversOurFIN {
			ackSeg = seg
			haveAck = true
		}
	}

	p.mu.Lock()
	p.peerFin = finSeg
	p.heldPeerFin = held
	p.phase = phasePeerClosed
	p.mu.Unlock()
	return ackSeg, finSeg, nil
}

// SendFinalACK delivers the held peer FIN and returns the ACK gVisor sends.
func (p *TCPProbe) SendFinalACK() (TCPSegment, error) {
	p.mu.Lock()
	if err := p.beginStep("SendFinalACK", phasePeerClosed); err != nil {
		p.mu.Unlock()
		return TCPSegment{}, err
	}
	held := append([][]byte(nil), p.heldPeerFin...)
	wantAck := p.peerFin.Seq + 1
	wantSeq := p.fin.Seq + 1
	p.mu.Unlock()
	if len(held) == 0 {
		return TCPSegment{}, fmt.Errorf("SendFinalACK: no peer FIN held")
	}
	for _, raw := range held {
		injectIPv4(p.link, raw)
	}

	ctx, cancel := p.stepCtx()
	defer cancel()
	for {
		seg, raw, err := p.readTCP(ctx, p.link, &p.clientStash, false, "final ACK")
		if err != nil {
			return TCPSegment{}, fmt.Errorf("SendFinalACK: %w", err)
		}
		if seg.ACK && !seg.FIN && !seg.SYN && seg.Ack == wantAck && seg.Seq == wantSeq {
			injectIPv4(p.peerLink, raw)
			p.mu.Lock()
			p.cleanClose = true
			p.phase = phaseClosed
			p.mu.Unlock()
			return seg, nil
		}
		injectIPv4(p.peerLink, raw)
	}
}

func (p *TCPProbe) beginStep(step string, want probePhase) error {
	if p.closed {
		return fmt.Errorf("%s: probe is closed", step)
	}
	if p.phase != want {
		return fmt.Errorf("%s: connection is %s, need %s", step, p.phase, want)
	}
	return nil
}

func (p *TCPProbe) stepCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(p.ctx, tcpProbeStepTimeout)
}

func (p *TCPProbe) startBridge() {
	p.mu.Lock()
	if p.closed || p.bridgeCancel != nil || p.phase != phaseEstablished {
		p.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(p.ctx)
	p.bridgeCancel = cancel
	p.bridgePaused = false
	p.bridgeWG.Add(2)
	p.mu.Unlock()
	go p.pump(ctx, p.link, p.peerLink, &p.clientStash)
	go p.pump(ctx, p.peerLink, p.link, &p.peerStash)
}

func (p *TCPProbe) pauseBridge() {
	p.mu.Lock()
	cancel := p.bridgeCancel
	p.bridgeCancel = nil
	p.bridgePaused = true
	p.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	p.bridgeWG.Wait()
}

func (p *TCPProbe) pump(ctx context.Context, src, dst *channel.Endpoint, stash *[][]byte) {
	defer p.bridgeWG.Done()
	for {
		pkt := src.ReadContext(ctx)
		if pkt == nil {
			return
		}
		raw, err := packetBytes(pkt)
		if err != nil {
			continue
		}
		p.mu.Lock()
		if p.bridgePaused || p.bridgeCancel == nil {
			*stash = append(*stash, raw)
			p.mu.Unlock()
			return
		}
		p.mu.Unlock()
		injectIPv4(dst, raw)
	}
}

var errSkipNonTCP = errors.New("not tcp")

func (p *TCPProbe) readTCP(ctx context.Context, link *channel.Endpoint, stash *[][]byte, received bool, what string) (TCPSegment, []byte, error) {
	for i := 0; i < 32; i++ {
		raw, err := p.nextRaw(ctx, link, stash)
		if err != nil {
			return TCPSegment{}, nil, fmt.Errorf("waiting for %s: %w", what, err)
		}
		seg, err := parseTCPSegment(raw, received)
		if errors.Is(err, errSkipNonTCP) {
			continue
		}
		if err != nil {
			return TCPSegment{}, nil, err
		}
		return seg, raw, nil
	}
	return TCPSegment{}, nil, fmt.Errorf("waiting for %s: too many non-TCP packets", what)
}

func (p *TCPProbe) nextRaw(ctx context.Context, link *channel.Endpoint, stash *[][]byte) ([]byte, error) {
	p.mu.Lock()
	if stash != nil && len(*stash) > 0 {
		raw := (*stash)[0]
		*stash = (*stash)[1:]
		p.mu.Unlock()
		return raw, nil
	}
	p.mu.Unlock()
	if link == nil {
		return nil, fmt.Errorf("no channel link")
	}
	pkt := link.ReadContext(ctx)
	if pkt == nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("packet queue closed")
	}
	return packetBytes(pkt)
}

func parseTCPSegment(raw []byte, received bool) (TCPSegment, error) {
	ip := header.IPv4(raw)
	if !ip.IsValid(len(raw)) {
		return TCPSegment{}, errSkipNonTCP
	}
	if ip.TransportProtocol() != header.TCPProtocolNumber {
		return TCPSegment{}, errSkipNonTCP
	}
	tcpBytes := ip.Payload()
	if len(tcpBytes) < header.TCPMinimumSize {
		return TCPSegment{}, fmt.Errorf("short tcp header (%d bytes)", len(tcpBytes))
	}
	tcpHdr := header.TCP(tcpBytes)
	offset := int(tcpHdr.DataOffset())
	if offset < header.TCPMinimumSize || offset > len(tcpHdr) {
		return TCPSegment{}, fmt.Errorf("bad tcp data offset %d", offset)
	}
	flags := tcpHdr.Flags()
	src := ipv4Copy(net.IP(ip.SourceAddressSlice()))
	dst := ipv4Copy(net.IP(ip.DestinationAddressSlice()))
	seg := TCPSegment{
		Seq:        tcpHdr.SequenceNumber(),
		Ack:        tcpHdr.AckNumber(),
		Flags:      uint8(flags),
		SYN:        flags&header.TCPFlagSyn != 0,
		ACK:        flags&header.TCPFlagAck != 0,
		FIN:        flags&header.TCPFlagFin != 0,
		RST:        flags&header.TCPFlagRst != 0,
		PSH:        flags&header.TCPFlagPsh != 0,
		URG:        flags&header.TCPFlagUrg != 0,
		PayloadLen: len(tcpBytes) - offset,
	}
	if received {
		seg.Direction = TCPSegmentReceived
		seg.LocalIP, seg.LocalPort = dst, tcpHdr.DestinationPort()
		seg.RemoteIP, seg.RemotePort = src, tcpHdr.SourcePort()
	} else {
		seg.Direction = TCPSegmentSent
		seg.LocalIP, seg.LocalPort = src, tcpHdr.SourcePort()
		seg.RemoteIP, seg.RemotePort = dst, tcpHdr.DestinationPort()
	}
	return seg, nil
}

func ipv4Copy(ip net.IP) net.IP {
	if v4 := ip.To4(); v4 != nil {
		return append(net.IP(nil), v4...)
	}
	return append(net.IP(nil), ip...)
}
