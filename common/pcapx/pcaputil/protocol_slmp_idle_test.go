package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type slmpIdleControl struct {
	bsapControl
	Alias      string `json:"input_alias"`
	Limit      int    `json:"max_message_bytes"`
	FrameLimit int    `json:"max_frame_bytes"`
	Events     []struct {
		Raw, Error string
		Direction  int
		Seconds    int64
		Fields     map[string]any
		Reply      *int `json:"response_to_index"`
	}
}

func slmpIdleControls(t *testing.T) []slmpIdleControl {
	t.Helper()
	raw, err := trafficfixture.ReadFile("slmp-idle-history/controls.json")
	require.NoError(t, err)
	var d struct {
		Schema string
		Cases  []slmpIdleControl
	}
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Equal(t, "owned-slmp-idle-history/v1", d.Schema)
	require.Len(t, d.Cases, 10)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for _, c := range d.Cases {
		answer, err := trafficfixture.ReadFile("slmp-idle-history/" + c.Answer)
		require.NoError(t, err)
		require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(answer)))
		var a slmpIdleControl
		require.NoError(t, json.Unmarshal(answer, &a))
		require.Equal(t, c.Events, a.Events)
		bound := 0
		for _, b := range all {
			for _, row := range b.Cases {
				if row.Input.OriginalPath != c.Alias {
					continue
				}
				require.Equal(t, c.Alias, row.Input.OriginalPath)
				require.Equal(t, c.SHA256, row.Input.SHA256)
				require.Equal(t, c.Packets, row.Facts.PacketCount)
				for _, e := range row.Expectations {
					var link struct {
						File string `json:"answer_file"`
						SHA  string `json:"answer_sha256"`
					}
					require.NoError(t, json.Unmarshal(e.PayloadConstraints, &link))
					if link.File == c.Answer {
						require.Equal(t, c.AnswerSHA, link.SHA)
						bound++
					}
				}
			}
		}
		require.Equal(t, 1, bound)
	}
	return d.Cases
}

func assertSLMPIdle(t *testing.T, c slmpIdleControl, es []*ProtocolEvent, deferred bool) {
	t.Helper()
	require.Len(t, es, len(c.Events))
	ids := map[uint64]bool{}
	for i, e := range es {
		a := c.Events[i]
		require.NotZero(t, e.ID)
		require.False(t, ids[e.ID])
		ids[e.ID] = true
		require.Equal(t, "slmp", e.Protocol)
		require.Equal(t, "slmp-binary-self-test", e.Profile)
		require.Equal(t, a.Direction, e.Direction)
		require.Zero(t, e.TransactionID)
		require.Equal(t, len(wrapperWire(t, a.Raw)), e.Length)
		if a.Reply == nil {
			require.Zero(t, e.ResponseTo)
		} else {
			require.Equal(t, es[*a.Reply].ID, e.ResponseTo)
			require.Equal(t, es[*a.Reply].FlowID, e.FlowID)
		}
		f, err := e.GetFields()
		rocTypedError(t, a.Error, err)
		if a.Error != "" {
			require.Nil(t, f)
			require.Nil(t, e.Session)
			require.NotEmpty(t, e.Error)
		} else {
			rocEqualFields(t, a.Fields, f)
			rocEqualFields(t, a.Fields, e.Session)
			status := "decoded"
			if deferred {
				status = "deferred"
			}
			require.Equal(t, status, e.Status)
		}
		if a.Error == "ResourceExceeded" {
			require.Empty(t, e.Raw)
		} else {
			require.Equal(t, wrapperWire(t, a.Raw), e.Raw)
		}
	}
}

func testSLMPIdleSealedMatrix(t *testing.T) {
	for _, c := range slmpIdleControls(t) {
		t.Run("idle-history/"+c.ID, func(t *testing.T) {
			raw := wrapperStructuredInput(t, c.Alias)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			discoveryMatrix(t, func(t *testing.T, w int, d, o bool) {
				es, stats := discoveryReplay(t, bytes.Clone(raw), w, d, o, WithProtocolDecodeAs("udp", 5000, "slmp"), WithProtocolBudget(slmpControlLimit(c), 1<<20))
				assertSLMPIdle(t, c, es, d)
				var limited uint64
				for i, e := range es {
					a := c.Events[i]
					ends := []string{"192.0.2.1:39000", "192.0.2.2:5000"}
					require.Equal(t, ends[a.Direction], e.Source)
					require.Equal(t, ends[1-a.Direction], e.Destination)
					require.Len(t, e.SourceBytes.PacketRefs, 1)
					require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
					require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
					if a.Error == "ResourceExceeded" {
						limited += uint64(len(wrapperWire(t, a.Raw)))
					}
				}
				require.Equal(t, limited, stats.LimitedBytes)
				require.Zero(t, stats.BufferedBytes)
				require.Zero(t, stats.CallbackPanics)
			})
		})
	}
}

