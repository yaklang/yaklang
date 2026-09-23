package netstackvm

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/adapters/gonet"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/link/channel"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/tcp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/udp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/waiter"
)

type sharedGateway struct {
	ctx    context.Context
	stack  *stack.Stack
	link   *channel.Endpoint
	dial   func(context.Context, string, string) (net.Conn, error)
	mu     sync.Mutex
	closed bool
	conns  map[net.Conn]struct{}
	wg     sync.WaitGroup
	slots  chan struct{}
}

func (v *NetworkVM) startShared(dial func(context.Context, string, string) (net.Conn, error)) error {
	s, err := NewNetStackFromConfig(NewDefaultConfig())
	if err != nil {
		return err
	}
	ep := channel.New(1024, 1500, "")
	g := &sharedGateway{ctx: v.ctx, stack: s, link: ep, dial: dial, conns: make(map[net.Conn]struct{}), slots: make(chan struct{}, 256)}
	if dial == nil {
		g.dial = (&net.Dialer{Timeout: 10 * time.Second}).DialContext
	}
	v.shared = g
	nic := s.NextNICID()
	if e := s.CreateNIC(nic, ep); e != nil {
		return fmt.Errorf("create shared gateway: %s", e)
	}
	s.SetPromiscuousMode(nic, true)
	s.SetSpoofing(nic, true)
	s.SetRouteTable([]tcpip.Route{{Destination: header.IPv4EmptySubnet, NIC: nic}})
	tf := tcp.NewForwarder(s, 0, 256, func(r *tcp.ForwarderRequest) {
		if !g.begin() {
			r.Complete(true)
			return
		}
		defer g.end()
		id := r.ID()
		upstream, e := g.dial(g.ctx, "tcp", net.JoinHostPort(id.LocalAddress.String(), strconv.Itoa(int(id.LocalPort))))
		if e != nil {
			r.Complete(true)
			return
		}
		if !g.track(upstream) {
			r.Complete(true)
			return
		}
		defer g.untrack(upstream)
		var wq waiter.Queue
		endpoint, te := r.CreateEndpoint(&wq)
		if te != nil {
			r.Complete(true)
			return
		}
		r.Complete(false)
		conn := gonet.NewTCPConn(&wq, endpoint)
		if !g.track(conn) {
			return
		}
		defer g.untrack(conn)
		relayTCP(conn, upstream)
	})
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, tf.HandlePacket)
	uf := udp.NewForwarder(s, func(r *udp.ForwarderRequest) {
		defer r.Release()
		if !g.begin() {
			return
		}
		var wq waiter.Queue
		endpoint, e := r.CreateEndpoint(&wq)
		if e != nil {
			g.end()
			return
		}
		conn := gonet.NewUDPConn(&wq, endpoint)
		if !g.track(conn) {
			g.end()
			return
		}
		id := r.ID()
		go func() {
			defer g.end()
			defer g.untrack(conn)
			upstream, e := g.dial(g.ctx, "udp", net.JoinHostPort(id.LocalAddress.String(), strconv.Itoa(int(id.LocalPort))))
			if e != nil {
				return
			}
			if !g.track(upstream) {
				return
			}
			defer g.untrack(upstream)
			// Each read is a datagram. Never use io.Copy's stream-sized buffers for UDP.
			done := make(chan struct{})
			go func() { defer close(done); relayDatagrams(upstream, conn); conn.Close() }()
			relayDatagrams(conn, upstream)
			upstream.Close()
			<-done
		}()
	})
	s.SetTransportProtocolHandler(udp.ProtocolNumber, uf.HandlePacket)
	v.wg.Add(2)
	go func() { defer v.wg.Done(); forwardChannel(v.ctx, v.packets, ep) }()
	go func() { defer v.wg.Done(); forwardChannel(v.ctx, ep, v.packets) }()
	return nil
}

func (g *sharedGateway) begin() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return false
	}
	select {
	case g.slots <- struct{}{}:
		g.wg.Add(1)
		return true
	default:
		return false
	}
}
func (g *sharedGateway) end() { <-g.slots; g.wg.Done() }
func (g *sharedGateway) track(c net.Conn) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		c.Close()
		return false
	}
	g.conns[c] = struct{}{}
	return true
}
func (g *sharedGateway) untrack(c net.Conn) {
	c.Close()
	g.mu.Lock()
	delete(g.conns, c)
	g.mu.Unlock()
}
func (g *sharedGateway) close() {
	g.mu.Lock()
	g.closed = true
	for c := range g.conns {
		c.Close()
	}
	g.mu.Unlock()
	g.stack.Close()
	g.link.Close()
	g.wg.Wait()
	g.stack.Wait()
}
func relayTCP(a, b net.Conn) {
	done := make(chan struct{})
	copyHalf := func(dst, src net.Conn) {
		_, err := io.Copy(dst, src)
		if err != nil {
			dst.Close()
			src.Close()
			return
		}
		if c, ok := dst.(interface{ CloseWrite() error }); ok {
			c.CloseWrite()
		} else {
			dst.Close()
		}
	}
	go func() { defer close(done); copyHalf(a, b) }()
	copyHalf(b, a)
	<-done
}
func relayDatagrams(dst, src net.Conn) {
	buf := make([]byte, 65535)
	for {
		src.SetReadDeadline(time.Now().Add(30 * time.Second))
		n, e := src.Read(buf)
		if e != nil {
			return
		}
		if _, e = dst.Write(buf[:n]); e != nil {
			return
		}
	}
}
