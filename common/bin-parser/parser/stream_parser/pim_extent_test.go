package stream_parser

import (
	"encoding/binary"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPIMWholeChecksumRequiresTheDeclaredParentBoundary(t *testing.T) {
	for _, v6 := range []bool{false, true} {
		context := PIMWireContext{Source: netip.MustParseAddr("192.0.2.1"), Destination: netip.MustParseAddr("224.0.0.13")}
		if v6 {
			context.Source, context.Destination = netip.MustParseAddr("fe80::1"), netip.MustParseAddr("ff02::d")
		}
		w := []byte{0x20, 0, 0, 0, 0, 1, 0, 2, 0, 105}
		binary.BigEndian.PutUint16(w[2:4], PIMChecksum(w, context, len(w)))
		for _, declared := range []int{-1, len(w) + 1, len(w), 0} {
			context.DeclaredBytes = declared
			m, err := DecodePIMControl(w, context, 64)
			complete := declared == 0 || declared == len(w)
			if complete {
				require.NoError(t, err)
				require.Equal(t, "message", m["Checksum Scope"])
			} else {
				require.ErrorIs(t, err, ErrObservedWireContext)
				require.Equal(t, pimPrefixScope, m["Observation Scope"])
				require.Equal(t, "unverified: enclosing IP message extent unavailable or incomplete", m["Checksum Scope"])
			}
			require.Equal(t, complete, m["Checksum Verified"])
			require.Equal(t, uint64(105), m["Options"].([]map[string]any)[0]["Hold Time"], "retain the actually complete prefix")
		}
		context.DeclaredBytes = len(w) - 1
		m, err := DecodePIMControl(w, context, 64)
		require.ErrorIs(t, err, ErrObservedWireContext)
		require.Nil(t, m, "bytes outside a declared boundary are not a PIM field tree")
	}
}

func TestPIMRegisterHeaderIntegrityIsDistinctFromMessageCompletion(t *testing.T) {
	context := PIMWireContext{Source: netip.MustParseAddr("192.0.2.1"), Destination: netip.MustParseAddr("192.0.2.2"), DeclaredBytes: -1}
	// A Null-Register's complete dummy IPv4 header advertises (S,G), not UDP.
	w := make([]byte, 28)
	w[0], w[4], w[8] = 0x21, 0x40, 0x45
	copy(w[20:24], []byte{192, 0, 2, 10})
	copy(w[24:28], []byte{239, 1, 2, 3})
	binary.BigEndian.PutUint16(w[2:4], PIMChecksum(w[:8], context, 8))
	m, err := DecodePIMControl(w, context, 64)
	require.ErrorIs(t, err, ErrObservedWireContext)
	require.Equal(t, true, m["Checksum Verified"], "only the complete eight-byte header was checked")
	require.Equal(t, "register-header", m["Checksum Scope"])
	require.Equal(t, pimPrefixScope, m["Observation Scope"])
	require.Equal(t, true, m["Null Register"])

	// An unavailable whole-message interop checksum cannot be called failed.
	w[2] ^= 1
	m, err = DecodePIMControl(w, context, 64)
	require.ErrorIs(t, err, ErrObservedWireContext)
	require.Equal(t, false, m["Checksum Verified"])
	require.Equal(t, "unverified: enclosing IP message extent unavailable or incomplete", m["Checksum Scope"])
}
