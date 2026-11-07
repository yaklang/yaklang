package pcaputil

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func smb1EmptyErrorReply(command byte, status uint32) []byte {
	w := make([]byte, 39)
	copy(w[4:], []byte{0xff, 'S', 'M', 'B', command})
	binary.LittleEndian.PutUint32(w[9:13], status)
	w[13] = 0x80
	binary.LittleEndian.PutUint16(w[14:16], 0xc803)
	w[36] = 0
	binary.BigEndian.PutUint32(w[:4], uint32(len(w)-4))
	return w
}

func smb1UnselectedReadAndX() []byte {
	w := make([]byte, 39+20)
	copy(w[4:], []byte{0xff, 'S', 'M', 'B', 0x2e})
	binary.LittleEndian.PutUint16(w[14:16], 0xc803)
	w[36] = 10
	binary.BigEndian.PutUint32(w[:4], uint32(len(w)-4))
	return w
}

// A failed SESSION_SETUP_ANDX is the MS-CIFS WC0/BC0 error layout, not a
// malformed handshake. It must be observable without stopping the stream.
func TestSMB1SetupEmptyErrorKeepsFlowAlive(t *testing.T) {
	session := smb1SetupSession(t)
	smb1FeedComplete(t, session, 0, smb1ExtendedOffer())
	r := session.Feed(1, time.Unix(2, 0), smb1EmptyErrorReply(0x73, 0xc000006d))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	event := r.Events[0]
	require.Equal(t, "cifs-error-layout", event.Profile)
	require.Equal(t, "message", event.Session["Error Scope"])
	require.Equal(t, true, event.Session["Error Layout"])
	require.Equal(t, "SMB_COM_SESSION_SETUP_ANDX", event.Session["Packet Name"])

	next := session.Feed(0, time.Unix(3, 0), smb1UnselectedReadAndX())
	require.Nil(t, next.Err)
	require.Len(t, next.Events, 1)
	require.Equal(t, "cifs-unselected-command", next.Events[0].Profile)
}

func TestSMB1NegotiateEmptyErrorKeepsFlowAlive(t *testing.T) {
	session := smb1SetupSession(t)
	smb1FeedComplete(t, session, 0, smb1ExtendedOffer())
	r := session.Feed(1, time.Unix(2, 0), smb1EmptyErrorReply(0x72, 0xc00000bb))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	require.Equal(t, "cifs-error-layout", r.Events[0].Profile)
	require.Equal(t, "SMB_COM_NEGOTIATE", r.Events[0].Session["Packet Name"])
}

func TestSMB1UnselectedCommandIsFramedAndObservable(t *testing.T) {
	session := smb1SetupSession(t)
	smb1FeedComplete(t, session, 0, smb1ExtendedOffer())
	r := session.Feed(0, time.Unix(2, 0), smb1UnselectedReadAndX())
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	event := r.Events[0]
	require.Equal(t, "cifs-unselected-command", event.Profile)
	require.Equal(t, "SMB_COM_READ_ANDX", event.Session["Command Name"])
	require.Equal(t, byte(10), event.Session["Body"].(map[string]any)["Word Count"])
	require.Equal(t, true, event.Session["Unselected Command"])
	require.NotContains(t, event.Session, "Matched Request")

	again := session.Feed(0, time.Unix(3, 0), smb1UnselectedReadAndX())
	require.Nil(t, again.Err)
	require.Len(t, again.Events, 1)
	require.Equal(t, "cifs-unselected-command", again.Events[0].Profile)
}
