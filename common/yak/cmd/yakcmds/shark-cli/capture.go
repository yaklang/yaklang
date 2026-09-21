package sharkcli

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

type captureConfig struct {
	device, input, output, filter string
	snaplen, count                int
	duration                      time.Duration
	promisc                       bool
	maxStreams, streamBytes       int
}

type packetReader interface {
	ReadPacketData() ([]byte, gopacket.CaptureInfo, error)
}

type captureSource struct {
	reader  packetReader
	file    *os.File
	bpf     *pcap.BPF
	snaplen uint32
	handle  *pcap.Handle
	name    string
	gateway string
	offline bool
	link    layers.LinkType
}

func openCapture(cfg captureConfig) (*captureSource, error) {
	var h *pcap.Handle
	var err error
	name := cfg.input
	gateway := ""
	if cfg.input != "" {
		return openOffline(cfg.input, cfg.filter)
	} else {
		name = cfg.device
		if name == "" {
			name, gateway, err = defaultCaptureInterface()
			if err != nil {
				return nil, fmt.Errorf("resolve default outbound interface: %w (choose one with --interface; see --list-interfaces)", err)
			}
		}
		name, err = pcaputil.IfaceNameToPcapIfaceName(name)
		if err != nil {
			return nil, fmt.Errorf("resolve capture interface: %w", err)
		}
		h, err = pcap.OpenLive(name, int32(cfg.snaplen), cfg.promisc, 100*time.Millisecond)
	}
	if err != nil {
		return nil, fmt.Errorf("open capture %q: %w (live capture requires capture permissions and libpcap/Npcap)", name, err)
	}
	if cfg.filter != "" {
		if err = h.SetBPFFilter(cfg.filter); err != nil {
			h.Close()
			return nil, fmt.Errorf("invalid capture BPF %q: %w", cfg.filter, err)
		}
	}
	return &captureSource{handle: h, reader: h, snaplen: uint32(cfg.snaplen), name: name, gateway: gateway, link: h.LinkType()}, nil
}
func (s *captureSource) Close() {
	if s.handle != nil {
		s.handle.Close()
	}
	if s.file != nil {
		_ = s.file.Close()
	}
}

// Use pcapgo for offline input: the dynamically loaded pcap implementation does
// not support all pcapng files. Reject mixed link types instead of silently
// skipping interfaces or exporting their bytes with the wrong link type.
func openOffline(name, filter string) (_ *captureSource, result error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open capture: %w", err)
	}
	defer func() {
		if result != nil {
			_ = f.Close()
		}
	}()
	reader := bufio.NewReader(f)
	magic, err := reader.Peek(4)
	if err != nil {
		return nil, fmt.Errorf("read capture header: %w", err)
	}
	src := &captureSource{file: f, name: name, offline: true}
	if binary.LittleEndian.Uint32(magic) == 0x0a0d0d0a {
		ng, err := pcaputil.NewBoundedNgReader(reader, pcapgo.NgReaderOptions{ErrorOnMismatchingLinkType: true})
		if err != nil {
			return nil, fmt.Errorf("read pcapng header: %w", err)
		}
		src.reader = ng
		src.link = ng.LinkType()
		src.snaplen = 262144
	} else {
		p, err := pcaputil.NewBoundedPcapReader(reader)
		if err != nil {
			return nil, fmt.Errorf("read pcap header: %w", err)
		}
		src.reader = p
		src.link = p.LinkType()
		src.snaplen = p.Snaplen()
	}
	if filter != "" {
		src.bpf, err = pcap.NewBPF(src.link, int(src.snaplen), filter)
		if err != nil {
			return nil, fmt.Errorf("invalid capture BPF %q: %w", filter, err)
		}
	}
	return src, nil
}

type capturedPacket struct {
	streamID              uint64
	application, evidence string
	number                uint64
	data                  []byte
	ci                    gopacket.CaptureInfo
	link                  layers.LinkType
}

type packetWriter interface {
	WritePacket(gopacket.CaptureInfo, []byte) error
}

type captureSession struct {
	streams       *streamStore
	packets       chan *capturedPacket
	done          chan struct{}
	cancel        context.CancelFunc
	err           error // Published by closing done.
	captured      atomic.Uint64
	bytes         atomic.Uint64
	skipped       atomic.Uint64
	kernelDropped atomic.Uint64
	stopOnce      sync.Once
}

