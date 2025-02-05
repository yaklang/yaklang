package netstackvm

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os/exec"
	"time"

	"github.com/yaklang/yaklang/common/log"
	tun "github.com/yaklang/yaklang/common/lowtun"
	"github.com/yaklang/yaklang/common/lowtun/netstack"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/adapters/gonet"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/ipv4"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/ipv6"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/icmp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/tcp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/udp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/waiter"
	"github.com/yaklang/yaklang/common/lowtun/netstack/rwendpoint"
	"github.com/yaklang/yaklang/common/utils"
)

// TUN_MTU is the default MTU for TUN device. 1420 is wg default MTU, use it for compatibility.
const TUN_MTU = 1420

const UTUNINDEXSTART = 410

type TunVirtualMachine struct {
	ctx    context.Context
	cancel context.CancelFunc

	tunnelDevice tun.Device
	tunnelName   string
	tunEp        stack.LinkEndpoint

	stack     *stack.Stack
	mainNicID tcpip.NICID

	tcpHijacked *utils.AtomicBool
}

func NewTunVirtualMachine(ctx context.Context) (*TunVirtualMachine, error) {
	start := time.Now()
	defer func() {
		log.Infof("NewTunVirtualMachine took %v", time.Since(start))
	}()

	if ctx == nil {
		ctx = context.Background()
	}

	idxStart := UTUNINDEXSTART
	var utunName string
	for i := 0; i < 10; i++ {
		utunName = fmt.Sprintf("utun%d", idxStart)
		_, err := net.InterfaceByName(utunName)
		if err == nil {
			log.Errorf("utun%d already exists", idxStart)
			utunName = ""
			idxStart++
			continue
		}
	}
	if utunName == "" {
		return nil, utils.Error("failed to find available utun index")
	}

	log.Infof("Creating TUN device with name: %s", utunName)
	device, err := tun.CreateTUN(utunName, TUN_MTU)
	if err != nil {
		return nil, utils.Errorf("tun.CreateTUN failed: %v", err)
	}

	baseCtx, cancel := context.WithCancel(ctx)

	mtu := uint32(TUN_MTU)
	offset := 4
	log.Infof("Creating TUN endpoint with MTU: %d", mtu)
	tunEp, err := rwendpoint.NewReadWriteCloserEndpointContext(
		ctx, rwendpoint.NewWireGuardReadWriteCloserWrapper(device, mtu, offset),
		uint32(TUN_MTU),
		offset,
	)
	if err != nil {
		cancel()
		return nil, utils.Errorf("create tun endpoint failed: %v", err)
	}

	// 172.16.0.0/12 choose 2 random ip as tunnel ip
	ipMin, err := utils.IPv4ToUint32(net.ParseIP("172.16.0.1").To4())
	if err != nil {
		cancel()
		return nil, utils.Errorf("IPv4ToUint32(%s) failed: %v", "172.16.0.1", err)
	}
	ipMax, err := utils.IPv4ToUint32(net.ParseIP("172.31.255.254").To4())
	if err != nil {
		cancel()
		return nil, utils.Errorf("IPv4ToUint32(%s) failed: %v", "172.31.255.254", err)
	}
	delta := int(ipMax - ipMin)
	ip1 := ipMin + uint32(rand.Intn(delta))
	ip2 := ipMin + uint32(rand.Intn(delta))
	ip1Str := net.ParseIP(utils.Uint32ToIPv4(ip1).String())
	ip2Str := net.ParseIP(utils.Uint32ToIPv4(ip2).String())
	log.Infof("Tunnel IP: %s -> %s", ip1Str, ip2Str)

	ifconfigTimeout, ifConfigTimeoutCancel := context.WithTimeout(ctx, 10*time.Second)
	defer ifConfigTimeoutCancel()
	cmd := exec.CommandContext(ifconfigTimeout, "ifconfig", utunName, ip1Str.String(), ip2Str.String(), "up")
	raw, err := cmd.CombinedOutput()
	if err != nil {
		cancel()
		log.Infof("ifconfig failed: %v\nmsg: %s", err, string(raw))
		return nil, utils.Errorf("ifconfig failed: %v", err)
	}

	log.Infof("Initializing network stack")
	s := stack.New(stack.Options{
		NetworkProtocols: []stack.NetworkProtocolFactory{
			ipv4.NewProtocol,
			ipv6.NewProtocol,
		},
		TransportProtocols: []stack.TransportProtocolFactory{
			tcp.NewProtocol,
			udp.NewProtocol,
			icmp.NewProtocol4,
			icmp.NewProtocol6,
		},
		HandleLocal: false,
	})
	err = defaultInitNetStack(s)
	if err != nil {
		cancel()
		return nil, utils.Errorf("defaultInitNetStack failed: %v", err)
	}
	mainNICId := s.NextNICID()
	if tErr := s.CreateNIC(mainNICId, tunEp); tErr != nil {
		cancel()
		return nil, utils.Errorf("create NIC failed: %v", tErr)
	}
	// Set NIC to promiscuous mode and spoofing mode to receive all packets and feedback them.
	s.SetPromiscuousMode(mainNICId, true)
	s.SetSpoofing(mainNICId, true)
	log.Infof("Setting up route table for NIC: %d", mainNICId)
	for _, ipAddr := range []net.IP{ip1Str, ip2Str} {
		s.AddProtocolAddress(mainNICId, tcpip.ProtocolAddress{
			Protocol: header.IPv4ProtocolNumber,
			AddressWithPrefix: tcpip.AddressWithPrefix{
				Address:   tcpip.AddrFrom4([4]byte(ipAddr.To4())),
				PrefixLen: 32,
			},
		}, stack.AddressProperties{})
	}

	s.SetRouteTable([]tcpip.Route{
		{
			Destination: header.IPv4EmptySubnet,
			NIC:         mainNICId,
			MTU:         uint32(TUN_MTU),
		},
		//{
		//	Destination: header.IPv6EmptySubnet,
		//	NIC:         mainNICId,
		//	MTU:         uint32(TUN_MTU),
		//},
	})

	tvm := &TunVirtualMachine{
		ctx:          baseCtx,
		cancel:       cancel,
		tunnelDevice: device,
		tunnelName:   utunName,
		tunEp:        tunEp,
		stack:        s,
		mainNicID:    mainNICId,
	}
	return tvm, nil
}