func TestSLMPIdleHistoryExistingAPI(t *testing.T) {
	for _, c := range slmpIdleControls(t) {
		for _, d := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/deferred%t", c.ID, d), func(t *testing.T) {
				b := DefaultParserBudget()
				b.MaxMessageBytes, b.MaxFrameBytes = c.Limit, slmpControlLimit(c)
				s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(39000, 5000))
				require.NoError(t, err)
				defer s.Close("cleanup")
				s.(*captureSession).f.a.config.Deferred = d
				var es []*ProtocolEvent
				for _, step := range c.Events {
					wire := wrapperWire(t, step.Raw)
					out := s.Feed(step.Direction, time.Unix(step.Seconds, 0), wire)
					clear(wire)
					require.Len(t, out.Events, 1)
					es = append(es, out.Events...)
				}
				assertSLMPIdle(t, c, es, d)
				for i, e := range es {
					if c.Events[i].Error != "" {
						continue
					}
					fields, err := e.GetFields()
					require.NoError(t, err)
					fields["Loopback Data"] = []byte("mutation")
					e.Session["Loopback Data"] = []byte("public")
					clear(e.Raw)
					fields, err = e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, c.Events[i].Fields, fields)
				}
				require.Empty(t, s.Close("complete"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}

func slmpIdleHistoricalAnswer(t *testing.T, c bsapControl, a bsapAnswer) bsapAnswer {
	t.Helper()
	if c.ID != "3e-idle-new-exchange" && c.ID != "4e-idle-new-exchange" {
		return a
	}
	raw, err := trafficfixture.ReadFile("slmp-idle-history/historical-idle-upgrade.json")
	require.NoError(t, err)
	var doc struct {
		Cases []struct {
			ID      string
			Capture string `json:"capture_sha256"`
			Answer  string `json:"original_answer_sha256"`
			Changes map[int]struct {
				Error  string
				Fields map[string]any
				Reply  *int `json:"response_to_index"`
			}
		}
	}
	require.NoError(t, json.Unmarshal(raw, &doc))
	for _, u := range doc.Cases {
		if u.ID != c.ID {
			continue
		}
		require.Equal(t, c.SHA256, u.Capture)
		require.Equal(t, c.AnswerSHA, u.Answer)
		require.Len(t, u.Changes, 2)
		for i, change := range u.Changes {
			require.True(t, i == 2 || i == 3)
			require.NotNil(t, a.Events[i].Fields)
			a.Events[i].Fields, a.Events[i].Error, a.Events[i].Reply = change.Fields, change.Error, change.Reply
		}
		return a
	}
	t.Fatal("missing SHA-bound historical idle correction")
	return a
}

func slmpControlLimit(c slmpIdleControl) int {
	if c.FrameLimit > 0 {
		return min(c.FrameLimit, c.Limit)
	}
	return c.Limit
}

// Constant untracked-refusal state must preserve already observed independent
// keys and remain bounded under new keys/domains, without resetting on idle.
func TestSLMPUntrackedRefusalIsolation(t *testing.T) {
	controls := slmpIdleControls(t)
	c := controls[2]
	q := wrapperWire(t, c.Events[0].Raw)
	r := wrapperWire(t, c.Events[2].Raw)
	large := wrapperWire(t, controls[7].Events[0].Raw)
	for _, kind := range []string{"frame", "ingress", "memory", "slots"} {
		t.Run(kind, func(t *testing.T) {
			original := DefaultParserBudget()
			s, err := NewProtocolSessionWithOptions(original, WithSessionTransport("udp"), WithSessionPorts(39000, 5000))
			require.NoError(t, err)
			defer s.Close("cleanup")
			a := s.(*captureSession).f.a
			originalBuffered := a.config.MaxBufferedBytes
			feed := func(src, dst string, d CaptureDomain, w []byte) *ProtocolEvent {
				e := &ProtocolEvent{Source: src, Destination: dst, Transport: "udp", Domain: d, Timestamp: time.Unix(1, 0), Length: len(w)}
				require.True(t, a.decodeSLMPDatagram(e, bytes.Clone(w), false))
				return e
			}
			d := CaptureDomain{Interface: 1}
			keep := feed("192.0.2.10:39000", "192.0.2.20:5000", d, q)
			require.Nil(t, keep.sessionError)
			rocEqualFields(t, c.Events[0].Fields, keep.Session)
			a.budget.MaxFrameBytes = 64
			if kind == "memory" {
				a.config.MaxBufferedBytes = int(a.buffered.Load()) + 64
			}
			if kind == "slots" {
				a.budget.MaxCollectionElements = 1
			}
			deniedWire := large
			if kind == "slots" || kind == "memory" {
				deniedWire = q
			}
			var denied *ProtocolEvent
			if kind == "ingress" {
				denied = &ProtocolEvent{Source: "192.0.2.30:39000", Destination: "192.0.2.40:5000", Domain: d, Timestamp: time.Unix(1, 0), Transport: "udp", Length: len(large)}
				require.True(t, a.refuseUDPOversizeAssociation(denied, large, 39000, 5000))
			} else {
				denied = feed("192.0.2.30:39000", "192.0.2.40:5000", d, deniedWire)
			}
			rocTypedError(t, "ResourceExceeded", denied.sessionError)
			require.Nil(t, denied.Session)
			require.Zero(t, denied.ResponseTo)
			require.Zero(t, denied.TransactionID)
			require.True(t, a.slmpUntrackedRefusal)
			a.budget = original
			a.config.MaxBufferedBytes = originalBuffered
			peer := feed("192.0.2.20:5000", "192.0.2.10:39000", d, r)
			require.Nil(t, peer.sessionError)
			require.Equal(t, keep.ID, peer.ResponseTo)
			rocEqualFields(t, c.Events[2].Fields, peer.Session)
			for _, domain := range []CaptureDomain{d, {Interface: 2}} {
				req := feed("192.0.2.30:39000", "192.0.2.40:5000", domain, q)
				late := feed("192.0.2.40:5000", "192.0.2.30:39000", domain, r)
				for _, e := range []*ProtocolEvent{req, late} {
					rocTypedError(t, "ContextRequired", e.sessionError)
					require.Nil(t, e.Session)
					require.Zero(t, e.ResponseTo)
					require.Zero(t, e.TransactionID)
				}
			}
			require.Len(t, a.udpSessions.entries, 3)
			require.EqualValues(t, 1536, a.buffered.Load())
			require.Empty(t, s.Close("complete"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	t.Run("malformed-refusal-hint", func(t *testing.T) {
		mutations := map[string]func([]byte){"declared-length": func(w []byte) { w[7]++ }, "command": func(w []byte) { w[11]++ }, "subcommand": func(w []byte) { w[13]++ }, "loopback-count": func(w []byte) { w[15]++ }}
		for name, mutate := range mutations {
			t.Run(name, func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(39000, 5000))
				require.NoError(t, err)
				defer s.Close("cleanup")
				a := s.(*captureSession).f.a
				a.budget.MaxFrameBytes = 64
				bad := bytes.Clone(large)
				mutate(bad)
				require.False(t, slmpUDPRefusalHint(bad))
				e := &ProtocolEvent{Source: "192.0.2.1:39000", Destination: "192.0.2.2:5000", Transport: "udp", Timestamp: time.Unix(1, 0)}
				require.False(t, a.decodeSLMPDatagram(e, bad, false))
				require.False(t, a.slmpUntrackedRefusal)
				a.budget = DefaultParserBudget()
				req := s.Feed(0, time.Unix(2, 0), q)
				reply := s.Feed(1, time.Unix(3, 0), r)
				require.Nil(t, req.Err)
				require.Nil(t, reply.Err)
				require.Equal(t, req.Events[0].ID, reply.Events[0].ResponseTo)
				rocEqualFields(t, c.Events[2].Fields, reply.Events[0].Session)
			})
		}
	})
}
