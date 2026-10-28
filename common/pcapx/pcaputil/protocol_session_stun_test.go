package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func stunTestMessage(method, class uint16, id byte, attrs ...[]byte) []byte {
	w := make([]byte, 20)
	binary.BigEndian.PutUint16(w, method&15|(method&0x70)<<1|(method&0xf80)<<2|(class&1)<<4|(class&2)<<7)
	binary.BigEndian.PutUint32(w[4:], 0x2112a442)
	w[19] = id
	for _, v := range attrs {
		w = append(w, v...)
	}
	binary.BigEndian.PutUint16(w[2:], uint16(len(w)-20))
	return w
}
func stunTestAttr(typ uint16, v []byte) []byte {
	w := make([]byte, 4)
	binary.BigEndian.PutUint16(w, typ)
	binary.BigEndian.PutUint16(w[2:], uint16(len(v)))
	w = append(w, v...)
	for len(w)%4 != 0 {
		w = append(w, 0)
	}
	return w
}
func TestSTUNSessionAssociationAndBounds(t *testing.T) {
	steps := []sessionStep{{0, stunTestMessage(1, 0, 1)}, {1, stunTestMessage(1, 0, 1)}, {1, stunTestMessage(1, 2, 1)}, {0, stunTestMessage(1, 2, 1)}}
	for _, chunk := range []int{0, 1, 7} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("chunk=%d/deferred=%v", chunk, deferred), func(t *testing.T) {
				events, _ := sessionTestFlow(t, "stun", steps, chunk, deferred)
				require.Len(t, events, 4)
				for _, e := range events {
					require.Empty(t, e.Error)
					_, err := e.Decode()
					require.NoError(t, err)
				}
				require.Equal(t, true, events[2].Session["Matched"])
				require.Equal(t, true, events[3].Session["Matched"])
				require.Equal(t, 0, events[3].Session["Pending Transactions"])
			})
		}
	}
	s, err := NewProtocolSession(ParserBudget{MaxCollectionElements: 1})
	require.NoError(t, err)
	require.Nil(t, s.Feed(0, time.Unix(1, 0), stunTestMessage(1, 0, 1)).Err)
	require.Nil(t, s.Feed(0, time.Unix(2, 0), stunTestMessage(1, 0, 1)).Err)
	r := s.Feed(0, time.Unix(3, 0), stunTestMessage(1, 0, 2))
	require.NotEmpty(t, r.Events)
	require.Equal(t, "limited", r.Events[0].Status)
	s.Close("done")
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestSTUNUnknownMethodIsNotMisclassifiedAsTURN(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	r := s.Feed(0, time.Unix(1, 0), stunTestMessage(0x155, 0, 9))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Len(t, r.Events, 1)
	require.Equal(t, "stun", r.Events[0].Protocol, "an unknown STUN method is not sufficient evidence for TURN")
	require.NotEqual(t, "turn-rfc8656-udp", r.Events[0].Profile)
	require.Equal(t, false, r.Events[0].Session["TURN"])
}

