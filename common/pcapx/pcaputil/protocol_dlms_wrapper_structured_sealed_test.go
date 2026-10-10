package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"path/filepath"
	"testing"
	"time"
)

type wrapperStructuredControl struct {
	wrapperListControl
	InputAlias string `json:"input_alias"`
	Depth      int
}

func wrapperStructuredInput(t *testing.T, alias string) []byte {
	t.Helper()
	root, err := trafficfixture.RepositoryRoot()
	require.NoError(t, err)
	w, err := trafficfixture.ReadFile(filepath.Join(root, alias))
	require.NoError(t, err)
	return w
}

func wrapperStructuredControls(t *testing.T) []wrapperStructuredControl {
	t.Helper()
	raw, err := trafficfixture.ReadFile("dlms-structured/controls.json")
	require.NoError(t, err)
	var d struct{ Cases []json.RawMessage }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, 28)
	cases := make([]wrapperStructuredControl, 0, len(d.Cases))
	for _, r := range d.Cases {
		var c wrapperStructuredControl
		require.NoError(t, json.Unmarshal(r, &c))
		var limits struct {
			Limits struct {
				Depth int `json:"depth_limit"`
			} `json:"static_limits"`
		}
		require.NoError(t, json.Unmarshal(r, &limits))
		c.Depth = limits.Limits.Depth
		a, err := trafficfixture.ReadFile("dlms-structured/answers/" + c.Name + ".json")
		require.NoError(t, err)
		require.JSONEq(t, string(r), string(a))
		input := wrapperStructuredInput(t, c.InputAlias)
		require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(input)))
		for _, a := range c.Answers {
			require.Equal(t, a.SHA, fmt.Sprintf("%x", sha256.Sum256(wrapperWire(t, a.Wire))))
		}
		cases = append(cases, c)
	}
	return cases
}
func wrapperStructuredHistoricalFields(t *testing.T, wire string) map[string]any {
	t.Helper()
	for _, c := range wrapperStructuredControls(t) {
		if c.Name == "historical-structure-now-supported" {
			require.Len(t, c.Answers, 1)
			require.Equal(t, wire, c.Answers[0].Wire)
			require.Nil(t, c.Answers[0].Error)
			return c.Answers[0].Fields
		}
	}
	t.Fatal("missing independently answered historical structure")
	return nil
}
func wrapperStructuredBudget(c wrapperStructuredControl) ParserBudget {
	b := wrapperListBudget(c.wrapperListControl)
	if c.Depth > 0 {
		b.MaxRecursionDepth = c.Depth
	}
	return b
}
func wrapperStructuredPairs(name string, custom bool) [][2]int {
	switch name {
	case "structured-exchange-tcp", "structured-exchange-udp", "split-structured":
		return [][2]int{{0, 1}}
	case "coalesced-structured":
		return [][2]int{{0, 2}, {1, 3}}
	case "budget-retires-pending-udp":
		if !custom {
			return [][2]int{{0, 1}}
		}
	}
	return nil
}
func poisonWrapperStructured(v any) {
	switch x := v.(type) {
	case map[string]any:
		for _, child := range x {
			poisonWrapperStructured(child)
		}
		x["raw_hex"] = "caller"
	case []map[string]any:
		for _, child := range x {
			poisonWrapperStructured(child)
		}
	case []any:
		for _, child := range x {
			poisonWrapperStructured(child)
		}
	}
}
func assertWrapperStructuredAnswers(t *testing.T, c wrapperStructuredControl, events []*ProtocolEvent, custom bool) {
	t.Helper()
	require.Len(t, events, len(c.Answers))
	offsets := [2]uint64{}
	for i, e := range events {
		a := c.Answers[i]
		fields, problem := a.Fields, a.Error
		if !custom && a.Default != nil {
			fields, problem = a.Default.Fields, a.Default.Error
		}
		wire := wrapperWire(t, a.Wire)
		f, err := e.GetFields()
		require.Equal(t, "dlms-wrapper", e.Protocol)
		require.Equal(t, "dlms-wrapper-v1-get-list", e.Profile)
		require.Equal(t, len(wire), e.Length)
		if problem != nil {
			rocTypedError(t, problem.Kind, err)
			require.Nil(t, f)
			require.Nil(t, e.Session)
			require.Zero(t, e.ResponseTo)
			require.Zero(t, e.TransactionID)
			if problem.Kind == "ResourceExceeded" {
				require.Equal(t, "limited", e.Status)
				if c.Transport == "udp" {
					require.Nil(t, e.Raw)
				} else {
					// These are field-level refusals after the complete raw frame
					// was admitted and charged, not pre-frame byte-budget previews.
					require.Equal(t, wire, e.Raw)
				}
			} else {
				require.Equal(t, wire, e.Raw)
			}
		} else {
			require.NoError(t, err)
			require.Empty(t, e.Error)
			require.Equal(t, wire, e.Raw)
			rocEqualFields(t, fields, f)
			require.Equal(t, offsets[e.Direction], e.Offset)
			poisonWrapperStructured(f)
			again, err := e.GetFields()
			require.NoError(t, err)
			rocEqualFields(t, fields, again)
		}
		if c.Transport == "tcp" {
			offsets[e.Direction] += uint64(len(wire))
		}
		paired := false
		for _, pair := range wrapperStructuredPairs(c.Name, custom) {
			if i == pair[1] {
				require.Equal(t, events[pair[0]].ID, e.ResponseTo)
				require.Equal(t, e.ResponseTo, e.TransactionID)
				require.NotEqual(t, e.Direction, events[pair[0]].Direction)
				paired = true
			}
		}
		if !paired {
			require.Zero(t, e.ResponseTo)
		}
	}
}
func TestDLMSWrapperStructuredSealedMatrix(t *testing.T) {
	for _, c := range wrapperStructuredControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			input := wrapperStructuredInput(t, c.InputAlias)
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				var options []CaptureOption
				if c.Transport == "udp" {
					options = append(options, WithProtocolDecodeAs("udp", 4059, "dlms-wrapper"))
				}
				events, stats := discoveryReplay(t, input, workers, deferred, observe, options...)
				if c.Name == "structured-selection-cursor" {
					require.Len(t, events, 2)
					assertWrapperStructuredClose(t, events[1:])
					require.EqualValues(t, 1, stats.Incomplete)
					events = events[:1]
				}
				assertWrapperStructuredAnswers(t, c, events, false)
				require.EqualValues(t, len(c.Answers), stats.Messages)
				for i, e := range events {
					require.Len(t, e.SourceBytes.PacketRefs, len(c.Answers[i].Refs))
					for j, ref := range e.SourceBytes.PacketRefs {
						require.EqualValues(t, c.Answers[i].Refs[j], ref.Number)
						require.Equal(t, e.Domain, ref.Domain)
					}
				}
			})
		})
	}
}
func TestDLMSWrapperStructuredBudgetsOwnership(t *testing.T) {
	for _, c := range wrapperStructuredControls(t) {
		for _, deferred := range []bool{false, true} {
			t.Run(c.Name+fmt.Sprint(deferred), func(t *testing.T) {
				chunkSizes := []int{1, 7, 64, 65543}
				if c.Transport == "udp" {
					chunkSizes = []int{65543}
				} // Datagram boundaries cannot be repartitioned.
				for _, size := range chunkSizes {
					s, err := NewProtocolSessionWithOptions(wrapperStructuredBudget(c), WithSessionTransport(c.Transport), WithSessionPorts(40000, 4059))
					require.NoError(t, err)
					s.(*captureSession).f.a.config.Deferred = deferred
					var events []*ProtocolEvent
					for _, step := range c.Steps {
						input := wrapperWire(t, step.Hex)
						for at := 0; at < len(input); {
							end := min(len(input), at+size)
							chunk := append([]byte(nil), input[at:end]...)
							o := s.Feed(step.Dir, time.Unix(100, 0), chunk)
							for j := range chunk {
								chunk[j] ^= 0xff
							}
							events = append(events, o.Events...)
							at = end
							if o.Err != nil && o.Err.Kind != ErrNeedMore {
								require.Contains(t, []ProtocolErrorKind{ErrMalformedMessage, ErrUnsupportedFeature, ErrResourceExceeded}, o.Err.Kind)
								break
							}
						}
					}
					assertWrapperStructuredAnswers(t, c, events, true)
					if c.Name == "budget-retires-pending-udp" {
						require.Equal(t, "ambiguous-conversation", events[2].Session["Association"])
						require.EqualValues(t, 0, events[2].Session["Outstanding"])
					}
					require.LessOrEqual(t, s.Stats().BufferedBytes, int64(wrapperStructuredBudget(c).MaxBufferedBytes))
					closed := s.Close("structured")
					if c.Name == "structured-selection-cursor" {
						assertWrapperStructuredClose(t, closed)
					} else {
						require.Empty(t, closed)
					}
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				}
			})
		}
	}
}

