package pcaputil

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func testTFTPObservedOACKExistingAPI(t *testing.T) {
	for _, which := range []string{"changed-negotiation", "exact-late-oack"} {
		t.Run(which, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 69))
			require.NoError(t, err)
			defer s.Close("cleanup")
			feed := func(dir int, w []byte) *ProtocolEvent {
				r := s.Feed(dir, time.Unix(100, 0), w)
				require.Len(t, r.Events, 1)
				return r.Events[0]
			}
			q := feed(0, []byte("\x00\x01minimum\x00octet\x00blksize\x001024\x00"))
			require.Empty(t, q.Error)
			o := feed(1, []byte("\x00\x06blksize\x00512\x00"))
			require.Empty(t, o.Error)
			ack := feed(0, []byte{0, 4, 0, 0})
			require.Empty(t, ack.Error)
			if which == "changed-negotiation" {
				changed := feed(1, []byte("\x00\x06blksize\x001024\x00"))
				require.NotEmpty(t, changed.Error, "observed OACK must not silently renegotiate its earlier accepted value")
			} else {
				retry := feed(1, []byte("\x00\x06blksize\x00512\x00"))
				require.Empty(t, retry.Error)
				data := feed(1, []byte{0, 3, 0, 1, 42})
				require.Empty(t, data.Error, "duplicate OACK must not erase an already observed ACK0")
				require.Equal(t, 1, data.Session["Data Length"])
				done := feed(0, []byte{0, 4, 0, 1})
				require.Empty(t, done.Error)
				require.Equal(t, true, done.Session["Transfer Complete"])
			}
			require.Empty(t, s.Close("done"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func testTFTPFieldsOwnedExistingAPI(t *testing.T) {
	s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 69))
	require.NoError(t, e)
	defer s.Close("cleanup")
	wire := []byte("\x00\x01minimum\x00octet\x00blksize\x001024\x00")
	r := s.Feed(0, time.Unix(100, 0), wire)
	clear(wire)
	require.Len(t, r.Events, 1)
	ev := r.Events[0]
	f, e := ev.GetFields()
	require.NoError(t, e)
	before, e := json.Marshal(f)
	require.NoError(t, e)
	f["Opcode"] = uint16(9999)
	ev.Raw[0] ^= 255
	ev.Session["Filename"] = "forged"
	again, e := ev.GetFields()
	require.NoError(t, e)
	after, e := json.Marshal(again)
	require.NoError(t, e)
	require.Equal(t, string(before), string(after), "returned fields must not alter later observations")
	require.Empty(t, s.Close("done"))
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}

func testTFTPRejectedQueryExistingAPI(t *testing.T) {
	s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 69))
	require.NoError(t, e)
	defer s.Close("cleanup")
	require.Len(t, s.Feed(0, time.Unix(100, 0), []byte("\x00\x01minimum\x00octet\x00blksize\x001024\x00")).Events, 1)
	r := s.Feed(1, time.Unix(101, 0), []byte("\x00\x06tsize\x001\x00"))
	require.Len(t, r.Events, 1)
	require.NotEmpty(t, r.Events[0].Error)
	f, e := r.Events[0].GetFields()
	require.Error(t, e, "semantic refusal cannot become successful field decoding")
	require.Nil(t, f)
}

func testTFTPByteRefusalExistingAPI(t *testing.T) {
	b := DefaultParserBudget()
	b.MaxMessageBytes = 128
	b.MaxFrameBytes = 128
	s, e := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 69))
	require.NoError(t, e)
	defer s.Close("cleanup")
	feed := func(d int, w []byte) *ProtocolEvent {
		r := s.Feed(d, time.Unix(100, 0), w)
		require.Len(t, r.Events, 1)
		return r.Events[0]
	}
	require.Empty(t, feed(0, []byte("\x00\x01minimum\x00octet\x00blksize\x001024\x00")).Error)
	require.Empty(t, feed(1, []byte("\x00\x06blksize\x00512\x00")).Error)
	require.Empty(t, feed(0, []byte{0, 4, 0, 0}).Error)
	denied := feed(1, append([]byte{0, 3, 0, 1}, make([]byte, 129)...))
	require.Equal(t, "limited", denied.Status)
	late := feed(1, []byte{0, 3, 0, 1, 42})
	require.NotEmpty(t, late.Error, "unobserved refused DATA cannot become a fresh verified block with changed bytes")
	require.Nil(t, late.Session)
	require.Empty(t, s.Close("done"))
	require.Zero(t, s.Stats().BufferedBytes)
}
