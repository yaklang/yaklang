package pcaputil

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIMAPSASLAssociationFragmentationAndReuse(t *testing.T) {
	value := []byte("\x00dummy\x00secret")
	encoded := base64.StdEncoding.EncodeToString(value)
	steps := []sessionStep{
		{1, mailCR("* OK IMAP4rev1 ready")}, {0, mailCR("a AUTHENTICATE PLAIN")},
		{1, mailCR("+ ")}, {0, mailCR(encoded)}, {1, mailCR("a OK authenticated")},
		{0, mailCR("a NOOP")}, {1, mailCR("a OK noop")},
		{0, mailCR("a AUTHENTICATE PLAIN =")}, {1, mailCR("a NO rejected")},
		{0, mailCR("a AUTHENTICATE PLAIN")}, {1, mailCR("+ ")}, {0, mailCR("*")}, {1, mailCR("a BAD canceled")},
		{0, mailCR("a NOOP")}, {1, mailCR("a OK reused")},
	}
	for _, chunk := range []int{0, 1, 7} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/%v", chunk, deferred), func(t *testing.T) {
				events, flow := sessionTestFlow(t, "imap", steps, chunk, deferred)
				require.Len(t, events, len(steps))
				assertSessionEvents(t, events, "imap", deferred)
				require.Equal(t, events[1].ID, events[2].ResponseTo)
				require.Equal(t, events[2].ID, events[3].ResponseTo)
				require.Equal(t, events[1].ID, events[3].TransactionID)
				require.Equal(t, events[1].ID, events[4].ResponseTo)
				require.Equal(t, value, events[3].Session["Transport Value"].(map[string]any)["Decoded Payload"])
				require.Equal(t, "OK", events[4].Session["Server Result Observed"])
				require.Equal(t, false, events[4].Session["Authentication Verified"])
				require.Equal(t, "cancel", events[11].Session["SASL Stage"])
				require.Nil(t, flow.imap, "closed connection releases authentication state")
				for _, e := range []*ProtocolEvent{events[1], events[2], events[3], events[4]} {
					want, err := e.GetFields()
					require.NoError(t, err)
					e.Protocol, e.Status = "tls", "decoded"
					e.Fields, e.Structured, e.Session = map[string]any{"Forged": true}, map[string]any{"fields": map[string]any{"Forged": true}}, map[string]any{"Authentication Verified": true}
					clear(e.Raw)
					got, err := e.GetFields()
					require.NoError(t, err)
					require.Equal(t, want, got)
					decoded, err := e.Decode()
					require.NoError(t, err)
					require.Equal(t, want, decoded["fields"])
					require.NotContains(t, e.DisplayFields()["imap.authentication.verified"], true)
				}
			})
		}
	}
}

func TestIMAPSASLInvalidValueRecoveryAndIsolation(t *testing.T) {
	steps := []sessionStep{
		{1, mailCR("* OK IMAP4rev1 ready")}, {0, mailCR("a AUTHENTICATE PLAIN")}, {1, mailCR("+ ")},
		{0, mailCR("%%%invalid")}, {1, mailCR("a BAD invalid base64")},
		{0, mailCR("a AUTHENTICATE PLAIN")}, {1, mailCR("+ ")}, {0, mailCR("ZHVtbXk=")}, {1, mailCR("a NO refused")},
		{0, mailCR("a NOOP")}, {1, mailCR("a OK done")},
	}
	for _, deferred := range []bool{false, true} {
		events, _ := sessionTestFlow(t, "imap", steps, 1, deferred)
		require.Len(t, events, len(steps))
		require.Equal(t, "malformed", events[3].Status)
		require.Equal(t, "message", events[3].Session["Error Scope"])
		require.Zero(t, events[3].ResponseTo)
		require.Equal(t, events[5].ID, events[6].ResponseTo)
		assertSessionEvents(t, events[4:], "imap", deferred)
		e := events[3]
		_, expected := e.GetFields()
		require.Error(t, expected)
		e.Protocol, e.Status = "imap", "decoded"
		e.Fields, e.Structured, e.Session = map[string]any{"Accepted": true}, map[string]any{"fields": map[string]any{"Accepted": true}}, map[string]any{"Authentication Verified": true}
		fields, err := e.GetFields()
		require.Nil(t, fields)
		require.EqualError(t, err, expected.Error())
		_, err = e.Decode()
		require.EqualError(t, err, expected.Error())
	}
	for _, raw := range []string{"ZHVtbXk=\r\n", "*\r\n", "\r\n"} {
		s := &binIMAP{}
		_, err := s.consume(0, []byte(raw))
		require.Error(t, err, "no authentication context may be invented")
		require.Nil(t, s.sasl)
	}
	s := &binIMAP{}
	_, err := s.consume(0, mailCR("a AUTHENTICATE PLAIN"))
	require.NoError(t, err)
	_, err = s.consume(0, mailCR("ZHVtbXk="))
	require.Error(t, err, "no challenge")
	require.True(t, s.sasl.quarantined)
	_, err = s.consume(1, mailCR("a BAD retry"))
	require.NoError(t, err)
	_, err = s.consume(0, mailCR("a AUTHENTICATE PLAIN"))
	require.NoError(t, err)
	_, err = s.consume(1, mailCR("+ "))
	require.NoError(t, err)
	_, err = s.consume(1, mailCR("+ "))
	require.Error(t, err, "overlapping challenge")
	_, err = s.consume(0, mailCR("ZHVtbXk="))
	require.Error(t, err, "quarantined exchange")
}