func assertWrapperStructuredClose(t *testing.T, events []*ProtocolEvent) {
	t.Helper()
	require.Len(t, events, 1)
	e := events[0]
	require.Equal(t, "dlms-wrapper", e.Protocol)
	require.Equal(t, "incomplete", e.Status)
	require.Empty(t, e.Raw)
	require.Zero(t, e.Length)
	require.Zero(t, e.ResponseTo)
	require.EqualValues(t, 1, e.Session["Outstanding"])
	require.Contains(t, e.Summary, "DLMS Wrapper exchange ended with unmatched observed requests")
	f, err := e.GetFields()
	require.EqualError(t, err, "protocol parser: event is incomplete, not an exact message")
	require.Nil(t, f)
}
func TestDLMSWrapperUDPIdleObservationBoundary(t *testing.T) {
	// The independently executed loopback peer labels the first identical reply
	// as requestA's. Wire bytes cannot expose that label after requestB's new epoch.
	var c wrapperStructuredControl
	for _, v := range wrapperStructuredControls(t) {
		if v.Name == "structured-exchange-udp" {
			c = v
		}
	}
	q, r := wrapperWire(t, c.Steps[0].Hex), wrapperWire(t, c.Steps[1].Hex)
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprint(deferred), func(t *testing.T) {
			fresh := func() ProtocolSession {
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
				require.NoError(t, err)
				s.(*captureSession).f.a.config.Deferred = deferred
				return s
			}
			feed := func(s ProtocolSession, dir int, at int64, wire []byte) *ProtocolEvent {
				o := s.Feed(dir, time.Unix(at, 0), wire)
				require.Nil(t, o.Err)
				require.Len(t, o.Events, 1)
				f, err := o.Events[0].GetFields()
				require.NoError(t, err)
				require.Equal(t, false, f["association_observed"])
				require.Equal(t, false, f["authentication_verified"])
				return o.Events[0]
			}
			s := fresh()
			a := feed(s, 0, 100, q)
			b := feed(s, 0, 131, q)
			late := feed(s, 1, 131, r)
			require.NotEqual(t, a.FlowID, b.FlowID)
			require.Equal(t, b.ID, late.ResponseTo)
			require.NotEqual(t, a.ID, late.ResponseTo)
			require.Equal(t, "observed-response", late.Session["Association"])
			duplicate := feed(s, 1, 131, r)
			require.Zero(t, duplicate.ResponseTo)
			require.Equal(t, "unmatched-response", duplicate.Session["Association"])
			require.Empty(t, s.Close("ambiguous-origin"))
			require.Zero(t, s.Stats().BufferedBytes)
			s = fresh()
			feed(s, 0, 100, q)
			expired := feed(s, 1, 130, r)
			require.Zero(t, expired.ResponseTo)
			require.Equal(t, "unmatched-response", expired.Session["Association"])
			require.Zero(t, s.Stats().BufferedBytes)
			s.Close("expired")
			s = fresh()
			normal := feed(s, 0, 100, q)
			reply := feed(s, 1, 101, r)
			require.Equal(t, normal.ID, reply.ResponseTo)
			s.Close("normal")
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
