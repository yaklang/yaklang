package pcaputil

import (
	"encoding/hex"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRADIUSEAPIndependentMissingAPI(t *testing.T) {
	t.Run("actual-identity-fields", func(t *testing.T) {
		w, _ := hex.DecodeString("0105008becfe3d2fe4473ec6299095ee46aedf7704060a00000105060000c35c3d060000000f010e4a6f686e2e4d63477569726b1e1330302d31392d30362d45412d42382d38431f1330302d31342d32322d45392d35342d35450606000000020c06000005dc4f1302000011014a6f686e2e4d63477569726b501228c5beb8842486da70db51316f9d7889")
		s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41000, 1812))
		require.NoError(t, e)
		defer s.Close("cleanup")
		out := s.Feed(0, time.Unix(100, 0), w)
		require.Nil(t, out.Err)
		require.Len(t, out.Events, 1)
		ev := out.Events[0]
		require.Equal(t, "radius", ev.Protocol)
		require.Equal(t, w, ev.Raw)
		require.Equal(t, 5, ev.Session["Identifier"])
		// RFC3748 4.1/5.1, fixed received bytes; EAP identifier0 != outer5.
		eap, ok := ev.Session["EAP"].(map[string]any)
		require.True(t, ok, "existing API lacks the complete typed EAP Response Identity fields")
		require.Equal(t, byte(2), eap["Code"])
		require.Equal(t, byte(0), eap["Identifier"])
		require.Equal(t, uint16(17), eap["Length"])
		require.Equal(t, byte(1), eap["Type"])
		require.Equal(t, []byte("John.McGuirk"), eap["Type Data"])
	})
	t.Run("access-adjacent", func(t *testing.T) {
		s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41000, 1812))
		require.NoError(t, e)
		defer s.Close("cleanup")
		q := radiusPkt(1, 7, append(radiusAttr(1, []byte("alice")), radiusAttr(4, []byte{192, 0, 2, 9})...))
		out := s.Feed(0, time.Unix(100, 0), q)
		require.Nil(t, out.Err)
		require.Len(t, out.Events, 1)
		owner := out.Events[0].ID
		out = s.Feed(1, time.Unix(101, 0), radiusPkt(2, 7, nil))
		require.Nil(t, out.Err)
		require.Len(t, out.Events, 1)
		require.Equal(t, "Access-Request", out.Events[0].Session["In Reply To"])
		require.Equal(t, owner, out.Events[0].ResponseTo)
		require.Equal(t, owner, out.Events[0].TransactionID)
		require.Empty(t, s.Close("complete"))
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
}
