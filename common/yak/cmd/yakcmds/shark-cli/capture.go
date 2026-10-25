package sharkcli

import (
	"context"
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
	ignoreEmbeddedSecrets         bool
	maxStreams, streamBytes       int
	tlsSecrets                    pcaputil.TLSSecretProvider
	decodeAs                      []pcaputil.CaptureOption
	initialDisplayFilter          string
	retentionPackets              int
	freezePacketFields            bool
}

func protocolHistoryBudget(packets int) (int, int) {
	// Size history with an explicitly enlarged offline packet window, with
	// hard ceilings. Defaults remain 80k PDUs/64 MiB.
	packets = max(20000, min(packets, 100000))
	return min(256000, packets*4), int(min(int64(256<<20), int64(64<<20)*int64(packets)/20000))
}

type packetReader interface {
	ReadPacketData() ([]byte, gopacket.CaptureInfo, error)
}

type captureSource struct {
	reader  packetReader
	file    *os.File
	bpf     *pcap.BPF
	filter  string
	bpfs    map[layers.LinkType]*pcap.BPF
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
		return openOffline(cfg.input, cfg.filter, cfg.ignoreEmbeddedSecrets)
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
// not support all pcapng files. Link types are carried per packet.
func openOffline(name, filter string, ignoreSecrets ...bool) (_ *captureSource, result error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open capture: %w", err)
	}
	defer func() {
		if result != nil {
			_ = f.Close()
		}
	}()
	reader, err := pcaputil.NewCaptureReaderWithOptions(f, pcaputil.PcapReaderOptions{IgnoreEmbeddedSecrets: len(ignoreSecrets) > 0 && ignoreSecrets[0]})
	if err != nil {
		return nil, fmt.Errorf("read capture header: %w", err)
	}
	src := &captureSource{file: f, name: name, offline: true, reader: reader, link: reader.LinkType(), snaplen: reader.Snaplen(), filter: filter, bpfs: make(map[layers.LinkType]*pcap.BPF)}

	if filter != "" {
		src.bpf, err = pcap.NewBPF(bpfLinkType(src.link), int(src.snaplen), filter)
		if err != nil {
			return nil, fmt.Errorf("invalid capture BPF %q: %w", filter, err)
		}
	}
	if src.bpf != nil {
		src.bpfs[src.link] = src.bpf
	}
	return src, nil
}

