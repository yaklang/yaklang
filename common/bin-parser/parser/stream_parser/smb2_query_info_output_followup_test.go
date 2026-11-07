package stream_parser

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func fileStreamEntry(next uint32, name string) []byte {
	u := make([]byte, len(name)*2)
	for i := range name {
		u[i*2] = name[i]
	}
	entry := make([]byte, 24+len(u))
	binary.LittleEndian.PutUint32(entry[0:4], next)
	binary.LittleEndian.PutUint32(entry[4:8], uint32(len(u)))
	binary.LittleEndian.PutUint64(entry[8:16], uint64(len(name)))
	binary.LittleEndian.PutUint64(entry[16:24], uint64(len(name)*2))
	copy(entry[24:], u)
	return entry
}

// MS-FSCC 2.4.40: alternate data streams chain entries with NextEntryOffset.
func TestSMB2FileStreamInformationMultiEntry(t *testing.T) {
	first := fileStreamEntry(40, ":$DATA")
	first = append(first, make([]byte, 4)...) // pad the 36-byte entry to NextEntryOffset 40
	wire := append(first, fileStreamEntry(0, ":stream")...)
	decoded, err := DecodeSMB2QueryInfoOutput(wire, 1, 22)
	require.NoError(t, err)
	require.Equal(t, 2, decoded["Stream Count"])
	streams := decoded["Streams"].([]map[string]any)
	require.Equal(t, ":$DATA", streams[0]["Stream Name"])
	require.Equal(t, ":stream", streams[1]["Stream Name"])
	require.NotContains(t, decoded, "Stream Name", "multi-entry outputs do not claim one stream name")
}

// A declared DACL can hold far more ACEs than one record should expand. The
// first 4096 are published and the truncation is explicit.
func TestSMB2SecurityDescriptorDACLTruncation(t *testing.T) {
	const declared = 4097
	dacl := make([]byte, 8+4*declared)
	dacl[0] = 2
	binary.LittleEndian.PutUint16(dacl[2:4], uint16(len(dacl)))
	binary.LittleEndian.PutUint16(dacl[4:6], declared)
	for i := 0; i < declared; i++ {
		binary.LittleEndian.PutUint16(dacl[8+4*i+2:], 4)
	}
	info := map[string]any{}
	fields, size, reason := smb2ACLFields(dacl, "DACL", info)
	require.Empty(t, reason)
	require.Equal(t, len(dacl), size)
	require.Len(t, fields, 5+4096*4)
	require.Equal(t, declared, info["DACL Ace Count Declared"])
	require.Equal(t, 4096, info["DACL Ace Count Parsed"])
	require.Contains(t, info["DACL Truncated"], "4096")
}
