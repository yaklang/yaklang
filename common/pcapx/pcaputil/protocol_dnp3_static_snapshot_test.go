package pcaputil

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDNP3StaticSnapshotDuplicateExistingAPI(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionDNP3TypedStaticReads(true))
	require.NoError(t, err)
	out := s.Feed(0, time.Unix(1, 0), dnp3Link(0xc4, 2, 3, []byte{0xc0, 0xc8, 1, 30, 0, 6}))
	require.Nil(t, out.Err)
	body := []byte{30, 1, 0, 7, 7, 1, 1, 0, 0, 0, 30, 2, 0, 7, 7, 1, 2, 0}
	out = s.Feed(1, time.Unix(2, 0), dnp3Link(0x44, 3, 2, append([]byte{0xc0, 0xc8, 129, 0, 0}, body...)))
	rocTypedError(t, "ContextRequired", out.Err)
	require.Len(t, out.Events, 1)
	require.NotContains(t, out.Events[0].Session, "In Reply To")
	out = s.Feed(1, time.Unix(3, 0), dnp3Link(0x44, 3, 2, []byte{0xc0, 0xc8, 129, 0, 0, 30, 1, 0, 7, 7, 1, 1, 0, 0, 0}))
	require.Nil(t, out.Err)
	require.Len(t, out.Events, 1)
	require.Equal(t, "READ", out.Events[0].Session["In Reply To"])
	s.Close("end")
	require.Zero(t, s.Stats().BufferedBytes)
}
