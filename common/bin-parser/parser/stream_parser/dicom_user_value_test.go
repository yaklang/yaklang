package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func dicomNativeUserItem(kind, reserved byte, value []byte) []byte {
	item := make([]byte, 4+len(value))
	item[0], item[1] = kind, reserved
	binary.BigEndian.PutUint16(item[2:], uint16(len(value)))
	copy(item[4:], value)
	return item
}

func dicomNativeUserRequired() []byte {
	return []byte{81, 0xf1, 0, 4, 0x12, 0x34, 0xab, 0xcd, 82, 0xf2, 0, 5, '1', '.', '2', '.', '3'}
}

func dicomNativeUserWithUID(uid []byte) []byte {
	return append(append([]byte(nil), dicomNativeUserRequired()[:8]...), dicomNativeUserItem(82, 0xfe, uid)...)
}

func dicomNativeUserReject(t *testing.T, body []byte, initial uint64, diagnostic string) {
	t.Helper()
	records, err := decodeDICOMUserInformationBody(body, initial)
	if diagnostic == "" {
		require.Error(t, err)
	} else {
		require.EqualError(t, err, "dicom: "+diagnostic)
	}
	require.Nil(t, records, "no valid earlier descriptor may escape a failed body")
}

func TestDICOMUserInformationBodyOffsetsAndValues(t *testing.T) {
	// These literal fields pin big-endian lengths, arbitrary sub-item order,
	// nonzero reserved bytes, and opaque unknown values including an empty one.
	body := []byte{
		85, 0xa5, 0, 2, 'V', '1',
		0xfe, 0xbb, 0, 0,
		82, 0xcc, 0, 5, '1', '.', '2', '.', '3',
		81, 0xdd, 0, 4, 0x12, 0x34, 0xab, 0xcd,
		0xfd, 0xee, 0, 3, 0, 0xff, 0x80,
	}
	before := append([]byte(nil), body...)
	records, err := decodeDICOMUserInformationBody(body, 4)
	require.NoError(t, err)
	require.Equal(t, []dicomUserRecord{
		{Offset: 0, Type: 85, Length: 2, ValueOffset: 4, ValueLength: 2},
		{Offset: 6, Type: 0xfe, Length: 0, ValueOffset: 10, ValueLength: 0},
		{Offset: 10, Type: 82, Length: 5, ValueOffset: 14, ValueLength: 5},
		{Offset: 19, Type: 81, Length: 4, ValueOffset: 23, ValueLength: 4},
		{Offset: 27, Type: 0xfd, Length: 3, ValueOffset: 31, ValueLength: 3},
	}, records)
	for index, want := range [][]byte{{'V', '1'}, {}, {'1', '.', '2', '.', '3'}, {0x12, 0x34, 0xab, 0xcd}, {0, 0xff, 0x80}} {
		record := records[index]
		require.Equal(t, want, body[record.ValueOffset:record.ValueOffset+record.ValueLength])
	}
	require.Equal(t, before, body)
	other, err := decodeDICOMUserInformationBody(body, 0)
	require.NoError(t, err)
	records[0].Offset = 1234
	require.Zero(t, other[0].Offset, "descriptor storage must not be shared across calls")
}

func TestDICOMUserInformationBodyPrefixesAndLengths(t *testing.T) {
	// Required UID is last: even a prefix ending at an earlier item boundary
	// is not a complete valid User Information body.
	body := append(dicomNativeUserItem(0xfe, 0xab, []byte{0xaa, 0xbb}), dicomNativeUserItem(85, 0xee, []byte("V1"))...)
	body = append(body, dicomNativeUserRequired()...)
	for cut := 0; cut < len(body); cut++ {
		t.Run(fmt.Sprintf("prefix-%d", cut), func(t *testing.T) {
			dicomNativeUserReject(t, body[:cut], 0, "")
		})
	}
	for _, offset := range []int{0, 6, 12, 20} {
		for _, delta := range []int{-1, 1} {
			changed := append([]byte(nil), body...)
			length := int(binary.BigEndian.Uint16(changed[offset+2:])) + delta
			binary.BigEndian.PutUint16(changed[offset+2:], uint16(length))
			t.Run(fmt.Sprintf("length-%d/%+d", offset, delta), func(t *testing.T) {
				dicomNativeUserReject(t, changed, 0, "")
			})
		}
	}
	for tail := 1; tail <= 3; tail++ {
		dicomNativeUserReject(t, append(append([]byte(nil), body...), make([]byte, tail)...), 0, "user sub-item length exceeds parent")
	}
	for _, size := range []int{0, 1, 2, 3, 5, 255, 256} {
		dicomNativeUserReject(t, append(dicomNativeUserItem(81, 0, make([]byte, size)), dicomNativeUserRequired()[8:]...), 0, "maximum length sub-item must contain four bytes")
	}
	for _, octet := range []byte{0, 0xff} {
		complete := append(dicomNativeUserItem(81, 0xff, bytes.Repeat([]byte{octet}, 4)), dicomNativeUserRequired()[8:]...)
		records, err := decodeDICOMUserInformationBody(complete, 0)
		require.NoError(t, err, "all uint32 maximum-PDU values are retained")
		require.Equal(t, uint16(4), records[0].Length)
	}
	for _, length := range []uint16{1, 255, 256, 65535} {
		tail := []byte{0xfe, 0, 0, 0}
		binary.BigEndian.PutUint16(tail[2:], length)
		dicomNativeUserReject(t, append(dicomNativeUserRequired(), tail...), 0, "user sub-item length exceeds parent")
	}
	for _, size := range []int{0, 1, 255, 256, 65535} {
		value := bytes.Repeat([]byte{0xff}, size)
		complete := append(dicomNativeUserRequired(), dicomNativeUserItem(0xff, 0xff, value)...)
		records, err := decodeDICOMUserInformationBody(complete, 0)
		require.NoError(t, err)
		require.Equal(t, uint16(size), records[2].Length)
		require.Equal(t, size, records[2].ValueLength)
		require.Equal(t, len(complete), records[2].ValueOffset+records[2].ValueLength)
	}
}