type capturedPacket struct {
	transportView         *packetTransportView
	physicalRecord        bool
	tcpFlowID             uint64
	tcpFlowDomain         pcaputil.CaptureDomain
	messageTransport      string
	messageSource         string
	messageDestination    string
	messageDomain         pcaputil.CaptureDomain
	conversationID        uint64
	conversationTransport string
	conversationDomain    pcaputil.CaptureDomain
	conversationDomainSet bool
	connectionStack       []string
	connectionDiagnostic  string
	opaqueTransport       bool
	presentation          packetPresentation
	reference             pcaputil.PacketReference
	events                []*pcaputil.ProtocolEvent
	observedPDUs          []map[string]any
	observedFields        []pcaputil.DisplayFieldSnapshot
	observedFieldBudget   int
	observedFieldsLimited bool
	fieldsAtObservation   bool
	eventIDs              []uint64
	history               *pcaputil.ProtocolInspector
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
	queue := 2048
	if cfg.freezePacketFields {
		queue = 64
	} // bounded field snapshots backpressure plain/JSON output
	s := &captureSession{packets: make(chan *capturedPacket, queue), done: make(chan struct{}), cancel: cancel, streams: newStreamStore(cfg.maxStreams, cfg.streamBytes)}
	s.streams.captureReplay = src.offline
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
	// A retained physical packet can contribute multiple carrier/inner PDUs.
	// Keep the default 20k packet window useful after a full-file replay while
	// bounding both observation count and retained structured/raw bytes.
	messages, bytes := protocolHistoryBudget(cfg.retentionPackets)
	history, _ := pcaputil.NewProtocolInspector(messages, bytes)
	if s.streams != nil {
		s.streams.mu.Lock()
		s.streams.history = history
		s.streams.mu.Unlock()
	}
	var current *capturedPacket
	var normalizedPreview gopacket.Packet
	var currentProtocolEvents []*pcaputil.ProtocolEvent
	// Inspection must retain useful packets from imperfect captures. The engine
	// still reports gaps and never concatenates bytes across a missing range.
	options := append([]pcaputil.CaptureOption(nil), cfg.decodeAs...)
	if src.offline {
		options = append(options, pcaputil.WithCaptureReplayClock())
	}
	options = append(options, pcaputil.WithBeforeTransportPacket(func(packet gopacket.Packet) {
		if current != nil {
			current.observeTransportHeader(packet)
			if current.transportView != nil && current.transportView.Protocol == "tcp" {
				normalizedPreview = ownedNormalizedPreview(packet)
			}
		}
	}))
	options = append(options, pcaputil.WithTCPReassemblyOptions(pcaputil.TCPReassemblyOptions{AllowIncomplete: true}), pcaputil.WithTLSSecrets(cfg.tlsSecrets), pcaputil.WithOnTrafficFlowPacket(func(flow *pcaputil.TrafficFlow, _ *pcaputil.TrafficConnection, refs []pcaputil.PacketReference, _ time.Time) {
		if current != nil {
			for _, ref := range refs {
				if ref.Number == current.reference.Number {
					current.tcpFlowID = flow.Index // Copy a scalar; never retain the pooled flow.
					current.tcpFlowDomain = flow.CaptureDomain()
					break
				}
			}
		}
	}), pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) {
		history.OnEvent(e)
		if s.streams != nil {
			s.streams.observeProtocol(e)
		}
		if current != nil {
			current.history = history
			// Time-based flushing can emit events from another flow. Only an
			// event with this packet among its contributors may label its row.
			for _, ref := range e.SourceBytes.PacketRefs {
				// CLI record numbers are global across sections/interfaces. Outer
				// fragment reassembly can add a proven tunnel domain to this same
				// record; exact domain equality would hide its completed PDU.
				if ref.Number == current.reference.Number {
					recordProtocolConversation(current, e)
					if current.fieldsAtObservation {
						current.recordObservedPDU(e)
					}
					if e.Transport == "tcp" && len(currentProtocolEvents) < 128 {
						currentProtocolEvents = append(currentProtocolEvents, e)
					}
					if len(current.eventIDs) < 128 {
						current.eventIDs = append(current.eventIDs, e.ID)
					}
					current.presentation.add(e)
					if (e.Status == "decoded" || e.Status == "deferred" || protectedSSHEvent(e) || protectedConversationEvent(e)) && (e.Transport == "tcp" || e.Transport == "udp") {
						current.messageTransport, current.messageSource, current.messageDestination = e.Transport, e.Source, e.Destination
						current.messageDomain = e.Domain
					}
					break
				}
			}
		}
	}))
	analyzer, err := pcaputil.NewPacketAnalyzer(options...)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, analyzer.Close()) }()

	if s.streams != nil {
		defer s.streams.finish()
	}
	var writer packetWriter
	var ngWriter *pcapgo.NgWriter
	type outputDomain struct {
		domain pcaputil.CaptureDomain
		link   layers.LinkType
	}
	interfaces := make(map[outputDomain]int)
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
			ngWriter = ng
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
		link := src.link
		if reader, ok := src.reader.(*pcaputil.CaptureReader); ok {
			link = reader.LinkType()
		}
		if src.offline && src.filter != "" {
			bpf := src.bpfs[link]
			if bpf == nil {
				bpf, err = pcap.NewBPF(bpfLinkType(link), int(src.snaplen), src.filter)
				if err != nil {
					return fmt.Errorf("compile capture BPF for link %d: %w", link, err)
				}
				src.bpfs[link] = bpf
			}
			if !bpf.Matches(ci, data) {
				continue
			}
		}

		if writer != nil {
			outputInfo := ci
			outputInfo.InterfaceIndex = 0
			if ngWriter != nil {
				domain := pcaputil.CapturePacketReference(ci).Domain
				key := outputDomain{domain: domain, link: link}
				index, ok := interfaces[key]
				if !ok {
					if len(interfaces) == 0 && link == src.link {
						index = 0
					} else {
						index, err = ngWriter.AddInterface(pcapgo.NgInterface{Name: fmt.Sprintf("section-%d-interface-%d", domain.Section, domain.Interface), LinkType: link, TimestampResolution: 9})
						if err != nil {
							return fmt.Errorf("write output interface: %w", err)
						}
					}
					interfaces[key] = index
				}
				outputInfo.InterfaceIndex = index
			} else if link != src.link {
				return fmt.Errorf("classic PCAP cannot represent link change %d to %d; use .pcapng output", src.link, link)
			}
			if err := writer.WritePacket(outputInfo, data); err != nil {
				return fmt.Errorf("write output capture: %w", err)
			}
		}
		n := s.captured.Add(1)
		s.bytes.Add(uint64(len(data)))
		ref := pcaputil.CapturePacketReference(ci)
		if ref.Number == 0 {
			ref.Number = n
		}
		ref.Domain = pcaputil.PacketDomain(data, ci, link)
		packet := &capturedPacket{physicalRecord: true, number: ref.Number, data: data, ci: ci, link: link, history: history, reference: ref}
		if cfg.freezePacketFields {
			packet.fieldsAtObservation = true
			packet.observedFieldBudget = observedPacketFieldBytes - 1024
		}
		current = packet
		normalizedPreview = nil
		currentProtocolEvents = nil
		if err := analyzer.Feed(data, ci, link); err != nil {
			return err
		}
		if s.streams != nil {
			if normalizedPreview != nil && packet.transportView != nil {
				s.streams.addDecoded(packet, normalizedPreview, packet.transportView.Domain)
			} else {
				s.streams.add(packet)
			}
			// The first PDU can be published before its preview record exists.
			// Replay only retained TCP contributors after the core's generation ID
			// is known; identity updates are idempotent, bytes are never fed twice.
			for _, event := range currentProtocolEvents {
				s.streams.observeProtocol(event)
			}
			s.streams.annotate(packet)
		}
		normalizedPreview = nil
		currentProtocolEvents = nil
		current = nil
		if packet.conversationID != 0 && len(packet.connectionStack) == 0 {
			packet.connectionStack = append([]string(nil), packet.presentation.stack...)
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

// libpcap consumes DLT_RAW (12), while capture files record LINKTYPE_RAW (101).
func bpfLinkType(link layers.LinkType) layers.LinkType {
	if link == layers.LinkTypeRaw {
		return layers.LinkType(12)
	}
	return link
}