func TestTURNObservedChannelLifecycle(t *testing.T) {
	s := &binSTUN{}
	ts := time.Unix(100, 0)
	peer := stunTestAttr(0x12, []byte{0, 1, 0x21, 0x13, 0xe1, 0x12, 0xa6, 0x43}) // 192.0.2.1:1
	ch := stunTestAttr(0xc, []byte{0x40, 1, 0, 0})
	_, err := s.consume(0, ts, stunTestMessage(9, 0, 1, peer, ch), 8, false)
	require.NoError(t, err)
	out, err := s.consume(1, ts, stunTestMessage(9, 2, 1), 8, false)
	require.NoError(t, err)
	require.Equal(t, true, out["Matched"])
	for _, dir := range []int{0, 1} {
		out, err = s.consume(dir, ts, []byte{0x40, 1, 0, 1, 42}, 8, false)
		require.NoError(t, err)
		require.Equal(t, "192.0.2.1:1", out["Peer Address"])
	}
	out, err = s.consume(0, ts.Add(601*time.Second), []byte{0x40, 1, 0, 1, 42}, 8, false)
	require.NoError(t, err)
	require.Equal(t, false, out["Matched"])
	_, err = s.consume(0, ts, []byte{0x40, 1, 0, 4, 42}, 8, false)
	require.Error(t, err)
}
func TestSTUNIPv6AndMalformed(t *testing.T) {
	t.Run("owned-error-query", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 3478))
		require.NoError(t, err)
		defer s.Close("cleanup")
		wire := stunTestMessage(1, 2, 1, stunTestAttr(0x8028, []byte{0, 0, 0, 0}))
		out := s.Feed(1, time.Unix(1, 0), wire)
		require.Len(t, out.Events, 1)
		require.Equal(t, ErrMalformedMessage, out.Err.Kind)
		fields, err := out.Events[0].GetFields()
		require.Nil(t, fields)
		assertOwnedProtocolRefusal(t, out.Events[0], "MalformedMessage", err)
		require.Equal(t, ErrMalformedMessage, out.Err.Kind)
		require.Zero(t, out.Events[0].ResponseTo)
		require.Zero(t, out.Events[0].TransactionID)
		require.Empty(t, s.Close("complete"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
	// RFC 8489 section 14.7: the message length includes FINGERPRINT;
	// this fixed request CRC was also checked using Python's independent
	// binascii.crc32. No credentials or integrity/authentication claim is made.
	valid, err := hex.DecodeString("000100082112a44200000000000000000000000180280004c5adc960")
	require.NoError(t, err)
	for _, deferred := range []bool{false, true} {
		for _, chunk := range []int{0, 1, 7} {
			for _, bad := range []bool{false, true} {
				t.Run(fmt.Sprintf("fingerprint/deferred=%v/chunk=%d/bad=%v", deferred, chunk, bad), func(t *testing.T) {
					wire := append([]byte(nil), valid...)
					if bad {
						wire[len(wire)-1] ^= 1
					}
					events, _ := sessionTestFlow(t, "stun", []sessionStep{{0, wire}}, chunk, deferred)
					require.Len(t, events, 1)
					e := events[0]
					require.Equal(t, wire, e.Raw)
					fields, queryErr := e.GetFields()
					if bad {
						require.Nil(t, fields)
						require.Equal(t, "malformed", e.Status)
						require.Nil(t, e.Session)
						require.Zero(t, e.ResponseTo)
						require.Zero(t, e.TransactionID)
						assertOwnedProtocolRefusal(t, e, "MalformedMessage", queryErr)
					} else {
						require.NoError(t, queryErr)
						encoded, err := json.Marshal(fields)
						require.NoError(t, err)
						require.JSONEq(t, `{"Message Type":1,"Length":8,"Magic Cookie":554869826,"Transaction ID":"AAAAAAAAAAAAAAAB","Attributes":[{"Type":32808,"Length":4,"Fingerprint":3316500832}]}`, string(encoded))
						require.Equal(t, map[string]any{"Packet Name": "Binding", "Class": uint16(0), "Transaction ID": "000000000000000000000001", "TURN": false, "Integrity": "absent", "Fingerprint Valid": true, "Attributes": []any{map[string]any{"Type": uint16(32808), "Length": 4}}, "Retransmission": false, "Pending Transactions": 1}, e.Session)
					}
				})
			}
		}
	}
	w := stunTestMessage(1, 2, 3)
	v := make([]byte, 20)
	v[1] = 2
	binary.BigEndian.PutUint16(v[2:], 3478^0x2112)
	ip := []byte{0x20, 1, 0xd, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	for i := range ip {
		v[4+i] = ip[i] ^ w[4+i]
	}
	w = stunTestMessage(1, 2, 3, stunTestAttr(0x20, v))
	events, _ := sessionTestFlow(t, "stun", []sessionStep{{1, w}}, 1, false)
	for _, e := range events {
		t.Logf("%s %s %s: %s", e.Protocol, e.Entry, e.Status, e.Error)
	}
	require.Len(t, events, 1)
	require.Empty(t, events[0].Error)
	require.Equal(t, "[2001:db8::1]:3478", events[0].Session["Mapped Address"])
	for _, bad := range [][]byte{w[:len(w)-1], stunTestMessage(1, 2, 1, stunTestAttr(0x20, []byte{0, 2, 0, 0, 0, 0, 0, 0})), stunTestMessage(1, 2, 1, stunTestAttr(0x8028, []byte{0, 0, 0, 0}))} {
		_, err := (&binSTUN{}).consume(0, time.Time{}, bad, 8, false)
		require.Error(t, err)
	}
}
