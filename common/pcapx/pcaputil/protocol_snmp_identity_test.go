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

func testSNMPOversizeRefusalExistingAPI(t *testing.T) {
	b := DefaultParserBudget()
	b.MaxMessageBytes = 128
	b.MaxFrameBytes = 128
	s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 161))
	require.NoError(t, err)
	defer s.Close("cleanup")
	q := s.Feed(0, time.Unix(100, 0), snmpV1Get())
	require.Len(t, q.Events, 1)
	denied := snmpCommunityMessage(0, []byte("public"), snmpPDU(0xa3, 1, 0, 0, snmpVarBind(snmpSysDescr(), snmpEncOctet(bytes.Repeat([]byte{'X'}, 512)))))
	r := s.Feed(0, time.Unix(101, 0), denied)
	require.NotNil(t, r.Err)
	require.Equal(t, ErrResourceExceeded, r.Err.Kind)
	late := s.Feed(1, time.Unix(102, 0), snmpV1Response(1, "late"))
	require.Len(t, late.Events, 1)
	require.NotContains(t, late.Events[0].Session, "Matched Request", "byte refusal must retire association before a late response")
	require.Zero(t, late.Events[0].ResponseTo)
	require.Zero(t, late.Events[0].TransactionID)
	require.Empty(t, s.Close("done"))
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
	s, err = NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 161))
	require.NoError(t, err)
	defer s.Close("cleanup")
	require.Len(t, s.Feed(0, time.Unix(100, 0), snmpV1Get()).Events, 1)
	normal := s.Feed(1, time.Unix(101, 0), snmpV1Response(1, "normal"))
	require.Len(t, normal.Events, 1)
	require.Equal(t, "GetRequest", normal.Events[0].Session["Matched Request"])
	require.Empty(t, s.Close("normal"))
	require.Zero(t, s.Stats().BufferedBytes)
}

