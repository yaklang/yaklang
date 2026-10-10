package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"testing"
	"time"
)

func dlmsBlockControls(t *testing.T) []dlmsListControl {
	return dlmsNamedBlockControls(t, "dlms-hdlc-blocks", "owned-dlms-hdlc-blocks/v1", 112)
}
func dlmsNamedBlockControls(t *testing.T, name, schema string, count int) []dlmsListControl {
	t.Helper()
	b, err := trafficfixture.ReadFile(name + "/controls.json")
	require.NoError(t, err)
	var doc struct {
		Schema string
		Cases  []dlmsListControl
	}
	require.NoError(t, json.Unmarshal(b, &doc))
	require.Equal(t, schema, doc.Schema)
	require.Len(t, doc.Cases, count)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for _, c := range doc.Cases {
		var b []byte
		var err error
		b, err = trafficfixture.ReadFile(name + "/" + c.Answer)
		require.NoError(t, err)
		require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(b)))
		var a dlmsListControl
		require.NoError(t, json.Unmarshal(b, &a))
		require.Equal(t, c.Events, a.Events)
		bound := 0
		for _, batch := range all {
			for _, input := range batch.Cases {
				if input.ID != name+"/"+c.ID {
					continue
				}
				require.Equal(t, c.SHA256, input.Input.SHA256)
				require.Equal(t, c.InputAlias, input.Input.OriginalPath)
				require.Equal(t, c.Packets, input.Facts.PacketCount)
				for _, expectation := range input.Expectations {
					var bind struct {
						File string `json:"answer_file"`
						SHA  string `json:"answer_sha256"`
					}
					require.NoError(t, json.Unmarshal(expectation.PayloadConstraints, &bind))
					if bind.File == c.Answer {
						require.Equal(t, c.AnswerSHA, bind.SHA)
						bound++
					}
				}
			}
		}
		require.Equal(t, 1, bound, c.ID)
	}
	return doc.Cases
}

func TestDLMSHDLCBlocksOwnershipAndChunks(t *testing.T) {
	for _, c := range dlmsBlockControls(t) {
		t.Run(c.ID, func(t *testing.T) { dlmsOwnershipAndChunks(t, []dlmsListControl{c}) })
	}
}

