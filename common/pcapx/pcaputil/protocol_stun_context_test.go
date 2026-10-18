package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
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

type stunContextEvent struct {
	InputWire                     string `json:"input_wire"`
	Raw, Protocol, Profile, Error string
	QueryError                    string `json:"query_error"`
	WireDirection                 int    `json:"wire_direction"`
	Direction, Interface, Port    int
	Timestamp                     int64 `json:"timestamp_sec"`
	FlowOwner                     int   `json:"flow_owner"`
	Offset                        uint64
	Refs                          []uint64 `json:"packet_refs"`
	Fields, Session               map[string]any
}
type stunContextCase struct {
	ID, Transport, Capture, SHA256 string
	MaxMessage                     int `json:"max_message_bytes"`
	MaxCollection                  int `json:"max_collection"`
	Events                         []stunContextEvent
}

func stunContextControls(t *testing.T) []stunContextCase {
	t.Helper()
	var d struct {
		Schema string
		Cases  []stunContextCase
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "stun-context", "STUN_CONTEXT_TEST_ROOT", "controls.json"), &d))
	require.Equal(t, "owned-stun-context/v1", d.Schema)
	require.Len(t, d.Cases, 42)
	return d.Cases
}

// JSON normalizes numbers and invalid UTF-8, so separately compare the exact
// bytes of legacy rule string fields carrying otherwise opaque attribute data.
func stunContextRawAttributes(t *testing.T, w []byte, fields map[string]any) {
	t.Helper()
	if len(w) < 20 || w[0]&0xc0 != 0 {
		return
	}
	var attrs []map[string]any
	switch v := fields["Attributes"].(type) {
	case []any:
		for _, a := range v {
			attrs = append(attrs, a.(map[string]any))
		}
	case []map[string]any:
		attrs = v
	}
	i := 0
	for at := 20; at < len(w); i++ {
		require.Less(t, i, len(attrs))
		ty := binary.BigEndian.Uint16(w[at:])
		n := int(binary.BigEndian.Uint16(w[at+2:]))
		end := at + 4 + n
		require.LessOrEqual(t, end, len(w))
		if ty != 0x20 && ty != 0x8022 && n > 0 {
			value, ok := attrs[i]["Octets"].(string)
			require.True(t, ok)
			require.Equal(t, w[at+4:end], []byte(value))
		}
		at = (end + 3) &^ 3
	}
	require.Len(t, attrs, i)
}
func stunContextQuery(t *testing.T, e *ProtocolEvent, w stunContextEvent) {
	t.Helper()
	// The immutable older SIP carrier recorded a generic limited-query error.
	// Native SIP now preserves a typed resource refusal; retain that stronger
	// assertion without changing the original carrier or application answers.
	if w.Protocol == "sip" && w.QueryError == "LegacyLimited" {
		w.QueryError = "ResourceExceeded"
	}
	f, err := e.GetFields()
	if w.QueryError != "" {
		if w.QueryError == "LegacyLimited" {
			require.EqualError(t, err, "protocol parser: event is limited, not an exact message")
		} else if w.QueryError == "Unrecognized" {
			require.EqualError(t, err, "protocol parser: event is unrecognized, not an exact message")
		} else {
			rocTypedError(t, w.QueryError, err)
			assertOwnedProtocolRefusal(t, e, w.QueryError, err)
		}
		require.Nil(t, f)
		out, x := e.Decode()
		require.Nil(t, out)
		if w.QueryError == "LegacyLimited" {
			require.EqualError(t, x, "protocol parser: event is limited, not an exact message")
		} else if w.QueryError == "Unrecognized" {
			require.EqualError(t, x, "protocol parser: event is unrecognized, not an exact message")
		} else {
			rocTypedError(t, w.QueryError, x)
		}
		return
	}
	require.NoError(t, err)
	rocEqualFields(t, w.Fields, f)
	stunContextRawAttributes(t, wrapperWire(t, w.InputWire), f)
	mutateRTPSQuery(f)
	clear(f)
	out, err := e.Decode()
	require.NoError(t, err)
	rocEqualFields(t, w.Fields, out["fields"].(map[string]any))
	rocEqualFields(t, w.Session, out["session"].(map[string]any))
	mutateRTPSQuery(out)
	clear(out)
	f, err = e.GetFields()
	require.NoError(t, err)
	rocEqualFields(t, w.Fields, f)
}
func stunContextCheck(t *testing.T, c stunContextCase, events []*ProtocolEvent, deferred, captured bool) {
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
		require.Equal(t, w.Offset, e.Offset)
		require.True(t, time.Unix(w.Timestamp, 0).Equal(e.Timestamp))
		require.Zero(t, e.TransactionID)
		require.Zero(t, e.ResponseTo)
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
				for _, id := range flows {
					require.NotEqual(t, id, e.FlowID)
				}
				flows[w.FlowOwner] = e.FlowID
			}
		}
		src, dst := fmt.Sprintf("192.0.2.1:%d", 40000+w.Port), "192.0.2.2:3478"
		if w.WireDirection == 1 {
			src, dst = dst, src
		}
		require.Equal(t, src, e.Source)
		require.Equal(t, dst, e.Destination)
		require.Equal(t, CaptureDomain{Interface: w.Interface}, e.Domain)
		if captured {
			require.Len(t, e.SourceBytes.PacketRefs, len(w.Refs))
			for j, ref := range w.Refs {
				require.Equal(t, ref, e.SourceBytes.PacketRefs[j].Number)
				require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[j].Domain)
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
		case "Unrecognized":
			status = "unrecognized"
		}
		require.Equal(t, status, e.Status)
		if w.Error != "" && w.Error != "Unrecognized" {
			require.Equal(t, w.Error, e.ExpertCode)
		}
		if w.Error == "" || w.Error == "Unrecognized" {
			require.Empty(t, e.Error)
		} else {
			require.NotEmpty(t, e.Error)
		}
		rocEqualFields(t, w.Session, e.Session)
		stunContextQuery(t, e, w)
		if w.Protocol == "sip" {
			continue
		} // carrier refusal sentinel; SIP semantics are not this profile
		for _, alias := range []string{"caller", "redis", "stun"} {
			tlsDTLSPoison(e, alias)
			stunContextQuery(t, e, w)
		}
	}
}

