package netstackvm

import (
	"context"
	"fmt"
	"io"
	"sync"
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

type pcapFanOut struct {
	m         sync.Mutex
	writeGate chan struct{}
	handle    *pcap.Handle
	device    string
	chans     map[string]chan gopacket.Packet
	stop      chan struct{}
	done      chan struct{}
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
		p = &pcapFanOut{writeGate: make(chan struct{}, 1), handle: h, device: name, chans: make(map[string]chan gopacket.Packet), stop: make(chan struct{}), done: make(chan struct{})}
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
	return p.handle.WritePacketData(data)
}
func (p *pcapFanOut) background() {
	defer close(p.done)
	linkType := p.handle.LinkType()
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
		p.dispatch(data, ci, linkType)
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
	once          sync.Once
}

func newPcapBroker(in chan gopacket.Packet, closeFunc func(), writer func([]byte) error) *pcapAdaptor {
	return &pcapAdaptor{inChan: in, close: closeFunc, writer: writer}
}
func (p *pcapAdaptor) PacketSource() chan gopacket.Packet { return p.inChan }
func (p *pcapAdaptor) WritePacketData(data []byte) error {
	return p.WritePacketDataContext(context.Background(), data)
}
func (p *pcapAdaptor) WritePacketDataContext(ctx context.Context, data []byte) error {
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
		if p.close != nil {
			p.close()
		}
	})
}
