package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDLMSWrapperSNMultiResultThreeModesPreserveOtherOwners(t *testing.T) {
	snMultiResultThreeModes(t, "tcp", []int{0, 1, 7})
}
func TestDLMSWrapperSNMultiResultThreeModesRetainedUDP(t *testing.T) {
	raw := finsReadBatch(t, "dlms-sn-multiresult-source", "DLMS_SN_MULTIRESULT_ROOT", "three-mode-state-udp-projection.json")
	require.Equal(t, "86dc9a531669120ab61b20d36713a33dd8216eaee99c12566b2f611c3b8cf725", fmt.Sprintf("%x", sha256.Sum256(raw)))
	snMultiResultThreeModes(t, "udp", []int{0})
}
func snMultiResultThreeModes(t *testing.T, tr string, chunks []int) {
	raw := finsReadBatch(t, "dlms-sn-multiresult-source", "DLMS_SN_MULTIRESULT_ROOT", "three-mode-state.json")
	require.Equal(t, "ee0efed53584607283958d432847d1aa8ad138ad7deb39a0143a95b82f1bfddf", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var gold struct {
		Steps []struct {
			Wire            string `json:"input_hex"`
			Hash            string `json:"input_sha256"`
			Dir             int    `json:"caller_direction"`
			Profile         string
			Fields, Session map[string]any
			Tx              int `json:"Transaction_index"`
			Reply           int `json:"Response_index"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &gold))
	require.Len(t, gold.Steps, 9)
	for _, deferred := range []bool{false, true} {
		for _, chunk := range chunks {
			t.Run(fmt.Sprintf("%s/d%t/chunk%d", tr, deferred, chunk), func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(tr), WithSessionPorts(40000, 4059))
				require.NoError(t, err)
				defer s.Close("cleanup")
				s.(*captureSession).f.a.config.Deferred = deferred
				ids := make([]uint64, 9)
				offsets := [2]uint64{}
				var flow uint64
				endpoints := [2]string{"192.0.2.1:40000", "192.0.2.2:4059"}
				for i, step := range gold.Steps {
					wire := wrapperWire(t, step.Wire)
					require.Equal(t, step.Hash, fmt.Sprintf("%x", sha256.Sum256(wire)))
					size := chunk
					if size == 0 {
						size = len(wire)
					}
					var events []*ProtocolEvent
					for at := 0; at < len(wire); at += size {
						input := append([]byte(nil), wire[at:min(at+size, len(wire))]...)
						result := s.Feed(step.Dir, time.Unix(int64(100+i), 0), input)
						clear(input)
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
					ids[i] = e.ID
					require.NotZero(t, e.ID)
					for _, old := range ids[:i] {
						require.NotEqual(t, old, e.ID)
					}
					require.NotZero(t, e.FlowID)
					if flow == 0 {
						flow = e.FlowID
					}
					require.Equal(t, flow, e.FlowID)
					require.Equal(t, "dlms-wrapper", e.Protocol)
					require.Equal(t, step.Profile, e.Profile)
					require.Equal(t, tr, e.Transport)
					require.Equal(t, step.Dir, e.Direction)
					require.Equal(t, CaptureDomain{}, e.Domain)
					require.Equal(t, endpoints[step.Dir], e.Source)
					require.Equal(t, endpoints[1-step.Dir], e.Destination)
					require.Equal(t, time.Unix(int64(100+i), 0), e.Timestamp)
					if tr == "tcp" {
						require.Equal(t, offsets[step.Dir], e.Offset)
						offsets[step.Dir] += uint64(len(wire))
					} else {
						require.Zero(t, e.Offset)
					}
					require.Equal(t, wire, e.Raw)
					require.Equal(t, len(wire), e.Length)
					require.Empty(t, e.Error)
					status := "decoded"
					if deferred {
						status = "deferred"
					}
					require.Equal(t, status, e.Status)
					require.Equal(t, "message", e.Completeness)
					tx, reply := uint64(0), uint64(0)
					if step.Tx > 0 {
						tx = ids[step.Tx-1]
					}
					if step.Reply > 0 {
						reply = ids[step.Reply-1]
					}
					require.Equal(t, tx, e.TransactionID)
					require.Equal(t, reply, e.ResponseTo)
					want := wrapperCloseRemap(step.Fields, ids).(map[string]any)
					wantSession := wrapperCloseRemap(step.Session, ids).(map[string]any)
					fields, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, want, fields)
					snWriteTypes(t, fields)
					rocEqualFields(t, wantSession, e.Session)
					snWriteTypes(t, e.Session)
					decoded, err := e.Decode()
					require.NoError(t, err)
					rocEqualFields(t, want, decoded["fields"].(map[string]any))
					rocEqualFields(t, wantSession, decoded["session"].(map[string]any))
					poisonWrapperStructured(fields)
					poisonWrapperStructured(e.Session)
					poisonWrapperStructured(decoded)
					again, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, want, again)
					againDecoded, err := e.Decode()
					require.NoError(t, err)
					rocEqualFields(t, want, againDecoded["fields"].(map[string]any))
					rocEqualFields(t, wantSession, againDecoded["session"].(map[string]any))
					snWriteTypes(t, again)
					snWriteTypes(t, againDecoded["session"])
				}
				require.Empty(t, s.Close("three-modes"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}
