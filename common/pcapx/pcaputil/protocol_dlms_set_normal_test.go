package pcaputil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func setControls(t *testing.T) []actionCase {
	var x struct{ Cases []actionCase }
	raw := finsReadBatch(t, "dlms-set-normal", "DLMS_SET_TEST_ROOT", "controls.json")
	require.NoError(t, json.Unmarshal(raw, &x))
	require.Len(t, x.Cases, 33)
	return setBlockHistoricalUpgrade(t, raw, setSelectiveLegacy(t, raw, x.Cases))
}
func setCloseCheck(t *testing.T, c actionCase, es []*ProtocolEvent, transport, reason string) {
	t.Helper()
	require.Len(t, es, len(c.Close.Events))
	for i, e := range es {
		a := c.Close.Events[i]
		require.Equal(t, "dlms-wrapper", e.Protocol)
		profile := "dlms-wrapper-v1-set-normal"
		if a.Session["request_command"] == float64(195) {
			profile = "dlms-wrapper-v1-action-normal"
		}
		require.Equal(t, setBlockHistoricalCloseProfile(c, profile), e.Profile)
		require.Equal(t, "incomplete", e.Status)
		require.Equal(t, "incomplete", e.Completeness)
		require.Nil(t, e.Raw)
		require.Zero(t, e.Length)
		require.Zero(t, e.ResponseTo)
		require.Zero(t, e.TransactionID)
		require.Equal(t, a.Direction, e.Direction)
		rocEqualFields(t, a.Session, e.Session)
		want := a.UDP
		if transport == "tcp" {
			want = strings.Replace(a.TCP, "<reason>", reason, 1)
		} else {
			require.Equal(t, a.At, e.Timestamp.Unix())
		}
		require.Equal(t, want, e.Summary)
		require.Equal(t, "NeedMore: "+want, e.Error)
		actionQueries(t, e, nil, nil, "NeedMore")
		if e.Session != nil {
			poisonWrapperStructured(e.Session)
		}
		e.Protocol = "redis"
		e.Profile = "redis"
		e.Fields = map[string]any{"caller": true}
		e.Structured = map[string]any{"caller": true}
		e.Raw = []byte{0}
		actionQueries(t, e, nil, nil, "NeedMore")
	}
}
func TestDLMSSetNormalNative(t *testing.T) {
	for _, c := range setControls(t) {
		for _, transport := range []string{"udp", "tcp"} {
			if transport == "tcp" {
				bad := false
				for _, a := range c.Steps {
					bad = bad || a.Error != ""
				}
				if bad {
					continue
				}
			}
			for _, d := range []bool{false, true} {
				for _, size := range []int{0, 1, 7} {
					if transport == "udp" && size != 0 {
						continue
					}
					t.Run(fmt.Sprintf("%s/%s/d%t/chunk%d", c.Name, transport, d, size), func(t *testing.T) {
						b := DefaultParserBudget()
						if c.Budget.Depth > 0 {
							b.MaxRecursionDepth = c.Budget.Depth
						}
						if c.Budget.Collection > 0 {
							b.MaxCollectionElements = c.Budget.Collection
						}
						s, err := NewProtocolSessionWithOptions(b, WithSessionTransport(transport), WithSessionPorts(41000, 4059))
						require.NoError(t, err)
						s.(*captureSession).f.a.config.Deferred = d
						var es []*ProtocolEvent
						for _, a := range c.Steps {
							w := wrapperWire(t, a.Wire)
							for at := 0; at < len(w); {
								end := len(w)
								if size > 0 {
									end = min(at+size, end)
								}
								in := bytes.Clone(w[at:end])
								o := s.Feed(a.Direction, time.Unix(a.At, 0), in)
								clear(in)
								if at == 0 && end == len(w) {
									if a.Error == "" {
										require.Nil(t, o.Err)
									} else {
										rocTypedError(t, a.Error, o.Err)
									}
								} else if o.Err != nil {
									require.Equal(t, ErrNeedMore, o.Err.Kind)
								}
								for _, e := range o.Events {
									snap := actionSnapshot(e)
									es = append(es, snap)
									actionPoison(e)
								}
								at = end
							}
						}
						actionCheck(t, c, es, transport, false, d)
						setCloseCheck(t, c, s.Close("action-test"), transport, "action-test")
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
						require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
					})
				}
			}
		}
	}
}
func TestDLMSSetNormalMatrix(t *testing.T) {
	for _, c := range setControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			raw := finsReadBatch(t, "dlms-set-normal", "DLMS_SET_TEST_ROOT", c.Name+".pcap")
			discoveryMatrix(t, func(t *testing.T, w int, d, o bool) {
				es, stats := actionReplay(t, c, raw, w, d, o)
				require.Len(t, es, len(c.Steps)+len(c.Close.Events))
				actionCheck(t, c, es[:len(c.Steps)], "udp", true, d)
				setCloseCheck(t, c, es[len(c.Steps):], "udp", "")
				require.EqualValues(t, len(c.Steps), stats.Messages)
				require.EqualValues(t, len(c.Close.Events), stats.Incomplete)
			})
		})
	}
}

