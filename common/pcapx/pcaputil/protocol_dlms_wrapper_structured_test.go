package pcaputil

import (
	"encoding/hex"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDLMSWrapperStructuredExistingAPIBaseline(t *testing.T) {
	for _, h := range []struct{ name, wire string }{
		{"normal-neighbor", "0001000100100008c403c10100120100"},
		{"structured-positive", "000100010010000dc403c101000202120001120002"},
	} {
		t.Run(h.name, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			b, err := hex.DecodeString(h.wire)
			require.NoError(t, err)
			o := s.Feed(1, time.Unix(1, 0), b)
			require.Nil(t, o.Err)
			require.Len(t, o.Events, 1)
			f, err := o.Events[0].GetFields()
			require.NoError(t, err)
			require.NotNil(t, f)
			require.Zero(t, o.Events[0].ResponseTo)
			require.Empty(t, s.Close("baseline"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
