package pcaputil

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// Independent mixed-family Close oracle from fixed RFC packet fields and the
// explicitly selected passive lifecycle policy, never obtained from IUT output.
func TestRADIUSAccountingMixedClose(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41000, 1813))
	require.NoError(t, err)
	defer s.Close("cleanup")
	wires := [][]byte{[]byte{1, 7, 0, 33, 0, 17, 34, 51, 68, 85, 102, 119, 136, 153, 170, 187, 204, 221, 238, 255, 1, 7, 97, 108, 105, 99, 101, 4, 6, 192, 0, 2, 9}, []byte{4, 7, 0, 49, 31, 185, 202, 128, 211, 173, 224, 207, 88, 39, 31, 83, 100, 124, 59, 134, 4, 6, 192, 0, 2, 9, 40, 6, 0, 0, 0, 2, 44, 17, 102, 105, 120, 116, 117, 114, 101, 45, 115, 101, 115, 115, 105, 111, 110}}
	wants := []map[string]any{map[string]any{"Attribute Count": int(2), "Attributes": []map[string]any{{"Type": byte(1), "Raw": []byte{97, 108, 105, 99, 101}}, {"Type": byte(4), "Raw": []byte{192, 0, 2, 9}}}, "Authentication": "not-verified", "Authenticator": []byte{0, 17, 34, 51, 68, 85, 102, 119, 136, 153, 170, 187, 204, 221, 238, 255}, "Code": int(1), "Identifier": int(7), "NAS-IP-Address": "192.0.2.9", "Observation Scope": "conversation", "Packet Length": int(33), "Packet Name": "Access-Request", "Padding Bytes": int(0), "Role": "request", "User-Name": "alice"}, map[string]any{"Acct-Session-Id": []byte{102, 105, 120, 116, 117, 114, 101, 45, 115, 101, 115, 115, 105, 111, 110}, "Acct-Status-Type": uint32(2), "Attribute Count": int(3), "Attributes": []map[string]any{{"Type": byte(4), "Raw": []byte{192, 0, 2, 9}}, {"Type": byte(40), "Raw": []byte{0, 0, 0, 2}}, {"Type": byte(44), "Raw": []byte{102, 105, 120, 116, 117, 114, 101, 45, 115, 101, 115, 115, 105, 111, 110}}}, "Authentication": "not-verified", "Authenticator": []byte{31, 185, 202, 128, 211, 173, 224, 207, 88, 39, 31, 83, 100, 124, 59, 134}, "Code": int(4), "Identifier": int(7), "NAS-IP-Address": "192.0.2.9", "Observation Scope": "conversation", "Packet Length": int(49), "Packet Name": "Accounting-Request", "Padding Bytes": int(0), "Role": "request"}}
	var ids []uint64
	for i, w := range wires {
		o := s.Feed(0, time.Unix(int64(100+i), 0), w)
		require.Nil(t, o.Err)
		require.Len(t, o.Events, 1)
		e := o.Events[0]
		ids = append(ids, e.ID)
		require.NotZero(t, e.ID)
		require.Zero(t, e.ResponseTo)
		require.Equal(t, e.ID, e.TransactionID)
		require.Equal(t, "radius", e.Protocol)
		profile := "radius-access-observed"
		if i == 1 {
			profile = "radius-accounting-observed"
		}
		require.Equal(t, profile, e.Profile)
		require.Equal(t, 0, e.Direction)
		require.True(t, time.Unix(int64(100+i), 0).Equal(e.Timestamp))
		require.Equal(t, w, e.Raw)
		require.Equal(t, len(w), e.Length)
		require.Equal(t, wants[i], e.Session)
		f, err := e.GetFields()
		require.NoError(t, err)
		require.Equal(t, wants[i], f)
	}
	require.NotEqual(t, ids[0], ids[1])
	end := s.Close("complete")
	require.Len(t, end, 1)
	e := end[0]
	require.Equal(t, "radius", e.Protocol)
	require.Equal(t, "radius-observed", e.Profile)
	require.Equal(t, "incomplete", e.Status)
	require.Equal(t, "incomplete", e.Completeness)
	require.Equal(t, map[string]any{"Outstanding": int(2)}, e.Session)
	require.Zero(t, e.TransactionID)
	require.Zero(t, e.ResponseTo)
	require.Empty(t, e.Raw)
	require.Zero(t, e.Length)
	f, err := e.GetFields()
	require.Nil(t, f)
	rocTypedError(t, "NeedMore", err)
	tlsDTLSPoison(e, "http")
	f, err = e.GetFields()
	require.Nil(t, f)
	rocTypedError(t, "NeedMore", err)
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}
