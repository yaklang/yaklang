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

func hdlcSetBlockControls(t *testing.T) []hdlcOpCase {
	t.Helper()
	var x struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-hdlc-set-block", "DLMS_HDLC_SET_BLOCK_TEST_ROOT", "controls.json"), &x))
	require.Len(t, x.Cases, 21)
	return x.Cases
}
func TestDLMSHDLCSetBlockNative(t *testing.T) { hdlcOpNative(t, hdlcSetBlockControls(t), 41000, 4059) }
func TestDLMSHDLCSetBlockOffPort(t *testing.T) {
	hdlcOpNative(t, hdlcSetBlockControls(t)[:2], 45000, 45001)
}
func TestDLMSHDLCSetBlockMatrix(t *testing.T) {
	for _, c := range hdlcSetBlockControls(t) {
		for _, transport := range []string{"udp", "tcp"} {
			t.Run(c.Name+"/"+transport, func(t *testing.T) {
				var reuse map[string]string
				require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-hdlc-set-block", "DLMS_HDLC_SET_BLOCK_TEST_ROOT", "reuse-inputs.json"), &reuse))
				name := c.Name + "." + transport + ".pcap"
				raw := []byte(nil)
				if path, ok := reuse[name]; ok {
					parts := strings.SplitN(path, "/", 2)
					require.Len(t, parts, 2)
					raw = finsReadBatch(t, parts[0], "DLMS_HDLC_SET_BLOCK_TEST_ROOT", parts[1])
				} else {
					raw = finsReadBatch(t, "dlms-hdlc-set-block", "DLMS_HDLC_SET_BLOCK_TEST_ROOT", name)
				}
				discoveryMatrix(t, func(t *testing.T, w int, d, o bool) {
					es, stats := hdlcOpReplay(t, c, raw, w, d, o, transport)
					count := len(c.Steps)
					if transport == "tcp" {
						for _, s := range c.Steps {
							if s.TCPOverride != nil {
								count--
							}
						}
					}
					closeCount := len(c.Close.Events)
					if transport == "tcp" && c.TCPCloseOverride != nil {
						closeCount = 0
					}
					require.Len(t, es, count+closeCount)
					hdlcOpCheck(t, c, es[:count], transport, true, d)
					hdlcOpCloseCheck(t, c, es[count:], transport, "ctx-canceled")
					require.EqualValues(t, count, stats.Messages)
					require.EqualValues(t, closeCount, stats.Incomplete)
				})
			})
		}
	}
}

// The boundary answers are independently derived from the existing UDP idle
// policy and the selected pre-decode graph/retention reservation. They are not
// capture replays or TCP idle requirements.
func TestDLMSHDLCSetBlockBoundaryNative(t *testing.T) {
	var raw struct{ Cases []json.RawMessage }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-hdlc-set-block", "DLMS_HDLC_SET_BLOCK_TEST_ROOT", "native-guards.json"), &raw))
	require.Len(t, raw.Cases, 4)
	for _, v := range raw.Cases {
		var c hdlcOpCase
		require.NoError(t, json.Unmarshal(v, &c))
		var bound struct {
			Budget struct{ MaxBufferedBytes int }
		}
		require.NoError(t, json.Unmarshal(v, &bound))
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/d%t", c.Name, deferred), func(t *testing.T) {
				b := DefaultParserBudget()
				if bound.Budget.MaxBufferedBytes > 0 {
					b.MaxBufferedBytes = bound.Budget.MaxBufferedBytes
				}
				session, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(41000, 4059))
				require.NoError(t, err)
				session.(*captureSession).f.a.config.Deferred = deferred
				var events []*ProtocolEvent
				for _, step := range c.Steps {
					wire := wrapperWire(t, step.Wire)
					input := bytes.Clone(wire)
					o := session.Feed(step.Direction, time.Unix(step.At, 0), input)
					clear(input)
					require.Equal(t, len(wire), o.Consumed)
					require.Len(t, o.Events, 1)
					require.False(t, o.NeedMore)
					require.Equal(t, "dlms", o.State)
					if step.Error == "" {
						require.Nil(t, o.Err)
					} else {
						rocTypedError(t, step.Error, o.Err)
					}
					events = append(events, actionSnapshot(o.Events[0]))
					actionPoison(o.Events[0])
				}
				hdlcOpCheck(t, c, events, "udp", false, deferred)
				hdlcOpCloseCheck(t, c, session.Close("hdlc-test"), "udp", "hdlc-test")
				require.Empty(t, session.Close("again"))
				require.Zero(t, session.Stats().BufferedBytes)
				require.LessOrEqual(t, session.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
			})
		}
	}
}
