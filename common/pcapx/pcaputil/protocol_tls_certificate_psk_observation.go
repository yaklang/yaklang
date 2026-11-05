package pcaputil

import (
	"bytes"
	"encoding/binary"
)

// TLSHelloExtensionRecordObservation describes a complete cleartext Hello
// envelope containing an observed extension. Its contents may be rejected. All
// offsets address this record, not a captured frame or negotiated session.
type TLSHelloExtensionRecordObservation struct {
	HandshakeType       byte
	RecordVersion       uint16
	LegacyVersion       uint16
	Extensions          int
	ExtensionOffset     int
	ExtensionDataLength int
}

type TLSCertificatePSKHelloRecordObservation = TLSHelloExtensionRecordObservation

// ObserveTLSCertificatePSKHelloRecord observes only complete outer vectors. It
// deliberately does not admit extension grammar, companions, an external PSK,
// negotiation or authentication. It supports the inspector of a separately
// rejected native event; it must never be used to establish connection identity.
func ObserveTLSCertificatePSKHelloRecord(record []byte, maxElements int) (*TLSCertificatePSKHelloRecordObservation, error) {
	return observeTLSHelloExtensionRecord(record, maxElements, 33, "certificate/PSK")
}

// ObserveTLSEarlyDataHelloRecord reports literal extension 42 in a complete
// Hello envelope, including a rejected offer lacking pre_shared_key. It does
// not accept the extension, establish negotiation or restore connection state.
func ObserveTLSEarlyDataHelloRecord(record []byte, maxElements int) (*TLSHelloExtensionRecordObservation, error) {
	return observeTLSHelloExtensionRecord(record, maxElements, 42, "early_data")
}

func observeTLSHelloExtensionRecord(record []byte, maxElements int, observedExtension uint16, name string) (*TLSHelloExtensionRecordObservation, error) {
	bad := func() (*TLSCertificatePSKHelloRecordObservation, error) {
		return nil, protocolError(ErrMalformedMessage, "TLS "+name+" Hello observation framing")
	}
	if len(record) < 47 || record[0] != 22 || record[1] != 3 || record[2] < 1 || record[2] > 3 ||
		int(binary.BigEndian.Uint16(record[3:5])) != len(record)-5 || len(record)-5 > 16384 ||
		(record[5] != 1 && record[5] != 2) || tlsU24(record[6:9]) != len(record)-9 {
		return bad()
	}
	body, client := record[9:], record[5] == 1
	// Bound the complete outer collection before calling the existing native
	// framing-only reader (the no-limit form of tlsHello). Known inner payload
	// checks must not turn an observed rejection into "handshake not captured".
	at := 35 + int(body[34])
	if body[34] > 32 || at > len(body) {
		return bad()
	}
	if client {
		if len(body)-at < 2 {
			return bad()
		}
		n := int(binary.BigEndian.Uint16(body[at:]))
		at += 2
		if n < 2 || n%2 != 0 || n > len(body)-at {
			return bad()
		}
		at += n
		if at >= len(body) || body[at] == 0 || int(body[at]) > len(body)-at-1 {
			return bad()
		}
		at += 1 + int(body[at])
	} else {
		at += 3
	}
	if len(body)-at < 2 || int(binary.BigEndian.Uint16(body[at:])) != len(body)-at-2 {
		return bad()
	}
	at += 2
	count, flagOffset, flagLength := 0, -1, 0
	var seen [1024]uint64
	for at < len(body) {
		if len(body)-at < 4 {
			return bad()
		}
		kind, n := binary.BigEndian.Uint16(body[at:]), int(binary.BigEndian.Uint16(body[at+2:]))
		bit := uint64(1) << (kind % 64)
		if n > len(body)-at-4 || seen[kind/64]&bit != 0 {
			return bad()
		}
		seen[kind/64] |= bit
		if kind == observedExtension {
			flagOffset, flagLength = 9+at, n
		}
		count++
		at += 4 + n
	}
	if maxElements < 0 || count > maxElements {
		return nil, protocolError(ErrResourceExceeded, "TLS "+name+" Hello observation collection limit")
	}
	hello, err := tlsHello(body, client)
	if err != nil {
		return nil, err
	}
	hasTLS13Cipher := tlsPSKHashSize(hello.suite) != 0
	if client {
		for v := hello.suites; len(v) >= 2; v = v[2:] {
			hasTLS13Cipher = hasTLS13Cipher || tlsPSKHashSize(binary.BigEndian.Uint16(v)) != 0
		}
	}
	if flagOffset < 0 || binary.BigEndian.Uint16(body) != 0x303 || !bytes.Equal(hello.compression, []byte{0}) || !hasTLS13Cipher {
		return nil, protocolError(ErrContextRequired, "TLS "+name+" Hello observation needs a complete TLS 1.3 declaration")
	}
	return &TLSCertificatePSKHelloRecordObservation{HandshakeType: record[5], RecordVersion: binary.BigEndian.Uint16(record[1:3]),
		LegacyVersion: binary.BigEndian.Uint16(body), Extensions: count, ExtensionOffset: flagOffset, ExtensionDataLength: flagLength}, nil
}
