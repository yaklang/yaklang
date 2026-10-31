package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type tlsAlertStep struct {
	Wire         string `json:"input_hex"`
	SHA          string `json:"input_sha256"`
	Direction    int
	At           int64          `json:"timestamp_unix"`
	Fields       map[string]any `json:"fields_hexbytes"`
	Error        string
	Completeness string
	Offset       uint64
}
type tlsAlertProbe struct {
	Wire      string `json:"input_hex"`
	Direction int
	At        int64 `json:"timestamp_unix"`
	Error     string
}
type tlsAlertCase struct {
	Name   string
	Ports  [2]uint16
	Steps  []tlsAlertStep
	Budget struct {
		Collection int `json:"MaxCollectionElements"`
	}
	Close      []tlsAlertStep  `json:"close_events"`
	Probes     []tlsAlertProbe `json:"after_failure"`
	Statistics map[string]uint64
}

func tlsAlertControls(t *testing.T) []tlsAlertCase {
	var x struct{ Cases []tlsAlertCase }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "tls-alert", "TLS_ALERT_TEST_ROOT", "controls.json"), &x))
	require.Len(t, x.Cases, 15)
	// These three controls contain an observed TLS 1.3 ServerHello selecting
	// TLS_AES_128_GCM_SHA256. Keep that context on subsequent refused/opaque
	// records too; their authentication and error expectations stay exact.
	for i := range x.Cases {
		if strings.HasPrefix(x.Cases[i].Name, "known-tls13-") {
			for j := range x.Cases[i].Steps {
				fields := x.Cases[i].Steps[j].Fields
				if fields != nil && fields["Negotiated Version"] == nil {
					fields["Negotiated Version"], fields["Cipher Suite"] = 0x304, 0x1301
				}
			}
		}
	}
	return x.Cases
}
func tlsAlertWant(t *testing.T, src map[string]any) map[string]any {
	b, err := json.Marshal(src)
	require.NoError(t, err)
	var x map[string]any
	require.NoError(t, json.Unmarshal(b, &x))
	var walk func(map[string]any)
	walk = func(m map[string]any) {
		for k, v := range m {
			switch y := v.(type) {
			case float64:
				switch k {
				case "Record Type", "Alert Level", "Alert Description", "Level", "Description", "Type":
					m[k] = byte(y)
				case "Record Version", "Negotiated Version", "Cipher Suite":
					m[k] = uint16(y)
				default:
					m[k] = int(y)
				}
			case string:
				if k == "Raw" {
					m[k] = wrapperWire(t, y)
				}
			case []any:
				rows := make([]map[string]any, len(y))
				for i, r := range y {
					rows[i] = r.(map[string]any)
					walk(rows[i])
				}
				m[k] = rows
			case map[string]any:
				walk(y)
			}
		}
	}
	walk(x)
	return x
}
func tlsAlertCheckQuery(t *testing.T, e *ProtocolEvent, want map[string]any, kind string) {
	t.Helper()
	got, err := e.GetFields()
	rocTypedError(t, kind, err)
	if kind != "" {
		require.Nil(t, got)
		assertOwnedProtocolRefusal(t, e, kind, err)
		return
	}
	require.Equal(t, want, got)
	mutateRTPSQuery(got)
	d, err := e.Decode()
	require.NoError(t, err)
	require.Equal(t, want, d["fields"])
	require.Equal(t, want, d["session"])
	mutateRTPSQuery(d)
	again, err := e.GetFields()
	require.NoError(t, err)
	require.Equal(t, want, again)
}
func tlsAlertCheckEvent(t *testing.T, e *ProtocolEvent, c tlsAlertCase, w tlsAlertStep, captured, deferred bool, flow uint64) {
	t.Helper()
	var raw []byte
	if w.Wire != "" {
		raw = wrapperWire(t, w.Wire)
	}
	want := tlsAlertWant(t, w.Fields)
	want = tlsFrozenExpectedMetadata(t, c, w, want)
	require.Equal(t, "tls", e.Protocol)
	require.Equal(t, "tcp", e.Transport)
	require.Equal(t, w.Direction, e.Direction)
	require.Equal(t, w.Offset, e.Offset)
	require.Equal(t, raw, e.Raw)
	require.Equal(t, len(raw), e.Length)
	require.True(t, time.Unix(w.At, 0).Equal(e.Timestamp))
	require.NotZero(t, e.ID)
	require.Equal(t, flow, e.FlowID)
	require.Zero(t, e.TransactionID)
	require.Zero(t, e.ResponseTo)
	require.Equal(t, CaptureDomain{}, e.Domain)
	src, dst := fmt.Sprintf("192.0.2.1:%d", c.Ports[0]), fmt.Sprintf("192.0.2.2:%d", c.Ports[1])
	if w.Direction == 1 {
		src, dst = dst, src
	}
	require.Equal(t, src, e.Source)
	require.Equal(t, dst, e.Destination)
	status := "decoded"
	if deferred {
		status = "deferred"
	}
	switch w.Error {
	case "NeedMore":
		status = "incomplete"
	case "MalformedMessage":
		status = "malformed"
	case "ResourceExceeded":
		status = "limited"
	case "ContextRequired":
		status = "context-required"
	}
	require.Equal(t, status, e.Status)
	require.Equal(t, w.Completeness, e.Completeness)
	require.Equal(t, want, e.Session)
	if captured && len(raw) > 0 {
		require.NotEmpty(t, e.SourceBytes.PacketRefs)
		require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(raw)), e.SourceBytes.SHA256)
	}
	tlsAlertCheckQuery(t, e, want, w.Error)
	tlsDTLSPoison(e, "radius")
	tlsAlertCheckQuery(t, e, want, w.Error)
}
func tlsAlertCheckStats(t *testing.T, stats ProtocolStats, want map[string]uint64) {
	t.Helper()
	v := reflect.ValueOf(stats)
	for k, w := range want {
		f := v.FieldByName(k)
		require.True(t, f.IsValid(), "named statistics field %s", k)
		require.Equal(t, w, f.Uint(), k)
	}
	require.Zero(t, stats.BufferedBytes)
	require.Zero(t, stats.CallbackPanics)
}
func TestTLSAlertNative(t *testing.T) {
	for _, c := range tlsAlertControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			for _, d := range []bool{false, true} {
				for _, chunk := range []int{0, 1, 7} {
					t.Run(fmt.Sprintf("d%t/c%d", d, chunk), func(t *testing.T) {
						b := DefaultParserBudget()
						if c.Budget.Collection > 0 {
							b.MaxCollectionElements = c.Budget.Collection
						}
						s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("tcp"), WithSessionPorts(c.Ports[0], c.Ports[1]))
						require.NoError(t, err)
						defer s.Close("cleanup")
						s.(*captureSession).f.a.config.Deferred = d
						flow := uint64(0)
						ids := map[uint64]bool{}
						for _, w := range c.Steps {
							raw := wrapperWire(t, w.Wire)
							require.Equal(t, w.SHA, fmt.Sprintf("%x", sha256.Sum256(raw)))
							var events []*ProtocolEvent
							var last *ProtocolError
							for at := 0; at < len(raw); {
								n := len(raw) - at
								if chunk > 0 {
									n = min(n, chunk)
								}
								input := bytes.Clone(raw[at : at+n])
								o := s.Feed(w.Direction, time.Unix(w.At, 0), input)
								clear(input)
								at += n
								if at < len(raw) {
									if o.Err != nil {
										rocTypedError(t, "NeedMore", o.Err)
									}
								} else {
									last = o.Err
								}
								events = append(events, o.Events...)
							}
							if w.Error == "" {
								require.Nil(t, last)
							} else {
								rocTypedError(t, w.Error, last)
							}
							require.Len(t, events, 1)
							e := events[0]
							if flow == 0 {
								flow = e.FlowID
							}
							require.False(t, ids[e.ID])
							ids[e.ID] = true
							tlsAlertCheckEvent(t, e, c, w, false, d, flow)
						}
						for _, p := range c.Probes {
							o := s.Feed(p.Direction, time.Unix(p.At, 0), wrapperWire(t, p.Wire))
							rocTypedError(t, p.Error, o.Err)
							require.Empty(t, o.Events)
						}
						closed := s.Close("done")
						require.Len(t, closed, len(c.Close))
						for i, w := range c.Close {
							e := closed[i]
							require.False(t, ids[e.ID])
							ids[e.ID] = true
							tlsAlertCheckEvent(t, e, c, w, false, d, flow)
						}
						require.Empty(t, s.Close("again"))
						tlsAlertCheckStats(t, s.Stats(), c.Statistics)
					})
				}
			}
		})
	}
}
func tlsAlertCapture(t *testing.T, c tlsAlertCase) []byte {
	var buf bytes.Buffer
	pw := pcapgo.NewWriter(&buf)
	require.NoError(t, pw.WriteFileHeader(65535, layers.LinkTypeEthernet))
	write := func(dir int, at int64, seq, ack uint32, syn, fin bool, payload []byte) {
		src, dst := net.IP{192, 0, 2, 1}, net.IP{192, 0, 2, 2}
		sp, dp := c.Ports[0], c.Ports[1]
		if dir == 1 {
			src, dst, sp, dp = dst, src, dp, sp
		}
		eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{0, 1, 2, 3, 4, 5}, DstMAC: net.HardwareAddr{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4}
		ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: src, DstIP: dst}
		tcp := &layers.TCP{SrcPort: layers.TCPPort(sp), DstPort: layers.TCPPort(dp), Seq: seq, Ack: ack, Window: 65535, SYN: syn, FIN: fin, ACK: !syn || dir == 1}
		require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
		b := gopacket.NewSerializeBuffer()
		require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, tcp, gopacket.Payload(payload)))
		raw := b.Bytes()
		require.NoError(t, pw.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(at, 0), CaptureLength: len(raw), Length: len(raw)}, raw))
	}
	write(0, 97, 0, 0, true, false, nil)
	write(1, 98, 0, 1, true, false, nil)
	write(0, 99, 1, 1, false, false, nil)
	seq := [2]uint32{1, 1}
	last := int64(100)
	for _, w := range c.Steps {
		p := wrapperWire(t, w.Wire)
		write(w.Direction, w.At, seq[w.Direction], seq[1-w.Direction], false, false, p)
		seq[w.Direction] += uint32(len(p))
		last = w.At
	}
	for _, w := range c.Probes {
		p := wrapperWire(t, w.Wire)
		write(w.Direction, w.At, seq[w.Direction], seq[1-w.Direction], false, false, p)
		seq[w.Direction] += uint32(len(p))
		last = w.At
	}
	write(0, last+1, seq[0], seq[1], false, true, nil)
	write(1, last+2, seq[1], seq[0]+1, false, true, nil)
	write(0, last+3, seq[0]+1, seq[1]+1, false, false, nil)
	return buf.Bytes()
}
func TestTLSAlertMatrix(t *testing.T) {
	for _, c := range tlsAlertControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			generated := tlsAlertCapture(t, c)
			if dir := os.Getenv("TLS_ALERT_EXPORT"); dir != "" {
				p := filepath.Join(dir, "tls-alert", c.Name+".pcap")
				require.NoError(t, os.MkdirAll(filepath.Dir(p), 0700))
				require.NoError(t, os.WriteFile(p, generated, 0600))
			}
			var raw []byte
			if os.Getenv("TLS_ALERT_TEST_ROOT") != "" {
				raw = generated
			} else {
				raw = finsReadBatch(t, "tls-alert", "TLS_ALERT_TEST_ROOT", c.Name+".pcap")
				require.Equal(t, generated, raw)
			}
			discoveryMatrix(t, func(t *testing.T, workers int, d, o bool) {
				var mu sync.Mutex
				var events []*ProtocolEvent
				var stats ProtocolStats
				var packets atomic.Int64
				b := DefaultParserBudget()
				if c.Budget.Collection > 0 {
					b.MaxCollectionElements = c.Budget.Collection
				}
				opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(d), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnProtocolMessage(func(e *ProtocolEvent) {
					mu.Lock()
					defer mu.Unlock()
					copy := *e
					copy.Raw = bytes.Clone(e.Raw)
					copy.Session = cloneSession(e.Session)
					copy.Fields = cloneSession(e.Fields)
					copy.Structured = cloneSession(e.Structured)
					copy.SourceBytes.PacketRefs = append([]PacketReference(nil), e.SourceBytes.PacketRefs...)
					events = append(events, &copy)
					// Change nested public state while later records are still outstanding.
					mutateRTPSQuery(e.Session)
					tlsDTLSPoison(e, "radius")
				})}
				if o {
					opts = append(opts, WithEveryPacket(func(gopacket.Packet) { packets.Add(1) }))
				}
				a, err := NewPacketAnalyzer(opts...)
				require.NoError(t, err)
				a.conf.binParser.budget = b
				reader, err := NewCaptureReader(bytes.NewReader(raw))
				require.NoError(t, err)
				for {
					packet, ci, readErr := reader.ReadPacketData()
					if readErr == io.EOF {
						break
					}
					require.NoError(t, readErr)
					require.NoError(t, a.Feed(packet, ci, reader.LinkType()))
					clear(packet)
				}
				require.NoError(t, a.Close())
				require.NoError(t, a.Close())
				require.Len(t, events, len(c.Steps)+len(c.Close))
				flow := events[0].FlowID
				ids := map[uint64]bool{}
				for i, w := range append(append([]tlsAlertStep{}, c.Steps...), c.Close...) {
					e := events[i]
					require.False(t, ids[e.ID])
					ids[e.ID] = true
					tlsAlertCheckEvent(t, e, c, w, true, d, flow)
					if i < len(c.Steps) {
						require.Len(t, e.SourceBytes.PacketRefs, 1)
						require.Equal(t, uint64(i+4), e.SourceBytes.PacketRefs[0].Number)
					}
				}
				tlsAlertCheckStats(t, stats, c.Statistics)
				if o {
					require.Equal(t, int64(6+len(c.Steps)+len(c.Probes)), packets.Load())
				} else {
					require.Zero(t, packets.Load())
				}
			})
		})
	}
}
