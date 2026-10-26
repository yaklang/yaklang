package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"testing"
	"time"
	"unsafe"
)

func TestQUICRetirementExistingAPI(t *testing.T) {
	cli := rfc9001ClientInitial()
	for _, tc := range []struct {
		name           string
		idle, pressure bool
	}{
		{"live-exact-retry", false, false}, {"idle-old-initial", true, false}, {"budget-old-initial", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			second := time.Unix(101, 0)
			if tc.idle {
				second = time.Unix(700, 0)
			}
			steps := []sessionStep{{0, cli}, {0, cli}}
			if tc.pressure {
				bad := bytes.Clone(cli)
				bad = append(bad, bytes.Repeat([]byte{1}, 200)...)
				binary.BigEndian.PutUint16(bad[16:18], 0x4000|1382)
				steps = []sessionStep{{0, cli}, {0, bad}, {0, cli}}
			}
			raw := sessionDatagramPCAP(t, steps, 14443)
			at := 24
			for i := range steps {
				n := int(binary.LittleEndian.Uint32(raw[at+8:]))
				sec := uint32(100)
				if i > 0 {
					sec = uint32(second.Unix())
				}
				binary.LittleEndian.PutUint32(raw[at:], sec)
				at += 16 + n
			}
			opts := []CaptureOption{}
			if tc.pressure {
				opts = append(opts, WithProtocolBudget(1600, 45000))
			}
			es, stats := discoveryReplay(t, raw, 1, false, false, opts...)
			require.Len(t, es, len(steps))
			require.Empty(t, es[0].Error)
			require.Equal(t, true, es[0].Session["Authentication Verified"])
			require.Equal(t, uint64(2), es[0].Session["Packet Number"])
			last := es[len(es)-1]
			if tc.idle || tc.pressure {
				require.Equal(t, "context-required", last.Status, "retired captured CID cannot become a verified fresh generation")
				require.Nil(t, last.Session)
				require.Equal(t, es[0].FlowID, last.FlowID)
			} else {
				require.Empty(t, last.Error)
				require.Equal(t, es[0].FlowID, last.FlowID)
				require.Equal(t, true, last.Session["Duplicate"])
			}
			require.Zero(t, stats.BufferedBytes)
		})
	}
	t.Run("owned-native-fields", func(t *testing.T) {
		raw := sessionDatagramPCAP(t, []sessionStep{{0, cli}}, 14443)
		es, _ := discoveryReplay(t, raw, 1, false, false)
		require.Len(t, es, 1)
		e := es[0]
		fields, err := e.GetFields()
		require.NoError(t, err)
		require.Equal(t, byte(0xc0), fields["First Byte"])
		fields["First Byte"] = byte(0xff)
		e.Raw[0] ^= 1
		if e.Structured != nil {
			e.Structured["fields"] = map[string]any{"First Byte": byte(0xee)}
		}
		again, err := e.GetFields()
		require.NoError(t, err)
		require.Equal(t, byte(0xc0), again["First Byte"], "owned wire fields must not depend on editable event projections")
	})
}

