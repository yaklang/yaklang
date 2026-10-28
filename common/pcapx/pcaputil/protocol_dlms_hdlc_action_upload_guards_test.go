package pcaputil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func hdlcActionAdjacentControls(t *testing.T) []hdlcOpCase {
	t.Helper()
	var x struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-hdlc-action-upload", "DLMS_HDLC_ACTION_UPLOAD_TEST_ROOT", "adjacent-guards.json"), &x))
	require.Len(t, x.Cases, 2)
	return x.Cases
}

func TestDLMSHDLCActionUploadForeignResponseNative(t *testing.T) {
	hdlcOpNative(t, hdlcActionAdjacentControls(t)[1:], 41000, 4059)
}
func TestDLMSHDLCActionUploadForeignResponseMatrix(t *testing.T) {
	c := hdlcActionAdjacentControls(t)[1]
	for _, transport := range []string{"udp", "tcp"} {
		t.Run(transport, func(t *testing.T) {
			raw := finsReadBatch(t, "dlms-hdlc-action-upload", "DLMS_HDLC_ACTION_UPLOAD_TEST_ROOT", c.Name+"."+transport+".pcap")
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				es, stats := hdlcOpReplay(t, c, raw, workers, deferred, observe, transport)
				require.Len(t, es, len(c.Steps))
				hdlcOpCheck(t, c, es, transport, true, deferred)
				require.EqualValues(t, len(c.Steps), stats.Messages)
				require.Zero(t, stats.Incomplete)
			})
		})
	}
}
func TestDLMSHDLCActionUploadColdLinkNative(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprintf("d%t", deferred), func(t *testing.T) {
			c := hdlcActionAdjacentControls(t)[0]
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41000, 4059))
			require.NoError(t, err)
			// Explicit observed decoder context, not a weaker automatic S/DM signature.
			f := s.(*captureSession).f
			f.a.datagramDecodeAs = map[uint16]string{4059: "dlms"}
			f.a.config.Deferred = deferred
			var events []*ProtocolEvent
			for _, step := range c.Steps {
				wire := wrapperWire(t, step.Wire)
				in := bytes.Clone(wire)
				o := s.Feed(step.Direction, time.Unix(step.At, 0), in)
				clear(in)
				require.Equal(t, len(wire), o.Consumed)
				require.False(t, o.NeedMore)
				require.Nil(t, o.Err)
				require.Equal(t, "dlms", o.State)
				require.Len(t, o.Events, 1)
				require.Equal(t, "explicit-decode-as", o.Events[0].Admission)
				events = append(events, actionSnapshot(o.Events[0]))
				actionPoison(o.Events[0])
			}
			hdlcOpCheck(t, c, events, "udp", false, deferred)
			require.Empty(t, s.Close("link-test"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
func TestDLMSHDLCActionUploadColdLinkMatrix(t *testing.T) {
	c := hdlcActionAdjacentControls(t)[0]
	raw := finsReadBatch(t, "dlms-hdlc-action-upload", "DLMS_HDLC_ACTION_UPLOAD_TEST_ROOT", c.Name+".udp.pcap")
	discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
		es, stats := hdlcOpReplay(t, c, raw, workers, deferred, observe, "udp")
		require.Len(t, es, 2)
		for i, e := range es {
			require.Equal(t, "explicit-decode-as", e.Admission)
			require.Equal(t, "192.0.2.1:41000", e.Source)
			require.Equal(t, "192.0.2.2:4059", e.Destination)
			require.Len(t, e.SourceBytes.PacketRefs, 1)
			require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
			require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
		}
		expected := c
		expected.Steps = append([]hdlcOpStep(nil), c.Steps...)
		// A created DM is a response-direction wire observation. Packet provenance
		// still names the captured source, independent of this logical direction.
		expected.Steps[1].Direction = 1
		hdlcOpCheck(t, expected, es, "udp", false, deferred)
		require.EqualValues(t, 2, stats.Messages)
		require.EqualValues(t, 18, stats.MessageBytes)
		require.Zero(t, stats.Incomplete)
		require.Zero(t, stats.Malformed)
		require.Zero(t, stats.ContextRequired)
	})
}

func hdlcActionGetOwnerControl(t *testing.T) hdlcOpCase {
	t.Helper()
	var x struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-hdlc-action-upload", "DLMS_HDLC_ACTION_UPLOAD_TEST_ROOT", "Get-foreign-Action-ACK-supplement.json"), &x))
	require.Len(t, x.Cases, 1)
	return x.Cases[0]
}
func TestDLMSHDLCActionUploadGetOwnerNative(t *testing.T) {
	hdlcOpNative(t, []hdlcOpCase{hdlcActionGetOwnerControl(t)}, 41000, 4059)
}
func TestDLMSHDLCActionUploadGetOwnerMatrix(t *testing.T) {
	c := hdlcActionGetOwnerControl(t)
	for _, transport := range []string{"udp", "tcp"} {
		t.Run(transport, func(t *testing.T) {
			raw := finsReadBatch(t, "dlms-hdlc-action-upload", "DLMS_HDLC_ACTION_UPLOAD_TEST_ROOT", c.Name+"."+transport+".pcap")
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				es, stats := hdlcOpReplay(t, c, raw, workers, deferred, observe, transport)
				require.Len(t, es, len(c.Steps))
				hdlcOpCheck(t, c, es, transport, true, deferred)
				require.EqualValues(t, 3, stats.Messages)
				require.EqualValues(t, 1, stats.ContextRequired)
				require.Zero(t, stats.Incomplete)
				require.Zero(t, stats.Malformed)
			})
		})
	}
}

func TestDLMSHDLCActionUploadBoundaryNative(t *testing.T) {
	var raw struct{ Cases []json.RawMessage }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-hdlc-action-upload", "DLMS_HDLC_ACTION_UPLOAD_TEST_ROOT", "native-guards.json"), &raw))
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
