package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTLSEarlyDataRejectedHelloLiteralObservation(t *testing.T) {
	for _, n := range []int{0, 1, 17} {
		body := tlsResumptionHello(true, nil, 0x1301, tlsResumptionExt{43, []byte{2, 3, 4}}, tlsResumptionExt{42, bytes.Repeat([]byte{7}, n)})
		wire := carrierTestRecord(22, reliabilityHS(1, body))
		before := bytes.Clone(wire)
		o, err := ObserveTLSEarlyDataHelloRecord(wire, 2)
		require.NoError(t, err, "observation must not turn a rejected complete Hello into missing handshake context")
		require.Equal(t, byte(1), o.HandshakeType)
		require.Equal(t, 2, o.Extensions)
		require.Equal(t, n, o.ExtensionDataLength)
		require.Equal(t, []byte{0, 42}, wire[o.ExtensionOffset:o.ExtensionOffset+2])
		require.Equal(t, uint16(n), binary.BigEndian.Uint16(wire[o.ExtensionOffset+2:]))
		require.Equal(t, before, wire)
		_, err = ObserveTLSEarlyDataHelloRecord(wire, 1)
		var refusal *ProtocolError
		require.ErrorAs(t, err, &refusal)
		require.Equal(t, ErrResourceExceeded, refusal.Kind)
		for cut := 0; cut < len(wire); cut++ {
			v, err := ObserveTLSEarlyDataHelloRecord(wire[:cut], 2)
			require.Error(t, err)
			require.Nil(t, v)
		}
	}
	validWithoutEarly := carrierTestRecord(22, reliabilityHS(1, tlsResumptionHello(true, nil, 0x1301, tlsResumptionExt{43, []byte{2, 3, 4}})))
	o, err := ObserveTLSEarlyDataHelloRecord(validWithoutEarly, 2)
	require.Error(t, err)
	require.Nil(t, o, "a TLS cipher offer alone cannot fabricate an early_data declaration")
}
