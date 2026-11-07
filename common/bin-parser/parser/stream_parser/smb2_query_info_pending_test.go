package stream_parser

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

// STATUS_PENDING interim responses keep the selected command layout. Only a
// real failure status selects the SMB2 ERROR body.
func TestDecodeSMB2QueryInfoStatusPendingKeepsResponseLayout(t *testing.T) {
	pending := make([]byte, 8)
	binary.LittleEndian.PutUint16(pending[0:2], 9)
	binary.LittleEndian.PutUint16(pending[2:4], 72)
	binary.LittleEndian.PutUint32(pending[4:8], 0)

	fields, err := DecodeSMB2QueryInfoBody(pending, true, 0x00000103)
	require.NoError(t, err)
	require.NotContains(t, fields, "Error Response")
	require.Equal(t, uint16(9), fields["Structure Size"])
	require.Equal(t, uint16(72), fields["Output Buffer Offset"])

	failure := make([]byte, 8)
	binary.LittleEndian.PutUint16(failure[0:2], 9)
	binary.LittleEndian.PutUint32(failure[4:8], 0)
	fields, err = DecodeSMB2QueryInfoBody(failure, true, 0xc0000022)
	require.NoError(t, err)
	require.Equal(t, true, fields["Error Response"])
}
