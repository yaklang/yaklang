package pcaputil

import (
	"encoding/binary"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// Fixed CFG-2 grammar from GPA/GSF 22a9aa1028c6e53d115c4ea81026055a57b803c2.
// One zero-channel PMU still has STAT, integer FREQ and DFREQ in DATA.
func c37ControlCFG(ver byte, id uint16, format uint16) []byte {
	b := make([]byte, 38)
	binary.BigEndian.PutUint32(b, 1000000)
	binary.BigEndian.PutUint16(b[4:], 1)
	copy(b[6:22], []byte("PMU             "))
	binary.BigEndian.PutUint16(b[22:], id)
	binary.BigEndian.PutUint16(b[24:], format)
	binary.BigEndian.PutUint16(b[32:], 1)
	binary.BigEndian.PutUint16(b[34:], 1)
	binary.BigEndian.PutUint16(b[36:], 50)
	return c37frame(3, ver, id, b)
}
func TestC37118RejectEmptyConfiguration(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer s.Close("test")
	r := s.Feed(0, time.Unix(1, 0), c37frame(4, 1, 7, []byte{0, 2}))
	require.Nil(t, r.Err)
	require.Equal(t, 2, r.Events[0].Session["Command"])
	r = s.Feed(1, time.Unix(2, 0), c37frame(3, 1, 7, []byte{0, 15, 66, 64, 0, 0, 0, 50}))
	require.NotNil(t, r.Err, "zero PMU count cannot be a validated CFG-2")
	require.Equal(t, ErrMalformedMessage, r.Err.Kind)
}
func TestC37118DirectedConfiguration(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer s.Close("test")
	r := s.Feed(1, time.Unix(1, 0), c37ControlCFG(1, 7, 0))
	require.Nil(t, r.Err)
	r = s.Feed(1, time.Unix(2, 0), c37frame(0, 1, 7, []byte{0, 0, 0, 5, 255, 254}))
	require.Nil(t, r.Err)
	r = s.Feed(0, time.Unix(3, 0), c37frame(0, 1, 7, []byte{0, 0, 0, 5, 255, 254}))
	require.NotNil(t, r.Err, "reverse publisher cannot borrow another direction's configuration")
	require.Equal(t, ErrContextRequired, r.Err.Kind)
}

func TestC37118EveryPMUChangeIndication(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer s.Close("test")
	one := c37ControlCFG(1, 7, 0)
	body := append([]byte(nil), one[14:len(one)-4]...)
	binary.BigEndian.PutUint16(body[4:], 2)
	body = append(body, one[20:len(one)-4]...)
	body = append(body, 0, 50)
	cfg := c37frame(3, 1, 7, body)
	r := s.Feed(1, time.Unix(1, 0), cfg)
	require.Nil(t, r.Err)
	data := []byte{0, 0, 0, 5, 255, 254, 0, 0, 0, 6, 0, 1}
	r = s.Feed(1, time.Unix(2, 0), c37frame(0, 1, 7, data))
	require.Nil(t, r.Err)
	data[6] = 4 // Only the second PMU signals configuration change.
	r = s.Feed(1, time.Unix(3, 0), c37frame(0, 1, 7, data))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrContextRequired, r.Err.Kind)
	require.NotContains(t, r.Events[0].Session, "Data")
	r = s.Feed(1, time.Unix(4, 0), cfg)
	require.Nil(t, r.Err)
	for i := 0; i < 2; i++ {
		r = s.Feed(1, time.Unix(int64(5+i), 0), c37frame(0, 1, 7, data))
		require.Nil(t, r.Err, "a captured replacement recovers even while the flag remains set")
	}
}
