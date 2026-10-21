package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// Remove the intervening late-echo probe from the independently specified
// trace: the exact old block1 must itself stay retired, never start a new group.
func TestDLMSWrapperSNMultiResultNoImplicitNewGeneration(t *testing.T) {
	raw := finsReadBatch(t, "dlms-sn-multiresult-source", "DLMS_SN_MULTIRESULT_ROOT", "state-trace.json")
	require.Equal(t, "8fe565dd26ac1fa4facf9daef9ab29b442d48346d01c343d4b9ec164c23eda60", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct {
		Steps []struct {
			Wire            string `json:"input_hex"`
			Dir             int    `json:"caller_direction"`
			Fields, Session map[string]any
			Profile         string
		}
	}
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Steps, 6)
	for _, tr := range []string{"tcp", "udp"} {
		for _, deferred := range []bool{false, true} {
			chunks := []int{0, 1, 7}
			if tr == "udp" {
				chunks = []int{0}
			}
			for _, chunk := range chunks {
				t.Run(fmt.Sprintf("%s/d%t/chunk%d", tr, deferred, chunk), func(t *testing.T) {
					session, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(tr), WithSessionPorts(40000, 4059))
					require.NoError(t, err)
					defer session.Close("cleanup")
					session.(*captureSession).f.a.config.Deferred = deferred
					ids := make([]uint64, 6)
					var flow, offset uint64
					for sequence, index := range []int{0, 1, 3} {
						expected := d.Steps[index]
						require.Equal(t, 1, expected.Dir)
						wire := wrapperWire(t, expected.Wire)
						size := chunk
						if size == 0 {
							size = len(wire)
						}
						var events []*ProtocolEvent
						for at := 0; at < len(wire); at += size {
							in := append([]byte(nil), wire[at:min(at+size, len(wire))]...)
							result := session.Feed(1, time.Unix(int64(100+sequence), 0), in)
							clear(in)
							require.Equal(t, min(size, len(wire)-at), result.Consumed)
							if len(result.Events) == 0 {
								rocTypedError(t, "NeedMore", result.Err)
								require.True(t, result.NeedMore)
							} else {
								require.Nil(t, result.Err)
								require.False(t, result.NeedMore)
							}
							events = append(events, result.Events...)
						}
						require.Len(t, events, 1)
						e := events[0]
						ids[index] = e.ID
						require.NotZero(t, e.ID)
						for _, old := range ids[:index] {
							if old != 0 {
								require.NotEqual(t, old, e.ID)
							}
						}
						require.NotZero(t, e.FlowID)
						if flow == 0 {
							flow = e.FlowID
						}
						require.Equal(t, flow, e.FlowID)
						require.Equal(t, "dlms-wrapper", e.Protocol)
						require.Equal(t, expected.Profile, e.Profile)
						require.Zero(t, e.TransactionID)
						require.Zero(t, e.ResponseTo)
						status := "decoded"
						if deferred {
							status = "deferred"
						}
						require.Equal(t, status, e.Status)
						require.Equal(t, "message", e.Completeness)
						require.Empty(t, e.Error)
						require.Equal(t, CaptureDomain{}, e.Domain)
						require.Equal(t, 1, e.Direction)
						require.Equal(t, "192.0.2.2:4059", e.Source)
						require.Equal(t, "192.0.2.1:40000", e.Destination)
						require.Equal(t, time.Unix(int64(100+sequence), 0), e.Timestamp)
						require.Equal(t, len(wire), e.Length)
						require.Equal(t, wire, e.Raw)
						if tr == "tcp" {
							require.Equal(t, offset, e.Offset)
						} else {
							require.Zero(t, e.Offset)
						}
						offset += uint64(len(wire))
						fields, err := e.GetFields()
						require.NoError(t, err)
						want := wrapperCloseRemap(expected.Fields, ids).(map[string]any)
						rocEqualFields(t, want, fields)
						snReadTypes(t, fields)
						wantSession := wrapperCloseRemap(expected.Session, ids).(map[string]any)
						rocEqualFields(t, wantSession, e.Session)
						snReadTypes(t, e.Session)
						decoded, err := e.Decode()
						require.NoError(t, err)
						rocEqualFields(t, want, decoded["fields"].(map[string]any))
						rocEqualFields(t, wantSession, decoded["session"].(map[string]any))
						if sequence == 2 {
							for _, key := range []string{"transfer_start_event", "observed_block_count", "assembled_data_length", "assembled_data", "duplicate_of_event", "acknowledged_block_event"} {
								require.NotContains(t, fields, key)
							}
						}
						poisonWrapperStructured(fields)
						poisonWrapperStructured(e.Session)
						poisonWrapperStructured(decoded)
						again, err := e.GetFields()
						require.NoError(t, err)
						rocEqualFields(t, want, again)
					}
					require.Empty(t, session.Close("retired"))
					require.Empty(t, session.Close("again"))
					require.Zero(t, session.Stats().BufferedBytes)
				})
			}
		}
	}
}
