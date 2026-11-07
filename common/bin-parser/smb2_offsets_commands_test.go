package bin_parser

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

func parseSMB2StructuredFields(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	result, err := parser.ParseStructured(raw, "application-layer.smb2", "SMB2")
	require.NoError(t, err, "ParseStructured must consume the whole SMB2 record")
	fields, ok := result["fields"].(map[string]any)
	require.True(t, ok)
	return fields
}

func smb2DecodedUTF16(value string) []byte {
	out := make([]byte, len(value)*2)
	for i := range value {
		out[i*2] = value[i]
	}
	return out
}

func TestSMB2VariableOffsetsAndUTF16ParseStructured(t *testing.T) {
	t.Run("tree-connect-path-offset", func(t *testing.T) {
		path := smb2DecodedUTF16(`\\srv\share`)
		body := make([]byte, 8+8+len(path))
		binary.LittleEndian.PutUint16(body[0:], 9)
		binary.LittleEndian.PutUint16(body[4:], 80)
		binary.LittleEndian.PutUint16(body[6:], uint16(len(path)))
		copy(body[16:], path)
		fields := parseSMB2StructuredFields(t, append(smb2SyncHeader(3, 0, 1), body...))
		request := fields["Tree Connect Request"].(map[string]any)
		require.Equal(t, `\\srv\share`, request["Path"])
		require.Equal(t, []byte{0, 0, 0, 0, 0, 0, 0, 0}, request["Path Padding"])
	})

	t.Run("create-name-offset", func(t *testing.T) {
		name := smb2DecodedUTF16("docs\\report.txt")
		body := make([]byte, 56+8+len(name))
		binary.LittleEndian.PutUint16(body[0:], 57)
		binary.LittleEndian.PutUint32(body[4:], 2)
		binary.LittleEndian.PutUint32(body[24:], 0x00120189)
		binary.LittleEndian.PutUint32(body[36:], 1)
		binary.LittleEndian.PutUint16(body[44:], 128)
		binary.LittleEndian.PutUint16(body[46:], uint16(len(name)))
		copy(body[64:], name)
		fields := parseSMB2StructuredFields(t, append(smb2SyncHeader(5, 0, 2), body...))
		request := fields["Create Request"].(map[string]any)
		require.Equal(t, "docs\\report.txt", request["Name"])
		require.Equal(t, []byte{0, 0, 0, 0, 0, 0, 0, 0}, request["Name Padding"])
	})

	t.Run("read-response-data-offset", func(t *testing.T) {
		data := []byte("abcdefgh")
		body := make([]byte, 16+len(data))
		binary.LittleEndian.PutUint16(body[0:], 17)
		body[2] = 80
		binary.LittleEndian.PutUint32(body[4:], uint32(len(data)))
		copy(body[16:], data)
		fields := parseSMB2StructuredFields(t, append(smb2SyncHeader(8, 1, 3), body...))
		response := fields["Read Response"].(map[string]any)
		require.Equal(t, string(data), response["Octets"])
	})

	t.Run("write-request-data-offset", func(t *testing.T) {
		data := []byte("payload")
		body := make([]byte, 48+8+len(data))
		binary.LittleEndian.PutUint16(body[0:], 49)
		binary.LittleEndian.PutUint16(body[2:], 120)
		binary.LittleEndian.PutUint32(body[4:], uint32(len(data)))
		copy(body[56:], data)
		fields := parseSMB2StructuredFields(t, append(smb2SyncHeader(9, 0, 4), body...))
		request := fields["Write Request"].(map[string]any)
		require.Equal(t, string(data), request["Octets"])
		require.Equal(t, []byte{0, 0, 0, 0, 0, 0, 0, 0}, request["Data Padding"])
	})

	t.Run("negotiate-response-security-buffer-offset", func(t *testing.T) {
		blob := []byte{0x60, 0x06, 0x06, 0x2b, 0x06, 0x01, 0x05, 0x05}
		body := make([]byte, 64+len(blob))
		binary.LittleEndian.PutUint16(body[0:], 65)
		binary.LittleEndian.PutUint16(body[4:], 0x0210)
		binary.LittleEndian.PutUint16(body[56:], 128)
		binary.LittleEndian.PutUint16(body[58:], uint16(len(blob)))
		copy(body[64:], blob)
		fields := parseSMB2StructuredFields(t, append(smb2SyncHeader(0, 1, 5), body...))
		response := fields["Negotiate Response"].(map[string]any)
		require.Equal(t, string(blob), response["Octets"])
	})

	t.Run("session-setup-security-buffer-offset", func(t *testing.T) {
		token := []byte{0xde, 0xad, 0xbe, 0xef}
		body := make([]byte, 24+8+len(token))
		binary.LittleEndian.PutUint16(body[0:], 25)
		binary.LittleEndian.PutUint16(body[12:], 96)
		binary.LittleEndian.PutUint16(body[14:], uint16(len(token)))
		copy(body[32:], token)
		fields := parseSMB2StructuredFields(t, append(smb2SyncHeader(1, 0, 6), body...))
		request := fields["Session Setup Request"].(map[string]any)
		require.Equal(t, string(token), request["Octets"])
	})

	t.Run("ioctl-input-output-offsets", func(t *testing.T) {
		in := []byte{1, 2, 3, 4}
		out := []byte{5, 6, 7, 8}
		body := make([]byte, 48+4+len(in)+len(out))
		binary.LittleEndian.PutUint16(body[0:], 49)
		binary.LittleEndian.PutUint32(body[4:], 0x0011c017)
		binary.LittleEndian.PutUint32(body[24:], 112)
		binary.LittleEndian.PutUint32(body[28:], uint32(len(in)))
		binary.LittleEndian.PutUint32(body[32:], 120)
		binary.LittleEndian.PutUint32(body[36:], uint32(len(out)))
		copy(body[48:], in)
		copy(body[56:], out)
		fields := parseSMB2StructuredFields(t, append(smb2SyncHeader(11, 1, 7), body...))
		response := fields["IOCTL Response"].(map[string]any)
		require.Equal(t, in, response["Input Buffer"])
		require.Equal(t, out, response["Output Buffer"])
	})
}

