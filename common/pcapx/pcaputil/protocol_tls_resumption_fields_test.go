package pcaputil

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func tlsResumptionRecord(body []byte) []byte {
	b := []byte{22, 3, 3, 0, 0}
	binary.BigEndian.PutUint16(b[3:], uint16(len(body)))
	return append(b, body...)
}

func TestTLSResumptionDisplayFieldsDistinguishPDUAndContext(t *testing.T) {
	id := []byte("identity-never-exported-as-bytes")
	offer := tlsResumptionPSKs([][]byte{id}, []uint32{42}, []int{32})
	state := newBinTLS()
	ch, err := state.handshakeMessage(0, reliabilityHS(1, tlsResumptionPSKClient(nil, offer, 0)), false)
	require.NoError(t, err)
	sh, err := state.handshakeMessage(1, reliabilityHS(2, tlsResumptionPSKServer(nil, 0)), false)
	require.NoError(t, err)
	for _, protocol := range []string{"tls", "quic"} {
		t.Run(protocol, func(t *testing.T) {
			key := "Handshake Messages"
			if protocol == "quic" {
				key = "TLS Handshakes"
			}
			e := &ProtocolEvent{Protocol: protocol, Session: map[string]any{key: []map[string]any{ch}}}
			values := e.DisplayFields()
			require.Equal(t, []any{uint64(0)}, values["tls.handshake.session_id_length"])
			require.Equal(t, []any{uint64(len(id))}, values["tls.handshake.extensions.psk.identity.identity_length"])
			require.Equal(t, []any{uint64(42)}, values["tls.handshake.extensions.psk.identity.obfuscated_ticket_age"])
			require.Equal(t, []any{uint64(32)}, values["tls.handshake.extensions.psk.binder.binder_length"])
			require.Equal(t, []any{uint64(1)}, values["tls.extension.psk_ke_modes_length"])
			require.Equal(t, []any{uint64(0)}, values["tls.extension.psk_ke_mode"])
			require.NotContains(t, values, "tls.session.psk.selected")
			e.Session = map[string]any{key: []map[string]any{sh}}
			state.resumption.project(e.Session)
			values = e.DisplayFields()
			require.Equal(t, []any{uint64(0)}, values["tls.handshake.extensions.psk.identity.selected"])
			require.Equal(t, []any{true}, values["tls.session.psk.selected"])
			require.Equal(t, []any{false}, values["tls.session.psk.binder_verified"])
			require.NotContains(t, values, "tls.session.resumption_indicated", "TLS 1.3 compatibility echoes are not session-ID resumption")
			for _, field := range e.DisplayFieldValues().Values {
				if field.Name == "tls.handshake.extensions.psk.identity.selected" {
					require.Equal(t, "pdu", field.Origin)
					require.Nil(t, field.Range, "logical message has no invented captured-record offset")
				}
				if field.Name == "tls.session.psk.selected_identity" {
					require.Equal(t, "context", field.Origin)
					require.Nil(t, field.Range)
				}
			}
			e.Session = map[string]any{}
			state.resumption.project(e.Session)
			values = e.DisplayFields()
			require.NotContains(t, values, "tls.handshake.extensions.psk.identity.selected", "later records cannot pretend to contain another ServerHello")
			require.Equal(t, []any{uint64(0)}, values["tls.session.psk.selected_identity"])
		})
	}
	for _, raw := range []string{"tls.handshake.session_ticket", "tls.handshake.extensions.psk.identity.identity", "tls.handshake.extensions.psk.binder.binder"} {
		_, exists := LookupDisplayField(raw)
		require.False(t, exists, "ticket/PSK/binder bytes are deliberately not projected")
	}
}

