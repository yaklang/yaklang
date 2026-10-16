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

func hdlcListControls(t *testing.T) []hdlcOpCase {
	t.Helper()
	var x struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-hdlc-operation-lists", "DLMS_HDLC_OPERATION_LISTS_TEST_ROOT", "controls.json"), &x))
	require.Len(t, x.Cases, 46)
	return x.Cases
}
func TestDLMSHDLCOperationListsNative(t *testing.T) {
	hdlcOpNative(t, hdlcListControls(t), 41000, 4059)
}
func TestDLMSHDLCOperationListsOffPort(t *testing.T) {
	hdlcOpNative(t, hdlcListControls(t)[:2], 45000, 45001)
}
func TestDLMSHDLCOperationListsMatrix(t *testing.T) {
	for _, c := range hdlcListControls(t) {
		for _, transport := range []string{"udp", "tcp"} {
			t.Run(c.Name+"/"+transport, func(t *testing.T) {
				var reuse map[string]string
				require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-hdlc-operation-lists", "DLMS_HDLC_OPERATION_LISTS_TEST_ROOT", "reuse-inputs.json"), &reuse))
				name := c.Name + "." + transport + ".pcap"
				raw := []byte(nil)
				if path, ok := reuse[name]; ok {
					parts := strings.SplitN(path, "/", 2)
					require.Len(t, parts, 2)
					raw = finsReadBatch(t, parts[0], "DLMS_HDLC_OPERATION_LISTS_TEST_ROOT", parts[1])
				} else {
					raw = finsReadBatch(t, "dlms-hdlc-operation-lists", "DLMS_HDLC_OPERATION_LISTS_TEST_ROOT", name)
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

func TestDLMSHDLCOperationListsProjectionRefusalLifecycle(t *testing.T) {
	controls := hdlcListControls(t)
	for _, v := range []struct{ index, divisor, nodes int }{{0, 10, 0}, {4, 9, 0}, {43, 10, 256}, {44, 10, 0}} {
		original := controls[v.index]
		q := wrapperWire(t, original.Steps[2].Wire)
		// Complete one-octet-address HDLC frame has14 non-APDU bytes; the
		// conservative list node/item bounds start after its4-byte service header.
		nodes := min(256, len(q)-18)
		if v.nodes != 0 {
			nodes = v.nodes
		}
		need := 512 + 32768 + 512*len(q) + 2048*nodes + 8192*min(64, (len(q)-18)/v.divisor)
		for _, delta := range []int{-1, 0} {
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/delta%d/d%t", original.Name, delta, deferred), func(t *testing.T) {
					b := DefaultParserBudget()
					b.MaxFrameBytes = 4096
					b.MaxMessageBytes = 4096
					b.MaxBufferedBytes = need + delta
					s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(41000, 4059))
					require.NoError(t, err)
					s.(*captureSession).f.a.config.Deferred = deferred
					c := original
					c.Steps = append([]hdlcOpStep(nil), original.Steps...)
					if delta < 0 {
						for i := 2; i < len(c.Steps); i++ {
							step := &c.Steps[i]
							step.Session = nil
							step.Fields = nil
							step.Response = 0
							step.Tx = 0
							step.Status = "context-required"
							step.Completeness = "context-required"
							step.Error = "ResourceExceeded"
							if i == 3 {
								step.Error = "ContextRequired"
							}
						}
					}
					var events []*ProtocolEvent
					for _, step := range c.Steps {
						w := wrapperWire(t, step.Wire)
						input := bytes.Clone(w)
						o := s.Feed(step.Direction, time.Unix(step.At, 0), input)
						clear(input)
						require.Equal(t, len(w), o.Consumed)
						require.Len(t, o.Events, 1)
						if step.Error == "" {
							require.Nil(t, o.Err)
						} else {
							rocTypedError(t, step.Error, o.Err)
						}
						events = append(events, actionSnapshot(o.Events[0]))
						actionPoison(o.Events[0])
					}
					hdlcOpCheck(t, c, events, "udp", false, deferred)
					require.Empty(t, s.Close("projection"))
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
					require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
				})
			}
		}
	}
}