// The same request-id cannot distinguish a completed/expired request from its
// later reuse, and a different PDU under a live key is not an exact retry.
func testSNMPIdentityExistingAPI(t *testing.T) {
	makeWire := func(version int64, tag byte, id int64, value []byte) []byte {
		pdu := snmpPDU(tag, id, 0, 0, snmpVarBind(snmpSysDescr(), value))
		if version != 3 {
			return snmpCommunityMessage(version, []byte("public"), pdu)
		}
		flags := byte(0)
		if tag != 0xa2 {
			flags = snmpFlagReportable
		}
		return snmpV3(id, 65507, flags, snmpPlainUSM(), snmpScoped(nil, nil, pdu))
	}
	for _, version := range []int64{0, 1, 3} {
		for _, transport := range []string{"tcp", "udp"} {
			for _, scenario := range []string{"conflict", "completed", "expired"} {
				name := snmpVersionName(version) + "/" + transport + "/" + scenario
				t.Run(name, func(t *testing.T) {
					s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(40000, 161))
					require.NoError(t, err)
					defer s.Close("cleanup")
					feed := func(dir int, at int64, w []byte) *ProtocolEvent {
						b := s.Feed(dir, time.Unix(at, 0), w)
						require.Len(t, b.Events, 1)
						return b.Events[0]
					}
					a := makeWire(version, 0xa0, 7, []byte{5, 0})
					b := makeWire(version, 0xa3, 7, snmpEncOctet([]byte("fresh")))
					r := makeWire(version, 0xa2, 7, snmpEncOctet([]byte("old")))
					first := feed(0, 100, a)
					require.Equal(t, "GetRequest", first.Session["Packet Name"])
					at := int64(101)
					if scenario == "completed" {
						normal := feed(1, 100, r)
						require.Equal(t, "GetRequest", normal.Session["Matched Request"])
					}
					if scenario == "expired" {
						at = 701
					}
					feed(0, at, b)
					late := feed(1, at, r)
					require.NotContains(t, late.Session, "Matched Request", "old response must not become SetRequest response")
					require.Zero(t, late.ResponseTo)
					require.Zero(t, late.TransactionID)
					// A distinct, fully observed identity must still work.
					feed(0, at, makeWire(version, 0xa0, 8, []byte{5, 0}))
					good := feed(1, at, makeWire(version, 0xa2, 8, snmpEncOctet([]byte("next"))))
					require.Equal(t, "GetRequest", good.Session["Matched Request"])
					require.Empty(t, s.Close("done"))
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
}

func testSNMPV3FullContextExistingAPI(t *testing.T) {
	for _, transport := range []string{"tcp", "udp"} {
		for _, mismatch := range []string{"context-engine", "context-name", "security-level"} {
			t.Run(transport+"/"+mismatch, func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(40000, 161))
				require.NoError(t, err)
				defer s.Close("cleanup")
				q := snmpV3(7, 65507, snmpFlagReportable, snmpPlainUSM(), snmpScoped(nil, nil, snmpPDU(0xa0, 7, 0, 0, snmpVarBind(snmpSysDescr(), []byte{5, 0}))))
				qp := s.Feed(0, time.Unix(100, 0), q)
				require.Len(t, qp.Events, 1)
				engine, name := []byte(nil), []byte(nil)
				flags := byte(0)
				usm := snmpPlainUSM()
				switch mismatch {
				case "context-engine":
					engine = []byte{1}
				case "context-name":
					name = []byte("other")
				case "security-level":
					flags = snmpFlagAuth
					usm = snmpUSM([]byte{0x80, 0, 0, 0, 1}, 1, 42, []byte("user"), make([]byte, 12), nil)
				}
				r := snmpV3(7, 65507, flags, usm, snmpScoped(engine, name, snmpPDU(0xa2, 7, 0, 0, snmpVarBind(snmpSysDescr(), snmpEncOctet([]byte("wrong"))))))
				rp := s.Feed(1, time.Unix(101, 0), r)
				require.Len(t, rp.Events, 1)
				require.NotContains(t, rp.Events[0].Session, "Matched Request", "mismatched full scoped/security context must not match")
				goodWire := snmpV3(7, 65507, 0, snmpPlainUSM(), snmpScoped(nil, nil, snmpPDU(0xa2, 7, 0, 0, snmpVarBind(snmpSysDescr(), snmpEncOctet([]byte("right"))))))
				good := s.Feed(1, time.Unix(102, 0), goodWire)
				require.Len(t, good.Events, 1)
				// A passive unmatched observation cannot consume another complete
				// key's live context. No authenticated endpoint success is inferred.
				require.Equal(t, "GetRequest", good.Events[0].Session["Matched Request"])
				require.Empty(t, s.Close("done"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}

func testSNMPFieldsOwnershipExistingAPI(t *testing.T) {
	for _, transport := range []string{"tcp", "udp"} {
		t.Run(transport, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(40000, 161))
			require.NoError(t, err)
			defer s.Close("cleanup")
			q := snmpV1Get()
			b := s.Feed(0, time.Unix(100, 0), q)
			require.Len(t, b.Events, 1)
			for i := range q {
				q[i] = 0
			}
			r := s.Feed(1, time.Unix(101, 0), snmpV1Response(1, "owned"))
			require.Len(t, r.Events, 1)
			e := r.Events[0]
			fields, err := e.GetFields()
			require.NoError(t, err)
			original := cloneSession(fields)
			fields["Matched Request"] = "forged"
			fields["Variable Bindings"].([]map[string]any)[0]["Value"].([]byte)[0] = 'X'
			again, err := e.GetFields()
			require.NoError(t, err)
			require.Equal(t, original, again, "a returned field tree cannot mutate private decoded fields")
			e.Session["Request ID"] = int64(999999)
			if e.Fields != nil {
				e.Fields["Request ID"] = int64(999999)
			}
			if e.Structured != nil {
				protocolFields(e.Structured)["Request ID"] = int64(999999)
			}
			again, err = e.GetFields()
			require.NoError(t, err)
			require.Equal(t, original, again, "public projections cannot mutate subsequent queries")
			require.Empty(t, s.Close("done"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

type snmpIdentityControl struct {
	ID, Protocol, Profile, Transport, Capture, Answer, SHA256 string
	AnswerSHA                                                 string `json:"answer_sha256"`
	InputAlias                                                string `json:"input_alias"`
	Packets                                                   int
	MaxMessageBytes                                           int `json:"max_message_bytes"`
	Events                                                    []struct {
		Raw, Error, State string
		InputWire         string `json:"input_wire"`
		Direction         int
		Timestamp         int64 `json:"timestamp_sec"`
		Fields, Session   map[string]any
		Reply             uint64   `json:"response_to"`
		Transaction       uint64   `json:"transaction_id"`
		Refs              []uint64 `json:"packet_refs"`
	}
}

func testSNMPIdentitySealedMatrix(t *testing.T) {
	const folder = "snmp-identity/"
	b, err := trafficfixture.ReadFile(folder + "controls.json")
	require.NoError(t, err)
	var d struct {
		Schema string
		Cases  []snmpIdentityControl
	}
	require.NoError(t, json.Unmarshal(b, &d))
	require.Equal(t, "owned-snmp-identity/v1", d.Schema)
	require.Len(t, d.Cases, 50)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for _, c := range d.Cases {
		t.Run(c.ID, func(t *testing.T) {
			ab, err := trafficfixture.ReadFile(folder + c.Answer)
			require.NoError(t, err)
			require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(ab)))
			var a snmpIdentityControl
			require.NoError(t, json.Unmarshal(ab, &a))
			require.Equal(t, c.Events, a.Events)
			bound := 0
			for _, batch := range all {
				for _, row := range batch.Cases {
					if row.ID == "snmp-identity/"+c.ID {
						require.Equal(t, c.InputAlias, row.Input.OriginalPath)
						require.Equal(t, c.SHA256, row.Input.SHA256)
						require.Equal(t, c.Packets, row.Facts.PacketCount)
						bound++
					}
				}
			}
			require.Equal(t, 1, bound)
			raw, err := trafficfixture.ReadFile(folder + c.Capture)
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				var extra []CaptureOption
				if c.MaxMessageBytes > 0 {
					extra = append(extra, WithProtocolBudget(c.MaxMessageBytes, 1<<20))
				}
				events, stats := discoveryReplay(t, raw, workers, deferred, observe, extra...)
				require.Len(t, events, len(c.Events))
				ids := map[uint64]bool{}
				for i, e := range events {
					w := c.Events[i]
					require.NotZero(t, e.ID)
					require.False(t, ids[e.ID])
					ids[e.ID] = true
					require.Equal(t, c.Protocol, e.Protocol)
					require.Equal(t, c.Profile, e.Profile)
					require.Equal(t, c.Transport, e.Transport)
					require.Equal(t, w.Direction, e.Direction)
					require.Equal(t, w.Timestamp, e.Timestamp.Unix())
					require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
					require.Equal(t, events[0].FlowID, e.FlowID)
					require.Equal(t, events[0].Domain, e.Domain)
					src, dst := "192.0.2.1:40000", "192.0.2.2:161"
					if w.Direction == 1 {
						src, dst = dst, src
					}
					require.Equal(t, src, e.Source)
					require.Equal(t, dst, e.Destination)
					require.Len(t, e.SourceBytes.PacketRefs, 1)
					require.Equal(t, w.Refs[0], e.SourceBytes.PacketRefs[0].Number)
					require.Equal(t, w.Reply, e.ResponseTo)
					require.Equal(t, w.Transaction, e.TransactionID)
					fields, err := e.GetFields()
					rocTypedError(t, w.Error, err)
					require.Equal(t, len(w.InputWire)/2, e.Length)
					if w.Error != "" {
						require.Nil(t, fields)
						require.Nil(t, e.Session)
						require.Nil(t, e.Structured)
						require.Equal(t, "limited", e.Status)
						require.NotEmpty(t, e.Error)
						continue
					}
					require.Empty(t, e.Error)
					rocEqualFields(t, w.Fields, fields)
					rocEqualFields(t, w.Session, e.Session)
					fields["Request ID"] = int64(999999)
					binds := fields["Variable Bindings"].([]map[string]any)
					binds[0]["OID"] = "9.9"
					if v, ok := binds[0]["Value"].([]byte); ok && len(v) > 0 {
						v[0] ^= 255
					}
					e.Session["Matched Request"] = "forged"
					if e.Structured != nil {
						protocolFields(e.Structured)["Request ID"] = int64(999999)
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

func testSNMPIdentityBudgetAndDomains(t *testing.T) {
	t.Run("retained-capacity-preserves-live-retry-and-rejects-reuse", func(t *testing.T) {
		p := &binSNMP{}
		at := time.Unix(100, 0)
		f, err := p.consumeAt(snmpV1Get(), 16, 0, at)
		require.NoError(t, err)
		require.Equal(t, true, f["Outstanding"])
		for id := int64(2); id <= 16; id++ {
			_, err = p.consumeAt(snmpV1GetWithID(id), 16, 0, at)
			require.NoError(t, err)
			f, err = p.consumeAt(snmpV1Response(id, "owned"), 16, 1, at)
			require.NoError(t, err)
			require.Equal(t, "GetRequest", f["Matched Request"])
		}
		require.Len(t, p.history, 16)
		require.Len(t, p.pending, 1)
		before := p.storage()
		f, err = p.consumeAt(snmpV1Get(), 16, 0, at)
		require.NoError(t, err)
		require.Equal(t, true, f["Retransmission"])
		require.Equal(t, before, p.storage())
		f, err = p.consumeAt(snmpV1GetWithID(17), 16, 0, at)
		rocTypedError(t, "ResourceExceeded", err)
		require.Equal(t, "untracked-resource-refusal", f["Association Status"])
		require.Len(t, p.pending, 1)
		require.False(t, p.blocked)
		f, err = p.consumeAt(snmpV1Response(1, "live"), 16, 1, at)
		require.NoError(t, err)
		require.Equal(t, "GetRequest", f["Matched Request"])
		f, err = p.consumeAt(snmpV1Response(17, "refused"), 16, 1, at)
		require.NoError(t, err)
		require.NotContains(t, f, "Matched Request")
		f, err = p.consumeAt(snmpV1GetWithID(17), 16, 0, at)
		rocTypedError(t, "ResourceExceeded", err)
		require.NotContains(t, f, "Outstanding")
		require.Empty(t, p.pending)
		require.Len(t, p.history, 16)
		require.Equal(t, int64(512+384*16), p.storage())
	})
	t.Run("malformed-unknown-context-retires-all-live-keys", func(t *testing.T) {
		p := &binSNMP{}
		at := time.Unix(100, 0)
		_, err := p.consumeAt(snmpV1Get(), 16, 0, at)
		require.NoError(t, err)
		bad := snmpCommunityMessage(0, []byte("public"), snmpPDU(0xa0, 2, 0, 0, snmpVarBind(snmpSysDescr(), []byte{5, 1, 0})))
		_, err = p.consumeAt(bad, 16, 0, at)
		require.Error(t, err)
		require.True(t, p.blocked)
		require.Empty(t, p.pending)
		for _, w := range [][]byte{snmpV1Response(1, "late"), snmpV1GetWithID(2), snmpV1Response(2, "refused")} {
			f, err := p.consumeAt(w, 16, 1, at)
			require.NoError(t, err)
			require.Equal(t, "ambiguous-conversation", f["Association Status"])
			require.NotContains(t, f, "Matched Request")
		}
	})
	t.Run("udp-idle-retains-domain-identities-and-releases-close", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"))
		require.NoError(t, err)
		defer s.Close("cleanup")
		a := s.(*captureSession).f.a
		feed := func(d CaptureDomain, dir int, at int64, w []byte) *ProtocolEvent {
			src, dst := "192.0.2.1:40000", "192.0.2.2:161"
			if dir == 1 {
				src, dst = dst, src
			}
			e := &ProtocolEvent{Source: src, Destination: dst, Domain: d, Timestamp: time.Unix(at, 0), Transport: "udp"}
			require.True(t, a.decodeSNMPDatagram(e, w, "snmp"))
			require.Nil(t, e.sessionError)
			return e
		}
		d := CaptureDomain{Section: 1, Interface: 0}
		q := feed(d, 0, 100, snmpV1Get())
		late := feed(d, 1, 700, snmpV1Response(1, "expired"))
		require.Equal(t, q.FlowID, late.FlowID)
		require.Equal(t, "ambiguous-request-id", late.Session["Association Status"])
		fresh := feed(d, 0, 701, snmpV1GetWithID(2))
		r := feed(d, 1, 702, snmpV1Response(2, "fresh"))
		require.Equal(t, fresh.FlowID, r.FlowID)
		require.Equal(t, "GetRequest", r.Session["Matched Request"])
		for _, other := range []CaptureDomain{{Section: 2, Interface: 0}, {Section: 1, Interface: 1}, {Section: 1, Interface: 0, Encapsulation: "/vxlan:outer:2"}} {
			q2 := feed(other, 0, 703, snmpV1Get())
			require.NotEqual(t, q.FlowID, q2.FlowID)
			r2 := feed(other, 1, 704, snmpV1Response(1, "isolated"))
			require.Equal(t, "GetRequest", r2.Session["Matched Request"])
		}
		require.Positive(t, s.Stats().BufferedBytes, "retained identities must stay charged until Close")
		require.Empty(t, s.Close("done"))
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
}

func testSNMPIdentityTCPChunks(t *testing.T) {
	b, err := trafficfixture.ReadFile("snmp-identity/controls.json")
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
					s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionPorts(40000, 161))
					require.NoError(t, err)
					defer s.Close("cleanup")
					s.(*captureSession).f.a.config.Deferred = deferred
					for _, w := range c.Events {
						raw, err := hex.DecodeString(w.Raw)
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
						require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
						require.Equal(t, w.Direction, e.Direction)
						require.Equal(t, w.Timestamp, e.Timestamp.Unix())
						require.Equal(t, w.Reply, e.ResponseTo)
						require.Equal(t, w.Transaction, e.TransactionID)
						f, err := e.GetFields()
						require.NoError(t, err)
						rocEqualFields(t, w.Fields, f)
						rocEqualFields(t, w.Session, e.Session)
					}
					require.Empty(t, s.Close("done"))
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
}
