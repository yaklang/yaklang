package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func smb2FileIdBothEntry(name string) []byte {
	u := smb2UTF16(name)
	entry := make([]byte, 104+len(u))
	binary.LittleEndian.PutUint32(entry[0:], 0)
	binary.LittleEndian.PutUint32(entry[4:], 7)
	binary.LittleEndian.PutUint64(entry[40:], 16)
	binary.LittleEndian.PutUint64(entry[48:], 32)
	binary.LittleEndian.PutUint32(entry[56:], 0x20)
	binary.LittleEndian.PutUint32(entry[60:], uint32(len(u)))
	copy(entry[104:], u)
	return entry
}

func smb2FileDirectoryEntry(name string) []byte {
	u := smb2UTF16(name)
	entry := make([]byte, 64+len(u))
	binary.LittleEndian.PutUint32(entry[0:], 0)
	binary.LittleEndian.PutUint32(entry[4:], 3)
	binary.LittleEndian.PutUint64(entry[40:], 8)
	binary.LittleEndian.PutUint64(entry[48:], 16)
	binary.LittleEndian.PutUint32(entry[56:], 0x20)
	binary.LittleEndian.PutUint32(entry[60:], uint32(len(u)))
	copy(entry[64:], u)
	return entry
}

func TestSMB2QueryDirectoryGenericEntriesWithoutRequest(t *testing.T) {
	response := smb2QueryDirectoryResp(1, 0x11, 1, smb2FileIdBothEntry("file.txt"))
	info, err := (&binSMB2{}).consume(response, 64)
	require.NoError(t, err)
	body := info["Body"].(map[string]any)
	require.Contains(t, body["Directory Entry Status"], "decoded:")
	require.Contains(t, body["Directory Entry Status"], "generic")
	entries, ok := body["Directory Entries"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, entries, 1)
	require.Equal(t, "file.txt", entries[0]["File Name"])
	require.EqualValues(t, 16, entries[0]["End Of File"])
}

func TestSMB2QueryDirectoryMatchedClassRefinesEntries(t *testing.T) {
	state := &binSMB2{client: 0}
	fid := bytes.Repeat([]byte{0x11}, 16)
	_, err := state.consumeFrom(0, smb2QueryDirectory(1, 0x11, 1, 1, fid), 64)
	require.NoError(t, err)
	info, err := state.consumeFrom(1, smb2QueryDirectoryResp(1, 0x11, 1, smb2FileDirectoryEntry("dir.txt")), 64)
	require.NoError(t, err)
	body := info["Body"].(map[string]any)
	require.Contains(t, body["Directory Entry Status"], "FileDirectoryInformation")
	require.Contains(t, body["Directory Entry Status"], "matched request")
	entries, ok := body["Directory Entries"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, entries, 1)
	require.Equal(t, "dir.txt", entries[0]["File Name"])
}

func TestSMB2QueryDirectoryMalformedChainKeepsRawOutput(t *testing.T) {
	entry := smb2FileIdBothEntry("file.txt")
	binary.LittleEndian.PutUint32(entry[0:], 1<<20) // NextEntryOffset outside the buffer
	response := smb2QueryDirectoryResp(1, 0x11, 1, entry)
	info, err := (&binSMB2{}).consume(response, 64)
	require.NoError(t, err)
	body := info["Body"].(map[string]any)
	require.Contains(t, body["Directory Entry Status"], "opaque:")
	require.NotEmpty(t, body["Output Buffer"])
	require.NotContains(t, body, "Directory Entries")
}
