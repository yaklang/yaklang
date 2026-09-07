package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func dicomNativePDV(context, control byte, fragment []byte) []byte {
	body := make([]byte, 6+len(fragment))
	binary.BigEndian.PutUint32(body, uint32(2+len(fragment)))
	body[4], body[5] = context, control
	copy(body[6:], fragment)
	return body
}

func TestDICOMPDVBodyOffsetsAndFragments(t *testing.T) {
	// Literal wire fields independently pin big-endian length, offsets, and
	// the allowance for both empty fragments and nonzero reserved control bits.
	body := []byte{0, 0, 0, 6, 255, 0xfd, 0x12, 0x34, 0xab, 0xcd, 0, 0, 0, 2, 255, 0xfe, 0, 0, 0, 4, 255, 3, 0xde, 0xad}
	before := append([]byte(nil), body...)
	records, err := decodeDICOMPDVBody(body)
	require.NoError(t, err)
	require.Equal(t, []dicomPDVRecord{
		{Offset: 0, Length: 6, ContextID: 255, Control: 0xfd, FragmentOffset: 6, FragmentLength: 4},
		{Offset: 10, Length: 2, ContextID: 255, Control: 0xfe, FragmentOffset: 16, FragmentLength: 0},
		{Offset: 16, Length: 4, ContextID: 255, Control: 3, FragmentOffset: 22, FragmentLength: 2},
	}, records)
	for i, want := range [][]byte{{0x12, 0x34, 0xab, 0xcd}, {}, {0xde, 0xad}} {
		record := records[i]
		require.Equal(t, want, body[record.FragmentOffset:record.FragmentOffset+record.FragmentLength])
	}
	require.Equal(t, before, body, "decoding must not modify input")
	// Returned values do not share storage with another parse's descriptors.
	other, err := decodeDICOMPDVBody(body)
	require.NoError(t, err)
	records[0].Offset = 99
	require.Zero(t, other[0].Offset)
}

func TestDICOMPDVBodyEveryContextAndControlByte(t *testing.T) {
	for context := 0; context < 256; context++ {
		body := dicomNativePDV(byte(context), 0xff, nil)
		records, err := decodeDICOMPDVBody(body)
		if context&1 == 0 {
			require.ErrorContains(t, err, "context ID", "context %d", context)
			require.Nil(t, records)
		} else {
			require.NoError(t, err)
			require.Equal(t, []dicomPDVRecord{{Offset: 0, Length: 2, ContextID: uint8(context), Control: 255, FragmentOffset: 6}}, records)
		}
	}
	for control := 0; control < 256; control++ {
		body := dicomNativePDV(1, byte(control), []byte{0xff, 0})
		records, err := decodeDICOMPDVBody(body)
		require.NoError(t, err, "control %d", control)
		require.Equal(t, uint8(control), records[0].Control)
		require.Equal(t, 2, records[0].FragmentLength)
	}
}

func TestDICOMPDVBodyLengthAndExactBoundary(t *testing.T) {
	valid := dicomNativePDV(1, 2, []byte{0xaa, 0xbb, 0xcc, 0xdd})
	for cut := 0; cut < len(valid); cut++ {
		records, err := decodeDICOMPDVBody(valid[:cut])
		require.Error(t, err, "prefix %d", cut)
		require.Nil(t, records)
	}
	for _, length := range []uint32{0, 1, 2, 3, 4, 5, 7, 8, 0x80000000, 0xffffffff} {
		body := append([]byte(nil), valid...)
		binary.BigEndian.PutUint32(body, length)
		records, err := decodeDICOMPDVBody(body)
		require.Error(t, err, "length %d", length)
		require.Nil(t, records)
	}
	for _, size := range []int{0, 2, 4, 254, 256, 65534, 65536} {
		body := dicomNativePDV(253, 0xfc, bytes.Repeat([]byte{0xa5}, size))
		records, err := decodeDICOMPDVBody(body)
		require.NoError(t, err)
		require.Equal(t, uint32(size+2), records[0].Length)
		require.Equal(t, size, records[0].FragmentLength)
		require.Equal(t, len(body), records[0].FragmentOffset+records[0].FragmentLength)
	}
	for _, size := range []int{1, 3, 255, 257, 65535} {
		records, err := decodeDICOMPDVBody(dicomNativePDV(1, 0, make([]byte, size)))
		require.ErrorContains(t, err, "even byte length")
		require.Nil(t, records)
	}
	// No trailing prefix may be silently discarded. Exact exhaustion may also
	// end at a complete earlier PDV; that is a valid, shorter complete body.
	for tail := 1; tail < 6; tail++ {
		records, err := decodeDICOMPDVBody(append(append([]byte(nil), valid...), make([]byte, tail)...))
		require.Error(t, err)
		require.Nil(t, records)
	}
	records, err := decodeDICOMPDVBody(append(append([]byte(nil), valid...), dicomNativePDV(1, 3, nil)...))
	require.NoError(t, err)
	require.Len(t, records, 2)
}

