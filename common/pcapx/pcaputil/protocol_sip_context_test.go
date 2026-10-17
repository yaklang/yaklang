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

type sipContextEvent struct {
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
type sipContextCase struct {
	ID, Transport, Capture, SHA256 string
	MaxMessage                     int `json:"max_message_bytes"`
	MaxCollection                  int `json:"max_collection"`
	Events                         []sipContextEvent
}

func sipContextControls(t *testing.T) []sipContextCase {
	t.Helper()
	var d struct {
		Schema string
		Cases  []sipContextCase
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "sip-context", "SIP_CONTEXT_TEST_ROOT", "controls.json"), &d))
	require.Equal(t, "owned-sip-context/v1", d.Schema)
	require.Len(t, d.Cases, 48)
	return d.Cases
}

func sipContextQuery(t *testing.T, e *ProtocolEvent, w sipContextEvent) {
	t.Helper()
	stunContextQuery(t, e, stunContextEvent(w))
}
func sipContextCheck(t *testing.T, c sipContextCase, events []*ProtocolEvent, deferred, captured bool) {
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
		src, dst := fmt.Sprintf("192.0.2.1:%d", 40000+w.Port), "192.0.2.2:5060"
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
		sipContextQuery(t, e, w)
		for _, alias := range []string{"caller", "redis", "sip"} {
			tlsDTLSPoison(e, alias)
			sipContextQuery(t, e, w)
		}
	}
}

// In observer-on configurations, edit events immediately during delivery, then
// validate the untouched snapshots and the originals' private query results.
func sipContextReplay(t *testing.T, c sipContextCase, raw []byte, workers int, deferred, observe bool) ([]*ProtocolEvent, ProtocolStats) {
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
		sipContextQuery(t, e, c.Events[i])
	}
	return snapshots, stats
}

func TestSIPContextSealedMatrix(t *testing.T) {
	for _, c := range sipContextControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw := finsReadBatch(t, "sip-context", "SIP_CONTEXT_TEST_ROOT", c.Capture)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				events, stats := sipContextReplay(t, c, raw, workers, deferred, observe)
				sipContextCheck(t, c, events, deferred, true)
				require.Zero(t, stats.BufferedBytes)
				require.Zero(t, stats.CallbackPanics)
			})
		})
	}
}
func TestSIPContextNativeChunks(t *testing.T) {
	for _, c := range sipContextControls(t) {
		if c.Transport != "tcp" {
			continue
		}
		t.Run(c.ID, func(t *testing.T) {
			for _, chunk := range []int{1, 2, 7, 64, 4096} {
				for _, deferred := range []bool{false, true} {
					t.Run(fmt.Sprintf("chunk%d/df%t", chunk, deferred), func(t *testing.T) {
						s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 5060))
						require.NoError(t, err)
						s.(*captureSession).f.a.config.Deferred = deferred
						var events []*ProtocolEvent
						for _, w := range c.Events {
							b := wrapperWire(t, w.InputWire)
							for at := 0; at < len(b); at += chunk {
								input := bytes.Clone(b[at:min(at+chunk, len(b))])
								o := s.Feed(w.WireDirection, time.Unix(w.Timestamp, 0), input)
								clear(input)
								if at+chunk >= len(b) {
									if w.Error != "" {
										require.NotNil(t, o.Err)
										require.Equal(t, ProtocolErrorKind(w.Error), o.Err.Kind)
									} else {
										require.Nil(t, o.Err)
									}
								} else if o.Err != nil {
									require.Equal(t, ErrNeedMore, o.Err.Kind)
								}
								events = append(events, o.Events...)
							}
						}
						require.Empty(t, s.Close("end"))
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
						sipContextCheck(t, c, events, deferred, false)
					})
				}
			}
		})
	}
}

// A retired/conflicting ACK must be refused before it can infer a dialog ACK
// or mutate BYE/ACK state. Literal RFC3261 fields are independent of the parser.
func TestSIPRetiredACKHasNoDialogAction(t *testing.T) {
	q := sipMsg("INVITE sip:bob@example.test SIP/2.0", [][2]string{{"Via", "SIP/2.0/UDP client.example.test;branch=z9hG4bKcontext"}, {"From", "<sip:alice@example.test>;tag=a"}, {"To", "<sip:bob@example.test>"}, {"Call-ID", "clock@example.test"}, {"CSeq", "1 INVITE"}}, "")
	r := sipResp("200", "OK", "z9hG4bKcontext", "1 INVITE", "clock@example.test", "<sip:alice@example.test>;tag=a", "<sip:bob@example.test>;tag=b")
	ack := sipMsg("ACK sip:bob@example.test SIP/2.0", [][2]string{{"Via", "SIP/2.0/UDP client.example.test;branch=z9hG4bKack"}, {"From", "<sip:alice@example.test>;tag=a"}, {"To", "<sip:bob@example.test>;tag=b"}, {"Call-ID", "clock@example.test"}, {"CSeq", "1 ACK"}}, "")
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 5060))
	require.NoError(t, err)
	for i, w := range [][]byte{q, r, ack, bytes.Replace(ack, []byte("sip:bob@example.test SIP/2.0"), []byte("sip:eve@example.test SIP/2.0"), 1)} {
		dir := 0
		if i == 1 {
			dir = 1
		}
		o := s.Feed(dir, time.Unix(int64(i+1), 0), w)
		require.Nil(t, o.Err)
		require.Len(t, o.Events, 1)
		if i < 2 {
			continue
		}
		e := o.Events[0]
		want := map[string]any{"Call-ID": "clock@example.test", "CSeq": 1, "CSeq Method": "ACK", "From": "<sip:alice@example.test>;tag=a", "To": "<sip:bob@example.test>;tag=b", "From Tag": "a", "To Tag": "b", "Via": "SIP/2.0/UDP client.example.test;branch=z9hG4bKack", "Via Branch": "z9hG4bKack", "Content Type": "", "Context Level": "observed", "Packet Name": "ACK", "Method": "ACK", "URI": "sip:bob@example.test", "Dialog Identity": map[string]any{"Call-ID": "clock@example.test", "Tag A": "a", "Tag B": "b"}, "Dialog State": "confirmed", "ACK For INVITE": "2xx-dialog", "Association Status": "invite-2xx-ack", "INVITE CSeq": 1, "Observation Scope": "conversation", "Transport": "UDP"}
		if i == 3 {
			delete(want, "ACK For INVITE")
			delete(want, "INVITE CSeq")
			want["URI"] = "sip:eve@example.test"
			want["Context Level"] = "partial"
			want["Association Status"] = "retired-or-conflicting-identity"
		}
		require.Equal(t, w, e.Raw)
		require.Zero(t, e.Direction)
		require.Zero(t, e.TransactionID)
		require.Zero(t, e.ResponseTo)
		rocEqualFields(t, want, e.Session)
		fields, x := e.GetFields()
		require.NoError(t, x)
		rocEqualFields(t, want, fields)
		clear(fields)
		out, x := e.Decode()
		require.NoError(t, x)
		rocEqualFields(t, want, out["session"].(map[string]any))
	}
	s.Close("done")
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}
