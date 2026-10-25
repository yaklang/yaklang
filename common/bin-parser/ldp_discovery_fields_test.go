package bin_parser

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func TestLDPDiscoveryCanonicalCompleteMessagesAndBoundaries(t *testing.T) {
	// Authored RFC5036 discovery control, not bytes from the public capture.
	w := []byte{0, 1, 0, 30, 192, 0, 2, 10, 0, 7, 1, 0, 0, 20, 0, 0, 0, 9, 4, 0, 0, 4, 0, 15, 0, 0, 4, 1, 0, 4, 192, 0, 2, 11}
	direct, err := stream_parser.DecodeLDPDiscovery(w, 32)
	require.NoError(t, err)
	require.Equal(t, "192.0.2.10", direct["LSR ID"])
	v, err := ParseStructured(w, "ldp_discovery", "LDPDiscoveryFields")
	require.NoError(t, err)
	require.Equal(t, direct, v["fields"].(map[string]any)["Message"])
	for i := 0; i < len(w); i++ {
		_, err = stream_parser.DecodeLDPDiscovery(w[:i], 32)
		require.Error(t, err, "cut %d", i)
		_, err = ParseStructured(w[:i], "ldp_discovery", "LDPDiscoveryFields")
		require.Error(t, err, "canonical cut %d", i)
	}
	for _, offset := range []int{1, 3, 11, 13, 18, 21, 29} {
		bad := append([]byte(nil), w...)
		bad[offset] ^= 1
		_, err = stream_parser.DecodeLDPDiscovery(bad, 32)
		require.Error(t, err, "invalid envelope or mandatory type/size at %d", offset)
	}
	_, err = stream_parser.DecodeLDPDiscovery(w, 2)
	require.ErrorIs(t, err, stream_parser.ErrObservedWireBudget)
	// Two independent Hello messages retain separate IDs and advertisements.
	two := append(append([]byte(nil), w...), w[10:]...)
	binary.BigEndian.PutUint16(two[2:], uint16(len(two)-4))
	two[41] = 10
	m, err := stream_parser.DecodeLDPDiscovery(two, 32)
	require.NoError(t, err)
	msgs := m["Messages"].([]map[string]any)
	require.Len(t, msgs, 2)
	require.NotEqual(t, msgs[0]["Message ID"], msgs[1]["Message ID"])
	// Unknown extensions are bounded owned bytes; U/F are never interpreted as
	// accepted adjacency or authenticated identity.
	ext := append(append([]byte(nil), w...), 0xc7, 0x77, 0, 3, 1, 2, 3)
	binary.BigEndian.PutUint16(ext[2:], uint16(len(ext)-4))
	binary.BigEndian.PutUint16(ext[12:], uint16(len(ext)-14))
	m, err = stream_parser.DecodeLDPDiscovery(ext, 32)
	require.NoError(t, err)
	tlv := m["Messages"].([]map[string]any)[0]["TLVs"].([]map[string]any)[2]
	require.Equal(t, true, tlv["Unknown Ignore"])
	require.Equal(t, true, tlv["Unknown Forward"])
	ext[len(ext)-1] = 99
	require.Equal(t, []byte{1, 2, 3}, tlv["Unknown Value"])
	// IPv6 transport and configuration sequence are optional advertisements;
	// the library does not select a TCP endpoint or calculate a negotiated hold.
	v6 := append(append([]byte(nil), w[:26]...), 4, 3, 0, 16)
	v6 = append(v6, []byte{0x20, 1, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}...)
	v6 = append(v6, 4, 2, 0, 4, 0, 0, 0, 123)
	binary.BigEndian.PutUint16(v6[2:], uint16(len(v6)-4))
	binary.BigEndian.PutUint16(v6[12:], uint16(len(v6)-14))
	v6[22], v6[23], v6[24] = 0, 0, 0xe0
	m, err = stream_parser.DecodeLDPDiscovery(v6, 32)
	require.NoError(t, err)
	tlvs := m["Messages"].([]map[string]any)[0]["TLVs"].([]map[string]any)
	require.Equal(t, "2001:db8::1", tlvs[1]["IPv6 Transport Address"])
	require.Equal(t, uint64(123), tlvs[2]["Configuration Sequence Number"])
	require.Contains(t, tlvs[0]["Hold Time Meaning"], "advertised default")
	require.Contains(t, tlvs[0]["GTSM Meaning"], "ignored for targeted")
}
