package pcaputil

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// RFC2865 literal header and the fixed legacy rule's typed projection. Public
// fields/caches are caller-owned; changing them must not change later queries.
func TestRADIUSEAPLegacyProjectionOwnership(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		t.Run(map[bool]string{false: "full", true: "deferred"}[deferred], func(t *testing.T) {
			s, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			defer s.Close("cleanup")
			s.(*captureSession).f.a.config.Deferred = deferred
			q := s.Feed(0, time.Unix(100, 0), agentRadiusRequest(7))
			require.Nil(t, q.Err)
			require.Len(t, q.Events, 1)
			v := q.Events[0]
			id := v.ID
			want := map[string]any{"Code": byte(1), "Identifier": byte(7), "Length": uint16(33), "Authenticator": []byte("0123456789abcdef"), "Attributes": []any{map[string]any{"Type": byte(1), "Length": byte(7), "User-Name": "alice"}, map[string]any{"Type": byte(4), "Length": byte(6), "NAS-IP-Address": []byte{192, 0, 2, 9}}}}
			fields, err := v.GetFields()
			require.NoError(t, err)
			require.Equal(t, want, fields)
			mutateRTPSQuery(fields)
			tlsDTLSPoison(v, "http")
			fields, err = v.GetFields()
			require.NoError(t, err)
			require.Equal(t, want, fields)
			r := s.Feed(1, time.Unix(101, 0), radiusPkt(2, 7, nil))
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			require.Equal(t, id, r.Events[0].ResponseTo)
			require.Equal(t, id, r.Events[0].TransactionID)
			require.Equal(t, "Access-Request", r.Events[0].Session["In Reply To"])
			require.Empty(t, s.Close("done"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
