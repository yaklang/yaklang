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

type wrapperRefusalControl struct {
	wrapperBlockControl
	FrameLimit   int    `json:"max_frame_bytes"`
	MessageLimit int    `json:"max_message_bytes"`
	Limited      uint64 `json:"limited_bytes"`
	Answer       string `json:"answer_file"`
	AnswerSHA    string `json:"answer_sha256"`
	Packets      int    `json:"packets"`
}

func wrapperRefusalControls(t *testing.T) []wrapperRefusalControl {
	t.Helper()
	raw, err := trafficfixture.ReadFile("wrapper-first-refusal/controls.json")
	require.NoError(t, err)
	var doc struct {
		Schema string
		Cases  []wrapperRefusalControl
	}
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Equal(t, "owned-wrapper-first-refusal/v1", doc.Schema)
	require.Len(t, doc.Cases, 3)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for _, c := range doc.Cases {
		capture := wrapperStructuredInput(t, c.Alias)
		require.Equal(t, c.SHA, fmt.Sprintf("%x", sha256.Sum256(capture)))
		answer, err := trafficfixture.ReadFile("wrapper-first-refusal/" + c.Answer)
		require.NoError(t, err)
		require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(answer)))
		var expected wrapperRefusalControl
		require.NoError(t, json.Unmarshal(answer, &expected))
		require.Equal(t, c.Answers, expected.Answers)
		bound := 0
		for _, batch := range all {
			for _, row := range batch.Cases {
				if row.Input.OriginalPath != c.Alias {
					continue
				}
				require.Equal(t, c.SHA, row.Input.SHA256)
				require.Equal(t, c.Alias, row.Input.OriginalPath)
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
	return doc.Cases
}

// Both first native-frame and shared-ingress refusals lost their unretained
// identity in the old implementation. This uses the same public Feed/GetFields
// API as the compiled old failure, with complete golden field/session maps.
func TestDLMSWrapperFirstUntrackedRefusalExistingAPI(t *testing.T) {
	for _, c := range wrapperRefusalControls(t) {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/deferred%t", c.Name, deferred), func(t *testing.T) {
				b := DefaultParserBudget()
				b.MaxMessageBytes, b.MaxFrameBytes, b.ProbeBytes = c.MessageLimit, c.FrameLimit, 64
				s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
				require.NoError(t, err)
				defer s.Close("cleanup")
				s.(*captureSession).f.a.config.Deferred = deferred
				var events []*ProtocolEvent
				for i, step := range c.Steps {
					wire := wrapperWire(t, step.Hex)
					out := s.Feed(step.Dir, time.Unix(int64(i+1), 0), wire)
					clear(wire)
					require.Len(t, out.Events, 1)
					require.Equal(t, step.Dir, out.Events[0].Direction)
					events = append(events, out.Events...)
				}
				assertWrapperBlocks(t, c.wrapperBlockControl, events, deferred)
				for i, e := range events {
					if c.Answers[i].Error != nil {
						continue
					}
					fields, err := e.GetFields()
					require.NoError(t, err)
					fields["apdu_hex"] = "returned mutation"
					e.Session["apdu_hex"] = "public mutation"
					clear(e.Raw)
					fields, err = e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, c.Answers[i].Fields, fields)
				}
				require.Equal(t, c.Limited, s.Stats().LimitedBytes)
				require.Empty(t, s.Close("complete"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}

func testWrapperFirstRefusalMatrix(t *testing.T) {
	for _, c := range wrapperRefusalControls(t) {
		t.Run("first-refusal/"+c.Name, func(t *testing.T) {
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				// Capture API shares the frame/ingress byte limit. The public
				// session cases above separately execute each distinct path.
				es, stats := discoveryReplay(t, bytes.Clone(wrapperStructuredInput(t, c.Alias)), workers, deferred, observe, WithProtocolBudget(min(c.FrameLimit, c.MessageLimit), 1<<20))
				assertWrapperBlocks(t, c.wrapperBlockControl, es, deferred)
				for i, e := range es {
					a := c.Answers[i]
					ends := []string{"192.0.2.1:40000", "192.0.2.2:4059"}
					require.Equal(t, ends[a.Dir], e.Source)
					require.Equal(t, ends[1-a.Dir], e.Destination)
					// A new stateless key has no flow-relative direction; literal
					// source/destination still preserve the carrier direction.
					dir := a.Dir
					if c.Limited != 0 {
						dir = 0
					}
					require.Equal(t, dir, e.Direction)
					require.Len(t, e.SourceBytes.PacketRefs, 1)
					require.Equal(t, a.Refs[0], e.SourceBytes.PacketRefs[0].Number)
					require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
				}
				require.Equal(t, c.Limited, stats.LimitedBytes)
				require.Zero(t, stats.BufferedBytes)
				require.Zero(t, stats.CallbackPanics)
			})
		})
	}
}

func TestDLMSWrapperUntrackedRefusalRetainedPeers(t *testing.T) {
	controls := wrapperRefusalControls(t)
	q, r := wrapperWire(t, controls[0].Steps[0].Hex), wrapperWire(t, controls[0].Steps[1].Hex)
	large := wrapperWire(t, controls[1].Steps[0].Hex)
	for _, kind := range []string{"frame", "ingress", "memory", "slots"} {
		t.Run(kind, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("cleanup")
			a := s.(*captureSession).f.a
			original := a.budget
			originalBuffered := a.config.MaxBufferedBytes
			at := time.Unix(1, 0)
			feed := func(src, dst string, domain CaptureDomain, wire []byte) *ProtocolEvent {
				at = at.Add(time.Millisecond)
				e := &ProtocolEvent{Source: src, Destination: dst, Domain: domain, Timestamp: at, Transport: "udp", Length: len(wire)}
				require.True(t, a.decodeWrapperDatagram(e, bytes.Clone(wire), 40000, 4059, false))
				return e
			}
			d := CaptureDomain{Interface: 1}
			keep := feed("192.0.2.10:40000", "192.0.2.20:4059", d, q)
			require.NotZero(t, keep.TransactionID)
			require.NotZero(t, a.buffered.Load())
			a.budget.MaxFrameBytes = 64
			if kind == "memory" {
				a.config.MaxBufferedBytes = int(a.buffered.Load()) + 64
			}
			if kind == "slots" {
				a.budget.MaxCollectionElements = 1
			}
			wire := large
			if kind == "memory" || kind == "slots" {
				wire = q
			}
			var denied *ProtocolEvent
			if kind == "ingress" {
				denied = &ProtocolEvent{Source: "192.0.2.30:40000", Destination: "192.0.2.40:4059", Domain: d, Timestamp: at, Transport: "udp", Length: len(large)}
				require.True(t, a.refuseUDPOversizeAssociation(denied, large, 40000, 4059))
			} else {
				denied = feed("192.0.2.30:40000", "192.0.2.40:4059", d, wire)
			}
			_, err = denied.GetFields()
			rocTypedError(t, "ResourceExceeded", err)
			require.Nil(t, denied.Session)
			require.Zero(t, denied.TransactionID)
			require.Zero(t, denied.ResponseTo)
			require.True(t, a.wrapperUntrackedRefusal)
			a.budget = original
			a.config.MaxBufferedBytes = originalBuffered
			peer := feed("192.0.2.20:4059", "192.0.2.10:40000", d, r)
			require.Equal(t, keep.ID, peer.ResponseTo, "unretained independent refusal must preserve existing context")
			rocEqualFields(t, controls[0].Answers[1].Fields, peer.semanticFields)
			for _, domain := range []CaptureDomain{d, {Interface: 2}} {
				newRequest := feed("192.0.2.30:40000", "192.0.2.40:4059", domain, q)
				late := feed("192.0.2.40:4059", "192.0.2.30:40000", domain, r)
				for i, e := range []*ProtocolEvent{newRequest, late} {
					fields, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, controls[0].Answers[i].Fields, fields)
					require.Equal(t, "ambiguous-conversation", e.Session["Association"])
					require.Zero(t, e.ResponseTo)
					require.Zero(t, e.TransactionID)
				}
			}
			require.Len(t, a.udpSessions.entries, 1, "constant refusal marker cannot create an unbudgeted key map")
			require.Empty(t, s.Close("complete"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDLMSWrapperRefusalHintMalformedNeighbors(t *testing.T) {
	c := wrapperRefusalControls(t)
	large := wrapperWire(t, c[1].Steps[0].Hex)
	for _, offset := range []int{1, 7, 8, 9, 10} {
		t.Run(fmt.Sprint(offset), func(t *testing.T) {
			wire := bytes.Clone(large)
			wire[offset] ^= 0x20
			require.False(t, wrapperUDPRefusalHint(wire))
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("cleanup")
			a := s.(*captureSession).f.a
			e := &ProtocolEvent{Source: "192.0.2.1:40000", Destination: "192.0.2.2:4059", Timestamp: time.Unix(1, 0), Length: len(wire)}
			a.refuseUDPOversizeAssociation(e, wire, 40000, 4059)
			require.False(t, a.wrapperUntrackedRefusal, "unadmitted malformed header must not retire unrelated context")
			var events []*ProtocolEvent
			for i, step := range c[0].Steps {
				out := s.Feed(step.Dir, time.Unix(int64(i+2), 0), wrapperWire(t, step.Hex))
				require.Len(t, out.Events, 1)
				events = append(events, out.Events...)
			}
			assertWrapperBlocks(t, c[0].wrapperBlockControl, events, false)
			require.Empty(t, s.Close("normal"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