func (t *TunVirtualMachine) SetHijackTCPHandler(handle func(conn netstack.TCPConn)) error {
	tcpForwarder := tcp.NewForwarder(t.stack, defaultWndSize, maxConnAttempts, func(r *tcp.ForwarderRequest) {
		var (
			wq  waiter.Queue
			ep  tcpip.Endpoint
			err tcpip.Error
			id  = r.ID()
		)

		defer func() {
			if err != nil {
				log.Debugf("forward tcp request: %s:%d->%s:%d: %s",
					id.RemoteAddress, id.RemotePort, id.LocalAddress, id.LocalPort, err)
			}
		}()

		log.Infof("hijack tcp connection: %s:%d->%s:%d", id.RemoteAddress, id.RemotePort, id.LocalAddress, id.LocalPort)

		// Perform a TCP three-way handshake.
		ep, err = r.CreateEndpoint(&wq)
		if err != nil {
			log.Errorf("create endpoint failed: %v, reset it", err)
			// RST: prevent potential half-open TCP connection leak.
			r.Complete(true)
			return
		}
		defer r.Complete(false)

		log.Infof("start to set socket options: %s:%d->%s:%d", id.RemoteAddress, id.RemotePort, id.LocalAddress, id.LocalPort)
		err = setSocketOptions(t.stack, ep)

		log.Infof("start to create tcp connection instance for userland: %s:%d->%s:%d", id.RemoteAddress, id.RemotePort, id.LocalAddress, id.LocalPort)
		conn := &tcpConn{
			TCPConn: gonet.NewTCPConn(&wq, ep),
			id:      id,
		}
		handle(conn)
	})
	t.stack.SetTransportProtocolHandler(header.TCPProtocolNumber, tcpForwarder.HandlePacket)
	return nil
}

func (t *TunVirtualMachine) Close() error {
	t.stack.Close()
	return t.tunnelDevice.Close()
}

func (t *TunVirtualMachine) GetTunnelName() string {
	return t.tunnelName
}
