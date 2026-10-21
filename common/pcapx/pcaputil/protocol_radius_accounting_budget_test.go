package pcaputil

import (
	"bytes"
	"crypto/md5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRADIUSAccountingRefusalGuard(t *testing.T) {
	big := []byte{4, 7, 0, 109, 162, 44, 122, 99, 223, 174, 94, 31, 200, 206, 87, 183, 70, 65, 144, 113, 4, 6, 192, 0, 2, 9, 40, 6, 0, 0, 0, 2, 44, 17, 102, 105, 120, 116, 117, 114, 101, 45, 115, 101, 115, 115, 105, 111, 110, 41, 6, 0, 0, 0, 9, 42, 6, 255, 255, 255, 254, 43, 6, 0, 0, 0, 123, 45, 6, 0, 0, 0, 1, 46, 6, 0, 0, 14, 16, 47, 6, 0, 0, 0, 19, 48, 6, 0, 0, 0, 17, 49, 6, 0, 0, 0, 1, 52, 6, 0, 0, 0, 2, 53, 6, 0, 0, 0, 3}
	reply := []byte{5, 7, 0, 38, 132, 167, 164, 134, 231, 130, 151, 204, 37, 125, 248, 167, 126, 196, 85, 225, 33, 9, 112, 114, 111, 120, 121, 45, 97, 33, 9, 112, 114, 111, 120, 121, 45, 98}
	small := []byte{4, 8, 0, 49, 85, 93, 103, 164, 209, 93, 55, 196, 163, 230, 113, 71, 108, 247, 229, 205, 4, 6, 192, 0, 2, 9, 40, 6, 0, 0, 0, 2, 44, 17, 102, 105, 120, 116, 117, 114, 101, 45, 115, 101, 115, 115, 105, 111, 110}
	r8 := []byte{5, 8, 0, 20, 11, 171, 35, 101, 32, 71, 22, 164, 193, 5, 140, 65, 33, 112, 171, 128}
	aq := []byte{1, 7, 0, 33, 0, 17, 34, 51, 68, 85, 102, 119, 136, 153, 170, 187, 204, 221, 238, 255, 1, 7, 97, 108, 105, 99, 101, 4, 6, 192, 0, 2, 9}
	ar := []byte{2, 7, 0, 20, 178, 248, 219, 53, 92, 23, 72, 250, 240, 249, 139, 106, 23, 21, 160, 100}
	t.Run("first-refusal-retains-fixed-header", func(t *testing.T) {
		b := DefaultParserBudget()
		b.MaxMessageBytes = 64
		b.MaxBufferedBytes = 4096
		s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(41000, 1813))
		require.NoError(t, err)
		defer s.Close("cleanup")
		out := s.Feed(0, time.Unix(100, 0), big)
		require.NotNil(t, out.Err)
		require.Equal(t, ErrResourceExceeded, out.Err.Kind)
		require.Len(t, out.Events, 1)
		e := out.Events[0]
		assert.NotZero(t, e.FlowID)
		assert.Positive(t, s.Stats().BufferedBytes)
		require.Zero(t, e.TransactionID)
		require.Zero(t, e.ResponseTo)
		r := s.Feed(1, time.Unix(101, 0), reply)
		require.Nil(t, r.Err)
		require.Len(t, r.Events, 1)
		assert.Equal(t, e.FlowID, r.Events[0].FlowID)
		assert.Equal(t, "ambiguous-identifier", r.Events[0].Session["Association"])
		require.Zero(t, r.Events[0].TransactionID)
		require.Zero(t, r.Events[0].ResponseTo)
		require.Empty(t, s.Close("done"))
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
		require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(4096))
	})
	t.Run("neighbor-access-owner-and-fresh-accounting", func(t *testing.T) {
		b := DefaultParserBudget()
		b.MaxMessageBytes = 64
		b.MaxBufferedBytes = 4096
		s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(41000, 1813))
		require.NoError(t, err)
		defer s.Close("cleanup")
		q := s.Feed(0, time.Unix(100, 0), aq)
		require.Nil(t, q.Err)
		require.Len(t, q.Events, 1)
		owner := q.Events[0].ID
		denied := s.Feed(0, time.Unix(101, 0), big)
		require.NotNil(t, denied.Err)
		require.Equal(t, ErrResourceExceeded, denied.Err.Kind)
		old := s.Feed(1, time.Unix(102, 0), reply)
		require.Nil(t, old.Err)
		require.Len(t, old.Events, 1)
		assert.Equal(t, "ambiguous-identifier", old.Events[0].Session["Association"])
		require.Zero(t, old.Events[0].ResponseTo)
		require.Zero(t, old.Events[0].TransactionID)
		accepted := s.Feed(1, time.Unix(103, 0), ar)
		require.Nil(t, accepted.Err)
		require.Len(t, accepted.Events, 1)
		require.Equal(t, owner, accepted.Events[0].ResponseTo)
		require.Equal(t, owner, accepted.Events[0].TransactionID)
		require.Equal(t, "Access-Request", accepted.Events[0].Session["In Reply To"])
		fresh := s.Feed(0, time.Unix(104, 0), small)
		require.Nil(t, fresh.Err)
		require.Len(t, fresh.Events, 1)
		assert.Equal(t, fresh.Events[0].ID, fresh.Events[0].TransactionID)
		require.Zero(t, fresh.Events[0].ResponseTo)
		done := s.Feed(1, time.Unix(105, 0), r8)
		require.Nil(t, done.Err)
		require.Len(t, done.Events, 1)
		assert.Equal(t, fresh.Events[0].ID, done.Events[0].ResponseTo)
		assert.Equal(t, fresh.Events[0].ID, done.Events[0].TransactionID)
		require.Empty(t, s.Close("done"))
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("no-state-byte-room-fallback", func(t *testing.T) {
		b := DefaultParserBudget()
		b.MaxMessageBytes = 64
		b.MaxBufferedBytes = 64
		s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(41000, 1813))
		require.NoError(t, err)
		defer s.Close("cleanup")
		q := s.Feed(0, time.Unix(100, 0), big)
		require.NotNil(t, q.Err)
		require.Equal(t, ErrResourceExceeded, q.Err.Kind)
		require.Len(t, q.Events, 1)
		require.Zero(t, q.Events[0].FlowID)
		require.Zero(t, q.Events[0].TransactionID)
		require.Zero(t, q.Events[0].ResponseTo)
		r := s.Feed(1, time.Unix(101, 0), reply)
		require.NotNil(t, r.Err)
		require.Equal(t, ErrResourceExceeded, r.Err.Kind)
		require.Len(t, r.Events, 1)
		require.Zero(t, r.Events[0].TransactionID)
		require.Zero(t, r.Events[0].ResponseTo)
		require.Empty(t, s.Close("done"))
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
		require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(64))
	})
	t.Run("identity-slot-fallback-retains-existing-owners", func(t *testing.T) {
		b := DefaultParserBudget()
		b.MaxMessageBytes = 64
		b.MaxBufferedBytes = 4096
		b.MaxCollectionElements = 3
		s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(41000, 1813))
		require.NoError(t, err)
		defer s.Close("cleanup")
		mk := func(id byte) []byte {
			w := bytes.Clone(small)
			w[1] = id
			clear(w[4:20])
			sum := md5.Sum(append(bytes.Clone(w), []byte("public-accounting-fixture-key")...))
			copy(w[4:20], sum[:])
			return w
		}
		var owners []uint64
		for i := byte(1); i <= 3; i++ {
			o := s.Feed(0, time.Unix(int64(99+i), 0), mk(i))
			require.Nil(t, o.Err)
			require.Len(t, o.Events, 1)
			owners = append(owners, o.Events[0].ID)
			require.Equal(t, o.Events[0].ID, o.Events[0].TransactionID)
		}
		denied := s.Feed(0, time.Unix(103, 0), big)
		require.NotNil(t, denied.Err)
		require.Equal(t, ErrResourceExceeded, denied.Err.Kind)
		require.Len(t, denied.Events, 1)
		require.Zero(t, denied.Events[0].ResponseTo)
		require.Zero(t, denied.Events[0].TransactionID)
		untracked := s.Feed(1, time.Unix(104, 0), reply)
		require.NotNil(t, untracked.Err)
		require.Equal(t, ErrResourceExceeded, untracked.Err.Kind)
		require.Len(t, untracked.Events, 1)
		require.Zero(t, untracked.Events[0].TransactionID)
		require.Zero(t, untracked.Events[0].ResponseTo)
		for i := byte(1); i <= 3; i++ {
			w := bytes.Clone(r8)
			w[1] = i
			req := mk(i)
			signed := append(bytes.Clone(w[:4]), req[4:20]...)
			signed = append(signed, []byte("public-accounting-fixture-key")...)
			sum := md5.Sum(signed)
			copy(w[4:20], sum[:])
			o := s.Feed(1, time.Unix(int64(104+i), 0), w)
			require.Nil(t, o.Err)
			require.Len(t, o.Events, 1)
			require.Equal(t, owners[i-1], o.Events[0].TransactionID)
			require.Equal(t, owners[i-1], o.Events[0].ResponseTo)
			require.Equal(t, "Accounting-Request", o.Events[0].Session["In Reply To"])
		}
		require.Empty(t, s.Close("done"))
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
		require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(4096))
	})

}
