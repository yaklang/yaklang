package pcaputil

import (
	"bytes"
	"encoding/binary"
)

// RFC8446 4.1.3: unlike missing captured records, absent key-exchange
// extensions inside an observed complete final ServerHello are provable.
// Future protocol versions keep their existing profile behavior. This does
// not validate a public key or PSK binder.
func tls13FinalServerKeyExchange(body []byte, h *tlsHelloView) error {
	if !bytes.Equal(h.ext[43], []byte{3, 4}) || bytes.Equal(body[2:34], tlsRetryRandom[:]) {
		return nil
	}
	_, psk := h.ext[41]
	share, present := h.ext[51]
	if !psk && !present {
		return protocolError(ErrMalformedMessage, "TLS 1.3 ServerHello requires key_share or pre_shared_key")
	}
	if present && (len(share) < 5 || int(binary.BigEndian.Uint16(share[2:])) != len(share)-4) {
		return protocolError(ErrMalformedMessage, "TLS 1.3 ServerHello key_share length")
	}
	return nil
}

// Private observations survive firstHello release. A missing ClientHello is
// unknown context, never an absent offer. The wire selection in a complete
// ServerHello is independently observable even when its ClientHello is absent.
func (t *binTLS) admitEarlyDataAcceptance() error {
	if t.retrySeen {
		return protocolError(ErrMalformedMessage, "TLS early_data accepted after HelloRetryRequest")
	}
	if t.earlyDataOfferKnown && !t.earlyDataOffered {
		return protocolError(ErrMalformedMessage, "TLS early_data was not offered")
	}
	if t.earlyDataSelectionKnown && (!t.earlyDataPSKSelected || t.earlyDataIdentity != 0) {
		return protocolError(ErrMalformedMessage, "TLS early_data requires selected first PSK identity")
	}
	return nil
}
