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
)

// halfOpenSYN is the only packet path. TCP calls HalfOpenSYN.ProbeSYN and UDP
// calls HalfOpenSYN.ProbeUDP. One call admits, sends, waits and releases.
// The scanner does not assemble frames or write them through a pcap queue.
type halfOpenSYN interface {
	Probe(context.Context, string) (netstackvm.TCPSegment, error)
	ProbeUDP(context.Context, string, []byte) (netstackvm.UDPSegment, error)
	Close() error
}

type netstackSYN struct {
	*netstackvm.HalfOpenSYN
	retry netstackvm.SYNRetryPolicy
}

func (h *netstackSYN) Probe(ctx context.Context, target string) (netstackvm.TCPSegment, error) {
	if h.retry == (netstackvm.SYNRetryPolicy{}) {
		return h.ProbeSYN(ctx, target)
	}
	return h.ProbeSYN(ctx, target, netstackvm.WithSYNRetry(h.retry))
}

// tcpRetryPolicy spreads a caller budget across three response waits at 1:2:4.
// The default policy stays in place when the caller did not set a budget.
func tcpRetryPolicy(budget time.Duration) netstackvm.SYNRetryPolicy {
	policy := netstackvm.DefaultSYNRetryPolicy()
	if budget <= 0 {
		return policy
	}
	unit := budget / 7
	if unit < 50*time.Millisecond {
		unit = 50 * time.Millisecond
	}
	maxWait := unit * 4
	if maxWait < unit {
		maxWait = unit
	}
	if maxWait > budget {
		maxWait = budget
	}
	policy.ResponseTimeout = unit
	policy.MaxResponseTimeout = maxWait
	return policy
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
	explicitConcurrency := false
	budget := 4 * time.Second
	if cfg != nil {
		explicitConcurrency = cfg.tcpProbeConcurrency > 0
		if explicitConcurrency {
			inFlight = cfg.tcpProbeConcurrency
		}
		if cfg.tcpProbeTimeout > 0 {
			budget = cfg.tcpProbeTimeout
		}
		if cfg.rateLimitDelayMs > 0 {
			gap := cfg.rateLimitDelayGap
			if gap < 1 {
				gap = 1
			}
			var rate float64
			if gap <= 1 {
				rate = 1000.0 / cfg.rateLimitDelayMs
			} else {
				// rateLimit(ms, count): count packets per ms milliseconds.
				rate = float64(gap) * 1000.0 / cfg.rateLimitDelayMs
			}
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
		}
	}
	// A filtered port holds its slot for the whole response budget. Size the
	// window from the packet rate unless the caller set concurrency explicitly.
	if !explicitConcurrency {
		seconds := int((budget + time.Second - 1) / time.Second)
		if seconds < 1 {
			seconds = 1
		}
		need := packetsPerSecond * seconds
		if need > inFlight {
			inFlight = need
		}
	}
	if inFlight > maxTCPProbeConcurrency {
		inFlight = maxTCPProbeConcurrency
	}
	if inFlight < 1 {
		inFlight = 1
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
		errors.Is(err, netstackvm.ErrUDPNoResponse) ||
		errors.Is(err, netstackvm.ErrUDPUnreachable) ||
		errors.Is(err, netstackvm.ErrProbeUnreachable) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

func (s *Scannerx) runProbes(targets <-chan *SynxTarget) {
	for {
		select {
		case <-s.ctx.Done():
			return
		case target, ok := <-targets:
			if !ok {
				return
			}
			if target != nil && target.Mode == UDP {
				s.finishUDPProbe(target)
				continue
			}
			s.finishTCPProbe(target)
		}
	}
}

func (s *Scannerx) finishTCPProbe(target *SynxTarget) {
	ack, err := s.probeTCP(s.ctx, target)
	if err != nil {
		if !quietProbeErr(err) {
			log.Debugf("synscanx probe %s:%d failed: %v", target.Host, target.Port, err)
		}
		return
	}
	// Scanner callbacks stay here. netstackvm only returns the segment.
	if s.ctx.Err() == nil && ack.RemoteIP != nil {
		s.reportOpen(ack.RemoteIP, int(ack.RemotePort), "tcp")
	}
}

func (s *Scannerx) probeUDP(ctx context.Context, target *SynxTarget) (netstackvm.UDPSegment, error) {
	if s == nil || s.halfOpen == nil || target == nil || target.Host == "" || target.Port < 1 || target.Port > 65535 {
		return netstackvm.UDPSegment{}, fmt.Errorf("synscanx: invalid UDP probe")
	}
	timeout := 5 * time.Second
	if s.config != nil && s.config.waiting > 0 {
		timeout = s.config.waiting
	}
	payload := udpProbePayload(target.Port)
	targetText := net.JoinHostPort(target.Host, strconv.Itoa(target.Port))
	var last netstackvm.UDPSegment
	var err error
	// One retry covers a single lost datagram. Each attempt gets the full wait
	// unless the parent context is already done.
	for attempt := 0; attempt < 2; attempt++ {
		if err = ctx.Err(); err != nil {
			return last, err
		}
		probeCtx, cancel := context.WithTimeout(ctx, timeout)
		last, err = s.halfOpen.ProbeUDP(probeCtx, targetText, payload)
		cancel()
		if !errors.Is(err, netstackvm.ErrUDPNoResponse) {
			return last, err
		}
	}
	return last, err
}

func (s *Scannerx) finishUDPProbe(target *SynxTarget) {
	reply, err := s.probeUDP(s.ctx, target)
	if err != nil {
		if !quietProbeErr(err) {
			log.Debugf("synscanx udp probe %s:%d failed: %v", target.Host, target.Port, err)
		}
		return
	}
	if s.ctx.Err() == nil && reply.RemoteIP != nil {
		s.reportOpen(reply.RemoteIP, int(reply.RemotePort), "udp")
	}
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
		MaxInFlight:      inFlight * 2,
		PacketsPerSecond: packetsPerSecond,
	})
	if err != nil {
		return err
	}
	wrapped := &netstackSYN{HalfOpenSYN: session}
	if s.config.tcpProbeTimeout > 0 {
		wrapped.retry = tcpRetryPolicy(s.config.tcpProbeTimeout)
	}
	s.halfOpen = wrapped
	log.Debugf("synscanx netstackvm probes on %s (%d in flight, %d pkt/s)", s.config.Iface.Name, inFlight, packetsPerSecond)
	return nil
}