func TestDICOMPDVBodySameContextAndNoPartialFailure(t *testing.T) {
	first := dicomNativePDV(3, 1, []byte{1, 2})
	for _, tc := range []struct {
		name, diagnostic string
		tail             []byte
	}{
		{"different-context", "same context", dicomNativePDV(5, 2, nil)},
		{"zero-context", "context ID", dicomNativePDV(0, 2, nil)},
		{"even-context", "context ID", dicomNativePDV(2, 2, nil)},
		{"odd-fragment", "even byte length", dicomNativePDV(3, 2, []byte{1})},
		{"missing-length", "PDV length", []byte{0}},
		{"truncated-length", "PDV length", []byte{0, 0, 0}},
		{"length-zero", "PDV length", []byte{0, 0, 0, 0}},
		{"length-one", "PDV length", []byte{0, 0, 0, 1, 3}},
		{"missing-context-control", "PDV length", []byte{0, 0, 0, 2}},
		{"missing-control", "PDV length", []byte{0, 0, 0, 2, 3}},
		{"missing-fragment", "PDV length", []byte{0, 0, 0, 4, 3, 2, 0}},
		{"length-max", "PDV length", []byte{255, 255, 255, 255, 3, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := append(append([]byte(nil), first...), tc.tail...)
			records, err := decodeDICOMPDVBody(body)
			require.ErrorContains(t, err, tc.diagnostic)
			require.Nil(t, records, "valid earlier records must not escape on failure")
		})
	}
}

func TestDICOMPDVBodyCountLimit(t *testing.T) {
	one := dicomNativePDV(255, 0xff, nil)
	for _, count := range []int{1, 2, 4095, 4096} {
		body := bytes.Repeat(one, count)
		records, err := decodeDICOMPDVBody(body)
		require.NoError(t, err)
		require.Len(t, records, count)
		for i, record := range records {
			require.Equal(t, dicomPDVRecord{Offset: i * 6, Length: 2, ContextID: 255, Control: 255, FragmentOffset: i*6 + 6}, record)
		}
	}
	for _, tail := range [][]byte{one, {0}, {255, 255, 255, 255}} {
		body := append(bytes.Repeat(one, 4096), tail...)
		records, err := decodeDICOMPDVBody(body)
		require.ErrorContains(t, err, "4096 limit")
		require.Nil(t, records)
	}
}

func TestDICOMPDVBodyParallelIsolation(t *testing.T) {
	for worker := 0; worker < 8; worker++ {
		t.Run(fmt.Sprintf("worker-%d", worker), func(t *testing.T) {
			t.Parallel()
			context := byte(worker*2 + 1)
			body := bytes.Repeat(dicomNativePDV(context, byte(worker), []byte{byte(worker), 0xff}), 64)
			for run := 0; run < 16; run++ {
				records, err := decodeDICOMPDVBody(body)
				require.NoError(t, err)
				require.Len(t, records, 64)
				for _, record := range records {
					require.Equal(t, context, record.ContextID)
					require.Equal(t, byte(worker), record.Control)
				}
				bad, err := decodeDICOMPDVBody(append(append([]byte(nil), body...), 0))
				require.Error(t, err)
				require.Nil(t, bad)
			}
		})
	}
}
