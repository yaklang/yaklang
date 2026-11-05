package pcaputil

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func phaContextTestFlow(t *testing.T, suite uint16, server int, offer string) (*binFlow, *binTLS) {
	t.Helper()
	flow, state := boundaryTLSFlow(t, suite)
	state.client = 1 - server
	if offer != "unknown" {
		extensions := append([]tlsResumptionExt{{43, []byte{2, 3, 4}}}, tlsResumptionDHExtensions(true)...)
		if offer == "offered" {
			extensions = append(extensions, tlsResumptionExt{49, nil})
		}
		state.client = -1
		body := tlsResumptionHello(true, nil, suite, extensions...)
		_, err := state.handshakeMessage(1-server, reliabilityHS(1, body), false)
		require.NoError(t, err, "offer knowledge comes from an accepted complete ClientHello")
		require.NotEmpty(t, state.firstHello)
		_, err = state.handshakeMessage(server, reliabilityHS(2, tlsResumptionHello(false, nil, suite, append([]tlsResumptionExt{{43, []byte{3, 4}}}, tlsResumptionDHExtensions(false)...)...)), false)
		require.NoError(t, err)
		require.Empty(t, state.firstHello, "final ServerHello releases retry body storage; offer knowledge must survive")
	}
	return flow, state
}

func phaContextTestRequest(context []byte) []byte {
	return certificateRequest13(context, certificateRequestExtension(13, []byte{0, 2, 8, 4}))
}

func TestTLS13CertificateRequestOfferAndContextHistory(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, server := range []int{0, 1} {
			for _, mode := range []string{"offered", "not-offered", "unknown", "duplicate", "after-keyupdate", "initial-then-empty", "first-application-empty", "new-connection"} {
				t.Run(fmt.Sprintf("suite%04x/server%d/%s", suite, server, mode), func(t *testing.T) {
					offer := "offered"
					if mode == "not-offered" || mode == "unknown" {
						offer = mode
					}
					flow, state := phaContextTestFlow(t, suite, server, offer)
					context := []byte{0xa5, 0x1b}
					if mode == "first-application-empty" || mode == "initial-then-empty" {
						context = nil // RFC8446 4.3.2 does not require nonempty PHA context.
					}
					if mode == "initial-then-empty" {
						state.app[server] = false
					}
					encoded := phaContextTestRequest(context)
					first := &ProtocolEvent{Protocol: "tls", Raw: boundaryTLSRecord(t, state, server, 2, encoded, 22, 0)}
					_, err := flow.consumeTLS(server, first)
					if mode == "not-offered" {
						var typed *ProtocolError
						require.ErrorAs(t, err, &typed)
						require.Equal(t, ErrMalformedMessage, typed.Kind)
						require.Contains(t, typed.Error(), "post-handshake authentication not offered")
						require.NotContains(t, first.Session, "Handshake Messages")
						require.Equal(t, true, first.Session["Authentication Verified"])
						return
					}
					require.NoError(t, err)
					messages := first.Session["Handshake Messages"].([]map[string]any)
					require.Len(t, messages, 1)
					if mode != "initial-then-empty" {
						require.Contains(t, messages[0], "Certificate Request Admission")
						admission := messages[0]["Certificate Request Admission"].(map[string]any)
						require.Equal(t, offer, admission["Client PHA Offer"])
						require.Equal(t, "unique among observed requests", admission["Context Uniqueness"])
						require.Equal(t, uint64(1), admission["Observed Request Count"])
					}
					if mode == "offered" || mode == "unknown" || mode == "first-application-empty" {
						return
					}
					if mode == "new-connection" {
						other, next := phaContextTestFlow(t, suite, server, "offered")
						event := &ProtocolEvent{Protocol: "tls", Raw: boundaryTLSRecord(t, next, server, 2, encoded, 22, 0)}
						_, err = other.consumeTLS(server, event)
						require.NoError(t, err, "a context is unique within one connection, not across all connections")
						m := event.Session["Handshake Messages"].([]map[string]any)[0]
						require.Equal(t, uint64(1), m["Certificate Request Admission"].(map[string]any)["Observed Request Count"])
						return
					}
					// Editing the public body or owned encoding cannot retire a
					// private observed context and make a later reuse admissible.
					body := messages[0]["Certificate Request"].(map[string]any)
					for i := range body["Context"].([]byte) {
						body["Context"].([]byte)[i] ^= 0xff
					}
					clear(messages[0]["Encoded Certificate Request"].([]byte))
					sequence := uint64(3)
					if mode == "after-keyupdate" {
						update := &ProtocolEvent{Protocol: "tls", Raw: boundaryTLSRecord(t, state, server, sequence, []byte{24, 0, 0, 1, 0}, 22, 0)}
						_, err = flow.consumeTLS(server, update)
						require.NoError(t, err)
						sequence = 0
					}
					state.app[server] = true
					secret, iv, key := bytes.Clone(state.secret[server]), bytes.Clone(state.iv[server]), state.keys[server]
					second := &ProtocolEvent{Protocol: "tls", Raw: boundaryTLSRecord(t, state, server, sequence, encoded, 22, 0)}
					_, err = flow.consumeTLS(server, second)
					var typed *ProtocolError
					require.ErrorAs(t, err, &typed)
					require.Equal(t, ErrMalformedMessage, typed.Kind)
					require.Contains(t, typed.Error(), "repeated TLS CertificateRequest context")
					require.NotContains(t, second.Session, "Handshake Messages")
					require.Equal(t, true, second.Session["Authentication Verified"])
					require.Equal(t, secret, state.secret[server])
					require.Equal(t, iv, state.iv[server])
					require.Same(t, key, state.keys[server])
					require.Equal(t, sequence+1, state.seq[server])
				})
			}
		}
	}
}