func TestSMB2NewCommandLayouts(t *testing.T) {
	t.Run("query-directory", func(t *testing.T) {
		name := smb2DecodedUTF16(`\*.txt`)
		body := make([]byte, 32+len(name))
		binary.LittleEndian.PutUint16(body[0:], 33)
		body[2] = 37
		binary.LittleEndian.PutUint16(body[24:], 96)
		binary.LittleEndian.PutUint16(body[26:], uint16(len(name)))
		copy(body[32:], name)
		fields := parseSMB2StructuredFields(t, append(smb2SyncHeader(14, 0, 1), body...))
		request := fields["Query Directory Request"].(map[string]any)
		require.Equal(t, uint8(37), request["FileInformationClass"])
		require.Equal(t, `\*.txt`, request["FileName"])

		output := []byte{1, 2, 3, 4, 5, 6, 7, 8}
		resp := make([]byte, 8+len(output))
		binary.LittleEndian.PutUint16(resp[0:], 9)
		binary.LittleEndian.PutUint16(resp[2:], 72)
		binary.LittleEndian.PutUint32(resp[4:], uint32(len(output)))
		copy(resp[8:], output)
		fields = parseSMB2StructuredFields(t, append(smb2SyncHeader(14, 1, 1), resp...))
		response := fields["Query Directory Response"].(map[string]any)
		require.Equal(t, output, response["Output Buffer"])
	})

	t.Run("change-notify", func(t *testing.T) {
		body := make([]byte, 32)
		binary.LittleEndian.PutUint16(body[0:], 32)
		binary.LittleEndian.PutUint16(body[2:], 1)
		binary.LittleEndian.PutUint32(body[4:], 0x1000)
		binary.LittleEndian.PutUint32(body[24:], 0x17)
		fields := parseSMB2StructuredFields(t, append(smb2SyncHeader(15, 0, 2), body...))
		request := fields["Change Notify Request"].(map[string]any)
		require.Equal(t, uint16(1), request["Flags"])
		require.Equal(t, uint32(0x17), request["CompletionFilter"])

		output := []byte{9, 8, 7, 6}
		resp := make([]byte, 8+len(output))
		binary.LittleEndian.PutUint16(resp[0:], 9)
		binary.LittleEndian.PutUint16(resp[2:], 72)
		binary.LittleEndian.PutUint32(resp[4:], uint32(len(output)))
		copy(resp[8:], output)
		fields = parseSMB2StructuredFields(t, append(smb2SyncHeader(15, 1, 2), resp...))
		response := fields["Change Notify Response"].(map[string]any)
		require.Equal(t, output, response["Output Buffer"])
	})

	t.Run("set-info", func(t *testing.T) {
		buffer := []byte{1, 2, 3, 4}
		body := make([]byte, 32+len(buffer))
		binary.LittleEndian.PutUint16(body[0:], 33)
		body[2] = 1
		body[3] = 4
		binary.LittleEndian.PutUint32(body[4:], uint32(len(buffer)))
		binary.LittleEndian.PutUint16(body[8:], 96)
		copy(body[32:], buffer)
		fields := parseSMB2StructuredFields(t, append(smb2SyncHeader(17, 0, 3), body...))
		request := fields["Set Info Request"].(map[string]any)
		require.Equal(t, uint8(1), request["InfoType"])
		require.Equal(t, buffer, request["Buffer"])

		resp := make([]byte, 2)
		binary.LittleEndian.PutUint16(resp[0:], 2)
		fields = parseSMB2StructuredFields(t, append(smb2SyncHeader(17, 1, 3), resp...))
		response := fields["Set Info Response"].(map[string]any)
		require.EqualValues(t, 2, response["StructureSize"])
	})

	t.Run("oplock-break", func(t *testing.T) {
		body := make([]byte, 24)
		binary.LittleEndian.PutUint16(body[0:], 24)
		body[2] = 1
		fields := parseSMB2StructuredFields(t, append(smb2SyncHeader(18, 1, 4), body...))
		response := fields["Oplock Break"].(map[string]any)
		require.Equal(t, uint8(1), response["OplockLevel"])
	})

	t.Run("lock", func(t *testing.T) {
		body := make([]byte, 48+24)
		binary.LittleEndian.PutUint16(body[0:], 48)
		binary.LittleEndian.PutUint16(body[2:], 2)
		binary.LittleEndian.PutUint32(body[4:], 1)
		binary.LittleEndian.PutUint64(body[24:], 0)
		binary.LittleEndian.PutUint64(body[32:], 16)
		binary.LittleEndian.PutUint32(body[40:], 1)
		binary.LittleEndian.PutUint64(body[48:], 32)
		binary.LittleEndian.PutUint64(body[56:], 32)
		binary.LittleEndian.PutUint32(body[64:], 1)
		fields := parseSMB2StructuredFields(t, append(smb2SyncHeader(10, 0, 5), body...))
		request := fields["Lock Request"].(map[string]any)
		require.Equal(t, uint16(2), request["LockCount"])
		locks, ok := request["Locks"].([]any)
		require.True(t, ok)
		require.Len(t, locks, 2)
		first, ok := locks[0].(map[string]any)
		require.True(t, ok)
		second, ok := locks[1].(map[string]any)
		require.True(t, ok)
		require.EqualValues(t, 0, first["Offset"])
		require.EqualValues(t, 32, second["Offset"])

		resp := make([]byte, 4)
		binary.LittleEndian.PutUint16(resp[0:], 4)
		fields = parseSMB2StructuredFields(t, append(smb2SyncHeader(10, 1, 5), resp...))
		response := fields["Lock Response"].(map[string]any)
		require.EqualValues(t, 4, response["StructureSize"])
	})

	t.Run("flush", func(t *testing.T) {
		body := make([]byte, 24)
		binary.LittleEndian.PutUint16(body[0:], 24)
		fields := parseSMB2StructuredFields(t, append(smb2SyncHeader(7, 0, 6), body...))
		request := fields["Flush Request"].(map[string]any)
		require.EqualValues(t, 24, request["StructureSize"])

		resp := make([]byte, 4)
		binary.LittleEndian.PutUint16(resp[0:], 4)
		fields = parseSMB2StructuredFields(t, append(smb2SyncHeader(7, 1, 6), resp...))
		response := fields["Flush Response"].(map[string]any)
		require.EqualValues(t, 4, response["StructureSize"])
	})
}

