package netstackvm

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// One capture reader serves every subscriber of a device/configuration. Holding
// the registry lock across open/subscribe/unsubscribe prevents double readers
// and reopening a fanout while its last handle is being closed.
var fanouts = struct {
	sync.Mutex
	entries map[string]*pcapFanOut
}{entries: make(map[string]*pcapFanOut)}

type packetCaptureHandle interface {
	ReadPacketData() ([]byte, gopacket.CaptureInfo, error)
	WritePacketData([]byte) error
	LinkType() layers.LinkType
	Close()
}

type pcapFanOut struct {
	m           sync.Mutex
	writeGate   chan struct{}
	handle      packetCaptureHandle
	inject      func([]byte) error
	closeInject func()
	device      string
	chans       map[string]chan gopacket.Packet
	stop        chan struct{}
	done        chan struct{}
}

func NewPCAPAdaptor(device string, mtu int32, promisc bool) (*pcapAdaptor, error) {
	key := fmt.Sprintf("%s/%d/%t", device, mtu, promisc)
	fanouts.Lock()
	defer fanouts.Unlock()
	p := fanouts.entries[key]
	if p == nil {
		name, err := pcaputil.IfaceNameToPcapIfaceName(device)
		if err != nil {
			return nil, err
		}
		h, err := pcap.OpenLive(name, mtu, promisc, 100*time.Millisecond)
		if err != nil {
			return nil, err
		}
		capture := captureWithChecksumMetadata(name, int(mtu), promisc, h)
		p = &pcapFanOut{writeGate: make(chan struct{}, 1), handle: capture, device: name, chans: make(map[string]chan gopacket.Packet), stop: make(chan struct{}), done: make(chan struct{})}
		p.inject, p.closeInject = newPCAPLoopbackWriter(device, capture.LinkType(), capture.WritePacketData)
		fanouts.entries[key] = p
		go p.background()
	}
	id := ksuid.New().String()
	ch := make(chan gopacket.Packet, 1000)
	p.m.Lock()
	p.chans[id] = ch
	p.m.Unlock()
	broker := newPcapBroker(ch, func() {
		fanouts.Lock()
		defer fanouts.Unlock()
		p.m.Lock()
		delete(p.chans, id)
		close(ch)
		last := len(p.chans) == 0
		p.m.Unlock()
		if last {
			delete(fanouts.entries, key)
			close(p.stop)
			// ReadPacketData is bounded by the capture timeout; no orphan packet-source goroutine.
			<-p.done
			p.writeGate <- struct{}{}
			p.closeInject()
			p.handle.Close()
			p.handle = nil
			<-p.writeGate
		}
	}, p.WritePacket)
	broker.contextWriter = p.WritePacketContext
	broker.linkType = p.handle.LinkType()
	return broker, nil
}

func (p *pcapFanOut) WritePacket(data []byte) error {
	return p.WritePacketContext(context.Background(), data)
}

func (p *pcapFanOut) WritePacketContext(ctx context.Context, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case p.writeGate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-p.writeGate }()
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.handle == nil {
		return io.ErrClosedPipe
	}
	// libpcap has no cancellable injection call. The gate wait is cancellable;
	// an injection already executing must return before the handle is closed.
	if p.inject != nil {
		return p.inject(data)
	}
	return p.handle.WritePacketData(data)
}
func (p *pcapFanOut) background() {
	defer close(p.done)
	decoder := pcapCaptureDecoder(p.handle.LinkType())
	for {
		select {
		case <-p.stop:
			return
		default:
		}
		data, ci, err := p.handle.ReadPacketData()
		if err == pcap.NextErrorTimeoutExpired {
			continue
		}
		if err != nil {
			return
		}
		p.dispatch(data, ci, decoder)
	}
}

// gopacket does not register every platform DLT_RAW / LINKTYPE_IP variant.
func pcapCaptureDecoder(link layers.LinkType) gopacket.Decoder {
	switch link {
	case layers.LinkTypeIPv4:
		return layers.LayerTypeIPv4
	case layers.LinkTypeIPv6:
		return layers.LayerTypeIPv6
	case 12, 14:
		return layers.LinkTypeRaw
	default:
		return link
	}
}

func (p *pcapFanOut) dispatch(data []byte, ci gopacket.CaptureInfo, decoder gopacket.Decoder) {
	p.m.Lock()
	defer p.m.Unlock()
	for _, ch := range p.chans {
		// Each subscriber owns its decoded layers; bridge/filter mutation must not
		// race with or corrupt another consumer of the same captured packet.
		packet := gopacket.NewPacket(data, decoder, gopacket.Default)
		packet.Metadata().CaptureInfo = ci
		packet.Metadata().AncillaryData = append([]interface{}(nil), ci.AncillaryData...)
		packet.Metadata().Truncated = packet.Metadata().Truncated || ci.CaptureLength < ci.Length
		select {
		case ch <- packet:
		default:
		}
	}
}

type pcapAdaptor struct {
	linkType      layers.LinkType
	inChan        chan gopacket.Packet
	close         func()
	writer        func([]byte) error
	contextWriter func(context.Context, []byte) error
	writeGate     chan struct{}
	once          sync.Once
	closed        atomic.Bool
	ctx           context.Context
	cancel        context.CancelFunc
}

func newPcapBroker(in chan gopacket.Packet, closeFunc func(), writer func([]byte) error) *pcapAdaptor {
	ctx, cancel := context.WithCancel(context.Background())
	return &pcapAdaptor{inChan: in, close: closeFunc, writer: writer, ctx: ctx, cancel: cancel, writeGate: make(chan struct{}, 1)}
}
func (p *pcapAdaptor) PacketSource() chan gopacket.Packet { return p.inChan }
func (p *pcapAdaptor) WritePacketData(data []byte) error {
	return p.WritePacketDataContext(context.Background(), data)
}
func (p *pcapAdaptor) WritePacketDataContext(ctx context.Context, data []byte) error {
	if p.closed.Load() {
		return io.ErrClosedPipe
	}
	if p.ctx != nil {
		joined, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(p.ctx, cancel)
		defer func() { stop(); cancel() }()
		if p.ctx.Err() != nil {
			return io.ErrClosedPipe
		}
		ctx = joined
	}
	if p.writeGate != nil {
		select {
		case p.writeGate <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		defer func() { <-p.writeGate }()
	}
	if p.closed.Load() {
		return io.ErrClosedPipe
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.contextWriter != nil {
		return p.contextWriter(ctx, data)
	}
	if p.writer == nil {
		return io.ErrClosedPipe
	}
	return p.writer(data)
}
func (p *pcapAdaptor) Close() {
	p.once.Do(func() {
		p.closed.Store(true)
		if p.cancel != nil {
			p.cancel()
		}
		// Close must not return with this subscriber still inside a write,
		// even while other subscribers keep the shared handle alive.
		if p.writeGate != nil {
			p.writeGate <- struct{}{}
			defer func() { <-p.writeGate }()
		}
		if p.close != nil {
			p.close()
		}
	})
}
