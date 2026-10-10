package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"testing"
	"time"
)

func testSTUNRetiredIdentityExistingAPI(t *testing.T) {
	for _, transport := range []string{"tcp", "udp"} {
		for _, scenario := range []string{"expired", "completed"} {
			t.Run(transport+"/"+scenario, func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(40000, 3478))
				require.NoError(t, err)
				defer s.Close("cleanup")
				q := stunTestMessage(1, 0, 1)
				r := stunTestMessage(1, 2, 1, stunTestAttr(0x0020, []byte{0, 1, 0xbd, 0x52, 0xe1, 0x12, 0xa6, 0x43}))
				first := s.Feed(0, time.Unix(100, 0), q)
				require.Len(t, first.Events, 1)
				at := int64(141)
				if scenario == "completed" {
					b := s.Feed(1, time.Unix(101, 0), r)
					require.Len(t, b.Events, 1)
					require.Equal(t, true, b.Events[0].Session["Matched"])
					at = 102
				}
				changed := stunTestMessage(1, 0, 1, stunTestAttr(0x8022, []byte("different-observed-query")))
				require.Len(t, s.Feed(0, time.Unix(at, 0), changed).Events, 1)
				late := s.Feed(1, time.Unix(at+1, 0), r)
				require.Len(t, late.Events, 1)
				require.NotEqual(t, true, late.Events[0].Session["Matched"], "old STUN response must not match a different request reusing an expired/completed ID")
				require.Zero(t, late.Events[0].ResponseTo)
				require.Zero(t, late.Events[0].TransactionID)
				fresh := s.Feed(0, time.Unix(at+2, 0), stunTestMessage(1, 0, 2))
				require.Len(t, fresh.Events, 1)
				normal := s.Feed(1, time.Unix(at+3, 0), stunTestMessage(1, 2, 2, stunTestAttr(0x0020, []byte{0, 1, 0xbd, 0x52, 0xe1, 0x12, 0xa6, 0x43})))
				require.Len(t, normal.Events, 1)
				require.Equal(t, true, normal.Events[0].Session["Matched"])
				require.Empty(t, s.Close("done"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}
func testSTUNMoreExistingAPI(t *testing.T) {
	for _, transport := range []string{"tcp", "udp"} {
		t.Run(transport+"/live-conflict", func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(40000, 3478))
			require.NoError(t, err)
			defer s.Close("cleanup")
			require.Len(t, s.Feed(0, time.Unix(100, 0), stunTestMessage(1, 0, 1)).Events, 1)
			bad := s.Feed(0, time.Unix(101, 0), stunTestMessage(1, 0, 1, stunTestAttr(0x8022, []byte("different"))))
			require.Len(t, bad.Events, 1)
			require.NotEmpty(t, bad.Events[0].Error)
			late := s.Feed(1, time.Unix(102, 0), stunTestMessage(1, 2, 1, stunTestAttr(0x20, []byte{0, 1, 0xbd, 0x52, 0xe1, 0x12, 0xa6, 0x43})))
			if transport == "tcp" {
				// Existing stream policy closes after desynchronization. Late
				// bytes cannot revive the closed stream or produce a pairing.
				require.Empty(t, late.Events)
			} else {
				require.Len(t, late.Events, 1)
				require.Equal(t, false, late.Events[0].Session["Matched"])
			}
			require.Empty(t, s.Close("done"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
		t.Run(transport+"/fields-ownership", func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(40000, 3478))
			require.NoError(t, err)
			defer s.Close("cleanup")
			ev := s.Feed(0, time.Unix(100, 0), stunTestMessage(1, 0, 1, stunTestAttr(0x8022, []byte("owned"))))
			require.Len(t, ev.Events, 1)
			e := ev.Events[0]
			f, err := e.GetFields()
			require.NoError(t, err)
			before, err := json.Marshal(f)
			require.NoError(t, err)
			f["Message Type"] = uint16(9999)
			if id, ok := f["Transaction ID"].([]byte); ok && len(id) > 0 {
				id[0] ^= 255
			}
			if attrs, ok := f["Attributes"]; ok {
				changed := false
				switch rows := attrs.(type) {
				case []any:
					if len(rows) > 0 {
						rows[0].(map[string]any)["Length"] = uint16(9999)
						changed = true
					}
				case []map[string]any:
					if len(rows) > 0 {
						rows[0]["Length"] = uint16(9999)
						changed = true
					}
				}
				require.True(t, changed, "nested attributes must actually be mutated")
			}
			if len(e.Raw) > 0 {
				e.Raw[0] ^= 255
			}
			if e.Session != nil {
				e.Session["Packet Name"] = "forged"
			}
			again, err := e.GetFields()
			require.NoError(t, err)
			after, err := json.Marshal(again)
			require.NoError(t, err)
			require.Equal(t, string(before), string(after))
			require.Empty(t, s.Close("done"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	t.Run("udp/byte-refusal", func(t *testing.T) {
		b := DefaultParserBudget()
		b.MaxMessageBytes = 128
		b.MaxFrameBytes = 128
		s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 3478))
		require.NoError(t, err)
		defer s.Close("cleanup")
		require.Len(t, s.Feed(0, time.Unix(100, 0), stunTestMessage(1, 0, 1)).Events, 1)
		deny := s.Feed(0, time.Unix(101, 0), stunTestMessage(1, 0, 1, stunTestAttr(0x8022, bytes.Repeat([]byte{'X'}, 512))))
		require.NotNil(t, deny.Err)
		require.Equal(t, ErrResourceExceeded, deny.Err.Kind)
		late := s.Feed(1, time.Unix(102, 0), stunTestMessage(1, 2, 1, stunTestAttr(0x20, []byte{0, 1, 0xbd, 0x52, 0xe1, 0x12, 0xa6, 0x43})))
		require.Len(t, late.Events, 1)
		require.NotEqual(t, true, late.Events[0].Session["Matched"])
		require.Empty(t, s.Close("done"))
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
}
func testSTUNInitialRefusalExistingAPI(t *testing.T) {
	b := DefaultParserBudget()
	b.MaxMessageBytes = 128
	b.MaxFrameBytes = 128
	s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 3478))
	require.NoError(t, err)
	defer s.Close("cleanup")
	deny := s.Feed(0, time.Unix(100, 0), stunTestMessage(1, 0, 1, stunTestAttr(0x8022, bytes.Repeat([]byte{'X'}, 512))))
	require.NotNil(t, deny.Err)
	require.Equal(t, ErrResourceExceeded, deny.Err.Kind)
	require.Len(t, s.Feed(0, time.Unix(101, 0), stunTestMessage(1, 0, 1)).Events, 1)
	late := s.Feed(1, time.Unix(102, 0), stunTestMessage(1, 2, 1, stunTestAttr(0x20, []byte{0, 1, 0xbd, 0x52, 0xe1, 0x12, 0xa6, 0x43})))
	require.Len(t, late.Events, 1)
	require.NotEqual(t, true, late.Events[0].Session["Matched"])
	require.Empty(t, s.Close("done"))
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}

func testSTUNIdentityMatrix(t *testing.T) {
	const folder = "stun-identity/"
	b, err := trafficfixture.ReadFile(folder + "controls.json")
	require.NoError(t, err)
	var doc struct {
		Schema string
		Cases  []snmpIdentityControl
	}
	require.NoError(t, json.Unmarshal(b, &doc))
	require.Equal(t, "owned-stun-identity/v1", doc.Schema)
	require.Len(t, doc.Cases, 28)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for _, c := range doc.Cases {
		t.Run(c.ID, func(t *testing.T) {
			ab, err := trafficfixture.ReadFile(folder + c.Answer)
			require.NoError(t, err)
			require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(ab)))
			var answer snmpIdentityControl
			require.NoError(t, json.Unmarshal(ab, &answer))
			require.Equal(t, c.Events, answer.Events)
			bindings := 0
			for _, batch := range all {
				for _, row := range batch.Cases {
					if row.ID == "stun-identity/"+c.ID {
						require.Equal(t, c.InputAlias, row.Input.OriginalPath)
						require.Equal(t, c.SHA256, row.Input.SHA256)
						require.Equal(t, c.Packets, row.Facts.PacketCount)
						bindings++
					}
				}
			}
			require.Equal(t, 1, bindings)
			raw, err := trafficfixture.ReadFile(folder + c.Capture)
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				var opts []CaptureOption
				if c.MaxMessageBytes > 0 {
					opts = append(opts, WithProtocolBudget(c.MaxMessageBytes, 1<<20))
				}
				events, stats := discoveryReplay(t, raw, workers, deferred, observe, opts...)
				require.Len(t, events, len(c.Events))
				ids := map[uint64]bool{}
				offsets := [2]uint64{}
				for i, e := range events {
					w := c.Events[i]
					require.NotZero(t, e.ID)
					require.False(t, ids[e.ID])
					ids[e.ID] = true
					require.NotZero(t, e.FlowID)
					require.Equal(t, events[0].FlowID, e.FlowID)
					require.Equal(t, events[0].Domain, e.Domain)
					require.Equal(t, c.Transport, e.Transport)
					require.Equal(t, w.Timestamp, e.Timestamp.Unix())
					require.Equal(t, w.Direction, e.Direction)
					require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
					require.Equal(t, len(w.InputWire)/2, e.Length)
					require.Equal(t, w.Reply, e.ResponseTo)
					require.Equal(t, w.Transaction, e.TransactionID)
					require.Len(t, e.SourceBytes.PacketRefs, 1)
					require.Equal(t, w.Refs[0], e.SourceBytes.PacketRefs[0].Number)
					protocol := "stun"
					if w.Session != nil && w.Session["TURN"] == true {
						protocol = "turn"
					}
					require.Equal(t, protocol, e.Protocol)
					profile := protocol
					if c.Transport == "udp" {
						profile = protocol + "-rfc8489-udp"
						if protocol == "turn" {
							profile = "turn-rfc8656-udp"
						}
					}
					require.Equal(t, profile, e.Profile)
					src, dst := "192.0.2.1:40000", "192.0.2.2:3478"
					dir := w.Direction
					if c.Transport == "udp" && c.ID == "udp-orphan" {
						dir = 1 - dir
					}
					if dir == 1 {
						src, dst = dst, src
					}
					require.Equal(t, src, e.Source)
					require.Equal(t, dst, e.Destination)
					if c.Transport == "tcp" {
						require.Equal(t, offsets[w.Direction], e.Offset)
						offsets[w.Direction] += uint64(e.Length)
					}
					if w.Error != "" {
						require.NotEmpty(t, e.Error)
						if w.Error == "ResourceExceeded" {
							require.Equal(t, "limited", e.Status)
						}
						if w.Error == "Desynchronized" {
							require.Equal(t, "malformed", e.Status)
						}
						if w.Error == "ContextRequired" {
							require.Equal(t, "context-required", e.Status)
						}
						if w.Fields == nil {
							f, err := e.GetFields()
							rocTypedError(t, w.Error, err)
							require.Nil(t, f)
							require.Nil(t, e.Session)
							continue
						}
					}
					if w.Error == "" {
						require.Empty(t, e.Error)
						status := "decoded"
						if deferred {
							status = "deferred"
						}
						require.Equal(t, status, e.Status)
					}
					rocEqualFields(t, w.Session, e.Session)
					f, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, w.Fields, f)
					f["Message Type"] = uint16(9999)
					if id, ok := f["Transaction ID"].([]byte); ok && len(id) > 0 {
						id[0] ^= 255
					}
					if attrs, ok := f["Attributes"]; ok {
						changed := false
						switch rows := attrs.(type) {
						case []any:
							if len(rows) > 0 {
								rows[0].(map[string]any)["Length"] = uint16(9999)
								changed = true
							}
						case []map[string]any:
							if len(rows) > 0 {
								rows[0]["Length"] = uint16(9999)
								changed = true
							}
						}
						require.True(t, changed, "nested attributes must actually be mutated")
					}
					if len(e.Raw) > 0 {
						e.Raw[0] ^= 255
					}
					if e.Session != nil {
						e.Session["Packet Name"] = "forged"
					}
					if e.Structured != nil {
						protocolFields(e.Structured)["Message Type"] = uint16(9999)
					}
					again, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, w.Fields, again)
				}
				require.Zero(t, stats.BufferedBytes)
				require.Zero(t, stats.CallbackPanics)
			})
		})
	}
}

func testSTUNIdentityBoundsAndDomains(t *testing.T) {
	t.Run("bounded-retired-slots-and-exact-live-retry", func(t *testing.T) {
		s := &binSTUN{}
		at := time.Unix(100, 0)
		_, err := s.consume(0, at, stunTestMessage(1, 0, 1), 2, false)
		require.NoError(t, err)
		_, err = s.consume(0, at, stunTestMessage(1, 0, 2), 2, false)
		require.NoError(t, err)
		before := s.bytes()
		f, err := s.consume(0, at, stunTestMessage(1, 0, 1), 2, false)
		require.NoError(t, err)
		require.Equal(t, true, f["Retransmission"])
		require.Equal(t, before, s.bytes())
		_, err = s.consume(0, at, stunTestMessage(1, 0, 3), 2, false)
		rocTypedError(t, "ResourceExceeded", err)
		require.Len(t, s.pending, 2)
		require.Len(t, s.history, 2)
		require.False(t, s.blocked)
		f, err = s.consume(1, at, stunTestMessage(1, 2, 2), 2, false)
		require.NoError(t, err)
		require.Equal(t, true, f["Matched"])
		f, err = s.consume(1, at, stunTestMessage(1, 2, 3), 2, false)
		require.NoError(t, err)
		require.Equal(t, false, f["Matched"])
		require.True(t, s.blocked)
		require.Empty(t, s.pending)
		require.Len(t, s.history, 2)
		require.Equal(t, int64(512+128*2), s.bytes())
	})
	t.Run("idle-keeps-capture-domain-and-identity", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"))
		require.NoError(t, err)
		defer s.Close("cleanup")
		a := s.(*captureSession).f.a
		feed := func(d CaptureDomain, dir int, ts int64, w []byte) *ProtocolEvent {
			src, dst := "192.0.2.1:40000", "192.0.2.2:3478"
			if dir == 1 {
				src, dst = dst, src
			}
			e := &ProtocolEvent{Source: src, Destination: dst, Domain: d, Timestamp: time.Unix(ts, 0), Transport: "udp"}
			require.True(t, a.decodeSTUNDatagram(e, w))
			require.Nil(t, e.sessionError)
			return e
		}
		d := CaptureDomain{Section: 1}
		q := feed(d, 0, 100, stunTestMessage(1, 0, 1))
		late := feed(d, 1, 701, stunTestMessage(1, 2, 1))
		require.Equal(t, q.FlowID, late.FlowID)
		require.Equal(t, false, late.Session["Matched"])
		feed(d, 0, 702, stunTestMessage(1, 0, 2))
		good := feed(d, 1, 703, stunTestMessage(1, 2, 2))
		require.Equal(t, true, good.Session["Matched"])
		for _, other := range []CaptureDomain{{Section: 2}, {Section: 1, Interface: 1}, {Section: 1, Encapsulation: "/vxlan:outer:2"}} {
			otherQ := feed(other, 0, 704, stunTestMessage(1, 0, 1))
			require.NotEqual(t, q.FlowID, otherQ.FlowID)
			otherR := feed(other, 1, 705, stunTestMessage(1, 2, 1))
			require.Equal(t, true, otherR.Session["Matched"])
		}
		require.Positive(t, s.Stats().BufferedBytes)
		require.Empty(t, s.Close("done"))
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
}

func testSTUNIdentityTCPChunks(t *testing.T) {
	b, err := trafficfixture.ReadFile("stun-identity/controls.json")
	require.NoError(t, err)
	var doc struct{ Cases []snmpIdentityControl }
	require.NoError(t, json.Unmarshal(b, &doc))
	for _, c := range doc.Cases {
		if c.Transport != "tcp" {
			continue
		}
		for _, chunk := range []int{1, 7, 64, 4096} {
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/chunk%d/deferred%t", c.ID, chunk, deferred), func(t *testing.T) {
					s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionPorts(40000, 3478))
					require.NoError(t, err)
					defer s.Close("cleanup")
					s.(*captureSession).f.a.config.Deferred = deferred
					for _, w := range c.Events {
						raw, err := hex.DecodeString(w.InputWire)
						require.NoError(t, err)
						var events []*ProtocolEvent
						for at := 0; at < len(raw); at += chunk {
							end := min(at+chunk, len(raw))
							part := append([]byte(nil), raw[at:end]...)
							batch := s.Feed(w.Direction, time.Unix(w.Timestamp, 0), part)
							clear(part)
							events = append(events, batch.Events...)
						}
						require.Len(t, events, 1)
						e := events[0]
						require.Empty(t, e.Error)
						require.Equal(t, w.Direction, e.Direction)
						require.Equal(t, w.Timestamp, e.Timestamp.Unix())
						require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
						require.Equal(t, w.Reply, e.ResponseTo)
						require.Equal(t, w.Transaction, e.TransactionID)
						rocEqualFields(t, w.Session, e.Session)
						f, err := e.GetFields()
						require.NoError(t, err)
						rocEqualFields(t, w.Fields, f)
					}
					require.Empty(t, s.Close("done"))
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
}

func testSTUNMalformedContextRetirement(t *testing.T) {
	s := &binSTUN{}
	at := time.Unix(100, 0)
	_, err := s.consume(0, at, stunTestMessage(1, 0, 1), 16, false)
	require.NoError(t, err)
	bad := stunTestMessage(1, 0, 2, stunTestAttr(0x20, []byte{0, 1, 0}))
	_, err = s.consume(0, at, bad, 16, false)
	require.Error(t, err)
	require.True(t, s.blocked)
	require.Empty(t, s.pending)
	require.Len(t, s.history, 1)
	require.Equal(t, int64(512+128), s.bytes())
	f, err := s.consume(1, at, stunTestMessage(1, 2, 1), 16, false)
	require.NoError(t, err)
	require.Equal(t, false, f["Matched"])
	require.Equal(t, "unverified-after-refusal", f["Association Status"])
	f, err = s.consume(0, at, stunTestMessage(1, 0, 3), 16, false)
	require.NoError(t, err)
	require.Equal(t, "ambiguous-transaction-id", f["Association Status"])
	require.Empty(t, s.pending)
}

func testSTUNUntrackedRefusalMemoryBound(t *testing.T) {
	b := DefaultParserBudget()
	b.MaxBufferedBytes = 256
	b.MaxFrameBytes = 128
	b.MaxMessageBytes = 128
	s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 3478))
	require.NoError(t, err)
	defer s.Close("cleanup")
	a := s.(*captureSession).f.a
	denied := s.Feed(0, time.Unix(100, 0), stunTestMessage(1, 0, 1, stunTestAttr(0x8022, bytes.Repeat([]byte{'X'}, 512))))
	require.NotNil(t, denied.Err)
	require.Equal(t, ErrResourceExceeded, denied.Err.Kind)
	require.True(t, a.stunUntrackedRefusal)
	require.Empty(t, a.udpSessions.entries)
	require.Zero(t, s.Stats().BufferedBytes)
	// Restoring a caller's available memory cannot recreate unseen wire history.
	a.config.MaxBufferedBytes = 1 << 20
	q := s.Feed(0, time.Unix(101, 0), stunTestMessage(1, 0, 1))
	require.Len(t, q.Events, 1)
	require.Equal(t, "ambiguous-transaction-id", q.Events[0].Session["Association Status"])
	r := s.Feed(1, time.Unix(102, 0), stunTestMessage(1, 2, 1))
	require.Len(t, r.Events, 1)
	require.Equal(t, false, r.Events[0].Session["Matched"])
	require.Empty(t, s.Close("done"))
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}
