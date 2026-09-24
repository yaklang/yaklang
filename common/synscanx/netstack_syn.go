package synscanx

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netstackvm"
	"github.com/yaklang/yaklang/common/utils"
)

// halfOpenSYN is the TCP send path. Implementations must not complete the handshake.
type halfOpenSYN interface {
	Emit(ctx context.Context, host string, port int) error
	Close() error
	Wait(context.Context) error
}

func (s *Scannerx) notePorts(ports string) {
	s.keepPacketWriter = portsIncludeUDP(ports)
}

func portsIncludeUDP(ports string) bool {
	for _, port := range utils.ParseStringToPorts(ports) {
		proto, _ := utils.ParsePortToProtoPort(port)
		if proto == "udp" {
			return true
		}
	}
	return false
}

func (s *Scannerx) startHalfOpen(ctx context.Context) error {
	if s == nil || s.config == nil || s.config.Iface == nil {
		return fmt.Errorf("synscanx: interface is not selected")
	}
	session, err := netstackvm.OpenHalfOpenSYN(ctx, netstackvm.HalfOpenSYNConfig{
		Iface:    s.config.Iface,
		SourceIP: s.config.SourceIP,
		Gateway:  s.config.GatewayIP,
		OnResult: func(target string, _ netstackvm.TCPSegment, err error) {
			if err != nil && !errors.Is(err, netstackvm.ErrProbeNoResponse) && !errors.Is(err, netstackvm.ErrProbeRefused) && !errors.Is(err, context.Canceled) {
				log.Errorf("synscanx probe %s failed: %v", target, err)
			}
		},
		OnOpen: func(ip net.IP, port int) {
			if s.OpenPortHandlers != nil {
				s.OpenPortHandlers(ip, port)
			}
		},
	})
	if err != nil {
		return err
	}
	s.halfOpen = session
	log.Debugf("synscanx TCP uses netstackvm on %s", s.config.Iface.Name)
	return nil
}