func TestIMAPSASLUnknownSecurityLayerAndInitialOffsets(t *testing.T) {
	s := &binIMAP{}
	command := []byte("a AUTHENTICATE PLAIN ZHVtbXk=\r\n")
	out, err := s.consume(0, command)
	require.NoError(t, err)
	initial := out["Initial Response"].(map[string]any)
	span := initial["Wire Field Ranges"].(map[string]any)["Encoded Value"].([]uint64)
	require.Equal(t, []byte("ZHVtbXk="), command[span[0]:span[1]])
	_, err = s.consume(1, mailCR("a NO retry"))
	require.NoError(t, err)
	_, err = s.consume(0, mailCR("a AUTHENTICATE GSSAPI ="))
	require.NoError(t, err)
	out, err = s.consume(1, mailCR("a OK accepted"))
	require.NoError(t, err)
	require.Equal(t, "unknown-mechanism-security-layer", out["Subsequent Protection"])
	out, err = s.consume(0, mailCR("b NOOP"))
	require.ErrorContains(t, err, "security layer")
	require.NotContains(t, out, "Command")
	for _, bad := range []string{"a AUTHENTICATE", "a AUTHENTICATE PLAIN *", "a AUTHENTICATE PLAIN A===", "a AUTHENTICATE PLAIN  =", "a AUTHENTICATE bad"} {
		s := &binIMAP{}
		_, err := s.consume(0, mailCR(bad))
		require.Error(t, err, bad)
		require.Empty(t, s.pending)
		require.Nil(t, s.sasl)
	}
}

func FuzzIMAPSASLNativeExchange(f *testing.F) {
	f.Add([]byte("ZHVtbXk=\r\n"), uint8(0))
	f.Add([]byte("*\r\n"), uint8(1))
	f.Fuzz(func(t *testing.T, raw []byte, choice uint8) {
		if len(raw) > imapSASLLineMax || bytes.Contains(raw, []byte("\r\n")) && !bytes.Equal(bytes.TrimSuffix(raw, []byte("\r\n")), bytes.Split(raw, []byte("\r\n"))[0]) {
			return
		}
		s := &binIMAP{}
		client := int(choice & 1)
		_, err := s.consume(client, mailCR("a AUTHENTICATE PLAIN"))
		require.NoError(t, err)
		_, err = s.consume(1-client, mailCR("+ "))
		require.NoError(t, err)
		out, err := s.consume(client, raw)
		if err == nil {
			require.Equal(t, false, out["Authentication Verified"])
		}
		_, err = s.consume(1-client, mailCR("a BAD retry"))
		require.NoError(t, err)
		require.Nil(t, s.sasl)
		require.Empty(t, s.pending)
		require.Zero(t, s.pendingBytes)
		_, err = s.consume(client, mailCR("a NOOP"))
		require.NoError(t, err)
		_, err = s.consume(1-client, mailCR("a OK done"))
		require.NoError(t, err)
	})
}
