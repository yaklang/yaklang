package pcaputil

import (
	"encoding/binary"
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