func TestTLSResumptionNativeFieldsSurviveDeferredOwnershipAndConnectionReuse(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 443))
		require.NoError(t, err)
		defer s.Close("cleanup")
		s.(*captureSession).f.a.config.Deferred = deferred
		offer := tlsResumptionPSKs([][]byte{[]byte("ticket")}, []uint32{3}, []int{32})
		ch := s.Feed(0, time.Unix(10, 0), tlsResumptionRecord(reliabilityHS(1, tlsResumptionPSKClient(nil, offer, 0))))
		require.Nil(t, ch.Err)
		require.Len(t, ch.Events, 1)
		sh := s.Feed(1, time.Unix(11, 0), tlsResumptionRecord(reliabilityHS(2, tlsResumptionPSKServer(nil, 0))))
		require.Nil(t, sh.Err)
		require.Len(t, sh.Events, 1)
		e := sh.Events[0]
		require.Equal(t, "tls", e.Protocol)
		status := "decoded"
		if deferred {
			status = "deferred"
		}
		require.Equal(t, status, e.Status)
		require.Equal(t, true, e.Session["Session Establishment"].(map[string]any)["PSK Selected"])
		fields, err := e.GetFields()
		require.NoError(t, err)
		require.Equal(t, e.Session, fields)
		decoded, err := e.Decode()
		require.NoError(t, err)
		require.Equal(t, fields, decoded["fields"])
		require.Equal(t, e.Session, decoded["session"])
		before := e.DisplayFields()
		require.Equal(t, []any{true}, before["tls.session.psk.selected"])
		require.Equal(t, []any{uint64(0)}, before["tls.handshake.session_id_length"])
		tlsDTLSPoison(e, "caller")
		// Status is editable event metadata. Hold it fixed while testing
		// that public field/protocol/raw edits cannot rewrite TLS authority.
		e.Status = status
		require.Equal(t, before, e.DisplayFields(), "public map/protocol/raw mutations cannot rewrite accepted metadata")
		afterFields, err := e.GetFields()
		require.NoError(t, err)
		require.Equal(t, fields, afterFields)
		decoded, err = e.Decode()
		require.NoError(t, err)
		require.Equal(t, fields, decoded["fields"])
		require.Equal(t, fields, decoded["session"])
		// A distinct native session on the same endpoints has no inherited
		// ClientHello, even if the wire ServerHello selects identity zero.
		fresh, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 443))
		require.NoError(t, err)
		defer fresh.Close("cleanup")
		o := fresh.Feed(1, time.Unix(12, 0), tlsResumptionRecord(reliabilityHS(2, tlsResumptionPSKServer(nil, 0))))
		require.Nil(t, o.Err)
		require.Len(t, o.Events, 1)
		require.Equal(t, []any{"unavailable-client-hello"}, o.Events[0].DisplayFields()["tls.session.psk.offer_match"])
		require.NotContains(t, o.Events[0].DisplayFields(), "tls.session.psk.identity_sha256")
	}
}

func TestTLSResumptionTicketFieldAliasesPreserveWireAndPolicyValues(t *testing.T) {
	s := newBinTLS()
	s.version, s.client, s.app[1] = 0x304, 0, true
	m, err := s.handshakeMessage(1, reliabilityHS(4, tlsResumptionNST13(0xffffffff, nil, []byte("ticket"), tlsResumptionExt{42, []byte{0, 0, 1, 0}})), true, 8)
	require.NoError(t, err)
	e := &ProtocolEvent{Protocol: "tls", Session: map[string]any{"Handshake Messages": []map[string]any{m}}}
	f := e.DisplayFields()
	require.Equal(t, []any{uint64(0xffffffff)}, f["tls.handshake.session_ticket_lifetime_hint"])
	require.Equal(t, []any{uint64(604800)}, f["tls.handshake.session_ticket.max_client_cache_seconds"])
	require.Equal(t, []any{false}, f["tls.handshake.session_ticket.lifetime_within_server_limit"])
	require.Equal(t, []any{uint64(0)}, f["tls.handshake.session_ticket_nonce_length"])
	require.Equal(t, []any{uint64(6)}, f["tls.handshake.session_ticket_length"])
	require.Equal(t, []any{uint64(256)}, f["tls.handshake.session_ticket.max_early_data_size"])
	require.Equal(t, []any{true}, f["tls.handshake.session_ticket.authenticated"])
	require.NotContains(t, f, "tls.session.psk.selected")
}
