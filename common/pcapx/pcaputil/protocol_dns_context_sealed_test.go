package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type dnsContextEvent struct {
	udpContextEvent
	Source, Destination, Completeness string
	LegacyFields                      map[string]any `json:"legacy_fields"`
}
type dnsContextCase struct {
	tftpFileCase
	Events   []dnsContextEvent
	MaxBytes int `json:"max_bytes"`
	Native   bool
}

func dnsContextControls(t *testing.T) []dnsContextCase {
	var d struct {
		Schema string
		Cases  []dnsContextCase
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dns-context", "DNS_CONTEXT_TEST_ROOT", "controls.json"), &d))
	require.Equal(t, "owned-dns-context/v1", d.Schema)
	require.Len(t, d.Cases, 33)
	return d.Cases
}
func dnsContextQuery(t *testing.T, e *ProtocolEvent, w dnsContextEvent) {
	t.Helper()
	stunContextQuery(t, e, stunContextEvent{Fields: w.Fields, Session: w.Session, QueryError: w.QueryError})
	if w.QueryError == "" {
		f, err := e.GetFields()
		require.NoError(t, err)
		canonical := f
		if h, ok := f["Header"].(map[string]any); ok {
			require.IsType(t, uint16(0), h["ID"])
			canonical = e.semanticSession["DNS"].(map[string]any)
		}
		require.IsType(t, uint16(0), canonical["ID"])
		require.IsType(t, uint16(0), canonical["Flags"])
		qs := canonical["Questions"].([]map[string]any)
		require.Len(t, qs, 1)
		require.IsType(t, []byte{}, qs[0]["Name Wire"])
		require.IsType(t, uint16(0), qs[0]["Type"])
		for _, a := range canonical["Answers"].([]map[string]any) {
			require.IsType(t, uint32(0), a["TTL"])
			require.IsType(t, []byte{}, a["RData"])
		}
	}
}
func dnsContextCheck(t *testing.T, c dnsContextCase, events []*ProtocolEvent, deferred, captured bool, transport string) {
	t.Helper()
	require.Len(t, events, len(c.Events))
	ids := map[uint64]bool{}
	flow := uint64(0)
	offsets := [2]uint64{}
	for i, w := range c.Events {
		e := events[i]
		require.Equal(t, w.Protocol, e.Protocol)
		profile := w.Profile
		raw := wrapperWire(t, w.Raw)
		dir := 0
		src, dst := w.Source, w.Destination
		length := len(w.InputWire) / 2
		offset := uint64(0)
		if !captured {
			if strings.HasPrefix(src, "192.0.2.53:") {
				dir = 1
			}
			src = strings.ReplaceAll(strings.ReplaceAll(src, "192.0.2.10:", "192.0.2.1:"), "192.0.2.53:", "192.0.2.2:")
			dst = strings.ReplaceAll(strings.ReplaceAll(dst, "192.0.2.10:", "192.0.2.1:"), "192.0.2.53:", "192.0.2.2:")
		}
		if transport == "tcp" {
			profile = "dns-tcp"
			raw = append(binary.BigEndian.AppendUint16(nil, uint16(len(raw))), raw...)
			length += 2
			offset = offsets[dir]
			offsets[dir] += uint64(length)
			if w.QueryError == "" {
				w.Fields = w.LegacyFields
			}
			require.NotZero(t, e.FlowID)
			if flow == 0 {
				flow = e.FlowID
			}
			require.Equal(t, flow, e.FlowID)
		} else {
			require.Zero(t, e.FlowID)
		}
		require.Equal(t, profile, e.Profile)
		require.Equal(t, transport, e.Transport)
		require.Equal(t, raw, e.Raw)
		require.Equal(t, dir, e.Direction)
		require.Equal(t, length, e.Length)
		require.Equal(t, offset, e.Offset)
		require.Equal(t, src, e.Source)
		require.Equal(t, dst, e.Destination)
		require.True(t, time.Unix(w.Timestamp, 0).Equal(e.Timestamp))
		require.Equal(t, w.Completeness, e.Completeness)
		domain := CaptureDomain{}
		if captured {
			domain.Interface = w.Interface
		}
		require.Equal(t, domain, e.Domain)
		if captured {
			require.Len(t, e.SourceBytes.PacketRefs, 1)
			require.Equal(t, uint64(i+1), e.SourceBytes.PacketRefs[0].Number)
			require.Equal(t, domain, e.SourceBytes.PacketRefs[0].Domain)
		} else {
			require.Empty(t, e.SourceBytes.PacketRefs)
		}
		require.NotZero(t, e.ID)
		require.False(t, ids[e.ID])
		ids[e.ID] = true
		for _, pair := range []struct {
			want *int
			got  uint64
		}{{w.ResponseTo, e.ResponseTo}, {w.Transaction, e.TransactionID}} {
			if pair.want == nil {
				require.Zero(t, pair.got)
			} else {
				require.GreaterOrEqual(t, *pair.want, 0)
				require.Less(t, *pair.want, len(events))
				require.Equal(t, events[*pair.want].ID, pair.got)
			}
		}
		status := "decoded"
		if deferred {
			status = "deferred"
		}
		if w.Error != "" {
			status = "malformed"
			if w.Error == "ResourceExceeded" {
				status = "limited"
			}
			require.Equal(t, w.Error, e.ExpertCode)
			require.NotEmpty(t, e.Error)
		} else {
			require.Empty(t, e.Error)
		}
		require.Equal(t, status, e.Status)
		rocEqualFields(t, w.Session, e.Session)
		dnsContextQuery(t, e, w)
		for _, alias := range []string{"caller", "redis", "dns"} {
			tlsDTLSPoison(e, alias)
			dnsContextQuery(t, e, w)
		}
	}
}
func dnsContextReplay(t *testing.T, c tftpFileCase, raw []byte, workers int, deferred, observe bool) ([]*ProtocolEvent, ProtocolStats) {
	t.Helper()
	var snapshots, originals []*ProtocolEvent
	var stats ProtocolStats
	var seen atomic.Int64
	opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnProtocolMessage(func(e *ProtocolEvent) {
		snap := *e
		snap.Raw = bytes.Clone(e.Raw)
		snap.Session = cloneSession(e.Session)
		snap.Fields = cloneSession(e.Fields)
		snap.SourceBytes.PacketRefs = append([]PacketReference(nil), e.SourceBytes.PacketRefs...)
		snapshots = append(snapshots, &snap)
		originals = append(originals, e)
		if observe {
			tlsDTLSPoison(e, "redis")
			e.ID = 0
			e.FlowID = 0
			e.Timestamp = time.Unix(999999, 0)
			e.Direction = 99
			e.Status = "decoded"
			e.Error = ""
			if len(e.SourceBytes.PacketRefs) > 0 {
				e.SourceBytes.PacketRefs[0].Number = 999999
			}
		}
	})}
	if c.FileEnabled {
		opts = append(opts, WithProtocolTFTPFileAssembly(69, c.FileLimit))
	}
	if c.MaxMessage > 0 || c.MaxBuffered > 0 {
		m, b := c.MaxMessage, c.MaxBuffered
		if m == 0 {
			m = 1 << 20
		}
		if b == 0 {
			b = 1 << 20
		}
		opts = append(opts, WithProtocolBudget(m, b))
	}
	if observe {
		opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen.Add(1) }))
	}
	a, err := NewPacketAnalyzer(opts...)
	require.NoError(t, err)
	if c.MaxCollection > 0 {
		a.conf.binParser.budget.MaxCollectionElements = c.MaxCollection
	}
	r, err := NewCaptureReader(bytes.NewReader(raw))
	require.NoError(t, err)
	packets := int64(0)
	for {
		b, ci, x := r.ReadPacketData()
		if x == io.EOF {
			break
		}
		require.NoError(t, x)
		require.NoError(t, a.Feed(b, ci, r.LinkType()))
		clear(b)
		packets++
	}
	require.NoError(t, a.Close())
	require.NoError(t, a.Close())
	require.Zero(t, stats.BufferedBytes)
	require.Zero(t, stats.CallbackPanics)
	require.EqualValues(t, len(c.Events), stats.Messages)
	expectedBytes := 0
	for _, w := range c.Events {
		expectedBytes += len(w.InputWire) / 2
	}
	require.EqualValues(t, expectedBytes, stats.MessageBytes)
	if observe {
		require.Equal(t, packets, seen.Load())
	}
	require.Len(t, originals, len(c.Events))
	for i, e := range originals {
		w := c.Events[i]
		w.Session, _ = tftpFileResolve(w.Session, snapshots).(map[string]any)
		w.Fields, _ = tftpFileResolve(w.Fields, snapshots).(map[string]any)
		tftpFileQuery(t, e, w)
	}
	return snapshots, stats
}

