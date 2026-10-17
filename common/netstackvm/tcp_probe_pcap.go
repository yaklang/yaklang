package netstackvm

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/tcp"
)

type pcapProbeTransport struct {
	session     *HalfOpenSYN
	vm          *NetStackVirtualMachineEntry
	probe       *TCPProbe
	flight      *synFlight
	connected   bool // Serialized by TCPProbe.probeStep.
	frame       *synFrame
	writeMu     sync.Mutex // Close waits for an already executing native write.
	closeOnce   sync.Once
	expectedISN uint32
}

func (t *pcapProbeTransport) sendSYN(ctx context.Context) (TCPSegment, error) {
	if err := ctx.Err(); err != nil {
		return TCPSegment{}, err
	}
	if !t.connected {
		err := t.probe.ep.Connect(t.probe.remote)
		if _, ok := err.(*tcpip.ErrConnectStarted); !ok {
			return TCPSegment{}, fmt.Errorf("active open: %v", err)
		}
		seq, ok := t.probe.ep.(*tcp.Endpoint).SYNSequenceNumber()
		if !ok {
			return TCPSegment{}, fmt.Errorf("active SYN sequence unavailable")
		}
		t.expectedISN = seq
		t.connected = true
	}
	for t.frame == nil {
		select {
		case <-ctx.Done():
			return TCPSegment{}, ctx.Err()
		case candidate := <-t.flight.generated:
			if candidate.segment.Seq != t.expectedISN {
				continue
			}
			t.frame = candidate
			t.session.mu.Lock()
			t.flight.frame = candidate
			t.session.mu.Unlock()
		}
	}
	// Rate applies to every attempt. Burst=1 avoids a retry storm after a
	// batch of response timers expires together. Only the queue before the
	// first successful SYN may ignore the response budget. After that send,
	// token waits and writes share the caller's deadline, and a reply that
	// arrives while the retry is queued is delivered instead of another SYN.
	t.session.mu.Lock()
	alreadySent := t.flight.sent
	t.session.mu.Unlock()
	limitCtx := ctx
	writeCtx := ctx
	limitCancel := func() {}
	writeCancel := func() {}
	defer func() {
		limitCancel()
		writeCancel()
	}()
	if probeResponseBudget(ctx) > 0 && !alreadySent {
		writeBudget := 3 * time.Second
		if deadline, ok := ctx.Deadline(); ok {
			if remain := time.Until(deadline); remain > 0 {
				writeBudget = remain
			}
		}
		var stop context.CancelFunc
		limitCtx, stop = contextWithoutDeadline(ctx)
		limitCancel = stop
		if err := t.session.waitLimiter(limitCtx); err != nil {
			return TCPSegment{}, err
		}
		writeCtx, writeCancel = context.WithTimeout(limitCtx, writeBudget)
	} else if alreadySent {
		if err := t.waitRetryToken(ctx); err != nil {
			return TCPSegment{}, err
		}
	} else if err := t.session.waitLimiter(ctx); err != nil {
		return TCPSegment{}, err
	}
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	if err := writeCtx.Err(); err != nil {
		return TCPSegment{}, err
	}
	h, f := t.session, t.flight
	h.mu.Lock()
	if h.closed || f.ctx.Err() != nil || h.flights[f.key] != f {
		h.mu.Unlock()
		return TCPSegment{}, context.Canceled
	}
	f.permit = true
	f.sending = true
	h.mu.Unlock()
	var err error
	if t.vm.driver.readOnly.Load() {
		err = ErrPassiveNetwork
	} else {
		err = t.vm.driver.writeFrameContext(writeCtx, t.frame.data, t.frame.linkType)
	}
	h.mu.Lock()
	// A declined gate is not a successful injection (writeFrame is also used
	// for passive capture where a filtered write intentionally returns nil).
	if f.permit && err == nil {
		err = fmt.Errorf("SYN injection was declined")
	}
	f.permit = false
	f.sending = false
	if err == nil {
		f.sent = true
	} else if !f.sent {
		select {
		case <-f.replies:
		default:
		}
	}
	h.mu.Unlock()
	if err != nil {
		return TCPSegment{}, err
	}
	return cloneTCPSegment(t.frame.segment), nil
}

