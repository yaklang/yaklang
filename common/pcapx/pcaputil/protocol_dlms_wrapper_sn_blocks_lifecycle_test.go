package pcaputil

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDLMSWrapperSNBlocksIdleAndUntrackedResource(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		for _, gap := range []int{29, 30} {
			t.Run(fmt.Sprintf("idle-gap%d/deferred%t", gap, deferred), func(t *testing.T) {
				var c snWriteControl
				for _, v := range snBlockControls(t, true) {
					if v.Name == "exact-block-ACK-and-final-repeats-do-not-advance" {
						c = v
					}
				}
				require.NotEmpty(t, c.Name)
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
				require.NoError(t, err)
				defer s.Close("cleanup")
				s.(*captureSession).f.a.config.Deferred = deferred
				ids := make([]uint64, len(c.Steps))
				var flow uint64
				for _, ix := range []int{0, 1, 3} {
					w := c.Steps[ix]
					wire := wrapperWire(t, w["input_hex"].(string))
					out := s.Feed(int(w["caller_direction"].(float64)), time.Unix(int64(100+lenNonzeroSNIDs(ids)), 0), wire)
					clear(wire)
					require.Equal(t, len(wire), out.Consumed)
					require.False(t, out.NeedMore)
					require.Nil(t, out.Err)
					require.False(t, out.NeedMore)
					require.Len(t, out.Events, 1)
					e := out.Events[0]
					snBlockLifecycleMetadata(t, e, w["input_hex"].(string), int(w["caller_direction"].(float64)), 100+lenNonzeroSNIDs(ids), deferred, false)
					ids[ix] = e.ID
					if ix == 1 {
						flow = e.FlowID
						require.NotZero(t, flow)
					}
					wrapperCloseQueries(t, e, w, ids)
					wrapperClosePoison(e)
					wrapperCloseQueries(t, e, w, ids)
				}
				w := c.Steps[5]
				wire := wrapperWire(t, w["input_hex"].(string))
				out := s.Feed(1, time.Unix(int64(102+gap), 0), wire)
				clear(wire)
				require.Equal(t, len(wire), out.Consumed)
				require.False(t, out.NeedMore)
				require.Nil(t, out.Err)
				require.False(t, out.NeedMore)
				require.Len(t, out.Events, 1)
				e := out.Events[0]
				snBlockLifecycleMetadata(t, e, w["input_hex"].(string), 1, 102+gap, deferred, false)
				ids[5] = e.ID
				require.Equal(t, flow, e.FlowID)
				require.Equal(t, 1, e.Direction)
				require.Zero(t, e.TransactionID)
				require.Zero(t, e.ResponseTo)
				if gap == 29 {
					wrapperCloseQueries(t, e, w, ids)
				} else {
					// Only the old UDP observation TTL changed. A late block cannot
					// resurrect Data or a new generation of the retired wire group.
					want := cloneSession(w["GetFields"].(map[string]any))
					for _, k := range []string{"assembled_data", "assembled_data_hex", "transfer_start_event", "assembled_data_length", "observed_block_count"} {
						delete(want, k)
					}
					want["transfer_complete"] = false
					f, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, want, f)
					session := cloneSession(want)
					session["Association"], session["Outstanding"] = "unverified-sn-block-after-conversation-invalidation", 0
					rocEqualFields(t, session, e.Session)
					d, err := e.Decode()
					require.NoError(t, err)
					require.Equal(t, map[string]any{"fields": f, "session": e.Session}, d)
					poisonWrapperStructured(d)
					poisonWrapperStructured(f)
					poisonWrapperStructured(e.Session)
					d, err = e.Decode()
					require.NoError(t, err)
					require.IsType(t, map[string]any{}, d["fields"])
					require.IsType(t, map[string]any{}, d["session"])
					rocEqualFields(t, want, d["fields"].(map[string]any))
					rocEqualFields(t, session, d["session"].(map[string]any))
				}
				require.Empty(t, s.Close("idle"))
				require.Empty(t, s.Close("repeat"))
				require.Zero(t, s.Stats().BufferedBytes)
				require.Zero(t, s.Stats().Incomplete)
			})
		}
		t.Run(fmt.Sprintf("untracked-frame-refusal/deferred%t", deferred), func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxFrameBytes, b.MaxMessageBytes = 64, 64
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("cleanup")
			s.(*captureSession).f.a.config.Deferred = deferred
			wire := wrapperWire(t, "00010001001000390c010200000132"+strings.Repeat("00", 50))
			out := s.Feed(1, time.Unix(100, 0), wire)
			clear(wire)
			require.Equal(t, len(wire), out.Consumed)
			require.False(t, out.NeedMore)
			rocTypedError(t, "ResourceExceeded", out.Err)
			require.Len(t, out.Events, 1)
			e := out.Events[0]
			snBlockLifecycleMetadata(t, e, "00010001001000390c010200000132"+strings.Repeat("00", 50), 1, 100, deferred, true)
			require.Equal(t, "dlms-wrapper-v1-sn-read-block", e.Profile)
			require.Equal(t, "limited", e.Status)
			require.Equal(t, "limited", e.Completeness)
			require.Equal(t, 65, e.Length)
			require.Nil(t, e.Raw)
			require.Nil(t, e.Session)
			require.Zero(t, e.FlowID)
			require.Zero(t, e.TransactionID)
			require.Zero(t, e.ResponseTo)
			snReadQueryError(t, e, "ResourceExceeded")
			// A smaller complete Data(NULL) block now fits the frame budget.
			// Rejection of the earlier untracked block still prevents assembly.
			hx := "00010001001000080c01020100010100"
			wire = wrapperWire(t, hx)
			out = s.Feed(1, time.Unix(101, 0), wire)
			clear(wire)
			require.Equal(t, len(wire), out.Consumed)
			require.False(t, out.NeedMore)
			require.Nil(t, out.Err)
			require.Len(t, out.Events, 1)
			e = out.Events[0]
			snBlockLifecycleMetadata(t, e, hx, 1, 101, deferred, false)
			require.NotZero(t, e.ID)
			require.Equal(t, "dlms-wrapper-v1-sn-read-block", e.Profile)
			wantStatus := "decoded"
			if deferred {
				wantStatus = "deferred"
			}
			require.Equal(t, wantStatus, e.Status)
			require.Equal(t, "message", e.Completeness)
			require.Zero(t, e.FlowID)
			require.Zero(t, e.TransactionID)
			require.Zero(t, e.ResponseTo)
			want := map[string]any{"wrapper_version": uint16(1), "source_wport": uint16(1), "destination_wport": uint16(16), "wrapper_header_hex": hx[:16], "payload_length": 8, "apdu_hex": hx[16:], "command": byte(12), "reference_mode": "short-name", "association_observed": false, "authentication_verified": false, "object_model_verified": false, "operation_success_verified": false, "list_count": uint64(1), "list_count_encoding_hex": "01", "kind": "ReadResponse", "transfer_complete": false, "results": []map[string]any{{"index": 0, "apdu_offset": 2, "result_choice_raw": byte(2), "result_choice": "data-block-result", "last_block_raw": byte(1), "last_block": true, "block_number": uint16(1), "raw_data_length": uint64(1), "length_encoding_hex": "01", "raw_data_hex": "00", "raw_hex": "020100010100", "encoded_length": 6}}}
			f, err := e.GetFields()
			require.NoError(t, err)
			require.Equal(t, want, f)
			session := cloneSession(want)
			session["Association"], session["Outstanding"] = "unverified-sn-block-after-conversation-invalidation", 0
			require.Equal(t, session, e.Session)
			d, err := e.Decode()
			require.NoError(t, err)
			require.Equal(t, map[string]any{"fields": want, "session": session}, d)
			poisonWrapperStructured(d)
			poisonWrapperStructured(e.Session)
			clear(e.Raw)
			d, err = e.Decode()
			require.NoError(t, err)
			require.Equal(t, map[string]any{"fields": want, "session": session}, d)
			require.EqualValues(t, 65, s.Stats().LimitedBytes)
			require.Empty(t, s.Close("refused"))
			require.Empty(t, s.Close("repeat"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func lenNonzeroSNIDs(ids []uint64) int {
	n := 0
	for _, id := range ids {
		if id != 0 {
			n++
		}
	}
	return n
}

// Carrier expectations are determined by the explicit public session ports,
// direction and timestamps, independently of the returned semantic fields.
func snBlockLifecycleMetadata(t *testing.T, e *ProtocolEvent, hx string, dir, ts int, deferred, rawRefused bool) {
	t.Helper()
	require.NotZero(t, e.ID)
	require.Equal(t, "dlms-wrapper", e.Protocol)
	require.Equal(t, "udp", e.Transport)
	require.Equal(t, dir, e.Direction)
	require.Equal(t, time.Unix(int64(ts), 0), e.Timestamp)
	require.EqualValues(t, 0, e.Offset)
	require.Equal(t, CaptureDomain{}, e.Domain)
	require.Equal(t, fmt.Sprintf("192.0.2.%d:%d", 1+dir, []int{40000, 4059}[dir]), e.Source)
	require.Equal(t, fmt.Sprintf("192.0.2.%d:%d", 2-dir, []int{4059, 40000}[dir]), e.Destination)
	require.Equal(t, len(hx)/2, e.Length)
	if rawRefused {
		require.Nil(t, e.Raw)
	} else {
		require.Equal(t, wrapperWire(t, hx), e.Raw)
	}
}
