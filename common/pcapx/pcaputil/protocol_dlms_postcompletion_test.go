package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func TestDLMSPostCompletionRefusalExistingAPI(t *testing.T) {
	for _, refused := range []bool{false, true} {
		name := "adjacent-normal"
		if refused {
			name = "refused-request-late-response"
		}
		t.Run(name, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("post-completion")
			feed := func(dir int, at int64, wire string) *ProtocolEvent {
				out := s.Feed(dir, time.Unix(at, 0), wrapperWire(t, wire))
				require.Len(t, out.Events, 1)
				return out.Events[0]
			}
			q := feed(0, 1, "7ea0190321107fdae6e600c001c1000f0000280000ff020091537e")
			r := feed(1, 2, "7ea013210330d381e6e700c401c1000403afda957e")
			_, err = r.GetFields()
			require.NoError(t, err)
			require.Equal(t, q.ID, r.ResponseTo)
			if refused {
				// Independently framed complete request: array257 exceeds the
				// local256-node pool. The prior request has already completed.
				bad := feed(0, 3, "7ea11e0321320a84e6e600c001c1000100002a0000ff02010101820100"+strings.Repeat("00", 256)+"15e17e")
				_, err = bad.GetFields()
				rocTypedError(t, "ResourceExceeded", err)
				require.Nil(t, bad.Session)
				require.Zero(t, bad.ResponseTo)
			}
			newRequest := feed(0, 4, "7ea0190321326fd8e6e600c001c1000f0000280000ff0300494a7e")
			late := feed(1, 5, "7ea013210352c7c1e6e700c401c1000403afda957e")
			if refused {
				require.Zero(t, late.ResponseTo, "late feedback from a refused request cannot attach to a subsequent request")
				for _, e := range []*ProtocolEvent{newRequest, late} {
					f, err := e.GetFields()
					rocTypedError(t, "ContextRequired", err)
					require.Nil(t, f)
					require.Nil(t, e.Session)
					require.Zero(t, e.ResponseTo)
					require.Zero(t, e.TransactionID)
				}
			} else {
				_, err = newRequest.GetFields()
				require.NoError(t, err)
				_, err = late.GetFields()
				require.NoError(t, err)
				require.Equal(t, newRequest.ID, late.ResponseTo)
			}
			require.Empty(t, s.Close("released"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDLMSPostCompletionOrphanExistingAPI(t *testing.T) {
	for _, unseenProgress := range []bool{false, true} {
		name := "previous-duplicate-adjacent"
		if unseenProgress {
			name = "unseen-acknowledged-request"
		}
		t.Run(name, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("orphan")
			feed := func(dir int, at int64, wire string) *ProtocolEvent {
				out := s.Feed(dir, time.Unix(at, 0), wrapperWire(t, wire))
				require.Len(t, out.Events, 1)
				return out.Events[0]
			}
			q := feed(0, 1, "7ea0190321107fdae6e600c001c1000f0000280000ff020091537e")
			prior := "7ea013210330d381e6e700c401c1000403afda957e"
			future := "7ea013210352c7c1e6e700c401c1000403afda957e"
			r := feed(1, 2, prior)
			require.Equal(t, q.ID, r.ResponseTo)
			orphanWire := prior
			if unseenProgress {
				orphanWire = future
			}
			orphan := feed(1, 3, orphanWire)
			_, err = orphan.GetFields()
			rocTypedError(t, "ContextRequired", err)
			require.Zero(t, orphan.ResponseTo)
			newRequest := feed(0, 4, "7ea0190321326fd8e6e600c001c1000f0000280000ff0300494a7e")
			late := feed(1, 5, future)
			if unseenProgress {
				require.Zero(t, late.ResponseTo, "a previously orphaned future response cannot acquire a later request identity")
				for _, e := range []*ProtocolEvent{newRequest, late} {
					f, err := e.GetFields()
					rocTypedError(t, "ContextRequired", err)
					require.Nil(t, f)
					require.Nil(t, e.Session)
					require.Zero(t, e.TransactionID)
					require.Zero(t, e.ResponseTo)
				}
			} else {
				_, err := newRequest.GetFields()
				require.NoError(t, err)
				_, err = late.GetFields()
				require.NoError(t, err)
				require.Equal(t, newRequest.ID, late.ResponseTo)
			}
			require.Empty(t, s.Close("orphan"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDLMSSupervisoryUnseenProgressExistingAPI(t *testing.T) {
	for _, unseen := range []bool{false, true} {
		name := "adjacent-observed-ack"
		if unseen {
			name = "acknowledged-unseen-request"
		}
		t.Run(name, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("supervisory")
			feed := func(dir int, at int64, wire string) *ProtocolEvent {
				out := s.Feed(dir, time.Unix(at, 0), wrapperWire(t, wire))
				require.Len(t, out.Events, 1)
				return out.Events[0]
			}
			q := feed(0, 1, "7ea0190321107fdae6e600c001c1000f0000280000ff020091537e")
			r := feed(1, 2, "7ea013210330d381e6e700c401c1000403afda957e")
			require.Equal(t, q.ID, r.ResponseTo)
			rr := "7ea00721033117217e"
			if unseen {
				rr = "7ea00721035111427e"
			}
			ack := feed(1, 3, rr)
			f, err := ack.GetFields()
			require.NoError(t, err)
			require.Equal(t, "RR", f["Supervisory Function"])
			require.Equal(t, "unassociated-link-observation", ack.Session["Association"])
			require.Zero(t, ack.ResponseTo)
			newRequest := feed(0, 4, "7ea0190321326fd8e6e600c001c1000f0000280000ff0300494a7e")
			late := feed(1, 5, "7ea013210352c7c1e6e700c401c1000403afda957e")
			if unseen {
				require.Zero(t, late.ResponseTo, "supervisory acknowledgement of an unobserved request cannot justify later pairing")
				for _, e := range []*ProtocolEvent{newRequest, late} {
					f, err := e.GetFields()
					rocTypedError(t, "ContextRequired", err)
					require.Nil(t, f)
					require.Nil(t, e.Session)
					require.Zero(t, e.ResponseTo)
					require.Zero(t, e.TransactionID)
				}
			} else {
				_, err := newRequest.GetFields()
				require.NoError(t, err)
				_, err = late.GetFields()
				require.NoError(t, err)
				require.Equal(t, newRequest.ID, late.ResponseTo)
			}
			require.Empty(t, s.Close("supervisory"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func dlmsPostCompletionControls(t *testing.T) []dlmsListControl {
	t.Helper()
	b, err := trafficfixture.ReadFile("dlms-postcompletion-refusal/controls.json")
	require.NoError(t, err)
	var m struct {
		Schema string
		Cases  []dlmsListControl
	}
	require.NoError(t, json.Unmarshal(b, &m))
	require.Equal(t, "owned-dlms-postcompletion-refusal/v1", m.Schema)
	require.Len(t, m.Cases, 52)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for _, c := range m.Cases {
		answer, err := trafficfixture.ReadFile("dlms-postcompletion-refusal/" + c.Answer)
		require.NoError(t, err)
		require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(answer)))
		var a dlmsListControl
		require.NoError(t, json.Unmarshal(answer, &a))
		require.Equal(t, c.Events, a.Events)
		bound := 0
		for _, batch := range all {
			for _, input := range batch.Cases {
				if input.ID != "dlms-postcompletion-refusal/"+c.ID {
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
	return m.Cases
}

func TestDLMSPostCompletionSealedMatrix(t *testing.T) {
	dlmsSealedMatrix(t, dlmsPostCompletionControls(t))
}
func TestDLMSPostCompletionOwnershipAndChunks(t *testing.T) {
	dlmsOwnershipAndChunks(t, dlmsPostCompletionControls(t))
}

func TestDLMSPostCompletionBudgetAndDomain(t *testing.T) {
	var adjacent, refusal, selected dlmsListControl
	for _, c := range dlmsPostCompletionControls(t) {
		if c.ID == "adjacent-udp" {
			adjacent = c
		}
		if c.ID == "selected-adjacent-udp" {
			selected = c
		}
		if c.ID == "crc-refusal-udp" {
			refusal = c
		}
	}
	require.NotEmpty(t, adjacent.ID)
	require.NotEmpty(t, refusal.ID)
	for _, deferred := range []bool{false, true} {
		for _, delta := range []int{-1, 0} {
			t.Run(fmt.Sprintf("shared-byte/deferred=%t/delta=%d", deferred, delta), func(t *testing.T) {
				b := DefaultParserBudget()
				b.MaxFrameBytes, b.MaxMessageBytes = 64, 64
				q := wrapperWire(t, selected.Events[2].Raw)
				b.MaxBufferedBytes = int(512+dlmsProjection(q)) + delta
				s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
				require.NoError(t, err)
				defer s.Close("budget")
				s.(*captureSession).f.a.config.Deferred = deferred
				var events []*ProtocolEvent
				for i, want := range selected.Events {
					out := s.Feed(want.Direction, time.Unix(int64(i+1), 0), wrapperWire(t, want.Raw))
					require.Len(t, out.Events, 1)
					events = append(events, out.Events...)
				}
				if delta == 0 {
					dlmsListAssert(t, selected, events)
				} else {
					prefix := selected
					prefix.Events = prefix.Events[:2]
					dlmsListAssert(t, prefix, events[:2])
					for i, kind := range []string{"ResourceExceeded", "ContextRequired"} {
						e := events[i+2]
						f, err := e.GetFields()
						rocTypedError(t, kind, err)
						require.Nil(t, f)
						require.Nil(t, e.Session)
						require.Zero(t, e.ResponseTo)
						require.Zero(t, e.TransactionID)
					}
				}
				require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
				require.Empty(t, s.Close("budget"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
	t.Run("retirement-isolated-by-capture-domain", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"))
		require.NoError(t, err)
		defer s.Close("domains")
		a := s.(*captureSession).f.a
		domains := []CaptureDomain{{Section: 1, Interface: 0}, {Section: 1, Interface: 1}, {Section: 2, Interface: 0}}
		byDomain := make([][]*ProtocolEvent, len(domains))
		feed := func(d int, want dlmsListAnswer) {
			src, dst := "192.0.2.1:40000", "192.0.2.2:4059"
			if want.Direction == 1 {
				src, dst = dst, src
			}
			e := &ProtocolEvent{Source: src, Destination: dst, Domain: domains[d], Transport: "udp", Timestamp: time.Unix(1, 0)}
			require.True(t, a.decodeDLMSDatagram(e, wrapperWire(t, want.Raw), true))
			byDomain[d] = append(byDomain[d], e)
		}
		for i := 0; i < 2; i++ {
			for d := range domains {
				feed(d, adjacent.Events[i])
			}
		}
		feed(0, refusal.Events[2])
		for i := 2; i < 4; i++ {
			for d := range domains {
				want := adjacent.Events[i]
				if d == 0 {
					want = refusal.Events[i+1]
				}
				feed(d, want)
			}
		}
		dlmsListAssert(t, refusal, byDomain[0])
		dlmsListAssert(t, adjacent, byDomain[1])
		dlmsListAssert(t, adjacent, byDomain[2])
		for d, events := range byDomain {
			for _, e := range events {
				require.Equal(t, domains[d], e.Domain)
			}
			for other := 0; other < d; other++ {
				require.NotEqual(t, events[0].FlowID, byDomain[other][0].FlowID)
			}
		}
		require.Len(t, a.udpSessions.entries, 3)
		require.Empty(t, s.Close("domains"))
		require.Nil(t, a.udpSessions)
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
}
