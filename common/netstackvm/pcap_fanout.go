// Package netstackvm implements network stack virtualization functionality

package netstackvm

import (
	"context"
	"sync"

	"github.com/gopacket/gopacket"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils"
)

// pcapFanOut represents a packet capture fan-out instance that can distribute packets to multiple consumers
type pcapFanOut struct {
	m                   sync.Mutex                      // Protects access to chans map
	wm                  sync.Mutex                      // Protects packet writing
	backgroundInitMutex sync.Mutex                      // Protects background goroutine initialization
	backgroundCtx       context.Context                 // Context for background packet processing
	backgroundCancel    context.CancelFunc              // Function to cancel background processing
	handle              *pcap.Handle                    // Underlying pcap handle
	device              string                          // Network interface device name
	chans               map[string]chan gopacket.Packet // Map of channels for packet distribution
}

var (
	_salt = "pcapFanOut"  // Salt used for hash generation
	fomap = new(sync.Map) // Global map to store pcapFanOut instances
)

// NewPCAPAdaptor creates a new pcap adaptor for the given interface.
// It reuses existing adaptors if one already exists for the interface.
func NewPCAPAdaptor(ifaceName string, promisc bool) (*pcapAdaptor, error) {
	// Generate unique hash for this interface configuration
	hash := utils.CalcSha256(ifaceName, promisc, _salt)

	// Check if adaptor already exists
	if v, ok := fomap.Load(hash); ok {
		fanOut, ok := v.(*pcapFanOut)
		if !ok {
			return nil, utils.Error("BUG: pcapFanout TypeAssert failed")
		}
		return fanOut.CreatePCAPAdaptor()
	}

	// Create new fanout instance if none exists
	ins, err := newPCAPFanOuter(ifaceName, promisc)
	if err != nil {
		return nil, utils.Errorf("create pcap fanout failed: %v", err)
	}
	fomap.Store(hash, ins)
	return ins.CreatePCAPAdaptor()
}

// newPCAPFanOuter creates a new pcapFanOut instance for packet capture
func newPCAPFanOuter(device string, promic bool) (*pcapFanOut, error) {
	name, err := pcaputil.IfaceNameToPcapIfaceName(device)
	if err != nil {
		return nil, utils.Errorf("failed to get pcap interface name (%v): %v", device, err)
	}

	// Open pcap handle with large snaplen to avoid truncation
	handle, err := pcap.OpenLive(name, 0, promic, pcap.BlockForever) // no snaplen limit
	if err != nil {
		return nil, utils.Errorf("failed to open pcap handle: %v", err)
	}

	return &pcapFanOut{
		handle: handle,
		device: name,
		chans:  map[string]chan gopacket.Packet{},
	}, nil
}

// WritePacket writes a packet to the pcap handle
func (p *pcapFanOut) WritePacket(data []byte) error {
	if p.handle == nil {
		return utils.Errorf("pcap handle is nil on: %v", p.device)
	}
	p.wm.Lock()
	defer p.wm.Unlock()
	return p.handle.WritePacketData(data)
}

// background runs the packet capture loop and distributes packets to consumers
func (p *pcapFanOut) background(notify chan error) error {
	p.backgroundInitMutex.Lock()
	if p.handle == nil {
		log.Errorf("BUG: pcap handle is nil on: %v", p.device)
		p.backgroundInitMutex.Unlock()
		err := utils.Errorf("BUG: pcap handle is nil on: %v", p.device)
		notify <- err
		return err
	}

	// Setup background context and packet source
	p.backgroundCtx, p.backgroundCancel = context.WithCancel(context.Background())
	handle := p.handle
	pcapChan := gopacket.NewPacketSource(handle, handle.LinkType()).Packets()
	p.backgroundInitMutex.Unlock()
	notify <- nil

	// Main packet processing loop
	lastLogTime := make(map[string]int64)
	droppedPackets := make(map[string]int64)
	for {
		select {
		case packet, ok := <-pcapChan:
			if !ok {
				log.Errorf("pcap channel is closed on: %v", p.device)
				return nil
			}
			// Distribute packet to all consumers
			p.m.Lock()
			for id, ch := range p.chans {
				select {
				case ch <- packet:
				default:
					now := utils.TimestampMs()
					droppedPackets[id]++
					if last, ok := lastLogTime[id]; !ok || now-last > 3000 {
						log.Errorf("pcap fanout channel is full on: adaptor Id: %v, dropped packets: %d", id, droppedPackets[id])
						lastLogTime[id] = now
					}
				}
			}
			p.m.Unlock()
		}
	}
}

// CreatePCAPAdaptor creates a new pcap adaptor instance
func (p *pcapFanOut) CreatePCAPAdaptor() (*pcapAdaptor, error) {
	p.m.Lock()
	defer p.m.Unlock()

	// Create channel for this adaptor
	insChan := make(chan gopacket.Packet, 1000)
	p.chans = make(map[string]chan gopacket.Packet)
	id := ksuid.New().String()
	p.chans[id] = insChan

	// Start background processing if this is the first adaptor
	if len(p.chans) == 1 {
		notify := make(chan error)
		defer func() {
			close(notify)
		}()
		go func() {
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("pcap fanout panic: %v", err)
				}
			}()
			_ = p.background(notify)
		}()
		err := <-notify
		if err != nil {
			return nil, err
		}
	}

	broker := newPcapBroker(insChan, func() {
		p.ClosePCAPAdaptor(id)
	}, p.WritePacket)
	return broker, nil
}

// ClosePCAPAdaptor closes a pcap adaptor and cleans up resources
func (p *pcapFanOut) ClosePCAPAdaptor(id string) {
	p.m.Lock()
	defer p.m.Unlock()

	if ch, ok := p.chans[id]; ok {
		close(ch)
		delete(p.chans, id)
	}

	// Clean up background processing if no more adaptors
	if len(p.chans) == 0 {
		p.backgroundInitMutex.Lock()
		if p.backgroundCancel != nil {
			p.backgroundCancel()
		}
		p.backgroundCtx = nil
		p.backgroundCancel = nil
		p.backgroundInitMutex.Unlock()
	}
}

// pcapAdaptor represents a single consumer of packet data
type pcapAdaptor struct {
	m      sync.Mutex           // Protects access to adaptor
	inChan chan gopacket.Packet // Channel for receiving packets
	close  func()               // Function to call on close
	writer func([]byte) error   // Function to write packets
}

// newPcapBroker creates a new pcap adaptor instance
func newPcapBroker(in chan gopacket.Packet, closeFunc func(), writer func([]byte) error) *pcapAdaptor {
	return &pcapAdaptor{
		inChan: in,
		close:  closeFunc,
		writer: writer,
	}
}

// PacketSource returns the channel for receiving packets
func (p *pcapAdaptor) PacketSource() chan gopacket.Packet {
	return p.inChan
}

// WritePacketData writes packet data using the configured writer
func (p *pcapAdaptor) WritePacketData(data []byte) error {
	if p.writer != nil {
		return p.writer(data)
	}
	return nil
}

// Close closes the adaptor and cleans up resources
func (p *pcapAdaptor) Close() {
	if p.close != nil {
		p.close()
	}
}
