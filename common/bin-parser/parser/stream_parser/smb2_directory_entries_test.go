package stream_parser

import (
	"encoding/binary"
	"fmt"
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/require"
)

func smb2DirectoryTestName(name string) []byte {
	units := utf16.Encode([]rune(name))
	out := make([]byte, len(units)*2)
	for i, unit := range units {
		binary.LittleEndian.PutUint16(out[i*2:], unit)
	}
	return out
}

func smb2DirectoryTestEntry(class uint8, name string) []byte {
	u := smb2DirectoryTestName(name)
	var entry []byte
	switch class {
	case 1:
		entry = make([]byte, 64+len(u))
		copy(entry[64:], u)
	case 2:
		entry = make([]byte, 68+len(u))
		copy(entry[68:], u)
	case 3:
		entry = make([]byte, 94+len(u))
		copy(entry[94:], u)
	case 38:
		entry = make([]byte, 76+len(u))
		binary.LittleEndian.PutUint64(entry[68:], 0x1122334455667788)
		copy(entry[76:], u)
	case 37:
		entry = make([]byte, 104+len(u))
		binary.LittleEndian.PutUint64(entry[96:], 0x1122334455667788)
		copy(entry[104:], u)
	case 12:
		entry = make([]byte, 12+len(u))
		binary.LittleEndian.PutUint32(entry[8:], uint32(len(u)))
		copy(entry[12:], u)
		return entry
	}
	binary.LittleEndian.PutUint32(entry[4:], 9)
	binary.LittleEndian.PutUint64(entry[40:], 16)
	binary.LittleEndian.PutUint64(entry[48:], 32)
	binary.LittleEndian.PutUint32(entry[56:], 0x20)
	binary.LittleEndian.PutUint32(entry[60:], uint32(len(u)))
	return entry
}

func TestDecodeSMB2DirectoryEntriesClassLayouts(t *testing.T) {
	for _, tc := range []struct {
		class     uint8
		name      string
		wantField string
		wantValue any
		wantError bool
	}{
		{class: 1, name: "file.txt", wantField: "File Name", wantValue: "file.txt"},
		{class: 2, name: "full.txt", wantField: "EA Size", wantValue: uint32(0)},
		{class: 3, name: "both.txt", wantField: "Short Name Length", wantValue: uint8(0)},
		{class: 38, name: "idfull.txt", wantField: "File ID", wantValue: "8877665544332211"},
		{class: 37, name: "idboth.txt", wantField: "File ID", wantValue: "8877665544332211"},
		{class: 12, name: "names.txt", wantField: "File Name", wantValue: "names.txt"},
	} {
		testName := smb2DirectoryClassName(tc.class)
		if testName == "" {
			testName = fmt.Sprintf("class-%d", tc.class)
		}
		t.Run(testName, func(t *testing.T) {
			entry := smb2DirectoryTestEntry(tc.class, tc.name)
			decoded, err := DecodeSMB2DirectoryEntries(entry, tc.class, 64)
			require.NoError(t, err)
			entries := decoded["Entries"].([]map[string]any)
			require.Len(t, entries, 1)
			require.Equal(t, tc.name, entries[0]["File Name"])
			if tc.wantField != "File Name" {
				require.EqualValues(t, tc.wantValue, entries[0][tc.wantField])
			}
		})
	}
}

func TestDecodeSMB2DirectoryEntriesAutoSelectsClassLayout(t *testing.T) {
	for _, class := range []uint8{1, 2, 3, 37, 38} {
		entry := smb2DirectoryTestEntry(class, "auto.txt")
		decoded, err := DecodeSMB2DirectoryEntries(entry, 0, 64)
		require.NoError(t, err, "class %d", class)
		entries := decoded["Entries"].([]map[string]any)
		require.Equal(t, "auto.txt", entries[0]["File Name"], "class %d", class)
		require.Contains(t, decoded["Status"], "generic")
	}
	names := smb2DirectoryTestEntry(12, "names.txt")
	decoded, err := DecodeSMB2DirectoryEntries(names, 0, 64)
	require.NoError(t, err)
	require.Contains(t, decoded["Status"], "FileNamesInformation")
}

func TestDecodeSMB2DirectoryEntriesRejectsMalformedChains(t *testing.T) {
	valid := smb2DirectoryTestEntry(1, "file.txt")
	outside := append([]byte(nil), valid...)
	binary.LittleEndian.PutUint32(outside[0:], 1<<20)
	_, err := DecodeSMB2DirectoryEntries(outside, 1, 64)
	require.Error(t, err)

	odd := append([]byte(nil), valid...)
	binary.LittleEndian.PutUint32(odd[60:], 3)
	_, err = DecodeSMB2DirectoryEntries(odd, 1, 64)
	require.Error(t, err)

	padded := append(append([]byte(nil), valid...), 0xaa)
	_, err = DecodeSMB2DirectoryEntries(padded, 1, 64)
	require.Error(t, err)
}
