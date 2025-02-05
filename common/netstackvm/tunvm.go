package netstackvm

import (
	"context"
	"fmt"
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
	"net"
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

	device, err := tun.CreateTUN(utunName, TUN_MTU)
	if err != nil {
		return nil, utils.Errorf("tun.CreateTUN failed: %v", err)
	}

	baseCtx, cancel := context.WithCancel(ctx)

	mtu := uint32(TUN_MTU)
	offset := 4
	tunEp, err := rwendpoint.NewReadWriteCloserEndpointContext(
		ctx, rwendpoint.NewWireGuardReadWriteCloserWrapper(device, mtu, offset),
		uint32(TUN_MTU),
		offset,
	)
	if err != nil {
		cancel()
		return nil, utils.Errorf("create tun endpoint failed: %v", err)
	}

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
	s.SetPromiscuousMode(mainNICId, true)
	s.SetRouteTable([]tcpip.Route{
		{
			Destination: header.IPv4EmptySubnet,
			NIC:         mainNICId,
			MTU:         uint32(TUN_MTU),
		},
		{
			Destination: header.IPv6EmptySubnet,
			NIC:         mainNICId,
			MTU:         uint32(TUN_MTU),
		},
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

		// Perform a TCP three-way handshake.
		ep, err = r.CreateEndpoint(&wq)
		if err != nil {
			// RST: prevent potential half-open TCP connection leak.
			r.Complete(true)
			return
		}
		defer r.Complete(false)

		err = setSocketOptions(t.stack, ep)

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