// Fixed protected inputs and independent answers, rather than IUT-derived parity.
func testQUICRetirementMatrix(t *testing.T) {
	const folder = "quic-retirement/"
	b, err := trafficfixture.ReadFile(folder + "controls.json")
	require.NoError(t, err)
	var doc struct {
		Schema string
		Cases  []struct {
			ID, Capture, SHA256, Answer string
			AnswerSHA                   string `json:"answer_sha256"`
			InputAlias                  string `json:"input_alias"`
			Packets                     int
			Pressure                    bool
			Events                      []struct {
				Raw, Error, Source, Destination string
				Generation, Direction           int
				Timestamp                       int64    `json:"timestamp_sec"`
				Reply                           uint64   `json:"response_to"`
				Transaction                     uint64   `json:"transaction_id"`
				Refs                            []uint64 `json:"packet_refs"`
				Fields, Session                 map[string]any
			}
		}
	}
	require.NoError(t, json.Unmarshal(b, &doc))
	require.Equal(t, "owned-quic-retirement/v1", doc.Schema)
	require.Len(t, doc.Cases, 8)
	for _, c := range doc.Cases {
		t.Run(c.ID, func(t *testing.T) {
			ab, err := trafficfixture.ReadFile(folder + c.Answer)
			require.NoError(t, err)
			require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(ab)))
			var answer struct{ Events json.RawMessage }
			require.NoError(t, json.Unmarshal(ab, &answer))
			var answerEvents []map[string]any
			require.NoError(t, json.Unmarshal(answer.Events, &answerEvents))
			require.Len(t, answerEvents, len(c.Events))
			for i, w := range c.Events {
				require.Equal(t, w.Raw, answerEvents[i]["raw"])
				if w.Fields == nil {
					require.Nil(t, answerEvents[i]["fields"])
				} else {
					require.Equal(t, w.Fields, answerEvents[i]["fields"])
				}
				if w.Session == nil {
					require.Nil(t, answerEvents[i]["session"])
				} else {
					require.Equal(t, w.Session, answerEvents[i]["session"])
				}
				require.Equal(t, w.Error, answerEvents[i]["error"])
			}
			all, err := trafficfixture.AllExpectations()
			require.NoError(t, err)
			bindings := 0
			for _, batch := range all {
				for _, row := range batch.Cases {
					if row.ID == "quic-retirement/"+c.ID {
						bindings++
						require.Equal(t, c.InputAlias, row.Input.OriginalPath)
						require.Equal(t, c.SHA256, row.Input.SHA256)
						require.Equal(t, c.Packets, row.Facts.PacketCount)
					}
				}
			}
			require.Equal(t, 1, bindings)
			raw, err := trafficfixture.ReadFile(folder + c.Capture)
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				var opts []CaptureOption
				if c.Pressure {
					opts = append(opts, WithProtocolBudget(1600, 48000))
				}
				es, stats := discoveryReplay(t, raw, workers, deferred, observe, opts...)
				require.Len(t, es, len(c.Events))
				flows := map[int]uint64{}
				used := map[uint64]bool{}
				for i, e := range es {
					w := c.Events[i]
					require.Equal(t, uint64(i+1), e.ID)
					require.NotZero(t, e.FlowID)
					if id, ok := flows[w.Generation]; ok {
						require.Equal(t, id, e.FlowID)
					} else {
						require.False(t, used[e.FlowID])
						flows[w.Generation] = e.FlowID
						used[e.FlowID] = true
					}
					require.Equal(t, "quic", e.Protocol)
					require.Equal(t, "quic-v1-native", e.Profile)
					require.Equal(t, "udp", e.Transport)
					require.Equal(t, w.Source, e.Source)
					require.Equal(t, w.Destination, e.Destination)
					require.Equal(t, w.Direction, e.Direction)
					require.Equal(t, w.Timestamp, e.Timestamp.Unix())
					require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
					require.Equal(t, len(w.Raw)/2, e.Length)
					require.Equal(t, w.Reply, e.ResponseTo)
					require.Equal(t, w.Transaction, e.TransactionID)
					require.Equal(t, es[0].Domain, e.Domain)
					require.Equal(t, "captured", e.SourceBytes.Kind)
					require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(e.Raw)), e.SourceBytes.SHA256)
					require.Len(t, e.SourceBytes.PacketRefs, len(w.Refs))
					for j, ref := range e.SourceBytes.PacketRefs {
						require.Equal(t, w.Refs[j], ref.Number)
						require.Equal(t, e.Domain, ref.Domain)
					}
					f, err := e.GetFields()
					if w.Error != "" {
						rocTypedError(t, w.Error, err)
						require.Nil(t, f)
						require.Nil(t, e.Session)
						require.Equal(t, w.Error, e.ExpertCode)
						// Connection reservation errors retain the established context-required
						// capture status, with the exact ResourceExceeded expert kind.
						status := "context-required"
						require.Equal(t, status, e.Status)
						continue
					}
					require.NoError(t, err)
					require.Empty(t, e.Error)
					status := "decoded"
					if deferred {
						status = "deferred"
					}
					require.Equal(t, status, e.Status)
					w.Session, w.Fields = quicReviewedLegacyExpectations(t, e, w.Session, w.Fields)
					rocEqualFields(t, w.Fields, f)
					rocEqualFields(t, w.Session, e.Session)
					f["First Byte"] = byte(255)
					if p, ok := f["Protected Payload"].([]any); ok {
						p[0] = byte(255)
					}
					e.Raw[0] ^= 255
					e.Fields = map[string]any{"First Byte": byte(255)}
					if e.Structured != nil {
						e.Structured["fields"] = e.Fields
					}
					again, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, w.Fields, again)
				}
				require.Zero(t, stats.BufferedBytes)
				require.EqualValues(t, len(es), stats.Messages)
			})
		})
	}
}

