package netstackvm

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/tcp"
)

var (
	// ErrProbeNoResponse is inconclusive: loss/filtering is not a closed port.
	ErrProbeNoResponse = errors.New("TCP probe received no valid response")
	ErrProbeRefused    = errors.New("TCP probe received a matching reset")
	ErrProbeSend       = errors.New("TCP probe could not send SYN")
	ErrHalfOpenACK     = errors.New("ACK is disabled for a half-open probe")
)

// SYNRetryPolicy bounds the whole probe, including failed local sends.
// MaxAttempts includes the first SYN. Retries replay the same tuple and ISN.
// ResponseTimeout doubles after each attempt up to MaxResponseTimeout;
// jitter adds 0..20 percent without exceeding that cap. SendTimeout bounds
// queue/neighbor-resolution waits. A caller/session context always wins.
type SYNRetryPolicy struct {
	MaxAttempts        int
	SendTimeout        time.Duration
	ResponseTimeout    time.Duration
	MaxResponseTimeout time.Duration
}

func DefaultSYNRetryPolicy() SYNRetryPolicy {
	return SYNRetryPolicy{3, 3 * time.Second, 500 * time.Millisecond, 2 * time.Second}
}

func (r SYNRetryPolicy) validate() error {
	if r.MaxAttempts < 1 || r.MaxAttempts > 10 || r.SendTimeout <= 0 || r.ResponseTimeout <= 0 || r.MaxResponseTimeout < r.ResponseTimeout {
		return fmt.Errorf("invalid SYN retry policy: attempts must be 1..10 and timeouts positive, with max >= initial")
	}
	return nil
}

func (r SYNRetryPolicy) delay(attempt int, jitter float64) time.Duration {
	d := r.ResponseTimeout
	for i := 1; i < attempt && d < r.MaxResponseTimeout; i++ {
		if d > r.MaxResponseTimeout/2 {
			d = r.MaxResponseTimeout
		} else {
			d *= 2
		}
	}
	extra := time.Duration(float64(d) * 0.2 * jitter)
	if extra > r.MaxResponseTimeout-d {
		return r.MaxResponseTimeout
	}
	return d + extra
}

type TCPProbeOption func(*SYNRetryPolicy)

// WithSYNRetry configures the total attempt budget, not retries in addition to it.
func WithSYNRetry(policy SYNRetryPolicy) TCPProbeOption {
	return func(r *SYNRetryPolicy) { *r = policy }
}

type tcpProbeTransport interface {
	sendSYN(context.Context) (TCPSegment, error)
	receive(context.Context) (TCPSegment, []byte, error)
	close() error
}

// channelProbeTransport preserves the in-memory stepped handshake backend.
// The two channel VMs must be dedicated to this probe (no competing readers).
type channelProbeTransport struct {
	p           *TCPProbe
	connected   bool
	raw         []byte
	syn         TCPSegment
	expectedISN uint32
}

func (t *channelProbeTransport) sendSYN(ctx context.Context) (TCPSegment, error) {
	if err := ctx.Err(); err != nil {
		return TCPSegment{}, context.Cause(ctx)
	}
	if !t.connected {
		err := t.p.ep.Connect(t.p.remote)
		if _, ok := err.(*tcpip.ErrConnectStarted); !ok {
			return TCPSegment{}, fmt.Errorf("active open: %v", err)
		}
		seq, ok := t.p.ep.(*tcp.Endpoint).SYNSequenceNumber()
		if !ok {
			return TCPSegment{}, fmt.Errorf("active SYN sequence unavailable")
		}
		t.expectedISN = seq
		t.connected = true
	}
	for t.raw == nil {
		seg, raw, err := t.p.readTCP(ctx, t.p.link, &t.p.clientStash, false, "SYN")
		if err != nil {
			return TCPSegment{}, err
		}
		if !seg.SYN || seg.ACK || seg.FIN || seg.RST || seg.Seq != t.expectedISN || seg.RemotePort != t.p.remote.Port || !sameIPv4(seg.RemoteIP, t.p.remote.Addr.AsSlice()) {
			continue
		}
		t.syn, t.raw = seg, raw
	}
	if err := ctx.Err(); err != nil {
		return TCPSegment{}, context.Cause(ctx)
	}
	injectIPv4(t.p.peerLink, t.raw)
	return cloneTCPSegment(t.syn), nil
}
func (t *channelProbeTransport) receive(ctx context.Context) (TCPSegment, []byte, error) {
	return t.p.readTCP(ctx, t.p.peerLink, &t.p.peerStash, true, "SYN-ACK")
}
func (t *channelProbeTransport) close() error { t.p.ep.Close(); return nil }

// probeContext joins the step context to the probe lifetime without a waiting
// goroutine per operation. It preserves a caller deadline on queue waits too.
func (p *TCPProbe) probeContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	joined, cancel := context.WithCancelCause(ctx)
	stop := context.AfterFunc(p.ctx, func() { cancel(context.Cause(p.ctx)) })
	if p.ctx.Err() != nil {
		cancel(context.Cause(p.ctx))
	}
	return joined, func() { stop(); cancel(context.Canceled) }
}

