package pcaputil

import "strings"

// Handshake fields describe messages completed in this PDU. Session fields are
// repeated observations from this connection and never fabricate a new wire
// handshake or a cross-connection ticket association.
func tlsResumptionDisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	handshake := func(name, label string, typ DisplayFieldType, bits int, tail ...string) {
		b := displayCustom("tls", name, label, typ, bits, "pdu", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
			root := "Handshake Messages"
			session := e.Session
			if e.nativeQUICObservation || e.Protocol == "quic" {
				root = "TLS Handshakes"
				if e.nativeQUICObservation {
					session = e.semanticSession
				}
			} else if e.Protocol != "tls" {
				// An application label alone is not QUIC/TLS evidence. Only
				// the admitted native carrier may retain these observations
				// after its protocol transitions to HTTP/3 or DNS over QUIC.
				return
			}
			path := append([]string{root, "*"}, tail...)
			p.walk(session, path, func(value any) {
				if modes, ok := value.([]byte); ok {
					for _, mode := range modes {
						emit(mode)
					}
				} else {
					emit(value)
				}
			})
		})
		b.protocols = "|tls|quic|http3|doq|"
		if strings.HasSuffix(name, ".sha256") {
			b.descriptor.Origin = "derived"
		}
		switch name {
		case "tls.handshake.extensions.psk.offer_match", "tls.handshake.extensions.psk.binder_verified", "tls.handshake.session_ticket.lifetime_within_server_limit", "tls.handshake.session_ticket.max_client_cache_seconds", "tls.handshake.session_ticket.authenticated":
			b.descriptor.Origin = "derived"
		}
		b.descriptor.Description = label + "; only an observed handshake completed in this PDU, not repeated connection context"
		out = append(out, b)
	}
	handshake("tls.handshake.session_id_length", "TLS session ID length", DisplayUnsigned, 16, "Session ID", "Length")
	handshake("tls.handshake.session_id.sha256", "TLS session ID SHA256", DisplayString, 0, "Session ID", "SHA256")
	handshake("tls.extension.psk_ke_modes_length", "TLS PSK key exchange mode vector length", DisplayUnsigned, 8, "PSK Key Exchange Modes Length")
	handshake("tls.extension.psk_ke_mode", "TLS offered PSK key exchange mode", DisplayUnsigned, 8, "PSK Key Exchange Modes")
	handshake("tls.handshake.session_ticket_offer.length", "TLS legacy offered ticket length", DisplayUnsigned, 16, "Session Ticket Offer", "Length")
	handshake("tls.handshake.session_ticket_offer.sha256", "TLS legacy offered ticket SHA256", DisplayString, 0, "Session Ticket Offer", "SHA256")
	handshake("tls.handshake.extensions.psk.identities.length", "TLS PSK identities vector length", DisplayUnsigned, 16, "PSK Offer", "Identities Length")
	handshake("tls.handshake.extensions.psk.binders_len", "TLS PSK binders vector length", DisplayUnsigned, 16, "PSK Offer", "Binders Length")
	handshake("tls.handshake.extensions.psk.identity.identity_length", "TLS offered PSK identity length", DisplayUnsigned, 16, "PSK Offer", "Identities", "*", "Length")
	handshake("tls.handshake.extensions.psk.identity.sha256", "TLS offered PSK identity SHA256", DisplayString, 0, "PSK Offer", "Identities", "*", "SHA256")
	handshake("tls.handshake.extensions.psk.identity.obfuscated_ticket_age", "TLS offered PSK obfuscated age", DisplayUnsigned, 32, "PSK Offer", "Identities", "*", "Obfuscated Ticket Age")
	handshake("tls.handshake.extensions.psk.binder.binder_length", "TLS offered PSK binder length", DisplayUnsigned, 8, "PSK Offer", "Identities", "*", "Binder Length")
	handshake("tls.handshake.extensions.psk.identity.selected", "TLS selected PSK identity index", DisplayUnsigned, 16, "PSK Selection", "Selected Identity")
	handshake("tls.handshake.extensions.psk.offer_match", "TLS selected PSK current-offer match", DisplayString, 0, "PSK Selection", "Offer Match")
	handshake("tls.handshake.extensions.psk.binder_verified", "TLS selected PSK binder verified", DisplayBoolean, 0, "PSK Selection", "Binder Verified")
	handshake("tls.handshake.session_ticket_lifetime_hint", "TLS ticket lifetime in seconds", DisplayUnsigned, 32, "New Session Ticket", "Lifetime Hint")
	handshake("tls.handshake.session_ticket.lifetime_within_server_limit", "TLS ticket lifetime within server limit", DisplayBoolean, 0, "New Session Ticket", "Lifetime Within Server Limit")
	handshake("tls.handshake.session_ticket.max_client_cache_seconds", "TLS ticket maximum client cache lifetime", DisplayUnsigned, 32, "New Session Ticket", "Maximum Client Cache Seconds")
	handshake("tls.handshake.session_ticket_age_add", "TLS ticket age add", DisplayUnsigned, 32, "New Session Ticket", "Age Add")
	handshake("tls.handshake.session_ticket_nonce_length", "TLS ticket nonce length", DisplayUnsigned, 8, "New Session Ticket", "Nonce", "Length")
	handshake("tls.handshake.session_ticket_nonce.sha256", "TLS ticket nonce SHA256", DisplayString, 0, "New Session Ticket", "Nonce", "SHA256")
	handshake("tls.handshake.session_ticket_length", "TLS ticket length", DisplayUnsigned, 16, "New Session Ticket", "Ticket", "Length")
	handshake("tls.handshake.session_ticket.sha256", "TLS ticket SHA256", DisplayString, 0, "New Session Ticket", "Ticket", "SHA256")
	handshake("tls.handshake.session_ticket.authenticated", "TLS ticket record authentication verified", DisplayBoolean, 0, "New Session Ticket", "Record Authentication Verified")
	handshake("tls.handshake.session_ticket.max_early_data_size", "TLS ticket maximum early data size", DisplayUnsigned, 32, "New Session Ticket", "Extensions", "*", "Maximum Early Data Size")
	context := func(name, label string, typ DisplayFieldType, bits int, tail ...string) {
		path := append([]string{"Session Establishment"}, tail...)
		b := displayCustom("tls", name, label, typ, bits, "context", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
			session := e.Session
			if e.nativeQUICObservation {
				session = e.semanticSession
			} else if e.Protocol != "tls" && e.Protocol != "quic" {
				return
			}
			p.walk(session, path, emit)
		})
		b.protocols = "|tls|quic|http3|doq|"
		b.descriptor.Description = label + "; retained current-connection hello observation, not binder/certificate verification or ticket-cache confirmation"
		out = append(out, b)
	}
	context("tls.session.evidence_scope", "TLS session evidence scope", DisplayString, 0, "Evidence Scope")
	context("tls.session.id_echo", "TLS observed session ID echo", DisplayBoolean, 0, "Session ID Echo")
	context("tls.session.compatibility_id_echo", "TLS 1.3 compatibility session ID echo", DisplayBoolean, 0, "Compatibility Session ID Echo")
	context("tls.session.resumption_indicated", "Legacy TLS session ID resumption indicated", DisplayBoolean, 0, "Session ID Resumption Indicated")
	context("tls.session.psk.selected", "TLS PSK selection observed", DisplayBoolean, 0, "PSK Selected")
	context("tls.session.psk.selected_identity", "TLS selected PSK identity index in connection", DisplayUnsigned, 16, "PSK Selection", "Selected Identity")
	context("tls.session.psk.offer_match", "TLS selected PSK current-offer match in connection", DisplayString, 0, "PSK Selection", "Offer Match")
	context("tls.session.psk.identity_length", "TLS selected PSK identity length in connection", DisplayUnsigned, 16, "PSK Selection", "Length")
	context("tls.session.psk.identity_sha256", "TLS selected PSK identity SHA256 in connection", DisplayString, 0, "PSK Selection", "SHA256")
	context("tls.session.psk.key_exchange_mode", "TLS selected PSK key exchange mode", DisplayUnsigned, 8, "PSK Selection", "Key Exchange Mode")
	context("tls.session.psk.binder_verified", "TLS PSK binder verified in connection", DisplayBoolean, 0, "PSK Selection", "Binder Verified")
	context("tls.session.psk.ticket_association", "TLS selected PSK ticket association", DisplayString, 0, "PSK Selection", "Ticket Association")
	return out
}
