package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"io"
	"sync/atomic"
	"testing"
	"time"
)

type snmpClockEvent struct {
	Raw, Protocol, Profile, Error string
	InputWire                     string `json:"input_wire"`
	Direction, Interface, Port    int
	FlowOwner                     int      `json:"flow_owner"`
	Timestamp                     int64    `json:"timestamp_sec"`
	Refs                          []uint64 `json:"packet_refs"`
	Fields, Session               map[string]any
}
type snmpClockCase struct {
	ID, Transport, Capture, SHA256 string
	MaxMessage                     int `json:"max_message_bytes"`
	MaxCollection                  int `json:"max_collection"`
	Explicit                       bool
	Events                         []snmpClockEvent
}

func snmpClockControls(t *testing.T) []snmpClockCase {
	t.Helper()
	var c struct {
		Schema string
		Cases  []snmpClockCase
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "snmp-clock", "SNMP_CLOCK_TEST_ROOT", "controls.json"), &c))
	require.Equal(t, "owned-snmp-clock/v1", c.Schema)
	require.Len(t, c.Cases, 51)
	return c.Cases
}
func snmpClockQuery(t *testing.T, e *ProtocolEvent, w snmpClockEvent) {
	t.Helper()
	fields, err := e.GetFields()
	rocTypedError(t, w.Error, err)
	if w.Error != "" {
		require.Nil(t, fields)
		assertOwnedProtocolRefusal(t, e, w.Error, err)
		out, err := e.Decode()
		rocTypedError(t, w.Error, err)
		require.Nil(t, out)
		return
	}
	rocEqualFields(t, w.Fields, fields)
	if rows, ok := fields["Variable Bindings"].([]map[string]any); ok {
		for _, r := range rows {
			r["OID"] = "9.9"
			if b, ok := r["Value"].([]byte); ok {
				clear(b)
			}
		}
	}
	clear(fields)
	out, err := e.Decode()
	require.NoError(t, err)
	rocEqualFields(t, w.Fields, out["fields"].(map[string]any))
	rocEqualFields(t, w.Session, out["session"].(map[string]any))
	clear(out["fields"].(map[string]any))
	clear(out["session"].(map[string]any))
}
func snmpClockCheck(t *testing.T, c snmpClockCase, events []*ProtocolEvent, deferred, captured bool) {
	t.Helper()
	require.Len(t, events, len(c.Events))
	ids := map[uint64]bool{}
	flows := map[int]uint64{}
	for i, w := range c.Events {
		e := events[i]
		require.Equal(t, w.Protocol, e.Protocol)
		require.Equal(t, w.Profile, e.Profile)
		require.Equal(t, c.Transport, e.Transport)
		require.Equal(t, w.Direction, e.Direction)
		require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
		require.Equal(t, len(w.InputWire)/2, e.Length)
		require.True(t, time.Unix(w.Timestamp, 0).Equal(e.Timestamp))
		require.Zero(t, e.ResponseTo)
		require.Zero(t, e.TransactionID)
		require.NotZero(t, e.ID)
		require.False(t, ids[e.ID])
		ids[e.ID] = true
		if w.FlowOwner < 0 {
			require.Zero(t, e.FlowID)
		} else {
			require.NotZero(t, e.FlowID)
			if id, ok := flows[w.FlowOwner]; ok {
				require.Equal(t, id, e.FlowID)
			} else {
				for _, old := range flows {
					require.NotEqual(t, old, e.FlowID)
				}
				flows[w.FlowOwner] = e.FlowID
			}
		}
		src, dst := fmt.Sprintf("192.0.2.1:%d", 40000+w.Port), "192.0.2.2:161"
		if w.Direction == 1 {
			src, dst = dst, src
		}
		require.Equal(t, src, e.Source)
		require.Equal(t, dst, e.Destination)
		require.Equal(t, CaptureDomain{Interface: w.Interface}, e.Domain)
		if captured {
			require.Len(t, e.SourceBytes.PacketRefs, len(w.Refs))
			for j, r := range e.SourceBytes.PacketRefs {
				require.Equal(t, w.Refs[j], r.Number)
				require.Equal(t, e.Domain, r.Domain)
			}
		}
		status := "decoded"
		if deferred {
			status = "deferred"
		}
		switch w.Error {
		case "ResourceExceeded":
			status = "limited"
		case "MalformedMessage":
			status = "malformed"
		case "ContextRequired":
			status = "context-required"
		}
		require.Equal(t, status, e.Status)
		if w.Error == "" {
			require.Empty(t, e.Error)
		} else {
			require.NotEmpty(t, e.Error)
		}
		rocEqualFields(t, w.Session, e.Session)
		snmpClockQuery(t, e, w)
		for _, alias := range []string{"caller", "redis", "snmp"} {
			tlsDTLSPoison(e, alias)
			snmpClockQuery(t, e, w)
		}
	}
}

