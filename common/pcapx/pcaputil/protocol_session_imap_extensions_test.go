package pcaputil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIMAPNamespaceAndIdleObservedLifecycle(t *testing.T) {
	steps := []sessionStep{
		{1, mailCR("* OK IMAP4rev1 ready")},
		{0, mailCR("n NAMESPACE")}, {1, mailCR("* NAMESPACE ((\"\" \"/\")) NIL NIL")}, {1, mailCR("n OK namespace")},
		{0, mailCR("a IDLE")}, {1, mailCR("* 3 EXISTS")}, {1, mailCR("+ idling")}, {1, mailCR("* 4 EXISTS")},
		{0, mailCR("DONE")}, {1, mailCR("a OK idle complete")},
		{0, mailCR("a NOOP")}, {1, mailCR("a OK reused")},
		{0, mailCR("a IDLE")}, {1, mailCR("a NO unavailable")},
	}
	for _, chunk := range []int{0, 1, 7} {
		for _, deferred := range []bool{false, true} {
			events, _ := sessionTestFlow(t, "imap", steps, chunk, deferred)
			require.Len(t, events, len(steps))
			assertSessionEvents(t, events, "imap", deferred)
			require.Equal(t, events[1].ID, events[2].ResponseTo)
			require.Equal(t, events[1].ID, events[3].ResponseTo)
			require.Equal(t, events[4].ID, events[6].ResponseTo)
			require.Equal(t, events[6].ID, events[8].ResponseTo)
			require.Equal(t, events[4].ID, events[8].TransactionID)
			require.Equal(t, events[4].ID, events[9].ResponseTo)
			require.Zero(t, events[5].ResponseTo, "unsolicited update is not a tagged transaction response")
			require.Equal(t, events[12].ID, events[13].ResponseTo)
			require.Equal(t, "done", events[8].Session["IDLE Stage"])
			info := events[2].Session["IMAP Fields"].(map[string]any)
			require.Equal(t, false, info["Mailbox Names Decoded"])
			for _, e := range []*ProtocolEvent{events[1], events[2], events[8], events[9]} {
				want, err := e.GetFields()
				require.NoError(t, err)
				e.Protocol, e.Fields, e.Session = "tls", map[string]any{"Forged": true}, map[string]any{"Forged": true}
				clear(e.Raw)
				got, err := e.GetFields()
				require.NoError(t, err)
				require.Equal(t, want, got)
			}
		}
	}
}

func TestIMAPNamespaceAmbiguityAndIdleRefusal(t *testing.T) {
	steps := []sessionStep{{1, mailCR("* OK IMAP4rev1 ready")}, {0, mailCR("n NAMESPACE\r\nm NAMESPACE")}, {1, mailCR("* NAMESPACE NIL NIL NIL")}, {1, mailCR("n OK done")}, {1, mailCR("* NAMESPACE NIL NIL NIL")}, {1, mailCR("m OK done")}}
	events, _ := sessionTestFlow(t, "imap", steps, 7, false)
	require.Len(t, events, 7)
	assertSessionEvents(t, events, "imap", false)
	require.Zero(t, events[3].ResponseTo)
	require.Equal(t, "ambiguous-outstanding-namespace", events[3].Session["Association"])
	require.Equal(t, events[2].ID, events[5].ResponseTo)
	for _, bad := range []string{"DONE", "a NOOP"} {
		s := &binIMAP{}
		_, err := s.consume(0, mailCR("a IDLE"))
		require.NoError(t, err)
		_, err = s.consume(0, mailCR(bad))
		require.Error(t, err)
		require.True(t, s.idle.quarantined)
		_, err = s.consume(1, mailCR("a BAD retry"))
		require.NoError(t, err)
		require.Nil(t, s.idle)
		require.Empty(t, s.pending)
	}
	s := &binIMAP{}
	_, err := s.consume(0, mailCR("a IDLE"))
	require.NoError(t, err)
	_, err = s.consume(1, mailCR("+ idling"))
	require.NoError(t, err)
	_, err = s.consume(1, mailCR("a OK too early"))
	require.Error(t, err)
	require.NotNil(t, s.idle)
	_, err = s.consume(1, mailCR("a BAD recovery"))
	require.NoError(t, err)
	_, err = s.consume(0, mailCR("a AUTHENTICATE PLAIN"))
	require.NoError(t, err)
	_, err = s.consume(0, mailCR("b IDLE"))
	require.Error(t, err, "IDLE cannot steal SASL response context")
	require.Nil(t, s.idle)
	_, err = (&binIMAP{}).consume(0, mailCR("DONE"))
	require.Error(t, err)
	_, err = (&binIMAP{}).consume(0, mailCR("a] AUTHENTICATE PLAIN"))
	require.NoError(t, err, "IMAP tag permits ]")
	role, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionClientDirection(0))
	require.NoError(t, err)
	defer role.Close("test")
	require.Nil(t, role.Feed(1, time.Unix(1, 0), mailCR("* OK IMAP4rev1 ready")).Err)
	r := role.Feed(1, time.Unix(1, 0), mailCR("a IDLE"))
	require.NotNil(t, r.Err)
}

func TestIMAPIdleMissingPeerBytesIsContextAndLateDeliveryRecovers(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionClientDirection(0))
	require.NoError(t, err)
	defer s.Close("test")
	native := s.(*captureSession)
	ts := time.Unix(1, 0)
	require.Nil(t, s.Feed(1, ts, mailCR("* OK IMAP4rev1 ready")).Err)
	command := s.Feed(0, ts, mailCR("a IDLE"))
	require.Nil(t, command.Err)
	native.f.plaintextGap[1] = true
	missing := s.Feed(0, ts, mailCR("DONE"))
	require.NotNil(t, missing.Err)
	require.Equal(t, ErrContextRequired, missing.Err.Kind)
	require.Len(t, missing.Events, 1)
	require.Equal(t, "context-required", missing.Events[0].Status)
	require.Zero(t, missing.Events[0].ResponseTo)
	originalSession := cloneSession(missing.Events[0].Session)
	late := s.Feed(1, ts, mailCR("+ idling"))
	require.Nil(t, late.Err)
	require.Equal(t, command.Events[0].ID, late.Events[0].ResponseTo)
	native.f.plaintextGap[1] = false
	completion := s.Feed(1, ts, mailCR("a OK idle complete"))
	require.Nil(t, completion.Err)
	require.Equal(t, command.Events[0].ID, completion.Events[0].ResponseTo)
	require.Equal(t, originalSession, missing.Events[0].Session, "late evidence never rewrites a published observation")
	require.Nil(t, s.Feed(0, ts, mailCR("a NOOP")).Err)
	require.Nil(t, s.Feed(1, ts, mailCR("a OK reused")).Err)
}
