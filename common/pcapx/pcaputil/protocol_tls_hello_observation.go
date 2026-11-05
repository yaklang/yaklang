package pcaputil

import "encoding/binary"

// TLSServerHelloRecordObservation contains literal, structurally complete wire
// declarations. It does not establish negotiation, peer identity or trust.
type TLSServerHelloRecordObservation struct {
	RecordVersion   uint16
	LegacyVersion   uint16
	SelectedVersion uint16
	CipherSuite     uint16
	Compression     byte
	SessionIDLength int
	Extensions      int
}

// ObserveTLSServerHelloRecord requires one complete cleartext record containing
// exactly one complete ServerHello, including its complete outer extension
// vectors. It uses the native hello grammar without applying or mutating
// connection state; unsupported extension contents remain opaque. A
// structurally complete but semantically rejected declaration remains useful
// as an observation; it must not be called an admitted negotiated version.
func ObserveTLSServerHelloRecord(record []byte, maxElements int) (*TLSServerHelloRecordObservation, error) {
	bad := func() (*TLSServerHelloRecordObservation, error) {
		return nil, protocolError(ErrMalformedMessage, "TLS ServerHello record observation framing")
	}
	if len(record) < 47 || record[0] != 22 || record[1] != 3 || record[2] < 1 || record[2] > 3 ||
		int(binary.BigEndian.Uint16(record[3:5])) != len(record)-5 || len(record)-5 > 16384 ||
		record[5] != 2 || tlsU24(record[6:9]) != len(record)-9 {
		return bad()
	}
	body := record[9:]
	hello, err := tlsHello(body, false, maxElements)
	if err != nil {
		return nil, err
	}
	if err = tlsHelloExtensionStructure(hello, false); err != nil {
		return nil, err
	}
	selected := binary.BigEndian.Uint16(body)
	if version, present := hello.ext[43]; present {
		selected = binary.BigEndian.Uint16(version)
	}
	return &TLSServerHelloRecordObservation{
		RecordVersion: binary.BigEndian.Uint16(record[1:3]), LegacyVersion: binary.BigEndian.Uint16(body),
		SelectedVersion: selected, CipherSuite: hello.suite, Compression: hello.compression[0],
		SessionIDLength: len(hello.session), Extensions: len(hello.ext),
	}, nil
}