// In observer-on configurations, edit events immediately during delivery, then
// validate the untouched snapshots and the originals' private query results.
func stunContextReplay(t *testing.T, c stunContextCase, raw []byte, workers int, deferred, observe bool) ([]*ProtocolEvent, ProtocolStats) {
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
		if observe && e.Protocol != "sip" {
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
	if c.MaxMessage > 0 {
		opts = append(opts, WithProtocolBudget(c.MaxMessage, 1<<20))
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
	if observe {
		require.Equal(t, packets, seen.Load())
	}
	require.Len(t, originals, len(c.Events))
	for i, e := range originals {
		stunContextQuery(t, e, c.Events[i])
	}
	return snapshots, stats
}

func TestSTUNContextSealedMatrix(t *testing.T) {
	for _, c := range stunContextControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw := finsReadBatch(t, "stun-context", "STUN_CONTEXT_TEST_ROOT", c.Capture)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				events, stats := stunContextReplay(t, c, raw, workers, deferred, observe)
				stunContextCheck(t, c, events, deferred, true)
				require.Zero(t, stats.BufferedBytes)
				require.Zero(t, stats.CallbackPanics)
			})
		})
	}
}
func TestSTUNContextNativeChunks(t *testing.T) {
	for _, c := range stunContextControls(t) {
		if c.Transport != "tcp" {
			continue
		}
		t.Run(c.ID, func(t *testing.T) {
			for _, chunk := range []int{1, 2, 7, 64, 4096} {
				for _, deferred := range []bool{false, true} {
					t.Run(fmt.Sprintf("chunk%d/df%t", chunk, deferred), func(t *testing.T) {
						s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 3478))
						require.NoError(t, err)
						s.(*captureSession).f.a.config.Deferred = deferred
						var events []*ProtocolEvent
						for _, w := range c.Events {
							b := wrapperWire(t, w.InputWire)
							for at := 0; at < len(b); at += chunk {
								input := bytes.Clone(b[at:min(at+chunk, len(b))])
								o := s.Feed(w.WireDirection, time.Unix(w.Timestamp, 0), input)
								clear(input)
								if o.Err != nil {
									require.Equal(t, ErrNeedMore, o.Err.Kind)
								}
								events = append(events, o.Events...)
							}
						}
						require.Empty(t, s.Close("end"))
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
						stunContextCheck(t, c, events, deferred, false)
					})
				}
			}
		})
	}
}

