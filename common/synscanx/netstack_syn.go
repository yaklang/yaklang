package synscanx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netstackvm"
	"github.com/yaklang/yaklang/common/utils"
)

// The scanner owns each probe and its context; netstackvm never calls back.
type tcpSYNProbe interface {
	ProbeSYNContext(context.Context) (netstackvm.TCPSegment, error)
	ReceiveSYNACKContext(context.Context) (netstackvm.TCPSegment, error)
	Close() error
}

type halfOpenSYN interface {
	startTCPProbe(context.Context, string) (tcpSYNProbe, error)
	Close() error
}

type netstackSYN struct{ *netstackvm.HalfOpenSYN }

func (h *netstackSYN) startTCPProbe(ctx context.Context, target string) (tcpSYNProbe, error) {
	return h.StartTCPProbe(ctx, target)
}

const defaultTCPProbeConcurrency = 256
const defaultTCPProbeTimeout = 15 * time.Second

// probeTCP returns the result directly and releases the per-target context and
// endpoint before returning. One target's deadline never cancels its siblings.
func (s *Scannerx) probeTCP(ctx context.Context, target *SynxTarget) (netstackvm.TCPSegment, error) {
	timeout := defaultTCPProbeTimeout
	if s.config != nil && s.config.tcpProbeTimeout > 0 {
		timeout = s.config.tcpProbeTimeout
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	probe, err := s.halfOpen.startTCPProbe(probeCtx, net.JoinHostPort(target.Host, strconv.Itoa(target.Port)))
	if err != nil {
		return netstackvm.TCPSegment{}, err
	}
	defer probe.Close()
	if _, err = probe.ProbeSYNContext(probeCtx); err != nil {
		return netstackvm.TCPSegment{}, err
	}
	return probe.ReceiveSYNACKContext(probeCtx)
}

func (s *Scannerx) runTCPProbes(targets <-chan *SynxTarget) {
	for {
		select {
		case <-s.ctx.Done():
			return
		case target, ok := <-targets:
			if !ok {
				return
			}
			ack, err := s.probeTCP(s.ctx, target)
			if err != nil {
				if !errors.Is(err, netstackvm.ErrProbeNoResponse) && !errors.Is(err, netstackvm.ErrProbeRefused) && !errors.Is(err, context.Canceled) {
					log.Debugf("synscanx probe %s:%d failed: %v", target.Host, target.Port, err)
				}
				continue
			}
			// Keep the existing scanner result API at this upper boundary. There
			// is no result callback or background batch worker inside netstackvm.
			if s.ctx.Err() == nil && s.OpenPortHandlers != nil {
				s.OpenPortHandlers(ack.RemoteIP, int(ack.RemotePort))
			}
		}
	}
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
	})
	if err != nil {
		return err
	}
	s.halfOpen = &netstackSYN{session}
	log.Debugf("synscanx TCP uses netstackvm on %s", s.config.Iface.Name)
	return nil
}
