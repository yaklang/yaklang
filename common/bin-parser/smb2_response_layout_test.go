package bin_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

type smb2BoundedReader struct {
	*bytes.Reader
	bits uint64
}

func (r *smb2BoundedReader) InputBitLength() uint64 { return r.bits }

func TestSMB2CloseResponseUsesResponseLayout(t *testing.T) {
	body := make([]byte, 60)
	binary.LittleEndian.PutUint16(body[0:], 60)
	binary.LittleEndian.PutUint64(body[8:], 0x1122334455667788)
	binary.LittleEndian.PutUint64(body[16:], 0x1122334455667788)
	binary.LittleEndian.PutUint64(body[24:], 0x1122334455667788)
	binary.LittleEndian.PutUint64(body[32:], 0x1122334455667788)
	binary.LittleEndian.PutUint64(body[40:], 0x1000)
	binary.LittleEndian.PutUint64(body[48:], 0x800)
	binary.LittleEndian.PutUint32(body[56:], 0x20)
	raw := append(smb2SyncHeader(6, 0x00000001, 1), body...)
	response := mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "Close Response")
	require.Equal(t, uint64(60), uintVal(t, response.Child("StructureSize")))
	require.Equal(t, uint64(0x1000), uintVal(t, response.Child("AllocationSize")))
	require.Equal(t, uint64(0x800), uintVal(t, response.Child("EndofFile")))
	require.Equal(t, uint64(0x20), uintVal(t, response.Child("FileAttributes")))
}

func TestSMB2IOCTLResponseUsesResponseLayout(t *testing.T) {
	// Fixed body 48 bytes; OutputOffset 120 means eight padding bytes after it,
	// followed by the eight-byte output buffer.
	body := make([]byte, 48+8+8)
	binary.LittleEndian.PutUint16(body[0:], 49)
	binary.LittleEndian.PutUint32(body[4:], 0x00060194)
	copy(body[8:24], []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	binary.LittleEndian.PutUint32(body[24:], 0)
	binary.LittleEndian.PutUint32(body[28:], 0)
	binary.LittleEndian.PutUint32(body[32:], 120)
	binary.LittleEndian.PutUint32(body[36:], 8)
	copy(body[56:], []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xf0, 0x11, 0x22})
	raw := append(smb2SyncHeader(11, 0x00000001, 1), body...)
	response := mustChild(t, parseRule(t, raw, "application-layer.smb2", "SMB2"), "IOCTL Response")
	require.Equal(t, uint64(49), uintVal(t, response.Child("StructureSize")))
	require.Equal(t, uint64(0x00060194), uintVal(t, response.Child("CtlCode")))
	require.Equal(t, uint64(120), uintVal(t, response.Child("OutputOffset")))
	require.Equal(t, uint64(8), uintVal(t, response.Child("OutputCount")))
	require.Equal(t, []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xf0, 0x11, 0x22}, bytesVal(t, response.Child("Output Buffer")))
	require.Nil(t, response.Child("MaxInputResponse"), "a response has no request-side MaxInputResponse")
}

func TestSMB2CloseErrorUsesErrorBody(t *testing.T) {
	header := smb2SyncHeader(6, 0x00000001, 1)
	binary.LittleEndian.PutUint32(header[8:], 0xc0000022)
	body := make([]byte, 8)
	binary.LittleEndian.PutUint16(body[0:], 9)
	raw := append(header, body...)
	reader := &smb2BoundedReader{Reader: bytes.NewReader(raw), bits: uint64(len(raw)) * 8}
	node, err := parser.ParseBinary(reader, "application-layer.smb2", "SMB2")
	require.NoError(t, err)
	value, err := node.Result()
	require.NoError(t, err)
	errorBody := mustChild(t, value, "Error Body")
	require.Equal(t, uint64(0), uintVal(t, errorBody.Child("Byte Count")))
	require.Nil(t, value.Child("Close Response"), "a failed CLOSE must not use the success layout")
}