func TestDLMSHDLCBlocksCloseAndBudget(t *testing.T) {
	var normal dlmsListControl
	for _, c := range dlmsBlockControls(t) {
		if c.ID == "normal-two-udp" {
			normal = c
		}
	}
	require.NotEmpty(t, normal.ID)
	for _, transport := range []string{"tcp", "udp"} {
		for _, deferred := range []bool{false, true} {
			for _, hops := range []int{2, 3} {
				t.Run(fmt.Sprintf("close/%s/deferred%t/hops%d", transport, deferred, hops), func(t *testing.T) {
					s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(40000, 4059))
					require.NoError(t, err)
					defer s.Close("cleanup")
					s.(*captureSession).f.a.config.Deferred = deferred
					if transport == "udp" {
						a := s.(*captureSession).f.a
						callback := a.config.OnEvent
						a.config.OnEvent = func(e *ProtocolEvent) {
							if e.Completeness == "incomplete" {
								unlocked := a.udpMu.TryLock()
								require.True(t, unlocked, "Close callback cannot hold the UDP session store lock")
								if unlocked {
									a.udpMu.Unlock()
								}
							}
							callback(e)
						}
					}
					var events []*ProtocolEvent
					for i, w := range normal.Events[:hops] {
						out := s.Feed(w.Direction, time.Unix(int64(i+1), 0), wrapperWire(t, w.Raw))
						require.Len(t, out.Events, 1)
						events = append(events, out.Events...)
					}
					prefix := normal
					prefix.Events = prefix.Events[:hops]
					dlmsListAssert(t, prefix, events)
					out := s.Close("unfinished")
					require.Len(t, out, 1)
					require.Equal(t, "dlms", out[0].Protocol)
					require.Equal(t, transport, out[0].Transport)
					require.Equal(t, events[0].FlowID, out[0].FlowID)
					require.Equal(t, events[0].Source, out[0].Source)
					require.Equal(t, events[0].Destination, out[0].Destination)
					require.Equal(t, "incomplete", out[0].Completeness)
					require.Zero(t, out[0].ResponseTo)
					require.Zero(t, out[0].TransactionID)
					rocEqualFields(t, map[string]any{"Outstanding": float64(1), "ObservedBlocks": float64(1), "EncodedBytes": float64(5)}, out[0].Session)
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
			for _, limit := range []struct {
				name         string
				nodes, depth int
				bad          bool
			}{{"bytes9", 9, 64, false}, {"bytes8", 8, 64, true}, {"depth4", 4096, 6, false}, {"depth3", 4096, 3, true}} {
				t.Run(fmt.Sprintf("caller/%s/deferred%t/%s", transport, deferred, limit.name), func(t *testing.T) {
					b := DefaultParserBudget()
					b.MaxCollectionElements, b.MaxRecursionDepth = limit.nodes, limit.depth
					s, err := NewProtocolSessionWithOptions(b, WithSessionTransport(transport), WithSessionPorts(40000, 4059))
					require.NoError(t, err)
					defer s.Close("caller")
					s.(*captureSession).f.a.config.Deferred = deferred
					var events []*ProtocolEvent
					for i, w := range normal.Events {
						out := s.Feed(w.Direction, time.Unix(int64(i+1), 0), wrapperWire(t, w.Raw))
						require.Len(t, out.Events, 1)
						events = append(events, out.Events...)
					}
					if !limit.bad {
						dlmsListAssert(t, normal, events)
					} else {
						prefix := normal
						prefix.Events = prefix.Events[:3]
						dlmsListAssert(t, prefix, events[:3])
						f, err := events[3].GetFields()
						rocTypedError(t, "ResourceExceeded", err)
						require.Nil(t, f)
						require.Nil(t, events[3].Session)
						require.Zero(t, events[3].ResponseTo)
						if transport == "udp" {
							late := s.Feed(1, time.Unix(5, 0), wrapperWire(t, normal.Events[3].Raw))
							require.Len(t, late.Events, 1)
							_, err := late.Events[0].GetFields()
							rocTypedError(t, "ContextRequired", err)
							require.Zero(t, late.Events[0].ResponseTo)
						}
					}
					require.Empty(t, s.Close("caller"))
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
	for _, delta := range []int{-1, 0} {
		t.Run(fmt.Sprintf("shared-bytes/delta%d", delta), func(t *testing.T) {
			b := DefaultParserBudget()
			q, r := wrapperWire(t, normal.Events[0].Raw), wrapperWire(t, normal.Events[1].Raw)
			b.MaxBufferedBytes = int(512+2*int64(len(q))+dlmsProjection(r)) + delta
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("shared")
			var events []*ProtocolEvent
			for i, w := range normal.Events {
				out := s.Feed(w.Direction, time.Unix(int64(i+1), 0), wrapperWire(t, w.Raw))
				require.Len(t, out.Events, 1)
				events = append(events, out.Events...)
			}
			if delta == 0 {
				dlmsListAssert(t, normal, events)
			} else {
				prefix := normal
				prefix.Events = prefix.Events[:1]
				dlmsListAssert(t, prefix, events[:1])
				for i, k := range []string{"ResourceExceeded", "ContextRequired", "ContextRequired"} {
					f, err := events[i+1].GetFields()
					rocTypedError(t, k, err)
					require.Nil(t, f)
					require.Nil(t, events[i+1].Session)
					require.Zero(t, events[i+1].ResponseTo)
				}
			}
			require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
			require.Empty(t, s.Close("shared"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDLMSHDLCBlocksDomainIsolation(t *testing.T) {
	var normal, bad dlmsListControl
	for _, c := range dlmsBlockControls(t) {
		if c.ID == "normal-two-udp" {
			normal = c
		}
		if c.ID == "normal-wrong-next-number-udp" {
			bad = c
		}
	}
	require.NotEmpty(t, normal.ID)
	require.NotEmpty(t, bad.ID)
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"))
	require.NoError(t, err)
	defer s.Close("domain")
	a := s.(*captureSession).f.a
	domains := []CaptureDomain{{Section: 1, Interface: 0}, {Section: 1, Interface: 1}, {Section: 2, Interface: 0}}
	byDomain := make([][]*ProtocolEvent, 3)
	for i := range normal.Events {
		for d, domain := range domains {
			want := normal.Events[i]
			if d == 0 {
				want = bad.Events[i]
			}
			src, dst := "192.0.2.1:40000", "192.0.2.2:4059"
			if want.Direction == 1 {
				src, dst = dst, src
			}
			e := &ProtocolEvent{Source: src, Destination: dst, Domain: domain, Transport: "udp", Timestamp: time.Unix(int64(i+1), 0)}
			require.True(t, a.decodeDLMSDatagram(e, wrapperWire(t, want.Raw), true))
			byDomain[d] = append(byDomain[d], e)
		}
	}
	dlmsListAssert(t, bad, byDomain[0])
	dlmsListAssert(t, normal, byDomain[1])
	dlmsListAssert(t, normal, byDomain[2])
	for d, events := range byDomain {
		for _, e := range events {
			require.Equal(t, domains[d], e.Domain)
		}
		for other := 0; other < d; other++ {
			require.NotEqual(t, events[0].FlowID, byDomain[other][0].FlowID)
		}
	}
	require.Len(t, a.udpSessions.entries, 3)
	require.Empty(t, s.Close("domain"))
	require.Nil(t, a.udpSessions)
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestDLMSHDLCBlocksExistingAPI(t *testing.T) {
	for _, transport := range []string{"tcp", "udp"} {
		for name, wires := range map[string][]string{"normal": {"7ea0190321107fdae6e600c001c1000f0000280000ff020091537e", "7ea01b2103300b64e6e700c402c10000000001000502021200011a047e", "7ea013032132c104e6e600c002c10000000151be7e", "7ea01a210352a438e6e700c402c10100000002000409026f6b055e7e"}, "list": {"7ea024032110021de6e600c003c102000100002a0000ff020000030100010800ff0200d5537e", "7ea01b2103300b64e6e700c402c10000000001000502001200016c3d7e", "7ea013032132c104e6e600c002c10000000151be7e", "7ea018210352d201e6e700c402c1010000000200020103b6c47e"}} {
			t.Run(transport+"/"+name, func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(40000, 4059))
				require.NoError(t, err)
				defer s.Close("blocks")
				var events []*ProtocolEvent
				for i, w := range wires {
					out := s.Feed(i%2, time.Unix(int64(i+1), 0), wrapperWire(t, w))
					require.Len(t, out.Events, 1)
					f, err := out.Events[0].GetFields()
					require.NoError(t, err, "existing API must observe the complete native data-block service")
					require.NotNil(t, f)
					require.Zero(t, out.Events[0].TransactionID)
					events = append(events, out.Events...)
				}
				require.Equal(t, events[0].ID, events[1].ResponseTo)
				require.Equal(t, events[2].ID, events[3].ResponseTo)
				require.Zero(t, events[2].ResponseTo)
				f, err := events[3].GetFields()
				require.NoError(t, err)
				block := f["Get Block"].(map[string]any)
				require.Equal(t, true, block["transfer_complete"])
				require.EqualValues(t, 2, block["observed_block_count"])
				if name == "normal" {
					require.Equal(t, "020212000109026f6b", block["assembled_data_hex"])
				} else {
					require.Equal(t, "02001200010103", block["assembled_data_hex"])
					require.EqualValues(t, 2, block["assembled_list_count"])
				}
				require.Empty(t, s.Close("blocks"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
		t.Run(transport+"/adjacent-normal", func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("adjacent")
			q := s.Feed(0, time.Unix(1, 0), wrapperWire(t, "7ea0190321107fdae6e600c001c1000f0000280000ff020091537e"))
			require.Len(t, q.Events, 1)
			_, err = q.Events[0].GetFields()
			require.NoError(t, err)
			r := s.Feed(1, time.Unix(2, 0), wrapperWire(t, "7ea013210330d381e6e700c401c1000403afda957e"))
			require.Len(t, r.Events, 1)
			_, err = r.Events[0].GetFields()
			require.NoError(t, err)
			require.Equal(t, q.Events[0].ID, r.Events[0].ResponseTo)
			require.Zero(t, r.Events[0].TransactionID)
			require.Empty(t, s.Close("adjacent"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDLMSHDLCBlocksSealedMatrix(t *testing.T) { dlmsSealedMatrix(t, dlmsBlockControls(t)) }