func TestDNSContextSealedMatrix(t *testing.T) {
	for _, c := range dnsContextControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw := finsReadBatch(t, "dns-context", "DNS_CONTEXT_TEST_ROOT", c.Capture)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			flat := c.tftpFileCase
			flat.MaxMessage = c.MaxBytes
			for _, w := range c.Events {
				flat.Events = append(flat.Events, w.tftpFileEvent)
			}
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				events, stats := dnsContextReplay(t, flat, raw, workers, deferred, observe)
				dnsContextCheck(t, c, events, deferred, true, "udp")
				require.Zero(t, stats.BufferedBytes)
				require.Zero(t, stats.CallbackPanics)
				require.EqualValues(t, len(c.Events), stats.Messages)
				expectedBytes := 0
				for _, w := range c.Events {
					expectedBytes += len(w.InputWire) / 2
				}
				require.EqualValues(t, expectedBytes, stats.MessageBytes)
			})
		})
	}
}
func TestDNSContextNativePaths(t *testing.T) {
	for _, c := range dnsContextControls(t) {
		if !c.Native {
			continue
		}
		t.Run(c.ID, func(t *testing.T) {
			transports := []string{"udp"}
			hasError := false
			for _, w := range c.Events {
				hasError = hasError || w.Error != ""
			}
			if !hasError {
				transports = append(transports, "tcp")
			}
			for _, transport := range transports {
				chunks := []int{0}
				if transport == "tcp" {
					chunks = []int{0, 1, 2, 7, 64, 4096}
				}
				for _, chunk := range chunks {
					for _, deferred := range []bool{false, true} {
						t.Run(fmt.Sprintf("%s/chunk%d/d%t", transport, chunk, deferred), func(t *testing.T) {
							b := DefaultParserBudget()
							if c.MaxCollection > 0 {
								b.MaxCollectionElements = c.MaxCollection
							}
							if c.MaxBytes > 0 {
								b.MaxFrameBytes, b.MaxMessageBytes = c.MaxBytes, c.MaxBytes
							}
							opts := []ProtocolSessionOption{WithSessionTransport(transport), WithSessionPorts(41707, 53)}
							if transport == "tcp" {
								opts = append(opts, WithSessionClientDirection(0))
							}
							s, err := NewProtocolSessionWithOptions(b, opts...)
							require.NoError(t, err)
							s.(*captureSession).f.a.config.Deferred = deferred
							var events, originals []*ProtocolEvent
							appendEvents := func(es []*ProtocolEvent) {
								for _, e := range es {
									snap := *e
									snap.Raw = bytes.Clone(e.Raw)
									snap.Session = cloneSession(e.Session)
									snap.Fields = cloneSession(e.Fields)
									snap.SourceBytes.PacketRefs = append([]PacketReference(nil), e.SourceBytes.PacketRefs...)
									events = append(events, &snap)
									originals = append(originals, e)
									tlsDTLSPoison(e, "redis")
									e.ID = 0
									e.FlowID = 0
								}
							}
							type step struct {
								dir  int
								ts   int64
								wire []byte
							}
							var groups []step
							allowed := map[ProtocolErrorKind]bool{ErrNeedMore: true}
							for _, w := range c.Events {
								dir := 0
								if strings.HasPrefix(w.Source, "192.0.2.53:") {
									dir = 1
								}
								raw := wrapperWire(t, w.InputWire)
								if transport == "tcp" {
									raw = append(binary.BigEndian.AppendUint16(nil, uint16(len(raw))), raw...)
								}
								if w.Error != "" {
									allowed[ProtocolErrorKind(w.Error)] = true
								}
								if transport == "tcp" && chunk == 0 && len(groups) > 0 && groups[len(groups)-1].dir == dir && groups[len(groups)-1].ts == w.Timestamp {
									groups[len(groups)-1].wire = append(groups[len(groups)-1].wire, raw...)
								} else {
									groups = append(groups, step{dir, w.Timestamp, raw})
								}
							}
							for _, g := range groups {
								size := chunk
								if size == 0 {
									size = len(g.wire)
								}
								for at := 0; at < len(g.wire); at += size {
									input := bytes.Clone(g.wire[at:min(at+size, len(g.wire))])
									o := s.Feed(g.dir, time.Unix(g.ts, 0), input)
									clear(input)
									if o.Err != nil {
										require.True(t, allowed[o.Err.Kind], "unexpected Feed error %v", o.Err)
									}
									appendEvents(o.Events)
								}
							}
							appendEvents(s.Close("EOF"))
							require.Empty(t, s.Close("again"))
							require.Zero(t, s.Stats().BufferedBytes)
							require.Len(t, originals, len(c.Events))
							for i, e := range originals {
								w := c.Events[i]
								if transport == "tcp" && w.QueryError == "" {
									w.Fields = w.LegacyFields
								}
								dnsContextQuery(t, e, w)
							}
							dnsContextCheck(t, c, events, deferred, false, transport)
						})
					}
				}
			}
		})
	}
}
