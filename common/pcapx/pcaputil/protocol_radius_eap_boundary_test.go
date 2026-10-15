package pcaputil

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func eapGuardCanonical(w []byte, user, reply, association string) map[string]any {
	attrs := make([]map[string]any, 0)
	if user != "" {
		attrs = append(attrs, map[string]any{"Type": byte(1), "Raw": []byte(user)})
	}
	name, role := "Access-Request", "request"
	if w[0] == 2 {
		name, role = "Access-Accept", "response"
	}
	m := map[string]any{"Packet Name": name, "Code": int(w[0]), "Identifier": int(w[1]), "Authenticator": []byte("0123456789abcdef"), "Packet Length": len(w), "Padding Bytes": 0, "Authentication": "not-verified", "Role": role, "Attributes": attrs, "Attribute Count": len(attrs)}
	if user != "" {
		m["User-Name"] = user
	}
	if reply != "" {
		m["In Reply To"] = reply
	}
	if association != "" {
		m["Association"] = association
	}
	return m
}
func eapGuardLegacy(w []byte, user string) map[string]any {
	m := map[string]any{"Code": w[0], "Identifier": w[1], "Length": uint16(len(w)), "Authenticator": []byte("0123456789abcdef")}
	if user != "" {
		m["Attributes"] = []any{map[string]any{"Type": byte(1), "Length": byte(len(user) + 2), "User-Name": user}}
	}
	return m
}
func TestRADIUSEAPIndependentRefusalBoundaryGuard(t *testing.T) {
	for _, badTail := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete-valid-tlv-recovers", true: "budget-hides-bad-tlv-no-resync"}[badTail], func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxCollectionElements = 3
			s, e := NewProtocolSession(b)
			require.NoError(t, e)
			defer s.Close("cleanup")
			attrs := append(radiusAttr(1, []byte("alice")), radiusAttr(4, []byte{192, 0, 2, 9})...)
			attrs = append(attrs, radiusAttr(32, []byte("nas"))...)
			attrs = append(attrs, radiusAttr(250, []byte("budget-crossing"))...)
			if badTail {
				attrs = append(attrs, 250, 1)
			}
			first := radiusPkt(1, 7, attrs)
			neighbor := radiusPkt(1, 8, radiusAttr(1, []byte("neighbor")))
			late, answer := radiusPkt(2, 7, nil), radiusPkt(2, 8, nil)
			o := s.Feed(0, time.Unix(100, 0), first)
			require.NotNil(t, o.Err)
			require.Equal(t, ErrResourceExceeded, o.Err.Kind)
			require.Len(t, o.Events, 1)
			v := o.Events[0]
			require.Equal(t, "radius", v.Protocol)
			require.Equal(t, "radius-access-observed", v.Profile)
			require.Equal(t, first, v.Raw)
			require.Equal(t, len(first), v.Length)
			require.Equal(t, "limited", v.Status)
			require.Equal(t, "limited", v.Completeness)
			require.Nil(t, v.Session)
			require.Zero(t, v.TransactionID)
			require.Zero(t, v.ResponseTo)
			f, e := v.GetFields()
			require.Nil(t, f)
			rocTypedError(t, "ResourceExceeded", e)
			o = s.Feed(0, time.Unix(101, 0), neighbor)
			if badTail {
				require.NotNil(t, o.Err, "malformedouterTLV cannotestablish recoveryboundary")
				require.Equal(t, ErrFatalSessionError, o.Err.Kind)
				require.Empty(t, o.Events)
			} else {
				require.Nil(t, o.Err)
				require.Len(t, o.Events, 1)
				q := o.Events[0]
				require.Equal(t, neighbor, q.Raw)
				require.Equal(t, eapGuardCanonical(neighbor, "neighbor", "", ""), q.Session)
				owner := q.ID
				require.NotZero(t, owner)
				require.Equal(t, owner, q.TransactionID)
				require.Zero(t, q.ResponseTo)
				f, e = q.GetFields()
				require.NoError(t, e)
				require.Equal(t, eapGuardLegacy(neighbor, "neighbor"), f)
				mutateRTPSQuery(f)
				f, e = q.GetFields()
				require.NoError(t, e)
				require.Equal(t, eapGuardLegacy(neighbor, "neighbor"), f)
				o = s.Feed(1, time.Unix(102, 0), late)
				require.Nil(t, o.Err)
				require.Len(t, o.Events, 1)
				r := o.Events[0]
				require.Equal(t, late, r.Raw)
				require.Equal(t, eapGuardCanonical(late, "", "", "ambiguous-identifier"), r.Session)
				require.Zero(t, r.ResponseTo)
				require.Zero(t, r.TransactionID)
				o = s.Feed(1, time.Unix(103, 0), answer)
				require.Nil(t, o.Err)
				require.Len(t, o.Events, 1)
				r = o.Events[0]
				require.Equal(t, answer, r.Raw)
				require.Equal(t, eapGuardCanonical(answer, "", "Access-Request", ""), r.Session)
				require.Equal(t, owner, r.TransactionID)
				require.Equal(t, owner, r.ResponseTo)
				f, e = r.GetFields()
				require.NoError(t, e)
				require.Equal(t, eapGuardLegacy(answer, ""), f)
			}
			require.Empty(t, s.Close("first"))
			require.Empty(t, s.Close("second"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
