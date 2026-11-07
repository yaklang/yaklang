package bin_parser

import (
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

func smb2CompoundCommand(header []byte, body []byte, next bool) []byte {
	command := append(append([]byte(nil), header...), body...)
	for len(command)%8 != 0 {
		command = append(command, 0)
	}
	if next {
		binary.LittleEndian.PutUint32(command[20:24], uint32(len(command)))
	}
	return command
}

func TestSMB2StructuredCompoundCommands(t *testing.T) {
	path := smb2DecodedUTF16(`\\srv\share`)
	tcBody := make([]byte, 8+len(path))
	binary.LittleEndian.PutUint16(tcBody[0:], 9)
	binary.LittleEndian.PutUint16(tcBody[4:], 72)
	binary.LittleEndian.PutUint16(tcBody[6:], uint16(len(path)))
	copy(tcBody[8:], path)
	tc := smb2CompoundCommand(smb2SyncHeader(3, 0, 1), tcBody, true)

	name := smb2DecodedUTF16("file.txt")
	crBody := make([]byte, 56+len(name))
	binary.LittleEndian.PutUint16(crBody[0:], 57)
	binary.LittleEndian.PutUint32(crBody[4:], 2)
	binary.LittleEndian.PutUint32(crBody[24:], 0x00120189)
	binary.LittleEndian.PutUint32(crBody[36:], 1)
	binary.LittleEndian.PutUint16(crBody[44:], 120)
	binary.LittleEndian.PutUint16(crBody[46:], uint16(len(name)))
	copy(crBody[56:], name)
	cr := smb2CompoundCommand(smb2SyncHeader(5, 0, 2), crBody, true)

	clBody := make([]byte, 24)
	binary.LittleEndian.PutUint16(clBody[0:], 24)
	cl := smb2CompoundCommand(smb2SyncHeader(6, 0, 3), clBody, false)

	raw := append(append(tc, cr...), cl...)
	result, err := parseStructuredRule(t, raw, "application-layer.smb2", "SMB2")
	require.NoError(t, err)
	fields := result["fields"].(map[string]any)
	require.Equal(t, true, fields["Compound"])
	require.EqualValues(t, 3, fields["Compound Count"])
	commands, ok := fields["Commands"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, commands, 3)

	tree, ok := commands[0]["Tree Connect Request"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, `\\srv\share`, tree["Path"])
	create, ok := commands[1]["Create Request"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "file.txt", create["Name"])
	closeReq, ok := commands[2]["Close"].(map[string]any)
	require.True(t, ok)
	require.EqualValues(t, 24, closeReq["StructureSize"])
	encoded, err := json.Marshal(fields)
	require.NoError(t, err, "the compound projection must not contain a JSON cycle")
	require.Contains(t, string(encoded), `"Commands"`)
}

func TestSMB2StructuredDirectoryEntriesWithoutMatchedRequest(t *testing.T) {
	name := smb2DecodedUTF16("file.txt")
	entry := make([]byte, 104+len(name))
	binary.LittleEndian.PutUint32(entry[0:], 0)     // NextEntryOffset: last entry
	binary.LittleEndian.PutUint32(entry[4:], 7)     // FileIndex
	binary.LittleEndian.PutUint64(entry[40:], 16)   // EndOfFile
	binary.LittleEndian.PutUint64(entry[48:], 32)   // AllocationSize
	binary.LittleEndian.PutUint32(entry[56:], 0x20) // FILE_ATTRIBUTE_ARCHIVE
	binary.LittleEndian.PutUint32(entry[60:], uint32(len(name)))
	copy(entry[104:], name)

	body := make([]byte, 8+len(entry))
	binary.LittleEndian.PutUint16(body[0:], 9)
	binary.LittleEndian.PutUint16(body[2:], 72)
	binary.LittleEndian.PutUint32(body[4:], uint32(len(entry)))
	copy(body[8:], entry)
	raw := append(smb2SyncHeader(14, 1, 9), body...)

	result, err := parseStructuredRule(t, raw, "application-layer.smb2", "SMB2")
	require.NoError(t, err)
	fields := result["fields"].(map[string]any)
	response, ok := fields["Query Directory Response"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, response["Directory Entry Status"], "decoded:")
	entries, ok := response["Directory Entries"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, entries, 1)
	require.Equal(t, "file.txt", entries[0]["File Name"])
	require.EqualValues(t, 16, entries[0]["End Of File"])
	require.EqualValues(t, 32, entries[0]["Allocation Size"])
	require.EqualValues(t, 7, entries[0]["File Index"])
}

func parseStructuredRule(t *testing.T, data []byte, rule string, keys ...string) (map[string]any, error) {
	t.Helper()
	return parser.ParseStructured(data, rule, keys...)
}

func TestSMB2StructuredFileNamesEntriesAutoSelected(t *testing.T) {
	name := smb2DecodedUTF16("names.txt")
	entry := make([]byte, 12+len(name))
	binary.LittleEndian.PutUint32(entry[4:], 4)
	binary.LittleEndian.PutUint32(entry[8:], uint32(len(name)))
	copy(entry[12:], name)

	body := make([]byte, 8+len(entry))
	binary.LittleEndian.PutUint16(body[0:], 9)
	binary.LittleEndian.PutUint16(body[2:], 72)
	binary.LittleEndian.PutUint32(body[4:], uint32(len(entry)))
	copy(body[8:], entry)
	raw := append(smb2SyncHeader(14, 1, 10), body...)

	result, err := parseStructuredRule(t, raw, "application-layer.smb2", "SMB2")
	require.NoError(t, err)
	fields := result["fields"].(map[string]any)
	response := fields["Query Directory Response"].(map[string]any)
	require.Contains(t, response["Directory Entry Status"], "FileNamesInformation")
	entries := response["Directory Entries"].([]map[string]any)
	require.Len(t, entries, 1)
	require.Equal(t, "names.txt", entries[0]["File Name"])
}