func TestTLS13PHAHelloExtensionGrammar(t *testing.T) {
	for _, c := range []struct {
		name   string
		client bool
		kind   uint16
		data   []byte
		refuse bool
	}{
		{name: "empty-client-offer", client: true, kind: 49},
		{name: "nonempty-client-offer", client: true, kind: 49, data: []byte{1}, refuse: true},
		{name: "server-empty-offer", kind: 49, refuse: true},
		{name: "server-nonempty-offer", kind: 49, data: []byte{1}, refuse: true},
		{name: "opaque-extension", client: true, kind: 0xffa5, data: []byte{1}},
	} {
		t.Run(c.name, func(t *testing.T) {
			body := tlsResumptionHello(c.client, nil, 0x1301, tlsResumptionExt{c.kind, c.data})
			hello, err := tlsHello(body, c.client)
			require.NoError(t, err)
			err = tlsHelloExtensionStructure(hello, c.client)
			if c.refuse {
				var typed *ProtocolError
				require.ErrorAs(t, err, &typed)
				require.Equal(t, ErrMalformedMessage, typed.Kind)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestTLS13PHAHistoryBudgetRefusalIsAtomic(t *testing.T) {
	for _, mode := range []string{"collection", "bytes"} {
		t.Run(mode, func(t *testing.T) {
			flow, state := phaContextTestFlow(t, 0x1301, 1, "offered")
			send := func(context byte) (*ProtocolEvent, error) {
				e := &ProtocolEvent{Protocol: "tls", Raw: boundaryTLSRecord(t, state, 1, state.seq[1], phaContextTestRequest([]byte{context}), 22, 0)}
				_, err := flow.consumeTLS(1, e)
				return e, err
			}
			_, err := send(1)
			require.NoError(t, err)
			next := byte(2)
			if mode == "collection" {
				flow.a.budget.MaxCollectionElements = 2
				_, err = send(2)
				require.NoError(t, err)
				next = 3
			} else {
				// Admit the record buffer but refuse the retained context entry.
				flow.a.config.MaxBufferedBytes = int(flow.a.buffered.Load()) + len(phaContextTestRequest([]byte{next})) + 16
			}
			before := make(map[string]struct{}, len(state.requestContexts))
			for key := range state.requestContexts {
				before[key] = struct{}{}
			}
			sequence := state.seq[1]
			count, storage := len(state.requestContexts), state.requestContextStorage()
			e, err := send(next)
			var typed *ProtocolError
			require.ErrorAs(t, err, &typed)
			require.Equal(t, ErrResourceExceeded, typed.Kind)
			require.Equal(t, true, e.Session["Authentication Verified"])
			require.NotContains(t, e.Session, "Handshake Messages")
			require.Equal(t, sequence+1, state.seq[1])
			require.Equal(t, count, len(state.requestContexts))
			require.Equal(t, storage, state.requestContextStorage())
			require.Equal(t, before, state.requestContexts)
			// A live capture terminates a refused decoder. This direct state
			// test discards the refused bytes explicitly to exercise retry.
			state.handshake[1] = nil
			flow.a.config.MaxBufferedBytes = 16 << 20
			flow.a.budget.MaxCollectionElements = 4096
			e, err = send(next)
			require.NoError(t, err, "refusal must not consume the context")
			admission := e.Session["Handshake Messages"].([]map[string]any)[0]["Certificate Request Admission"].(map[string]any)
			require.Equal(t, uint64(count+1), admission["Observed Request Count"])
		})
	}
}

func TestTLS13PHAFieldsKeepOwnedObservedContext(t *testing.T) {
	flow, state := phaContextTestFlow(t, 0x1301, 1, "offered")
	e := &ProtocolEvent{Protocol: "tls", Status: "decoded", Raw: boundaryTLSRecord(t, state, 1, 2, phaContextTestRequest([]byte{1}), 22, 0)}
	_, err := flow.consumeTLS(1, e)
	require.NoError(t, err)
	// The native event's accepted metadata is separate from public DTOs.
	e.nativeTLSObservation, e.semanticFields = true, cloneSession(e.Session)
	before := e.DisplayFieldValues()
	for _, spec := range []struct {
		name string
		want any
	}{
		{"tls.pha.offer_status", "offered"},
		{"tls.pha.context_status", "unique among observed requests"},
		{"tls.pha.requests_seen", uint64(1)},
	} {
		require.Equal(t, []any{spec.want}, before.FieldMap()[spec.name])
		seen := 0
		for _, field := range before.Values {
			if field.Name == spec.name {
				seen++
				require.Equal(t, "context", field.Origin)
				require.Nil(t, field.Range)
			}
		}
		require.Equal(t, 1, seen)
	}
	clear(e.Raw)
	clear(e.Session)
	e.Protocol = "http"
	// Public Raw edits may invalidate outer-record byte ranges. They must
	// not rewrite accepted values or attach a range to derived PHA context.
	after := e.DisplayFieldValues()
	require.Equal(t, before.FieldMap(), after.FieldMap())
	for _, field := range after.Values {
		if field.Name == "tls.pha.offer_status" || field.Name == "tls.pha.context_status" || field.Name == "tls.pha.requests_seen" {
			require.Equal(t, "context", field.Origin)
			require.Nil(t, field.Range)
		}
	}
	// Later records cannot present the historical request as a new occurrence.
	later := &ProtocolEvent{Protocol: "tls", Session: map[string]any{"Authentication Verified": true}}
	require.NotContains(t, later.DisplayFields(), "tls.pha.requests_seen")
}

func TestTLS13PHAOfferCannotBeServerEncryptedExtension(t *testing.T) {
	for _, data := range [][]byte{nil, {1}} {
		t.Run(fmt.Sprintf("length%d", len(data)), func(t *testing.T) {
			state := newBinTLS()
			state.version, state.client = 0x304, 0
			extension := certificateRequestExtension(49, data)
			body := append([]byte{0, byte(len(extension))}, extension...)
			message, err := state.handshakeMessage(1, reliabilityHS(8, body), true)
			var typed *ProtocolError
			require.ErrorAs(t, err, &typed)
			require.Equal(t, ErrMalformedMessage, typed.Kind)
			require.Nil(t, message)
			require.Empty(t, state.requestContexts)
		})
	}
}

func TestTLS13PHAOfferObservationIsTransactional(t *testing.T) {
	for _, offer := range []string{"offered", "not-offered", "unknown"} {
		t.Run(offer, func(t *testing.T) {
			state := newBinTLS()
			if offer != "unknown" {
				extensions := append([]tlsResumptionExt{{43, []byte{2, 3, 4}}}, tlsResumptionDHExtensions(true)...)
				if offer == "offered" {
					extensions = append(extensions, tlsResumptionExt{49, nil})
				}
				_, err := state.handshakeMessage(0, reliabilityHS(1, tlsResumptionHello(true, nil, 0x1301, extensions...)), false)
				require.NoError(t, err)
			}
			before := bytes.Clone(state.firstHello)
			_, err := state.handshakeMessage(0, reliabilityHS(1, tlsResumptionHello(true, nil, 0x1301, tlsResumptionExt{49, []byte{1}})), false)
			require.Error(t, err, "rejected ClientHello must not change an accepted or unknown offer")
			require.Equal(t, before, state.firstHello)
			_, err = state.handshakeMessage(1, reliabilityHS(2, tlsResumptionHello(false, nil, 0x1301, append([]tlsResumptionExt{{43, []byte{3, 4}}}, tlsResumptionDHExtensions(false)...)...)), false)
			require.NoError(t, err)
			require.Empty(t, state.firstHello)
			// This message-level fixture supplies the server direction even
			// when no admitted ClientHello established the client's role.
			state.client = 0
			state.app[1] = true
			message, err := state.handshakeMessage(1, phaContextTestRequest([]byte{1}), true)
			if offer == "not-offered" {
				require.Error(t, err)
				require.Nil(t, message)
			} else {
				require.NoError(t, err)
				require.Equal(t, offer, message["Certificate Request Admission"].(map[string]any)["Client PHA Offer"])
			}
		})
	}
}
