package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDLMSWrapperBitStringExistingAPIBaseline(t *testing.T) {
	for _, wire := range []string{
		"0001000100100008c403c10100120100",   // Adjacent unsigned16 remains supported.
		"0001000100100009c403c101000409a580", // Nine MSB-first bits, two value bytes.
	} {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
		require.NoError(t, err)
		o := s.Feed(1, time.Unix(1, 0), wrapperWire(t, wire))
		require.Nil(t, o.Err)
		require.Len(t, o.Events, 1)
		fields, err := o.Events[0].GetFields()
		require.NoError(t, err)
		require.NotNil(t, fields)
		require.Zero(t, o.Events[0].ResponseTo)
		require.Zero(t, o.Events[0].TransactionID)
		require.Empty(t, s.Close("baseline"))
		require.Zero(t, s.Stats().BufferedBytes)
	}
}

type wrapperBitsControl struct {
	wrapperStructuredControl
	DefaultPairs [][]int `json:"default_pairs"`
}

func wrapperBitsControls(t *testing.T) []wrapperBitsControl {
	return wrapperOwnedDataControls(t, "dlms-bits", 26)
}

func wrapperOwnedDataControls(t *testing.T, prefix string, count int) []wrapperBitsControl {
	t.Helper()
	raw, err := trafficfixture.ReadFile(prefix + "/controls.json")
	require.NoError(t, err)
	var d struct{ Cases []json.RawMessage }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, count)
	cases := make([]wrapperBitsControl, 0, len(d.Cases))
	for _, row := range d.Cases {
		var c wrapperBitsControl
		require.NoError(t, json.Unmarshal(row, &c))
		var limits struct {
			Limits struct {
				Depth int `json:"depth_limit"`
			} `json:"static_limits"`
		}
		require.NoError(t, json.Unmarshal(row, &limits))
		c.Depth = limits.Limits.Depth
		a, err := trafficfixture.ReadFile(prefix + "/answers/" + c.Name + ".json")
		require.NoError(t, err)
		require.JSONEq(t, string(row), string(a))
		require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(wrapperStructuredInput(t, c.InputAlias))))
		for _, a := range c.Answers {
			require.Equal(t, a.SHA, fmt.Sprintf("%x", sha256.Sum256(wrapperWire(t, a.Wire))))
		}
		cases = append(cases, c)
	}
	return cases
}

func TestDLMSWrapperBitStringSealedMatrix(t *testing.T) {
	wrapperOwnedDataMatrix(t, wrapperBitsControls(t))
}

func wrapperOwnedDataMatrix(t *testing.T, cases []wrapperBitsControl) {
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			input := wrapperStructuredInput(t, c.InputAlias)
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				var options []CaptureOption
				if c.Transport == "udp" {
					options = append(options, WithProtocolDecodeAs("udp", 4059, "dlms-wrapper"))
				}
				events, stats := discoveryReplay(t, input, workers, deferred, observe, options...)
				c.Targets.Pairs = c.DefaultPairs
				frameEvents := events
				expectedEvents := len(c.Answers)
				if c.ExpectedCloseOutstanding != 0 {
					require.Equal(t, 1, c.ExpectedCloseOutstanding)
					require.Len(t, events, expectedEvents+1)
					assertWrapperStructuredClose(t, events[expectedEvents:])
					require.Greater(t, events[expectedEvents].ID, events[expectedEvents-1].ID)
					frameEvents = events[:expectedEvents]
					require.EqualValues(t, 1, stats.Incomplete)
				}
				assertWrapperStructuredAnswers(t, c.wrapperStructuredControl, frameEvents, false)
				require.EqualValues(t, expectedEvents, stats.Messages)
				for i, e := range frameEvents {
					dir := -1
					for _, step := range c.Steps {
						if step.Ref == c.Answers[i].Refs[0] {
							dir = step.Dir
						}
					}
					require.NotEqual(t, -1, dir)
					endpoints := []string{"192.0.2.1:40000", "192.0.2.2:4059"}
					require.Equal(t, endpoints[dir], e.Source)
					require.Equal(t, endpoints[1-dir], e.Destination)
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

func TestDLMSWrapperBitStringBudgetsOwnership(t *testing.T) {
	wrapperOwnedDataBudgetsOwnership(t, wrapperBitsControls(t))
}

func wrapperOwnedDataBudgetsOwnership(t *testing.T, cases []wrapperBitsControl) {
	for _, c := range cases {
		for _, deferred := range []bool{false, true} {
			t.Run(c.Name+fmt.Sprint(deferred), func(t *testing.T) {
				sizes := []int{1, 7, 64, 65543}
				if c.Transport == "udp" {
					sizes = []int{65543} // UDP preserves datagram boundaries.
				}
				for _, size := range sizes {
					s, err := NewProtocolSessionWithOptions(wrapperStructuredBudget(c.wrapperStructuredControl), WithSessionTransport(c.Transport), WithSessionPorts(40000, 4059))
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
					assertWrapperStructuredAnswers(t, c.wrapperStructuredControl, events, true)
					for i, e := range events {
						if c.Answers[i].Error != nil {
							continue
						}
						poisonWrapperStructured(e.Session)
						f, err := e.GetFields()
						require.NoError(t, err)
						rocEqualFields(t, c.Answers[i].Fields, f)
					}
					closed := s.Close("bits")
					if c.ExpectedCloseOutstanding != 0 {
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