func TestDICOMUserInformationBodyRequiredDuplicateAndOrder(t *testing.T) {
	maximum := dicomNativeUserRequired()[:8]
	implementation := dicomNativeUserRequired()[8:]
	version := dicomNativeUserItem(85, 0xff, []byte("V1"))
	items := [][]byte{maximum, implementation, version}
	for a := 0; a < 3; a++ {
		for b := 0; b < 3; b++ {
			if a == b {
				continue
			}
			c := 3 - a - b
			body := append(append(append([]byte(nil), items[a]...), items[b]...), items[c]...)
			records, err := decodeDICOMUserInformationBody(body, 0)
			require.NoError(t, err)
			require.Equal(t, []byte{items[a][0], items[b][0], items[c][0]}, []byte{records[0].Type, records[1].Type, records[2].Type})
		}
	}
	for _, body := range [][]byte{nil, maximum, implementation, version, append(append([]byte(nil), maximum...), version...), append(append([]byte(nil), implementation...), version...)} {
		dicomNativeUserReject(t, body, 0, "required user information missing or duplicated")
	}
	for _, duplicate := range items {
		body := append(append(dicomNativeUserRequired(), version...), duplicate...)
		dicomNativeUserReject(t, body, 0, "required user information missing or duplicated")
	}
	// All other type octets are opaque extensions, including zero. Repeated
	// unknown types and zero values are allowed; reserved bytes are unrestricted.
	for octet := 0; octet < 256; octet++ {
		body := dicomNativeUserRequired()
		body[1], body[9] = byte(octet), byte(octet)
		if octet != 81 && octet != 82 && octet != 85 {
			item := dicomNativeUserItem(byte(octet), byte(octet), nil)
			body = append(append(body, item...), item...)
		}
		records, err := decodeDICOMUserInformationBody(body, 0)
		require.NoError(t, err, "octet %d", octet)
		require.GreaterOrEqual(t, len(records), 2)
	}
	// Legacy validates every item's value before the final duplicate check.
	body := append(append(dicomNativeUserRequired(), maximum...), dicomNativeUserItem(82, 0, []byte("01"))...)
	dicomNativeUserReject(t, body, 0, "leading zero in UID component")
}

func TestDICOMUserInformationBodyUID(t *testing.T) {
	for _, value := range []string{"0", "1", "0.0", "1.0.2", "9.1234567890", string(bytes.Repeat([]byte{'9'}, 64))} {
		records, err := decodeDICOMUserInformationBody(dicomNativeUserWithUID([]byte(value)), 0)
		require.NoError(t, err, value)
		require.Equal(t, len(value), records[1].ValueLength)
	}
	for _, tc := range []struct{ value, diagnostic string }{
		{"", "UID length must be 1 to 64"},
		{string(bytes.Repeat([]byte{'1'}, 65)), "UID length must be 1 to 64"},
		{".", "empty UID component"}, {".1", "empty UID component"}, {"1.", "empty UID component"}, {"1..2", "empty UID component"},
		{"00", "leading zero in UID component"}, {"01", "leading zero in UID component"}, {"1.00", "leading zero in UID component"}, {"1.02.3", "leading zero in UID component"},
		{"1.a", "invalid UID character"}, {"1.2\x00", "invalid UID character"}, {"1 2", "invalid UID character"},
	} {
		t.Run(fmt.Sprintf("%q", tc.value), func(t *testing.T) {
			dicomNativeUserReject(t, dicomNativeUserWithUID([]byte(tc.value)), 0, tc.diagnostic)
		})
	}
	for octet := 0; octet < 256; octet++ {
		body := dicomNativeUserWithUID([]byte{byte(octet)})
		if octet >= '0' && octet <= '9' {
			records, err := decodeDICOMUserInformationBody(body, 0)
			require.NoError(t, err)
			require.Len(t, records, 2)
		} else if octet == '.' {
			dicomNativeUserReject(t, body, 0, "empty UID component")
		} else {
			dicomNativeUserReject(t, body, 0, "invalid UID character")
		}
	}
}