func startCapture(parent context.Context, src *captureSource, cfg captureConfig, lossyDisplay bool) *captureSession {
	ctx, cancel := context.WithCancel(parent)
	s := &captureSession{packets: make(chan *capturedPacket, 2048), done: make(chan struct{}), cancel: cancel, streams: newStreamStore(cfg.maxStreams, cfg.streamBytes)}
	go func() {
		defer close(s.packets)
		defer close(s.done)
		defer cancel()
		s.err = s.capture(ctx, src, cfg, lossyDisplay)
	}()
	return s
}
func (s *captureSession) Stop()         { s.stopOnce.Do(s.cancel); <-s.done }
func (s *captureSession) result() error { <-s.done; return s.err }

func (s *captureSession) capture(ctx context.Context, src *captureSource, cfg captureConfig, lossyDisplay bool) (result error) {
	if s.streams != nil {
		defer s.streams.finish()
	}
	var writer packetWriter
	if cfg.output != "" {
		// Exclusive creation also protects the input capture, including symlink/hardlink aliases.
		file, err := os.OpenFile(cfg.output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return fmt.Errorf("create output capture: %w", err)
		}
		defer func() {
			if err := file.Close(); result == nil {
				result = err
			}
		}()
		if strings.EqualFold(filepath.Ext(cfg.output), ".pcapng") {
			ng, err := pcapgo.NewNgWriter(file, src.link)
			if err != nil {
				return err
			}
			writer = ng
			defer func() {
				if err := ng.Flush(); result == nil {
					result = err
				}
			}()
		} else {
			p := pcapgo.NewWriterNanos(file)
			snaplen := uint32(cfg.snaplen)
			if src.offline {
				snaplen = src.snaplen
			}
			if err := p.WriteFileHeader(snaplen, src.link); err != nil {
				return err
			}
			writer = p
		}
	}
	if cfg.duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.duration)
		defer cancel()
	}
	lastStats := time.Now()
	for {
		if ctx.Err() != nil {
			return nil
		}
		data, ci, err := src.reader.ReadPacketData()
		if !src.offline && src.handle != nil && time.Since(lastStats) >= time.Second {
			if stats, statsErr := src.handle.Stats(); statsErr == nil {
				s.kernelDropped.Store(uint64(stats.PacketsDropped))
			}
			lastStats = time.Now()
		}
		if errors.Is(err, pcap.NextErrorTimeoutExpired) {
			if s.streams != nil {
				s.streams.tick(time.Now())
			}
			continue
		}
		if errors.Is(err, io.EOF) {
			if len(data) > 0 {
				return fmt.Errorf("read capture: %w", io.ErrUnexpectedEOF)
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("read capture: %w", err)
		}
		if src.bpf != nil && !src.bpf.Matches(ci, data) {
			continue
		}
		if writer != nil {
			// A classic pcap source may carry a nonzero InterfaceIndex. The exported
			// file has one interface, regardless of the source OS index.
			outputInfo := ci
			outputInfo.InterfaceIndex = 0
			if err := writer.WritePacket(outputInfo, data); err != nil {
				return fmt.Errorf("write output capture: %w", err)
			}
		}
		n := s.captured.Add(1)
		s.bytes.Add(uint64(len(data)))
		packet := &capturedPacket{number: n, data: data, ci: ci, link: src.link}
		if s.streams != nil {
			s.streams.add(packet)
		}
		if lossyDisplay && !src.offline {
			s.enqueueLatest(packet)
		} else {
			select {
			case s.packets <- packet:
			case <-ctx.Done():
				return nil
			}
		}
		if cfg.count > 0 && n >= uint64(cfg.count) {
			return nil
		}
	}
}

// Keep the newest window under load. Discarding the incoming packet made the
// old UI perpetually lag behind live traffic when its queue filled up.
func (s *captureSession) enqueueLatest(packet *capturedPacket) {
	select {
	case s.packets <- packet:
		return
	default:
	}
	select {
	case <-s.packets:
		s.skipped.Add(1)
	default:
	}
	select {
	case s.packets <- packet:
	default:
		s.skipped.Add(1)
	}
}
