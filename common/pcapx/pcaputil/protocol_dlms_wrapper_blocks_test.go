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

func TestDLMSWrapperBlocksExistingAPIBaseline(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
	require.NoError(t, err)
	for _, w := range []string{"0001000100100007c401c100120100", "000100010010000dc402c101000000010003120100"} {
		out := s.Feed(1, time.Unix(1, 0), wrapperWire(t, w))
		require.Nil(t, out.Err)
		require.Len(t, out.Events, 1)
		fields, err := out.Events[0].GetFields()
		require.NoError(t, err)
		require.NotNil(t, fields)
	}
	require.Empty(t, s.Close("baseline"))
	require.Zero(t, s.Stats().BufferedBytes)
}

type wrapperBlockAnswer struct {
	Wire        string                         `json:"wire_hex"`
	SHA         string                         `json:"wire_sha256"`
	Fields      map[string]any                 `json:"fields"`
	Error       *struct{ Kind, Detail string } `json:"error"`
	Association string                         `json:"association"`
	Outstanding int                            `json:"outstanding"`
	Transaction int                            `json:"transaction_index"`
	Response    int                            `json:"response_index"`
	Dir         int                            `json:"dir"`
	Refs        []uint64                       `json:"packet_refs"`
}
type wrapperBlockControl struct {
	Name      string `json:"name"`
	Alias     string `json:"input_alias"`
	SHA       string `json:"sha256"`
	Transport string `json:"transport"`
	Limit     int    `json:"limit"`
	Close     int    `json:"close_outstanding"`
	Steps     []struct {
		Dir int    `json:"dir"`
		Hex string `json:"hex"`
	} `json:"steps"`
	Answers []wrapperBlockAnswer `json:"answers"`
	Default []wrapperBlockAnswer `json:"default_answers"`
}