// Capture byte limits are public; the fixed collection test limit is applied
// before any input, as in other collection-budget unit tests. All packets still
// pass through the existing public reader/PacketAnalyzer API and real callbacks.
func snmpClockPressureReplay(t *testing.T, raw []byte, workers int, deferred, observe bool, limit int, extra ...CaptureOption) ([]*ProtocolEvent, ProtocolStats) {
	t.Helper()
	var events []*ProtocolEvent
	var stats ProtocolStats
	var seen atomic.Int64
	opts := append(extra, WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }))
	if observe {
		opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen.Add(1) }))
	}
	a, err := NewPacketAnalyzer(opts...)
	require.NoError(t, err)
	a.conf.binParser.budget.MaxCollectionElements = limit
	r, err := NewCaptureReader(bytes.NewReader(raw))
	require.NoError(t, err)
	for {
		b, ci, err := r.ReadPacketData()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		require.NoError(t, a.Feed(b, ci, r.LinkType()))
		clear(b)
	}
	require.NoError(t, a.Close())
	require.NoError(t, a.Close())
	require.Zero(t, stats.BufferedBytes)
	require.Zero(t, stats.CallbackPanics)
	if observe {
		require.Positive(t, seen.Load())
	}
	return events, stats
}
func TestSNMPContextClockSealedMatrix(t *testing.T) {
	for _, c := range snmpClockControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw := finsReadBatch(t, "snmp-clock", "SNMP_CLOCK_TEST_ROOT", c.Capture)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				var extra []CaptureOption
				if c.MaxMessage > 0 {
					extra = append(extra, WithProtocolBudget(c.MaxMessage, 1<<20))
				}

				if c.Explicit {
					extra = append(extra, WithProtocolDecodeAs("udp", 161, "snmp"))
				}
				var events []*ProtocolEvent
				var stats ProtocolStats
				if c.MaxCollection > 0 {
					events, stats = snmpClockPressureReplay(t, raw, workers, deferred, observe, c.MaxCollection, extra...)
				} else {
					events, stats = discoveryReplay(t, raw, workers, deferred, observe, extra...)
				}
				snmpClockCheck(t, c, events, deferred, true)
				require.Zero(t, stats.BufferedBytes)
			})
		})
	}
}
func TestSNMPContextNativeChunks(t *testing.T) {
	for _, c := range snmpClockControls(t) {
		if c.Transport != "tcp" {
			continue
		}
		t.Run(c.ID, func(t *testing.T) {
			for _, chunk := range []int{1, 2, 7, 64, 4096} {
				for _, deferred := range []bool{false, true} {
					t.Run(fmt.Sprintf("chunk%d/df%t", chunk, deferred), func(t *testing.T) {
						s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 161))
						require.NoError(t, err)
						s.(*captureSession).f.a.config.Deferred = deferred
						var events []*ProtocolEvent
						for _, w := range c.Events {
							b := wrapperWire(t, w.InputWire)
							for at := 0; at < len(b); at += chunk {
								input := bytes.Clone(b[at:min(at+chunk, len(b))])
								o := s.Feed(w.Direction, time.Unix(w.Timestamp, 0), input)
								clear(input)
								if o.Err != nil {
									require.Equal(t, ErrNeedMore, o.Err.Kind)
								}
								events = append(events, o.Events...)
							}
						}
						require.Empty(t, s.Close("end"))
						require.Empty(t, s.Close("again"))
						snmpClockCheck(t, c, events, deferred, false)
						require.Zero(t, s.Stats().BufferedBytes)
					})
				}
			}
		})
	}
}

// Invalid BER must terminate this TCP affinity even when a callback replaces
// publicly editable routing/recovery projections. The following request shares
// the invalid PDU's TCP segment; preserving a packet boundary is insufficient.
func TestSNMPCallbackRecoverySealedMatrix(t *testing.T) {
	var c snmpClockCase
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "snmp-clock", "SNMP_CLOCK_TEST_ROOT", "callback.json"), &c))
	raw := finsReadBatch(t, "snmp-clock", "SNMP_CLOCK_TEST_ROOT", c.Capture)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
	for _, edit := range []bool{false, true} {
		t.Run(fmt.Sprintf("edit%t", edit), func(t *testing.T) {
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				var originals, snapshots []*ProtocolEvent
				callback := WithOnProtocolMessage(func(e *ProtocolEvent) {
					owned := *e
					owned.Session = cloneSession(e.Session)
					owned.Fields = cloneSession(e.Fields)
					owned.Raw = bytes.Clone(e.Raw)
					snapshots = append(snapshots, &owned)
					originals = append(originals, e)
					if edit && e.Status == "malformed" {
						e.Protocol = "http2"
						e.Profile = "caller"
						e.Session = map[string]any{"Error Scope": "stream"}
						e.Fields = map[string]any{"caller": true}
						e.Structured = map[string]any{"fields": e.Fields}
						clear(e.Raw)
					}
				})
				var stats ProtocolStats
				var seen atomic.Int64
				opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), callback, WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
				if observe {
					opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen.Add(1) }))
				}
				require.NoError(t, ReplayPcap(bytes.NewReader(raw), opts...))
				require.Zero(t, stats.BufferedBytes)
				require.Zero(t, stats.CallbackPanics)
				if observe {
					require.Positive(t, seen.Load())
				}
				snmpClockCheck(t, c, snapshots, deferred, true)
				var offset uint64
				for i, e := range snapshots {
					require.Equal(t, offset, e.Offset)
					offset += uint64(e.Length)
					snmpClockQuery(t, originals[i], c.Events[i])
					require.Equal(t, "reassembled", e.SourceBytes.Kind)
					require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(wrapperWire(t, c.Events[i].Raw))), e.SourceBytes.SHA256)
				}
			})
		})
	}
}
