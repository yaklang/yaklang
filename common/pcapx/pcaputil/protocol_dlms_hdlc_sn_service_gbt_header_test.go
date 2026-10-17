package pcaputil

import (
	"encoding/hex"
	"github.com/stretchr/testify/require"
	"testing"
)

// This gate delays retirement only; it cannot establish semantic acceptance.
// All six selected headers retain their bounded, definite count-one forms.
func TestDLMSHDLCSNServiceGBTHeaderGate(t *testing.T) {
	for _, command := range []struct{ name, head string }{
		{"ReadQ5", "050105"}, {"ReadQ6", "050106"}, {"ReadR2", "0c0102"},
		{"ReadR3", "0c0103"}, {"WriteQ7", "060107"}, {"WriteR2", "0d0102"},
	} {
		for _, count := range []string{"01", "8101", "820001", "83000001", "8400000001"} {
			t.Run(command.name+"/"+count, func(t *testing.T) {
				raw, err := hex.DecodeString(command.head[:2] + count + command.head[4:])
				require.NoError(t, err)
				before := append([]byte(nil), raw...)
				require.True(t, dlmsSNGBTSinglePhaseCandidate(&wrapperMessage{lastBlock: true, gbtNumber: 1, blockData: raw}))
				require.Equal(t, before, raw)
			})
		}
	}
	for _, c := range []struct {
		name, raw string
		last      bool
		number    uint16
	}{
		{"partial", "050106", false, 1}, {"second-block", "050106", true, 2},
		{"zero-block", "050106", true, 0}, {"empty-ack", "", true, 1},
		{"tag-only", "05", true, 1}, {"count-only", "0501", true, 1},
		{"count-indefinite", "058006", true, 1}, {"truncated-count", "058200", true, 1},
		{"oversize-count", "0585000000000106", true, 1}, {"zero-count", "050006", true, 1},
		{"multi-count", "050206", true, 1}, {"ordinary-read", "0c0100", true, 1},
		{"ordinary-write", "0d0100", true, 1}, {"LN-get", "c40101", true, 1},
		{"wrong-read-choice", "050107", true, 1}, {"wrong-write-choice", "060106", true, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			raw, err := hex.DecodeString(c.raw)
			require.NoError(t, err)
			before := append([]byte{}, raw...)
			require.False(t, dlmsSNGBTSinglePhaseCandidate(&wrapperMessage{lastBlock: c.last, gbtNumber: c.number, blockData: raw}))
			require.Equal(t, before, append([]byte{}, raw...))
		})
	}
}