func TestDLMSSetNormalProjectionRefusalLifecycle(t *testing.T) {
	q := wrapperWire(t, "0001001000010012c101c1000100002a0000ff02000903010203")
	r := wrapperWire(t, "0001000100100004c501c100")
	need := 512 + 32768 + 512*len(q) + 2048*min(256, len(q)-21) + 256
	for _, delta := range []int{-1, 0} {
		for _, d := range []bool{false, true} {
			t.Run(fmt.Sprintf("delta%d/d%t", delta, d), func(t *testing.T) {
				b := DefaultParserBudget()
				b.MaxFrameBytes = 4096
				b.MaxMessageBytes = 4096
				b.MaxBufferedBytes = need + delta
				s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(41000, 4059))
				require.NoError(t, err)
				s.(*captureSession).f.a.config.Deferred = d
				first := s.Feed(0, time.Unix(1, 0), bytes.Clone(q))
				require.Len(t, first.Events, 1)
				if delta == 0 {
					require.Nil(t, first.Err)
					f, err := first.Events[0].GetFields()
					require.NoError(t, err)
					rocEqualFields(t, map[string]any{"type": float64(9), "raw_hex": "0903010203", "length": float64(3), "length_encoding_hex": "03", "value_hex": "010203"}, f["data"].(map[string]any))
					require.Equal(t, first.Events[0].ID, first.Events[0].TransactionID)
				} else {
					rocTypedError(t, "ResourceExceeded", first.Err)
					require.Nil(t, first.Events[0].Raw)
					require.Nil(t, first.Events[0].Session)
					require.Zero(t, first.Events[0].ResponseTo)
					require.Zero(t, first.Events[0].TransactionID)
					actionQueries(t, first.Events[0], nil, nil, "ResourceExceeded")
				}
				late := s.Feed(1, time.Unix(2, 0), bytes.Clone(r))
				require.Nil(t, late.Err)
				require.Len(t, late.Events, 1)
				if delta == 0 {
					require.Equal(t, first.Events[0].ID, late.Events[0].ResponseTo)
					require.Equal(t, first.Events[0].ID, late.Events[0].TransactionID)
					require.Equal(t, "observed-response", late.Events[0].Session["Association"])
				} else {
					require.Zero(t, late.Events[0].ResponseTo)
					require.Zero(t, late.Events[0].TransactionID)
					require.Equal(t, "ambiguous-conversation", late.Events[0].Session["Association"])
				}
				require.Equal(t, 0, late.Events[0].Session["Outstanding"])
				require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
				require.Empty(t, s.Close("projection"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}
func TestDLMSSetNormalIdleRetiresOwner(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41000, 4059))
	require.NoError(t, err)
	q := wrapperWire(t, "0001001000010012c101c1000100002a0000ff02000903010203")
	r := wrapperWire(t, "0001000100100004c501c100")
	first := s.Feed(0, time.Unix(1, 0), bytes.Clone(q))
	require.Nil(t, first.Err)
	require.Len(t, first.Events, 1)
	late := s.Feed(1, time.Unix(31, 0), bytes.Clone(r))
	require.Nil(t, late.Err)
	require.Len(t, late.Events, 1)
	require.Zero(t, late.Events[0].ResponseTo)
	require.Zero(t, late.Events[0].TransactionID)
	require.Equal(t, "ambiguous-conversation", late.Events[0].Session["Association"])
	next := s.Feed(0, time.Unix(32, 0), bytes.Clone(q))
	require.Nil(t, next.Err)
	require.Len(t, next.Events, 1)
	require.Zero(t, next.Events[0].TransactionID)
	require.Equal(t, "ambiguous-conversation", next.Events[0].Session["Association"])
	require.Empty(t, s.Close("idle"))
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestDLMSSetNormalOffPortChunks(t *testing.T) {
	c := setControls(t)[0]
	for _, transport := range []string{"udp", "tcp"} {
		for _, d := range []bool{false, true} {
			for _, chunk := range []int{0, 1, 7} {
				if transport == "udp" && chunk != 0 {
					continue
				}
				t.Run(fmt.Sprintf("%s/d%t/chunk%d", transport, d, chunk), func(t *testing.T) {
					s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(45000, 45001))
					require.NoError(t, err)
					s.(*captureSession).f.a.config.Deferred = d
					var es []*ProtocolEvent
					for _, a := range c.Steps {
						w := wrapperWire(t, a.Wire)
						for at := 0; at < len(w); {
							end := len(w)
							if chunk > 0 {
								end = min(at+chunk, end)
							}
							input := bytes.Clone(w[at:end])
							o := s.Feed(a.Direction, time.Unix(a.At, 0), input)
							clear(input)
							if o.Err != nil {
								require.Equal(t, ErrNeedMore, o.Err.Kind)
							}
							for _, e := range o.Events {
								es = append(es, actionSnapshot(e))
								actionPoison(e)
							}
							at = end
						}
					}
					actionCheck(t, c, es, transport, false, d)
					require.Empty(t, s.Close("offport"))
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
}