func (p *TCPProbe) acquireProbeStep(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return context.Cause(ctx)
	}
	select {
	case p.probeStep <- struct{}{}:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

// ProbeSYNContext waits for a confirmed local write. Success means the writer
// accepted the SYN, not that a NIC/remote host received it. Local send errors
// consume the same budget as response-timeout retries.
func (p *TCPProbe) ProbeSYNContext(ctx context.Context) (TCPSegment, error) {
	ctx, cancel := p.probeContext(ctx)
	defer cancel()
	if err := p.acquireProbeStep(ctx); err != nil {
		return TCPSegment{}, err
	}
	defer func() { <-p.probeStep }()
	p.mu.Lock()
	err := p.beginStep("ProbeSYN", phaseInit)
	p.mu.Unlock()
	if err != nil {
		return TCPSegment{}, err
	}
	seg, err := p.sendWithRetry(ctx)
	if err != nil {
		return TCPSegment{}, err
	}
	p.mu.Lock()
	p.syn = cloneTCPSegment(seg)
	p.phase = phaseSynSent
	p.mu.Unlock()
	return seg, nil
}

func (p *TCPProbe) sendWithRetry(ctx context.Context) (TCPSegment, error) {
	last := errors.New("attempt budget exhausted")
	for p.attempts < p.retry.MaxAttempts {
		if err := ctx.Err(); err != nil {
			return TCPSegment{}, context.Cause(ctx)
		}
		p.attempts++
		sendCtx, cancel := context.WithTimeout(ctx, p.retry.SendTimeout)
		seg, err := p.transport.sendSYN(sendCtx)
		cancel()
		if ctx.Err() != nil {
			return TCPSegment{}, context.Cause(ctx)
		}
		if err == nil {
			return seg, nil
		}
		last = err
		if p.attempts < p.retry.MaxAttempts {
			if err := waitProbeDelay(ctx, p.retry.delay(p.attempts, rand.Float64())); err != nil {
				return TCPSegment{}, err
			}
		}
	}
	return TCPSegment{}, fmt.Errorf("%w after %d attempts: %w", ErrProbeSend, p.attempts, last)
}

// ReceiveSYNACKContext never injects the response into the TCP stack. It
// validates the tuple, flags and ACK before reporting open, retries on loss,
// and returns ErrProbeRefused only for a correlated RST+ACK. A timeout is
// ErrProbeNoResponse, never proof that a port is closed.
func (p *TCPProbe) ReceiveSYNACKContext(ctx context.Context) (TCPSegment, error) {
	ctx, cancel := p.probeContext(ctx)
	defer cancel()
	if err := p.acquireProbeStep(ctx); err != nil {
		return TCPSegment{}, err
	}
	defer func() { <-p.probeStep }()
	p.mu.Lock()
	err := p.beginStep("ReceiveSYNACK", phaseSynSent)
	syn := p.syn
	p.mu.Unlock()
	if err != nil {
		return TCPSegment{}, err
	}
	for {
		rxCtx, rxCancel := context.WithTimeout(ctx, p.retry.delay(p.attempts, rand.Float64()))
		var seg TCPSegment
		var raw []byte
		for {
			if err = rxCtx.Err(); err != nil {
				break
			}
			seg, raw, err = p.transport.receive(rxCtx)
			if err != nil {
				break
			}
			if validProbeReply(syn, seg) {
				break
			}
		}
		rxCancel()
		if ctx.Err() != nil {
			return TCPSegment{}, context.Cause(ctx)
		}
		if err == nil {
			if seg.RST {
				return TCPSegment{}, ErrProbeRefused
			}
			p.mu.Lock()
			p.synAck = cloneTCPSegment(seg)
			p.heldSynAck = append([]byte(nil), raw...)
			p.phase = phaseSynAckSeen
			p.mu.Unlock()
			return seg, nil
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			return TCPSegment{}, err
		}
		if p.attempts >= p.retry.MaxAttempts {
			return TCPSegment{}, fmt.Errorf("%w after %d attempts", ErrProbeNoResponse, p.attempts)
		}
		if _, err = p.sendWithRetry(ctx); err != nil {
			return TCPSegment{}, err
		}
	}
}

func validProbeReply(syn, reply TCPSegment) bool {
	if !sameIPv4(syn.LocalIP, reply.LocalIP) || !sameIPv4(syn.RemoteIP, reply.RemoteIP) || syn.LocalPort != reply.LocalPort || syn.RemotePort != reply.RemotePort || !reply.ACK || reply.Ack != syn.Seq+1 || reply.FIN || reply.PSH || reply.URG || reply.PayloadLen != 0 {
		return false
	}
	return (reply.SYN && !reply.RST) || (reply.RST && !reply.SYN)
}

func waitProbeDelay(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-timer.C:
		return nil
	}
}

func cloneTCPSegment(s TCPSegment) TCPSegment {
	s.LocalIP = ipv4Copy(s.LocalIP)
	s.RemoteIP = ipv4Copy(s.RemoteIP)
	return s
}
