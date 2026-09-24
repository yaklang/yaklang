package netstackvm

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/tcp"
	"sync"

	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
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
	// Rate applies to all attempts, not just new targets. Burst=1 avoids a
	// retry storm after a batch of response timers expires together.
	if err := t.session.limiter.Wait(ctx); err != nil {
		return TCPSegment{}, err
	}
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	if err := ctx.Err(); err != nil {
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
		err = t.vm.driver.writeFrameContext(ctx, t.frame.data, t.frame.linkType)
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

func (t *pcapProbeTransport) receive(ctx context.Context) (TCPSegment, []byte, error) {
	select {
	case <-ctx.Done():
		return TCPSegment{}, nil, ctx.Err()
	case r := <-t.flight.replies:
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