func TestSMB2NewCommandStatusGates(t *testing.T) {
	// A failed QUERY_DIRECTORY uses the SMB2 ERROR body, not the success layout.
	header := smb2SyncHeader(14, 1, 7)
	binary.LittleEndian.PutUint32(header[8:], 0xc0000022)
	body := make([]byte, 8)
	binary.LittleEndian.PutUint16(body[0:], 9)
	fields := parseSMB2StructuredFields(t, append(header, body...))
	require.Contains(t, fields, "Error Body")
	require.NotContains(t, fields, "Query Directory Response")

	// STATUS_BUFFER_OVERFLOW keeps the CHANGE_NOTIFY success layout.
	header = smb2SyncHeader(15, 1, 8)
	binary.LittleEndian.PutUint32(header[8:], 0x80000005)
	body = make([]byte, 8)
	binary.LittleEndian.PutUint16(body[0:], 9)
	fields = parseSMB2StructuredFields(t, append(header, body...))
	require.Contains(t, fields, "Change Notify Response")
	require.NotContains(t, fields, "Error Body")

	// A failed SET_INFO uses the ERROR body.
	header = smb2SyncHeader(17, 1, 9)
	binary.LittleEndian.PutUint32(header[8:], 0xc000000d)
	body = make([]byte, 8)
	binary.LittleEndian.PutUint16(body[0:], 9)
	fields = parseSMB2StructuredFields(t, append(header, body...))
	require.Contains(t, fields, "Error Body")
	require.NotContains(t, fields, "Set Info Response")
}
