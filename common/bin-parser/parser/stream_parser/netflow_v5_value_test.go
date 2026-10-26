package stream_parser

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func nf5TestWire(count int) []byte {
	w := make([]byte, 24+48*count)
	binary.BigEndian.PutUint16(w, 5)
	binary.BigEndian.PutUint16(w[2:], uint16(count))
	binary.BigEndian.PutUint32(w[4:], 1000)
	binary.BigEndian.PutUint32(w[8:], 1700000000)
	binary.BigEndian.PutUint32(w[12:], 999999999)
	binary.BigEndian.PutUint32(w[16:], 0xfffffffe)
	w[20], w[21] = 1, 2
	binary.BigEndian.PutUint16(w[22:], 0x8123)
	for i := 0; i < count; i++ {
		b := w[24+i*48 : 72+i*48]
		for j := range b {
			b[j] = byte(i + j)
		}
		b[36], b[44], b[45], b[46], b[47] = 0, 24, 32, 0, 0
	}
	return w
}

func TestNetFlowV5EveryFixedRecordAndLiteralRange(t *testing.T) {
	for _, count := range []int{1, 2, 29, 30} {
		w := nf5TestWire(count)
		m, err := DecodeNetFlowV5Wire(w, 4096)
		require.NoError(t, err)
		require.Equal(t, uint64(count), m["Data Record Count"])
		require.Equal(t, uint64(2), m["Sampling Mode"])
		require.Equal(t, uint64(0x123), m["Sampling Interval"])
		ranges := m["Wire Field Ranges"].(map[string]any)
		require.Equal(t, []any{uint64(22), uint64(24)}, ranges["Sampling Mode"])
		require.Equal(t, []any{uint64(22), uint64(24)}, ranges["Sampling Interval"])
		for i, row := range m["Records"].([]map[string]any) {
			require.Equal(t, uint64(i+1), row["Index"])
			fs := row["Fields"].([]map[string]any)
			require.Len(t, fs, 18)
			for _, f := range fs {
				rawSpan := f["Wire Field Ranges"].(map[string]any)["Value"].([]any)
				span := []int{int(rawSpan[0].(uint64)), int(rawSpan[1].(uint64))}
				require.GreaterOrEqual(t, span[0], 24+i*48)
				require.LessOrEqual(t, span[1], 72+i*48)
				require.Equal(t, int(f["Field Length"].(uint64)), span[1]-span[0])
				if f["Value Kind"] == "unsigned" {
					var value uint64
					for _, b := range w[span[0]:span[1]] {
						value = value<<8 | uint64(b)
					}
					require.Equal(t, value, f["Value"])
				}
			}
		}
	}
}

func TestNetFlowV5CompleteBoundsAndConformanceWarnings(t *testing.T) {
	w := nf5TestWire(2)
	for cut := 0; cut < len(w); cut++ {
		m, err := DecodeNetFlowV5Wire(w[:cut], 4096)
		require.Error(t, err)
		require.Nil(t, m)
	}
	for _, change := range []func([]byte){
		func(b []byte) { b[1] = 9 }, func(b []byte) { b[3] = 0 }, func(b []byte) { b[3] = 31 },
	} {
		bad := append([]byte(nil), w...)
		change(bad)
		m, err := DecodeNetFlowV5Wire(bad, 4096)
		require.Error(t, err)
		require.Nil(t, m)
	}
	_, err := DecodeNetFlowV5Wire(append(w, 0), 4096)
	require.Error(t, err)
	for _, limit := range []int{0, 37} {
		_, err = DecodeNetFlowV5Wire(w, limit)
		require.ErrorIs(t, err, ErrObservedWireBudget)
	}
	_, err = DecodeNetFlowV5Wire(w, 38)
	require.NoError(t, err)
	w[22] = 0xc0
	w[24+36] = 7
	w[24+44] = 255
	w[24+46] = 1
	m, err := DecodeNetFlowV5Wire(w, 4096)
	require.NoError(t, err)
	require.Contains(t, m["Conformance Warning"], "sampling")
	row := m["Records"].([]map[string]any)[0]
	require.Equal(t, uint64(7), row["Padding 1"])
	require.Contains(t, row["Conformance Warning"], "padding")
	f := row["Fields"].([]map[string]any)[16]
	require.Equal(t, uint64(255), f["Value"])
	require.Contains(t, f["Conformance Warning"], "no prefix")
	binary.BigEndian.PutUint32(w[12:], 1000000000)
	m, err = DecodeNetFlowV5Wire(w, 4096)
	require.ErrorContains(t, err, "nanoseconds")
	require.Equal(t, false, m["Export Timestamp Valid"])
	require.Equal(t, true, m["Wire Grammar Complete"])
	require.Len(t, m["Records"].([]map[string]any), 2)
}

func FuzzNetFlowV5FixedBounds(f *testing.F) {
	f.Add(nf5TestWire(1))
	f.Add(nf5TestWire(30))
	f.Add([]byte{0, 5, 0, 1})
	f.Fuzz(func(t *testing.T, w []byte) {
		m, err := DecodeNetFlowV5Wire(w, 4096)
		if err != nil {
			if m == nil {
				return
			}
			require.Equal(t, false, m["Export Timestamp Valid"])
			require.GreaterOrEqual(t, binary.BigEndian.Uint32(w[12:16]), uint32(1000000000))
		}
		count := int(binary.BigEndian.Uint16(w[2:]))
		require.Equal(t, 24+48*count, len(w))
		require.GreaterOrEqual(t, count, 1)
		require.LessOrEqual(t, count, 30)
		require.Len(t, m["Records"].([]map[string]any), count)
	})
}