func TestDICOMUserInformationBodyVersion(t *testing.T) {
	for octet := 0; octet < 256; octet++ {
		body := append(dicomNativeUserRequired(), dicomNativeUserItem(85, 0xff, []byte{byte(octet)})...)
		if octet >= 32 && octet <= 126 {
			records, err := decodeDICOMUserInformationBody(body, 0)
			require.NoError(t, err)
			require.Len(t, records, 3)
		} else {
			dicomNativeUserReject(t, body, 0, "invalid implementation version character")
		}
	}
	for _, size := range []int{1, 16} {
		records, err := decodeDICOMUserInformationBody(append(dicomNativeUserRequired(), dicomNativeUserItem(85, 0, bytes.Repeat([]byte{' '}, size))...), 0)
		require.NoError(t, err)
		require.Equal(t, size, records[2].ValueLength)
	}
	for _, size := range []int{0, 17, 65535} {
		dicomNativeUserReject(t, append(dicomNativeUserRequired(), dicomNativeUserItem(85, 0, bytes.Repeat([]byte{'X'}, size))...), 0, "implementation version length must be 1 to 16")
	}
	for _, offset := range []int{1, 7, 15} {
		version := bytes.Repeat([]byte{'V'}, 16)
		version[offset] = 0xff
		dicomNativeUserReject(t, append(dicomNativeUserRequired(), dicomNativeUserItem(85, 0, version)...), 0, "invalid implementation version character")
	}
}

func TestDICOMUserInformationBodyCountLimit(t *testing.T) {
	for _, initial := range []uint64{0, 1, 4093, 4094} {
		records, err := decodeDICOMUserInformationBody(dicomNativeUserRequired(), initial)
		require.NoError(t, err)
		require.Len(t, records, 2)
	}
	for _, initial := range []uint64{4095, 4096, 4097, ^uint64(0) - 1, ^uint64(0)} {
		dicomNativeUserReject(t, dicomNativeUserRequired(), initial, "item count exceeds 4096 limit")
	}
	for _, count := range []int{2, 4095, 4096} {
		body := append(dicomNativeUserRequired(), bytes.Repeat([]byte{0xfe, 0xff, 0, 0}, count-2)...)
		records, err := decodeDICOMUserInformationBody(body, uint64(4096-count))
		require.NoError(t, err)
		require.Len(t, records, count)
		for index := 2; index < count; index++ {
			require.Equal(t, dicomUserRecord{Offset: 17 + 4*(index-2), Type: 0xfe, ValueOffset: 21 + 4*(index-2)}, records[index])
		}
		dicomNativeUserReject(t, body, uint64(4097-count), "item count exceeds 4096 limit")
		for _, tail := range [][]byte{{0}, {0xfe, 0xff, 0, 0}} {
			dicomNativeUserReject(t, append(append([]byte(nil), body...), tail...), uint64(4096-count), "item count exceeds 4096 limit")
		}
	}
	dicomNativeUserReject(t, nil, 4096, "required user information missing or duplicated")
	dicomNativeUserReject(t, nil, 4097, "item count exceeds 4096 limit")
}

func TestDICOMUserInformationBodyParallelIsolation(t *testing.T) {
	shared := dicomNativeUserRequired()
	sharedBefore := append([]byte(nil), shared...)
	for worker := 0; worker < 8; worker++ {
		t.Run(fmt.Sprintf("worker-%d", worker), func(t *testing.T) {
			t.Parallel()
			body := append(dicomNativeUserRequired(), bytes.Repeat(dicomNativeUserItem(byte(0xf0+worker), byte(worker), []byte{byte(worker), 0xff}), 64)...)
			before := append([]byte(nil), body...)
			for run := 0; run < 16; run++ {
				records, err := decodeDICOMUserInformationBody(body, uint64(worker))
				require.NoError(t, err)
				require.Len(t, records, 66)
				for _, record := range records[2:] {
					require.Equal(t, uint8(0xf0+worker), record.Type)
					require.Equal(t, []byte{byte(worker), 0xff}, body[record.ValueOffset:record.ValueOffset+record.ValueLength])
				}
				sharedRecords, err := decodeDICOMUserInformationBody(shared, uint64(worker))
				require.NoError(t, err)
				require.Len(t, sharedRecords, 2)
				require.Equal(t, sharedBefore, shared)
				dicomNativeUserReject(t, body, 4095, "item count exceeds 4096 limit")
				dicomNativeUserReject(t, append(append([]byte(nil), body...), 0), 0, "user sub-item length exceeds parent")
			}
			require.Equal(t, before, body)
		})
	}
}
