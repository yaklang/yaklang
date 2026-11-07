package pcaputil

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func smb2DFSReferralV1(name string, padding int) []byte {
	u := make([]byte, len(name)*2+2)
	for i := range name {
		u[i*2] = name[i]
	}
	entry := make([]byte, 6+len(u))
	binary.LittleEndian.PutUint16(entry[0:2], 1)
	binary.LittleEndian.PutUint16(entry[2:4], uint16(len(entry)))
	copy(entry[6:], u)
	raw := make([]byte, 8+len(entry)+padding)
	binary.LittleEndian.PutUint16(raw[2:4], 1)
	copy(raw[8:], entry)
	return raw
}

// MS-DFSC 2.2.4 permits trailing padding after the V1 referral entries.
func TestSMB2DFSReferralV1AllowsPadding(t *testing.T) {
	raw := smb2DFSReferralV1(`\\srv\share`, 2)
	info, ok, err := smb2DecodeDFSReferralV1(raw, 16)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, uint16(1), info["Number Of Referrals"])
	entries := info["Referrals"].([]map[string]any)
	require.Equal(t, `\\srv\share`, entries[0]["Share Name"])
	require.Contains(t, info, "Referral Padding")
}

// A V3 path offset that points into the fixed entry is not a string; the
// referral stays opaque instead of refusing the whole record.
func TestSMB2DFSReferralV3OffsetIntoEntryStaysOpaque(t *testing.T) {
	raw := make([]byte, 8+18)
	binary.LittleEndian.PutUint16(raw[2:4], 1)
	binary.LittleEndian.PutUint16(raw[8:10], 3)
	binary.LittleEndian.PutUint16(raw[10:12], 18)
	binary.LittleEndian.PutUint16(raw[8+12:8+14], 2) // DFSPathOffset points at entry byte 2
	_, ok, err := smb2DecodeDFSReferralV3(raw, 16)
	require.NoError(t, err)
	require.False(t, ok)
}