func testQUICRetirementResourcesAndDomains(t *testing.T) {
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) {})(c))
	require.NoError(t, c.prepareBinParser())
	a := c.binParser
	feed := func(domain CaptureDomain, ts int64, wire []byte) *ProtocolEvent {
		es, ok := a.decodeQUICDatagram(&ProtocolEvent{Domain: domain, Transport: "udp", Source: "192.0.2.1:40000", Destination: "192.0.2.2:14443", Timestamp: time.Unix(ts, 0), FlowID: 999}, wire)
		require.True(t, ok)
		require.Len(t, es, 1)
		return es[0]
	}
	domain := CaptureDomain{Section: 1, Interface: 2, Encapsulation: "/vxlan:192.0.2.8~192.0.2.9:7"}
	cli := rfc9001ClientInitial()
	first := feed(domain, 100, cli)
	require.Empty(t, first.Error)
	live := a.buffered.Load()
	require.Greater(t, live, int64(4224))
	late := feed(domain, 700, cli)
	require.Equal(t, "context-required", late.Status)
	require.Equal(t, first.FlowID, late.FlowID)
	require.Equal(t, int64(4224), a.buffered.Load())
	require.GreaterOrEqual(t, a.buffered.Load(), int64(unsafe.Sizeof(binQUIC{})+unsafe.Sizeof(binFlow{})+unsafe.Sizeof(binUDPEntry{})), "retirement must retain the real base structures' charge")
	t.Logf("retained base+CID charge=%d; base Go structs=%d", a.buffered.Load(), unsafe.Sizeof(binQUIC{})+unsafe.Sizeof(binFlow{})+unsafe.Sizeof(binUDPEntry{}))
	for _, el := range a.udpSessions.entries {
		q := el.Value.(*binUDPEntry).flow.quic
		require.True(t, q.retired)
		require.Nil(t, q.keys)
		require.Nil(t, q.tls)
		require.Nil(t, q.streams)
		require.Empty(t, q.initialDCID)
		require.Len(t, q.cids, 1)
	}
	for _, other := range []CaptureDomain{{Section: 2, Interface: 2, Encapsulation: domain.Encapsulation}, {Section: 1, Interface: 3, Encapsulation: domain.Encapsulation}, {Section: 1, Interface: 2, Encapsulation: "/vxlan:192.0.2.8~192.0.2.9:8"}} {
		e := feed(other, 701, cli)
		require.Empty(t, e.Error)
		require.NotEqual(t, first.FlowID, e.FlowID)
	}
	require.Len(t, a.udpSessions.entries, 4)
	// Zero-CID evidence belongs to an endpoint tuple. A fresh observed CID may
	// establish a live generation; old server ciphertext must fail with its keys.
	raw, err := trafficfixture.ReadFile("quic-retirement/cases/fresh-CID-after-idle.pcap")
	require.NoError(t, err)
	at := 24 + 16 + int(binary.LittleEndian.Uint32(raw[32:36]))
	n := int(binary.LittleEndian.Uint32(raw[at+8 : at+12]))
	fresh := bytes.Clone(raw[at+16+42 : at+16+n])
	newFlow := feed(domain, 702, fresh)
	require.Empty(t, newFlow.Error)
	require.NotEqual(t, first.FlowID, newFlow.FlowID)
	refused, ok := a.decodeQUICDatagram(&ProtocolEvent{Domain: domain, Transport: "udp", Source: "192.0.2.2:14443", Destination: "192.0.2.1:40000", Timestamp: time.Unix(703, 0)}, rfc9001ServerInitial())
	require.True(t, ok)
	require.Len(t, refused, 1)
	require.Equal(t, newFlow.FlowID, refused[0].FlowID)
	require.Equal(t, 1, refused[0].Direction)
	f, err := refused[0].GetFields()
	rocTypedError(t, "AuthenticationFailed", err)
	require.Nil(t, f)
	require.Equal(t, map[string]any{"Packet Name": "Initial", "Version": uint32(1), "DCID": []byte(nil), "SCID": rfcHex("f067a5502a4262b5"), "Packet Number Space": "initial", "Connection State": "initial", "Header Form": "long", "Encrypted": true, "Content Visibility": "encrypted", "Protected Payload": true, "Authentication Verified": false, "Authentication Failure": "packet-protection-failed", "Observation Scope": "UDP capture domain and observed CID", "Datagram Offset": 0, "Native Carrier": true, "ALPN": "", "Wire Field Ranges": map[string]any{"Version": []any{1, 5}, "DCID Length": []any{5, 6}, "DCID": []any{6, 6}, "SCID Length": []any{6, 7}, "SCID": []any{7, 15}, "Long Packet Type": []any{0, 1}}}, refused[0].Session)
	retry := feed(domain, 704, fresh)
	require.Empty(t, retry.Error)
	require.Equal(t, newFlow.FlowID, retry.FlowID)
	require.Equal(t, true, retry.Session["Duplicate"])
	// Retained identity slots are bounded; collection refusal cannot erase them.
	a.budget.MaxCollectionElements = 1
	other := domain
	other.Interface = 9
	denied := feed(other, 705, cli)
	f, err = denied.GetFields()
	rocTypedError(t, "ResourceExceeded", err)
	require.Nil(t, f)
	require.Zero(t, denied.FlowID)
	require.Len(t, a.udpSessions.entries, 5)
	old := feed(domain, 706, cli)
	require.Equal(t, "context-required", old.Status)
	require.Equal(t, first.FlowID, old.FlowID)
	a.closeUDPSessions()
	require.Zero(t, a.buffered.Load())
	require.Nil(t, a.udpSessions)
	a.closeUDPSessions()
	require.Zero(t, a.buffered.Load())
	require.NoError(t, c.finishBinParser())
}
