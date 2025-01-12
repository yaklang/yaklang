package netstack

import (
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/adapters/gonet"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/tcp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/udp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/waiter"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/glog"
	"net"
	"time"
)

const (
	// defaultWndSize if set to zero, the default
	// receive window buffer size is used instead.
	defaultWndSize = 0

	// maxConnAttempts specifies the maximum number
	// of in-flight tcp connection attempts.
	maxConnAttempts = 2 << 10

	// tcpKeepaliveCount is the maximum number of
	// TCP keep-alive probes to send before giving up
	// and killing the connection if no response is
	// obtained from the other end.
	tcpKeepaliveCount = 9

	// tcpKeepaliveIdle specifies the time a connection
	// must remain idle before the first TCP keepalive
	// packet is sent. Once this time is reached,
	// tcpKeepaliveInterval option is used instead.
	tcpKeepaliveIdle = 60 * time.Second

	// tcpKeepaliveInterval specifies the interval
	// time between sending TCP keepalive packets.
	tcpKeepaliveInterval = 30 * time.Second
)

// TCPConn implements the net.Conn interface.
type TCPConn interface {
	net.Conn

	// ID returns the transport endpoint id of TCPConn.
	ID() *stack.TransportEndpointID
}

// UDPConn implements net.Conn and net.PacketConn.
type UDPConn interface {
	net.Conn
	net.PacketConn

	// ID returns the transport endpoint id of UDPConn.
	ID() *stack.TransportEndpointID
}

// TransportHandler is a TCP/UDP connection handler that implements
// HandleTCP and HandleUDP methods.
type TransportHandler interface {
	HandleTCP(TCPConn)
	HandleUDP(UDPConn)
}

func WithTCPHandler(handle func(TCPConn)) Option {
	return func(s *stack.Stack) error {
		tcpForwarder := tcp.NewForwarder(s, defaultWndSize, maxConnAttempts, func(r *tcp.ForwarderRequest) {
			var (
				wq  waiter.Queue
				ep  tcpip.Endpoint
				err tcpip.Error
				id  = r.ID()
			)

			defer func() {
				if err != nil {
					glog.Debugf("forward tcp request: %s:%d->%s:%d: %s",
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

			err = setSocketOptions(s, ep)

			conn := &tcpConn{
				TCPConn: gonet.NewTCPConn(&wq, ep),
				id:      id,
			}
			handle(conn)
		})
		s.SetTransportProtocolHandler(tcp.ProtocolNumber, tcpForwarder.HandlePacket)
		return nil
	}
}

func setSocketOptions(s *stack.Stack, ep tcpip.Endpoint) tcpip.Error {
	{ /* TCP keepalive options */
		ep.SocketOptions().SetKeepAlive(true)

		idle := tcpip.KeepaliveIdleOption(tcpKeepaliveIdle)
		if err := ep.SetSockOpt(&idle); err != nil {
			return err
		}

		interval := tcpip.KeepaliveIntervalOption(tcpKeepaliveInterval)
		if err := ep.SetSockOpt(&interval); err != nil {
			return err
		}

		if err := ep.SetSockOptInt(tcpip.KeepaliveCountOption, tcpKeepaliveCount); err != nil {
			return err
		}
	}
	{ /* TCP recv/send buffer size */
		var ss tcpip.TCPSendBufferSizeRangeOption
		if err := s.TransportProtocolOption(header.TCPProtocolNumber, &ss); err == nil {
			ep.SocketOptions().SetSendBufferSize(int64(ss.Default), false)
		}

		var rs tcpip.TCPReceiveBufferSizeRangeOption
		if err := s.TransportProtocolOption(header.TCPProtocolNumber, &rs); err == nil {
			ep.SocketOptions().SetReceiveBufferSize(int64(rs.Default), false)
		}
	}
	return nil
}

type tcpConn struct {
	*gonet.TCPConn
	id stack.TransportEndpointID
}

func (c *tcpConn) ID() *stack.TransportEndpointID {
	return &c.id
}

func WithUDPHandler(handle func(UDPConn)) Option {
	return func(s *stack.Stack) error {
		udpForwarder := udp.NewForwarder(s, func(r *udp.ForwarderRequest) {
			var (
				wq waiter.Queue
				id = r.ID()
			)
			ep, err := r.CreateEndpoint(&wq)
			if err != nil {
				glog.Debugf("forward udp request: %s:%d->%s:%d: %s",
					id.RemoteAddress, id.RemotePort, id.LocalAddress, id.LocalPort, err)
				return
			}

			conn := &udpConn{
				UDPConn: gonet.NewUDPConn(&wq, ep),
				id:      id,
			}
			handle(conn)
		})
		s.SetTransportProtocolHandler(udp.ProtocolNumber, udpForwarder.HandlePacket)
		return nil
	}
}

type udpConn struct {
	*gonet.UDPConn
	id stack.TransportEndpointID
}

func (c *udpConn) ID() *stack.TransportEndpointID {
	return &c.id
}
