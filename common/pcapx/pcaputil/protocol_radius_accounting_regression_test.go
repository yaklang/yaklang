package pcaputil

import (
	"crypto/md5"
	"encoding/binary"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func accountingIndependentPacket(code, id byte, attrs []byte) []byte {
	w := make([]byte, 20)
	w[0], w[1] = code, id
	w = append(w, attrs...)
	binary.BigEndian.PutUint16(w[2:4], uint16(len(w)))
	if code == 4 {
		h := md5.Sum(append(append([]byte(nil), w...), []byte("public-accounting-fixture-key")...))
		copy(w[4:20], h[:])
	} else {
		copy(w[4:20], []byte("0123456789abcdef"))
	}
	return w
}
func TestRADIUSAccountingIndependentMissingAPI(t *testing.T) {
	attrs := append(radiusAttr(4, []byte{192, 0, 2, 9}), radiusAttr(40, []byte{0, 0, 0, 2})...)
	attrs = append(attrs, radiusAttr(44, []byte("fixture-session"))...)
	for _, code := range []byte{4, 5} {
		name := "request4"
		if code == 5 {
			name = "response5"
		}
		t.Run(name, func(t *testing.T) {
			s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41000, 1813))
			require.NoError(t, e)
			defer s.Close("cleanup")
			a := attrs
			if code == 5 {
				a = nil
			}
			w := accountingIndependentPacket(code, 7, a)
			out := s.Feed(int(code-4), time.Unix(100, 0), w)
			require.Nil(t, out.Err)
			require.Len(t, out.Events, 1)
			require.Equal(t, "radius", out.Events[0].Protocol)
			require.Equal(t, int(code), out.Events[0].Session["Code"])
			require.Equal(t, w, out.Events[0].Raw)
		})
	}
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
