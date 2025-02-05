package netstackvm

import (
	"fmt"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/adapters/gonet"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/ipv4"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/ipv6"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/tcp"
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

func defaultInitNetStack(s *stack.Stack) error {
	opt := tcpip.DefaultTTLOption(defaultTimeToLive)
	if err := s.SetNetworkProtocolOption(ipv4.ProtocolNumber, &opt); err != nil {
		return fmt.Errorf("set ipv4 default TTL: %s", err)
	}
	if err := s.SetNetworkProtocolOption(ipv6.ProtocolNumber, &opt); err != nil {
		return fmt.Errorf("set ipv6 default TTL: %s", err)
	}

	if err := s.SetForwardingDefaultAndAllNICs(ipv4.ProtocolNumber, true); err != nil {
		return fmt.Errorf("set ipv4 forwarding: %s", err)
	}
	if err := s.SetForwardingDefaultAndAllNICs(ipv6.ProtocolNumber, true); err != nil {
		return fmt.Errorf("set ipv6 forwarding: %s", err)
	}
	s.SetICMPBurst(icmpBurst)
	s.SetICMPLimit(icmpLimit)
	sndOpt := tcpip.TCPSendBufferSizeRangeOption{Min: tcpMinBufferSize, Default: tcpDefaultSendBufferSize, Max: tcpMaxBufferSize}
	if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &sndOpt); err != nil {
		return fmt.Errorf("set TCP send buffer size range: %s", err)
	}
	rcvOpt := tcpip.TCPReceiveBufferSizeRangeOption{Min: tcpMinBufferSize, Default: tcpDefaultReceiveBufferSize, Max: tcpMaxBufferSize}
	if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &rcvOpt); err != nil {
		return fmt.Errorf("set TCP receive buffer size range: %s", err)
	}
	tcpOpt := tcpip.CongestionControlOption(tcpCongestionControlAlgorithm)
	if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &tcpOpt); err != nil {
		return fmt.Errorf("set TCP congestion control algorithm: %s", err)
	}
	delayOpt := tcpip.TCPDelayEnabled(tcpDelayEnabled)
	if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &delayOpt); err != nil {
		return fmt.Errorf("set TCP delay enabled: %s", err)
	}
	tcpModerateReceiveBufferOpt := tcpip.TCPModerateReceiveBufferOption(tcpModerateReceiveBufferEnabled)
	if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &tcpModerateReceiveBufferOpt); err != nil {
		return fmt.Errorf("set TCP moderate receive buffer: %s", err)
	}

	sackOpt := tcpip.TCPSACKEnabled(tcpSACKEnabled)
	if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &sackOpt); err != nil {
		return fmt.Errorf("set TCP SACK enabled: %s", err)
	}
	recoveryOpt := tcpRecovery
	if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &recoveryOpt); err != nil {
		return fmt.Errorf("set TCP Recovery: %s", err)
	}

	return nil
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
