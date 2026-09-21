package pcaputil

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
)

// CaptureReader is the common bounded pcap/pcapng input. LinkType refers to
// the most recently returned record; mixed interfaces are never skipped.
// ReadPacketData returns owned bytes and preserves the section/interface domain.
type CaptureReader struct {
	first     []byte
	firstInfo gopacket.CaptureInfo
	pending   bool
	ng        *pcapgo.NgReader
	classic   *classicPcapReader
	section   uint32
	number    uint64
	link      layers.LinkType
}

func NewCaptureReader(input io.Reader) (*CaptureReader, error) {
	b := bufio.NewReader(input)
	magic, err := b.Peek(4)
	if err != nil {
		return nil, err
	}
	r := &CaptureReader{}
	if binary.LittleEndian.Uint32(magic) == 0x0a0d0d0a {
		r.ng, err = NewBoundedNgReader(b, pcapgo.NgReaderOptions{WantMixedLinkType: true, SectionEndCallback: func(_ []pcapgo.NgInterface, _ pcapgo.NgSectionInfo) { r.section++ }})
		if err == nil {
			r.first, r.firstInfo, err = r.ReadPacketData()
			if err == nil {
				r.pending = true
			} else if errors.Is(err, io.EOF) {
				err = nil
				if iface, e := r.ng.Interface(0); e == nil {
					r.link = iface.LinkType
				}
			}
		}
	} else {
		r.classic, err = newClassicPcapReader(b)
		if err == nil {
			r.link = r.classic.link
		}
	}
	return r, err
}
func (r *CaptureReader) LinkType() layers.LinkType { return r.link }
func (r *CaptureReader) Snaplen() uint32 {
	if r.classic != nil {
		return r.classic.Snaplen()
	}
	return maxCaptureBlock
}
func (r *CaptureReader) ReadPacketData() ([]byte, gopacket.CaptureInfo, error) {
	return r.readPacketData(true)
}

// readBorrowed is internal to synchronous replay. Workers copy before returning
// from submit; public readers and shark retain the owned-byte contract.
func (r *CaptureReader) readBorrowed() ([]byte, gopacket.CaptureInfo, error) {
	return r.readPacketData(false)
}
func (r *CaptureReader) readPacketData(owned bool) ([]byte, gopacket.CaptureInfo, error) {
	if r.pending {
		b, ci := r.first, r.firstInfo
		r.first = nil
		r.pending = false
		return b, ci, nil
	}
	var b []byte
	var ci gopacket.CaptureInfo
	var err error
	if r.ng != nil {
		if owned {
			b, ci, err = r.ng.ReadPacketData()
		} else {
			b, ci, err = r.ng.ZeroCopyReadPacketData()
		}
		if err == nil {
			var iface pcapgo.NgInterface
			iface, err = r.ng.Interface(ci.InterfaceIndex)
			r.link = iface.LinkType
		}
	} else {
		if owned {
			b, ci, err = r.classic.ReadPacketData()
		} else {
			b, ci, err = r.classic.read()
		}
	}
	if err != nil {
		return nil, ci, err
	}
	r.number++
	ci = withEvidence(ci, captureEvidence{Ref: PacketReference{Number: r.number, Domain: CaptureDomain{Section: r.section, Interface: ci.InterfaceIndex}}})
	return b, ci, nil
}

// PacketAnalyzer feeds native packets into the same reassembly and protocol
// engine used by ReplayPcap. Calls (including Close) are serialized. Callbacks
// must not call back into this analyzer. Returned events own their data.
type PacketAnalyzer struct {
	mu     sync.Mutex
	conf   *CaptureConfig
	pool   *TrafficPool
	cancel context.CancelFunc
	number uint64
	closed bool
}

func NewPacketAnalyzer(options ...CaptureOption) (*PacketAnalyzer, error) {
	c := NewDefaultConfig()
	for _, o := range options {
		if err := o(c); err != nil {
			return nil, err
		}
	}
	if c.captureBuffer > 0 || c.BPFFilter != "" || c.Filename != "" || len(c.Device) != 0 || len(c.DeviceAdapter) != 0 || c.deviceAdapter != nil || c.outputFile != "" || c.Output != nil || c.onNetInterfaceCreated != nil || c.mock != nil || c.DisableAssembly || c.EnableCache {
		return nil, fmt.Errorf("PacketAnalyzer does not support capture-source, BPF, recording, cache, or disabled-reassembly options")
	}
	if err := c.prepareBinParser(); err != nil {
		return nil, err
	}
	if c.binParser == nil {
		return nil, fmt.Errorf("PacketAnalyzer requires a protocol subscription")
	}
	parent := c.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	c.Context = ctx
	p := newTrafficPool(ctx, c.reassemblyOptions)
	c.trafficPool, p.captureConf = p, c
	p.captureAccountingAvailable = true
	for _, init := range c.onPoolCreated {
		init(p)
	}
	p.startWorkers()
	return &PacketAnalyzer{conf: c, pool: p, cancel: cancel}, nil
}
func (a *PacketAnalyzer) Feed(raw []byte, ci gopacket.CaptureInfo, link layers.LinkType) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return fmt.Errorf("packet analyzer closed")
	}
	if err := a.conf.Context.Err(); err != nil {
		return err
	}
	if ci.CaptureLength != len(raw) || ci.Length < len(raw) || len(raw) > maxCaptureBlock {
		return fmt.Errorf("invalid capture lengths")
	}
	e := evidenceFrom(ci)
	if e.Ref.Number == 0 {
		a.number++
		e.Ref.Number = a.number
		ci = withEvidence(ci, e)
	} else if e.Ref.Number > a.number {
		a.number = e.Ref.Number
	}
	a.pool.observeCapture(len(raw))
	if len(a.conf.onEveryPacket) > 0 || a.conf.Debug {
		packet := gopacket.NewPacket(raw, link, gopacket.DecodeOptions{Lazy: true, NoCopy: false, DecodeStreamsAsDatagrams: true})
		packet.Metadata().CaptureInfo = ci
		a.conf.packetHandler(a.conf.Context, packet)
	} else {
		d := offlineDecoder{conf: a.conf, link: link}
		d.feed(a.conf.Context, raw, ci)
	}
	return a.pool.Err()
}
func (a *PacketAnalyzer) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil
	}
	a.closed = true
	a.pool.Close()
	a.cancel()
	if a.conf.onReassemblyStats != nil {
		a.conf.onReassemblyStats(a.pool.Stats())
	}
	return errors.Join(a.pool.Err(), a.conf.finishBinParser())
}
