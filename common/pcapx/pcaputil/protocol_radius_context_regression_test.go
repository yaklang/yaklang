package pcaputil

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRADIUSContextRegression(t *testing.T) {
	for _, scenario := range []string{"adjacent", "wrong-direction", "changed-request", "completed-reuse", "bad-nas-width", "bad-password-width"} {
		t.Run(scenario, func(t *testing.T) {
			s, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			defer s.Close("cleanup")
			q := radiusPkt(1, 7, append(radiusAttr(1, []byte("alice")), radiusAttr(4, []byte{192, 0, 2, 1})...))
			feed := func(d int, at int64, w []byte) *ProtocolEvent {
				r := s.Feed(d, time.Unix(at, 0), w)
				require.Nil(t, r.Err)
				require.Len(t, r.Events, 1)
				return r.Events[0]
			}
			if scenario == "bad-nas-width" || scenario == "bad-password-width" {
				attr := radiusAttr(4, []byte{1, 2, 3})
				if scenario == "bad-password-width" {
					attr = radiusAttr(2, []byte{1})
				}
				out := s.Feed(0, time.Unix(100, 0), radiusPkt(1, 7, attr))
				require.NotNil(t, out.Err)
				require.Equal(t, ErrMalformedMessage, out.Err.Kind)
				return
			}
			first := feed(0, 100, q)
			require.Equal(t, "Access-Request", first.Session["Packet Name"])
			if scenario == "changed-request" {
				feed(0, 101, radiusPkt(1, 7, append(radiusAttr(1, []byte("other")), radiusAttr(4, []byte{192, 0, 2, 1})...)))
			}
			if scenario == "completed-reuse" {
				normal := feed(1, 101, radiusPkt(2, 7, nil))
				require.Equal(t, "Access-Request", normal.Session["In Reply To"])
				feed(0, 102, q)
			}
			dir := 1
			if scenario == "wrong-direction" {
				dir = 0
			}
			reply := feed(dir, 103, radiusPkt(2, 7, nil))
			if scenario == "adjacent" {
				require.Equal(t, "Access-Request", reply.Session["In Reply To"])
			} else {
				require.NotContains(t, reply.Session, "In Reply To")
				require.Zero(t, reply.ResponseTo)
				require.Zero(t, reply.TransactionID)
			}
		})
	}
}
