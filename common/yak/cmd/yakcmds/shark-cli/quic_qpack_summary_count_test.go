package sharkcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func qpackSummaryCountSession(t *testing.T, worker bool, ids []any, counts []int) map[string]any {
	t.Helper()
	var rows []map[string]any
	for i, id := range ids {
		var frames []map[string]any
		for j := 0; j < counts[i]; j++ {
			frames = append(frames, map[string]any{"Frame Type": "HEADERS", "Stream ID": id})
		}
		rows = append(rows, map[string]any{"HTTP3 Stream ID": id, "HTTP3 Frames": frames})
	}
	session := map[string]any{"Authentication Verified": true, "QPACK Resumed Streams": rows}
	if worker {
		raw, err := json.Marshal(session)
		require.NoError(t, err)
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		require.NoError(t, decoder.Decode(&session))
	}
	return session
}

func TestHTTP3QPACKRecoveredSectionsCountDistinctStreams(t *testing.T) {
	for _, worker := range []bool{false, true} {
		for _, tc := range []struct {
			name, want string
			ids        []any
			counts     []int
		}{
			{"both-directions", "QPACK replayed 3 field section(s) on 2 stream(s)", []any{uint64(4), uint64(4), uint64(8)}, []int{1, 1, 1}},
			{"multiple-sections", "QPACK replayed 3 field section(s) on 1 stream(s)", []any{uint64(4), uint64(4)}, []int{2, 1}},
			{"uninterpreted", "QPACK rechecked 2 retained observation(s) on 1 stream(s)", []any{uint64(4), uint64(4)}, []int{0, 1}},
			{"distinct", "QPACK replayed retained fields on 2 stream(s)", []any{uint64(0), uint64(4)}, []int{1, 1}},
			{"large-exact", "QPACK replayed 3 field section(s) on 2 stream(s)", []any{uint64(1<<62 - 4), uint64(1<<62 - 4), uint64(1<<62 - 8)}, []int{1, 1, 1}},
		} {
			t.Run(fmt.Sprintf("%s/worker%v", tc.name, worker), func(t *testing.T) {
				session := qpackSummaryCountSession(t, worker, tc.ids, tc.counts)
				before, err := json.Marshal(session)
				require.NoError(t, err)
				require.Equal(t, tc.want, quicQPACKSummary(session))
				after, err := json.Marshal(session)
				require.NoError(t, err)
				require.Equal(t, before, after)
				session["Authentication Verified"] = false
				require.Empty(t, quicQPACKSummary(session))
			})
		}
	}
}

func TestHTTP3QPACKRecoveredSummaryCannotInventStreamZero(t *testing.T) {
	for _, worker := range []bool{false, true} {
		for i, value := range []any{nil, int64(-1), 1.5, float64(1 << 62), "0", true, uint64(1 << 62), json.Number("4611686018427387904")} {
			t.Run(fmt.Sprintf("%d/worker%v", i, worker), func(t *testing.T) {
				session := qpackSummaryCountSession(t, worker, []any{uint64(0), value}, []int{1, 1})
				require.Equal(t, "QPACK rechecked 2 retained stream observation(s)", quicQPACKSummary(session))
			})
		}
	}
}