// errProbeResponseReady means a correlated reply arrived while this retry
// was waiting for a rate token. The reply stays queued for Receive.
var errProbeResponseReady = fmt.Errorf("tcp probe response arrived during retry")

// waitRetryToken stays inside the caller deadline. The shared limiter refuses
// a wait that would end after that deadline instead of blocking; this still
// watches for a reply until the deadline, and does not send another SYN.
func (t *pcapProbeTransport) waitRetryToken(ctx context.Context) error {
	if err := t.takeQueuedReply(); err != nil {
		return err
	}
	reservation := t.session.limiter.Reserve()
	if !reservation.OK() {
		return fmt.Errorf("SYN rate limiter rejected the retry")
	}
	delay := reservation.Delay()
	if deadline, ok := ctx.Deadline(); ok && delay > time.Until(deadline) {
		reservation.Cancel()
		log.Debugf("tcp probe retry will not send because the rate token is after the response budget")
		return t.waitForRetryResponse(ctx)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case reply := <-t.flight.replies:
		reservation.Cancel()
		offerProbeReply(t.flight.replies, reply)
		log.Debugf("tcp probe retry found a response while waiting for a rate token")
		return errProbeResponseReady
	case <-timer.C:
		if err := t.takeQueuedReply(); err != nil {
			return err
		}
		return nil
	case <-ctx.Done():
		reservation.Cancel()
		return t.replyOrContext(ctx)
	case <-t.session.ctx.Done():
		reservation.Cancel()
		return t.replyOrContext(t.session.ctx)
	}
}

func (t *pcapProbeTransport) waitForRetryResponse(ctx context.Context) error {
	select {
	case reply := <-t.flight.replies:
		offerProbeReply(t.flight.replies, reply)
		log.Debugf("tcp probe retry found a response while waiting for a rate token")
		return errProbeResponseReady
	case <-ctx.Done():
		return t.replyOrContext(ctx)
	case <-t.session.ctx.Done():
		return t.replyOrContext(t.session.ctx)
	}
}

func (t *pcapProbeTransport) takeQueuedReply() error {
	select {
	case reply := <-t.flight.replies:
		offerProbeReply(t.flight.replies, reply)
		log.Debugf("tcp probe retry found a response while waiting for a rate token")
		return errProbeResponseReady
	default:
		return nil
	}
}

func (t *pcapProbeTransport) replyOrContext(ctx context.Context) error {
	if err := t.takeQueuedReply(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return context.Cause(ctx)
	}
	return context.DeadlineExceeded
}

func (t *pcapProbeTransport) receive(ctx context.Context) (TCPSegment, []byte, error) {
	select {
	case <-ctx.Done():
		t.session.mu.Lock()
		badChecksum := t.flight.badChecksum
		t.session.mu.Unlock()
		if badChecksum {
			return TCPSegment{}, nil, fmt.Errorf("%w: %w", ctx.Err(), ErrProbeChecksum)
		}
		return TCPSegment{}, nil, ctx.Err()
	case r := <-t.flight.replies:
		if r.unreachable {
			return TCPSegment{}, nil, ErrProbeUnreachable
		}
		return r.segment, r.raw, nil
	}
}

func (t *pcapProbeTransport) close() error {
	t.closeOnce.Do(func() {
		t.writeMu.Lock()
		h := t.session
		h.mu.Lock()
		if h.flights[t.flight.key] == t.flight {
			delete(h.flights, t.flight.key)
		}
		h.mu.Unlock()
		t.probe.ep.Close()
		t.writeMu.Unlock()
		<-h.slots
		h.wg.Done()
	})
	return nil
}
