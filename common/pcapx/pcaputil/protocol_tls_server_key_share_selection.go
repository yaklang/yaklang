package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// RFC8446 4.2.8: a final (EC)DHE selection must match an actual client share.
// A complete captured ClientHello makes absence provable; a missing one does
// not. PSK selection keeps its existing separate index/mode/share checks.
func tls13NonPSKServerKeyShareSelection(body []byte, server, client *tlsHelloView, limits ...int) error {
	if client == nil || !bytes.Equal(server.ext[43], []byte{3, 4}) || bytes.Equal(body[2:34], tlsRetryRandom[:]) {
		return nil
	}
	if _, selectedPSK := server.ext[41]; selectedPSK {
		return nil
	}
	share := server.ext[51]
	if len(share) < 5 || int(binary.BigEndian.Uint16(share[2:])) != len(share)-4 {
		return protocolError(ErrMalformedMessage, "TLS 1.3 ServerHello key_share length")
	}
	group := binary.BigEndian.Uint16(share)
	max := 4096
	if len(limits) > 0 {
		max = limits[0]
	}
	found, err := tlsClientShareSelection(client.ext[51], group, max)
	if err != nil {
		return err
	}
	if !found {
		return protocolError(ErrMalformedMessage, fmt.Sprintf("TLS 1.3 ServerHello key_share group 0x%04x was not offered in current ClientHello", group))
	}
	groups := client.ext[10]
	if len(groups) < 4 || int(binary.BigEndian.Uint16(groups)) != len(groups)-2 || (len(groups)-2)%2 != 0 {
		return protocolError(ErrMalformedMessage, "TLS 1.3 current ClientHello supported_groups vector")
	}
	if (len(groups)-2)/2 > max {
		return protocolError(ErrResourceExceeded, "TLS ClientHello supported_groups count")
	}
	if !tlsContains16(groups[2:], group) {
		return protocolError(ErrMalformedMessage, fmt.Sprintf("TLS 1.3 ServerHello key_share group 0x%04x was not in current ClientHello supported_groups", group))
	}
	return nil
}

// Inspect the complete vector without allocating an unbounded group map.
// Validate even non-selected entries before returning an offered membership.
func tlsClientShareSelection(v []byte, selected uint16, max int) (bool, error) {
	if v == nil {
		return false, nil
	}
	bad := func() (bool, error) {
		return false, protocolError(ErrMalformedMessage, "TLS client key_share entry or vector")
	}
	if len(v) < 2 || int(binary.BigEndian.Uint16(v)) != len(v)-2 {
		return bad()
	}
	var seen [1024]uint64
	count, found := 0, false
	for rest := v[2:]; len(rest) > 0; {
		if len(rest) < 4 {
			return bad()
		}
		group, n := binary.BigEndian.Uint16(rest), int(binary.BigEndian.Uint16(rest[2:]))
		rest = rest[4:]
		bit := uint64(1) << (group % 64)
		if n == 0 || n > len(rest) || seen[group/64]&bit != 0 {
			return bad()
		}
		seen[group/64] |= bit
		found = found || group == selected
		count++
		rest = rest[n:]
	}
	if count > max {
		return false, protocolError(ErrResourceExceeded, "TLS ClientHello key_share count")
	}
	return found, nil
}

// TLSKeyShareServerHelloRecordObservation describes only the server's literal
// declaration. It does not assert an offered match, a usable public key, a
// negotiated session, or an authenticated handshake.
type TLSKeyShareServerHelloRecordObservation struct {
	TLSHelloExtensionRecordObservation
	Group             uint16
	KeyExchangeLength int
}

func ObserveTLSKeyShareServerHelloRecord(record []byte, maxElements int) (*TLSKeyShareServerHelloRecordObservation, error) {
	server, err := ObserveTLSServerHelloRecord(record, maxElements)
	if err != nil {
		return nil, err
	}
	if server.SelectedVersion != 0x304 || bytes.Equal(record[11:43], tlsRetryRandom[:]) {
		return nil, protocolError(ErrContextRequired, "TLS key_share observation needs a final TLS 1.3 ServerHello")
	}
	literal, err := observeTLSHelloExtensionRecord(record, maxElements, 51, "ServerHello key_share")
	if err != nil {
		return nil, err
	}
	v := record[literal.ExtensionOffset+4 : literal.ExtensionOffset+4+literal.ExtensionDataLength]
	if len(v) < 5 || int(binary.BigEndian.Uint16(v[2:])) != len(v)-4 {
		return nil, protocolError(ErrMalformedMessage, "TLS 1.3 ServerHello key_share length")
	}
	return &TLSKeyShareServerHelloRecordObservation{TLSHelloExtensionRecordObservation: *literal,
		Group: binary.BigEndian.Uint16(v), KeyExchangeLength: len(v) - 4}, nil
}
