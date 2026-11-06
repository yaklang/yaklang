package stream_parser

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKerberosOpaqueStructureAndBrokenDERAreDistinct(t *testing.T) {
	for _, wire := range [][]byte{{0x6b, 0}, {0x6b, 2, 0x04, 0}} {
		_, _, err := decodeKerberosFields(wire, false)
		var context *KerberosContextRequired
		require.True(t, errors.As(err, &context), "%x: %v", wire, err)
	}
	for _, wire := range [][]byte{{0x6b, 3, 0x30, 0}, {0x6b, 0, 0}, {0x6b, 0x81}} {
		_, _, err := decodeKerberosFields(wire, false)
		require.Error(t, err)
		var context *KerberosContextRequired
		require.False(t, errors.As(err, &context), "%x: %v", wire, err)
	}
	// The same opaque structure is still invalid certificate DER/schema.
	_, _, err := decodeX509CertificateDER([]byte{0x30, 0})
	require.Error(t, err)
	var context *KerberosContextRequired
	require.False(t, errors.As(err, &context))
}
