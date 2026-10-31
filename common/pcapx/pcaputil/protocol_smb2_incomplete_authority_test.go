package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Public-session regressions use generated SMB2 records and retain separate
// authorities for complete accepted fields and non-message close diagnostics.
func smb2Diagnostic120Feed(t *testing.T, session ProtocolSession, dir int, wire []byte) *ProtocolEvent {
	t.Helper()
	r := session.Feed(dir, time.Unix(120, int64(dir)), wire)
	require.Equal(t, len(wire), r.Consumed)
	require.Nil(t, r.Err)
	require.False(t, r.NeedMore)
	require.Len(t, r.Events, 1)
	e := r.Events[0]
	require.Equal(t, "smb2", e.Protocol)
	require.Equal(t, "decoded", e.Status)
	require.Equal(t, wire, e.Raw)
	return e
}

func smb2Diagnostic120Poison(e *ProtocolEvent) {
	poison := map[string]any{"accepted": true, "Authentication Verified": true, "Header": map[string]any{"Message ID": uint64(999)}}
	e.Fields, e.Session, e.Structured = poison, poison, map[string]any{"fields": poison, "metadata": poison}
	e.Metadata = poison
	e.Protocol, e.Profile, e.Completeness = "tcp", "poisoned", "message"
	e.Status, e.Error, e.Summary = "decoded", "", "accepted by caller"
	e.Rule, e.Entry = "application-layer.smb2", "SMB2"
	e.Raw = bytes.Clone(smb2NegotiateReq(0x0202))
	e.Length = len(e.Raw)
}

func smb2Diagnostic120AcceptedAuthority(t *testing.T, e *ProtocolEvent) {
	t.Helper()
	fields, err := e.GetFields()
	require.NoError(t, err)
	require.NotNil(t, fields)
	header := fields["Header"].(map[string]any)
	require.Equal(t, uint32(0x424d53fe), header["Protocol ID"])
	require.Equal(t, uint16(0), header["Command"])
	require.Equal(t, uint64(1), header["Message ID"])
	before, err := json.Marshal(fields)
	require.NoError(t, err)
	beforeSession, err := json.Marshal(e.Session)
	require.NoError(t, err)
	// Also alter the already returned tree before replacing all public caches.
	header["Message ID"] = uint64(888)
	e.Fields["Header"].(map[string]any)["Message ID"] = uint64(777)
	smb2Diagnostic120Poison(e)
	fresh, err := e.GetFields()
	require.NoError(t, err, "a complete admitted native request retains its private fields")
	after, err := json.Marshal(fresh)
	require.NoError(t, err)
	require.Equal(t, before, after)
	decoded, err := e.Decode()
	require.NoError(t, err)
	after, err = json.Marshal(decoded["fields"])
	require.NoError(t, err)
	require.Equal(t, before, after, "Decode and GetFields must use the same private accepted tree")
	afterSession, err := json.Marshal(decoded["session"])
	require.NoError(t, err)
	require.Equal(t, beforeSession, afterSession, "public Session edits cannot replace captured context")
}

func smb2Diagnostic120IncompleteAuthority(t *testing.T, e *ProtocolEvent) {
	t.Helper()
	require.Equal(t, "smb2", e.Protocol)
	require.Equal(t, "incomplete", e.Status)
	const refusal = "protocol parser: event is incomplete, not an exact message"
	fields, err := e.GetFields()
	require.Nil(t, fields)
	require.EqualError(t, err, refusal)
	smb2Diagnostic120Poison(e)
	fields, err = e.GetFields()
	require.Nil(t, fields)
	require.EqualError(t, err, refusal, "public success projections cannot promote a close diagnostic")
	decoded, err := e.Decode()
	require.Nil(t, decoded)
	require.EqualError(t, err, refusal)
}