func wrapperBlockControls(t *testing.T) []wrapperBlockControl {
	raw, err := trafficfixture.ReadFile("dlms-blocks/controls.json")
	require.NoError(t, err)
	var d struct{ Cases []wrapperBlockControl }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, 33)
	for _, c := range d.Cases {
		input := wrapperStructuredInput(t, c.Alias)
		require.Equal(t, c.SHA, fmt.Sprintf("%x", sha256.Sum256(input)))
		for _, a := range c.Answers {
			require.Equal(t, a.SHA, fmt.Sprintf("%x", sha256.Sum256(wrapperWire(t, a.Wire))))
		}
	}
	return d.Cases
}
func assertWrapperBlocks(t *testing.T, c wrapperBlockControl, events []*ProtocolEvent, deferred bool) {
	t.Helper()
	require.Len(t, events, len(c.Answers))
	offsets := [2]uint64{}
	id := func(i int) uint64 {
		if i == 0 {
			return 0
		}
		return events[i-1].ID
	}
	for i, e := range events {
		a := c.Answers[i]
		w := wrapperWire(t, a.Wire)
		fields, err := e.GetFields()
		require.NotZero(t, e.ID)
		if i > 0 {
			require.Greater(t, e.ID, events[i-1].ID)
		}
		require.Equal(t, "dlms-wrapper", e.Protocol)
		require.Equal(t, wrapperProfile(w), e.Profile)
		require.Equal(t, len(w), e.Length)
		if a.Error != nil {
			rocTypedError(t, a.Error.Kind, err)
			require.Nil(t, fields)
			require.Nil(t, e.Session)
			require.Zero(t, e.ResponseTo)
			require.Zero(t, e.TransactionID)
			if a.Error.Kind == "ResourceExceeded" && c.Transport == "udp" {
				require.Nil(t, e.Raw)
			} else {
				require.Equal(t, w, e.Raw)
			}
		} else {
			require.NoError(t, err)
			require.Empty(t, e.Error)
			require.Equal(t, w, e.Raw)
			status := "decoded"
			if deferred {
				status = "deferred"
			}
			require.Equal(t, status, e.Status)
			require.Equal(t, "message", e.Completeness)
			rocEqualFields(t, a.Fields, fields)
			expected := cloneSession(a.Fields)
			expected["Association"], expected["Outstanding"] = a.Association, float64(a.Outstanding)
			rocEqualFields(t, expected, e.Session)
			require.Equal(t, id(a.Transaction), e.TransactionID)
			require.Equal(t, id(a.Response), e.ResponseTo)
			if c.Transport == "tcp" {
				require.Equal(t, a.Dir, e.Direction)
				require.Equal(t, offsets[a.Dir], e.Offset)
			}
		}
		offsets[a.Dir] += uint64(len(w))
	}
}
func TestDLMSWrapperBlocksSealedMatrix(t *testing.T) {
	for _, c := range wrapperBlockControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				var opts []CaptureOption
				if c.Transport == "udp" {
					opts = append(opts, WithProtocolDecodeAs("udp", 4059, "dlms-wrapper"))
				}
				if len(c.Default) != 0 {
					c.Answers = c.Default
				}

				events, stats := discoveryReplay(t, wrapperStructuredInput(t, c.Alias), workers, deferred, observe, opts...)
				if c.Close != 0 {
					require.Len(t, events, len(c.Answers)+1)
					assertWrapperStructuredClose(t, events[len(c.Answers):])
					events = events[:len(c.Answers)]
					require.EqualValues(t, 1, stats.Incomplete)
				}
				assertWrapperBlocks(t, c, events, deferred)
				for i, e := range events {
					a := c.Answers[i]
					ends := []string{"192.0.2.1:40000", "192.0.2.2:4059"}
					require.Equal(t, ends[a.Dir], e.Source)
					require.Equal(t, ends[1-a.Dir], e.Destination)
					require.Len(t, e.SourceBytes.PacketRefs, len(a.Refs))
					for j, r := range e.SourceBytes.PacketRefs {
						require.Equal(t, a.Refs[j], r.Number)
						require.Equal(t, e.Domain, r.Domain)
					}
				}
			})
		})
	}
}
func TestDLMSWrapperBlocksBudgetsOwnership(t *testing.T) {
	for _, c := range wrapperBlockControls(t) {
		for _, deferred := range []bool{false, true} {
			for _, size := range []int{1, 7, 64, 65543} {
				if c.Transport == "udp" && size != 65543 {
					continue
				}
				t.Run(fmt.Sprintf("%s/%v/%d", c.Name, deferred, size), func(t *testing.T) {
					b := DefaultParserBudget()
					b.MaxCollectionElements = c.Limit
					s, err := NewProtocolSessionWithOptions(b, WithSessionTransport(c.Transport), WithSessionPorts(40000, 4059))
					require.NoError(t, err)
					s.(*captureSession).f.a.config.Deferred = deferred
					var events []*ProtocolEvent
					for _, step := range c.Steps {
						wire := wrapperWire(t, step.Hex)
						for at := 0; at < len(wire); {
							end := min(at+size, len(wire))
							chunk := append([]byte(nil), wire[at:end]...)
							out := s.Feed(step.Dir, time.Unix(100, 0), chunk)
							for i := range chunk {
								chunk[i] ^= 255
							}
							events = append(events, out.Events...)
							at = end
							if out.Err != nil && out.Err.Kind != ErrNeedMore {
								break
							}
						}
					}
					assertWrapperBlocks(t, c, events, deferred)
					for i, e := range events {
						if c.Answers[i].Error != nil {
							continue
						}
						poisonWrapperStructured(e.Session)
						f, err := e.GetFields()
						require.NoError(t, err)
						rocEqualFields(t, c.Answers[i].Fields, f)
						poisonWrapperStructured(f)
						f, err = e.GetFields()
						require.NoError(t, err)
						rocEqualFields(t, c.Answers[i].Fields, f)
					}
					closed := s.Close("blocks")
					if c.Close != 0 {
						assertWrapperStructuredClose(t, closed)
					} else {
						require.Empty(t, closed)
					}
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
}
