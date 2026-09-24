package synscanx

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netstackvm"
	"github.com/yaklang/yaklang/common/utils"
)

// halfOpenSYN is the only TCP send path. Probe is HalfOpenSYN.ProbeSYN:
// one call admits, sends, retries, validates the reply and releases the probe.
// The scanner does not assemble SYNs or write them through the legacy pcap queue.
type halfOpenSYN interface {
	Probe(context.Context, string) (netstackvm.TCPSegment, error)
	Close() error
}

type netstackSYN struct{ *netstackvm.HalfOpenSYN }

func (h *netstackSYN) Probe(ctx context.Context, target string) (netstackvm.TCPSegment, error) {
	return h.ProbeSYN(ctx, target)
}

const (
	defaultTCPProbeConcurrency = 256
	defaultTCPProbeTimeout     = 15 * time.Second
	defaultSYNPacketsPerSecond = 1000
	maxTCPProbeConcurrency     = 4096
	maxSYNPacketsPerSecond     = 100000
)

// synPacketRate turns the Yak rateLimit/concurrent fields into the session's
// admission and SYN pace. Delay is the sustained interval; concurrent also
// sets how many probes may be outstanding. A non-positive delay keeps the
// session default of 1000 SYN/s. Burst stays 1 inside netstackvm so retries
// cannot stampede.
func synPacketRate(cfg *SynxConfig) (packetsPerSecond, inFlight int) {
	packetsPerSecond = defaultSYNPacketsPerSecond
	inFlight = defaultTCPProbeConcurrency
	if cfg == nil {
		return
	}
	if cfg.tcpProbeConcurrency > 0 {
		inFlight = cfg.tcpProbeConcurrency
	}
	if inFlight > maxTCPProbeConcurrency {
		inFlight = maxTCPProbeConcurrency
	}
	if inFlight < 1 {
		inFlight = 1
	}
	if cfg.rateLimitDelayMs <= 0 {
		return
	}
	rate := 1000.0 / cfg.rateLimitDelayMs
	switch {
	case rate < 1:
		packetsPerSecond = 1
	case rate > maxSYNPacketsPerSecond:
		packetsPerSecond = maxSYNPacketsPerSecond
	default:
		packetsPerSecond = int(math.Round(rate))
		if packetsPerSecond < 1 {
			packetsPerSecond = 1
		}
	}
	return
}

// probeTCP gives the target its own deadline and returns only after ProbeSYN
// has released that probe. One target's deadline does not cancel its siblings.
func (s *Scannerx) probeTCP(ctx context.Context, target *SynxTarget) (netstackvm.TCPSegment, error) {
	if s == nil || s.halfOpen == nil || target == nil || target.Host == "" || target.Port < 1 || target.Port > 65535 {
		return netstackvm.TCPSegment{}, fmt.Errorf("synscanx: invalid TCP probe")
	}
	timeout := defaultTCPProbeTimeout
	if s.config != nil && s.config.tcpProbeTimeout > 0 {
		timeout = s.config.tcpProbeTimeout
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return s.halfOpen.Probe(probeCtx, net.JoinHostPort(target.Host, strconv.Itoa(target.Port)))
}

func quietProbeErr(err error) bool {
	return err == nil ||
		errors.Is(err, netstackvm.ErrProbeNoResponse) ||
		errors.Is(err, netstackvm.ErrProbeRefused) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
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
				if !quietProbeErr(err) {
					log.Debugf("synscanx probe %s:%d failed: %v", target.Host, target.Port, err)
				}
				continue
			}
			// Scanner callbacks stay here. netstackvm only returns the segment.
			if s.ctx.Err() == nil && s.OpenPortHandlers != nil && ack.RemoteIP != nil {
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
	packetsPerSecond, inFlight := synPacketRate(s.config)
	session, err := netstackvm.OpenHalfOpenSYN(ctx, netstackvm.HalfOpenSYNConfig{
		Iface:            s.config.Iface,
		SourceIP:         s.config.SourceIP,
		Gateway:          s.config.GatewayIP,
		MaxInFlight:      inFlight,
		PacketsPerSecond: packetsPerSecond,
	})
	if err != nil {
		return err
	}
	s.halfOpen = &netstackSYN{session}
	log.Debugf("synscanx TCP ProbeSYN on %s (%d in flight, %d SYN/s)", s.config.Iface.Name, inFlight, packetsPerSecond)
	return nil
}