func TestSMB2TrailingIncompleteCannotPromoteAcceptedMessageStillOwnsFields(t *testing.T) {
	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	t.Cleanup(func() { session.Close("FIN"); require.Zero(t, session.Stats().BufferedBytes) })
	request := smb2Diagnostic120Feed(t, session, 0, smb2NegotiateReq(0x0202))
	smb2Diagnostic120AcceptedAuthority(t, request)
	// Resolve the first exchange so Close has only the real trailing record.
	smb2Diagnostic120Feed(t, session, 1, smb2NegotiateResp(0x0202))
	next := smb2QueryRequest(21, 17, 7, 1, 7, nil)
	tail := bytes.Clone(next[:19]) // Complete transport prefix, incomplete SMB2 fixed header.
	r := session.Feed(0, time.Unix(121, 0), tail)
	require.Equal(t, len(tail), r.Consumed)
	require.True(t, r.NeedMore)
	if r.Err != nil {
		require.Equal(t, ErrNeedMore, r.Err.Kind)
	}
	require.Empty(t, r.Events)
	closed := session.Close("FIN")
	require.Zero(t, session.Stats().BufferedBytes)
	require.Len(t, closed, 1)
	require.Equal(t, tail, closed[0].Raw, "do not substitute a complete fixture for retained partial bytes")
	smb2Diagnostic120IncompleteAuthority(t, closed[0])
}

func TestSMB2PendingCloseCannotPromoteAcceptedMessageStillOwnsFields(t *testing.T) {
	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	t.Cleanup(func() { session.Close("FIN"); require.Zero(t, session.Stats().BufferedBytes) })
	request := smb2Diagnostic120Feed(t, session, 0, smb2NegotiateReq(0x0202))
	smb2Diagnostic120AcceptedAuthority(t, request)
	closed := session.Close("FIN")
	require.Zero(t, session.Stats().BufferedBytes)
	require.Len(t, closed, 1)
	diagnostic := closed[0]
	require.Empty(t, diagnostic.Raw, "pending-close metadata is not a wire message")
	require.Contains(t, diagnostic.Summary, "unmatched MessageIds")
	require.Equal(t, 1, diagnostic.Session["Outstanding"])
	smb2Diagnostic120IncompleteAuthority(t, diagnostic)
}

func TestSMB2NativeRefusalsCannotFallBackToPublicCaches(t *testing.T) {
	for _, name := range []string{"invalid-next-command", "invalid-framing", "resource-limit"} {
		t.Run(name, func(t *testing.T) {
			budget := DefaultParserBudget()
			if name == "resource-limit" {
				budget.MaxMessageBytes = 128
			}
			s, err := NewProtocolSession(budget)
			require.NoError(t, err)
			t.Cleanup(func() { s.Close("FIN"); require.Zero(t, s.Stats().BufferedBytes) })
			smb2Diagnostic120Feed(t, s, 0, smb2NegotiateReq(0x0202))
			wire, direction := smb2NegotiateResp(0x0202), 1
			kind, status := ErrResourceExceeded, "limited"
			if name != "resource-limit" {
				smb2Diagnostic120Feed(t, s, 1, wire)
				wire, direction = smb2NegotiateReq(0x0202), 0
				kind, status = ErrMalformedMessage, "malformed"
				if name == "invalid-next-command" {
					binary.LittleEndian.PutUint32(wire[24:28], 8)
				} else {
					wire[0] = 1
				}
			}
			r := s.Feed(direction, time.Unix(125, 0), wire)
			require.Equal(t, len(wire), r.Consumed)
			require.Len(t, r.Events, 1)
			e := r.Events[0]
			require.Equal(t, "smb2", e.Protocol)
			require.Equal(t, status, e.Status)
			fields, err := e.GetFields()
			require.Error(t, err, "a refused native record cannot be reparsed by a weaker grammar")
			require.Nil(t, fields)
			message := err.Error()
			smb2Diagnostic120Poison(e)
			fields, err = e.GetFields()
			require.Nil(t, fields)
			require.EqualError(t, err, message)
			var typed *ProtocolError
			require.ErrorAs(t, err, &typed)
			require.Equal(t, kind, typed.Kind)
			decoded, err := e.Decode()
			require.Nil(t, decoded)
			require.EqualError(t, err, message)
			require.ErrorAs(t, err, &typed)
			require.Equal(t, kind, typed.Kind)
		})
	}
}