// A STUN transaction does not establish an arbitrary TURN channel. The four
// bytes below are a complete RFC7252 empty ACK, not a truncated relay payload.
func TestSTUNChannelAdmissionNeighbors(t *testing.T) {
	var doc struct {
		Schema string
		Cases  []stunContextCase
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "stun-admission", "STUN_ADMISSION_TEST_ROOT", "controls.json"), &doc))
	require.Equal(t, "owned-stun-admission/v1", doc.Schema)
	require.Len(t, doc.Cases, 2)
	for _, c := range doc.Cases {
		t.Run(c.ID, func(t *testing.T) {
			raw := finsReadBatch(t, "stun-admission", "STUN_ADMISSION_TEST_ROOT", c.Capture)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				events, stats := discoveryReplay(t, raw, workers, deferred, observe)
				require.Len(t, events, len(c.Events))
				ids := map[uint64]bool{}
				var owner uint64
				for i, w := range c.Events {
					e := events[i]
					require.Equal(t, w.Protocol, e.Protocol)
					require.Equal(t, w.Profile, e.Profile)
					require.Equal(t, "udp", e.Transport)
					require.Equal(t, w.Direction, e.Direction)
					require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
					require.Equal(t, len(w.InputWire)/2, e.Length)
					require.True(t, time.Unix(w.Timestamp, 0).Equal(e.Timestamp))
					require.Empty(t, e.Error)
					require.Empty(t, e.ExpertCode)
					status := "decoded"
					if deferred {
						status = "deferred"
					}
					require.Equal(t, status, e.Status)
					require.NotZero(t, e.ID)
					require.False(t, ids[e.ID])
					ids[e.ID] = true
					require.Zero(t, e.ResponseTo)
					require.Zero(t, e.TransactionID)
					require.Equal(t, CaptureDomain{}, e.Domain)
					require.Len(t, e.SourceBytes.PacketRefs, 1)
					require.Equal(t, w.Refs[0], e.SourceBytes.PacketRefs[0].Number)
					src, dst := "192.0.2.1:40000", "192.0.2.2:3478"
					if w.WireDirection == 1 {
						src, dst = dst, src
					}
					require.Equal(t, src, e.Source)
					require.Equal(t, dst, e.Destination)
					rocEqualFields(t, w.Session, e.Session)
					if w.Protocol == "stun" {
						require.NotZero(t, e.FlowID)
						if owner == 0 {
							owner = e.FlowID
						} else {
							require.Equal(t, owner, e.FlowID)
						}
						stunContextQuery(t, e, w)
					} else {
						require.Zero(t, e.FlowID)
						fields, err := e.GetFields()
						require.NoError(t, err)
						rocEqualFields(t, w.Fields, fields)
						out, err := e.Decode()
						require.NoError(t, err)
						rocEqualFields(t, w.Fields, out["fields"].(map[string]any))
						rocEqualFields(t, w.Session, out["session"].(map[string]any))
					}
				}
				require.Zero(t, stats.BufferedBytes)
				require.Zero(t, stats.CallbackPanics)
			})
		})
	}
}
