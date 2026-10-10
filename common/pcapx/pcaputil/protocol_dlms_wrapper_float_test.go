package pcaputil

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDLMSWrapperFloatExistingAPIBaseline(t *testing.T) {
	assertWrapperFloatNormalBoundary(t)
	for _, wire := range []string{"0001000100100008c403c10100120100", "000100010010000ac403c10100173fc00000"} {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
		require.NoError(t, err)
		out := s.Feed(1, time.Unix(1, 0), wrapperWire(t, wire))
		require.Nil(t, out.Err)
		require.Len(t, out.Events, 1)
		fields, err := out.Events[0].GetFields()
		require.NoError(t, err)
		require.NotNil(t, fields)
		require.Zero(t, out.Events[0].ResponseTo)
		require.Zero(t, out.Events[0].TransactionID)
		require.Empty(t, s.Close("float-baseline"))
		require.Zero(t, s.Stats().BufferedBytes)
	}
}

func assertWrapperFloatNormalBoundary(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
	require.NoError(t, err)
	o := s.Feed(1, time.Unix(1, 0), wrapperWire(t, "0001000100100009c401c100173fc00000"))
	require.NotNil(t, o.Err)
	require.Equal(t, ErrUnsupportedFeature, o.Err.Kind)
	require.Len(t, o.Events, 1)
	f, err := o.Events[0].GetFields()
	rocTypedError(t, "UnsupportedFeature", err)
	require.Nil(t, f)
	require.Zero(t, o.Events[0].ResponseTo)
	require.Zero(t, o.Events[0].TransactionID)
	require.Empty(t, s.Close("normal-boundary"))
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestDLMSWrapperFloatSealedMatrix(t *testing.T) {
	wrapperOwnedDataMatrix(t, wrapperOwnedDataControls(t, "dlms-float", 29))
}
func TestDLMSWrapperFloatBudgetsOwnership(t *testing.T) {
	wrapperOwnedDataBudgetsOwnership(t, wrapperOwnedDataControls(t, "dlms-float", 29))
}

func BenchmarkDLMSWrapperSelectedData(b *testing.B) {
	normal := []byte{0, 1, 0, 1, 0, 16, 0, 7, 0xc4, 1, 0xc1, 0, 18, 0, 1}
	payload := []byte{0xc4, 3, 0xc1, 1, 0, 1, 0x81, 128}
	for i := 0; i < 128; i++ {
		payload = append(payload, 18, 0, 1)
	}
	list := append([]byte{0, 1, 0, 1, 0, 16, byte(len(payload) >> 8), byte(len(payload))}, payload...)
	for _, c := range []struct {
		name string
		wire []byte
	}{{"normal-uint16", normal}, {"list-128-uint16", list}} {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(c.wire)))
			for i := 0; i < b.N; i++ {
				m, err := decodeDLMSWrapper(c.wire, 4096)
				if err != nil || m == nil {
					b.Fatal(err)
				}
			}
		})
	}
}
