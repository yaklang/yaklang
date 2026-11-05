package pcaputil

import (
	"bytes"
	"fmt"
)

// RFC 8446 4.1.3/4.2 and RFC 9001 8.2 place these recognized extensions
// outside a final TLS 1.3 ServerHello. Unknown extensions stay opaque; RFC8773
// certificate/PSK, HRR and legacy ServerHello retain their separate rules.
func misplacedTLS13ServerExtension(kind uint16) string {
	switch kind {
	case 0:
		return "server_name"
	case 1:
		return "max_fragment_length"
	case 5:
		return "status_request"
	case 10:
		return "supported_groups"
	case 13:
		return "signature_algorithms"
	case 16:
		return "application_layer_protocol_negotiation"
	case 18:
		return "signed_certificate_timestamp"
	case 19:
		return "client_certificate_type"
	case 20:
		return "server_certificate_type"
	case 21:
		return "padding"
	case 23:
		return "extended_master_secret"
	case 27:
		return "compress_certificate"
	case 28:
		return "record_size_limit"
	case 35:
		return "session_ticket"
	case 44:
		return "cookie"
	case 47:
		return "certificate_authorities"
	case 48:
		return "oid_filters"
	case 50:
		return "signature_algorithms_cert"
	case 57:
		return "quic_transport_parameters"
	}
	return ""
}

func misplacedTLS13ServerExtensionError(kind uint16) error {
	if name := misplacedTLS13ServerExtension(kind); name != "" {
		return protocolError(ErrMalformedMessage, fmt.Sprintf("TLS 1.3 ServerHello contains misplaced %s extension (%d)", name, kind))
	}
	return nil
}

func tls13ServerHelloLocations(body []byte, h *tlsHelloView) error {
	if !bytes.Equal(h.ext[43], []byte{3, 4}) || bytes.Equal(body[2:34], tlsRetryRandom[:]) {
		return nil
	}
	for _, kind := range h.order {
		if err := misplacedTLS13ServerExtensionError(kind); err != nil {
			return err
		}
	}
	return nil
}

// TLSMisplacedServerHelloExtensionObservation contains literal rejected
// declaration bytes. It grants neither negotiated extension values nor TLS
// session identity, authentication, or authority to change connection state.
type TLSMisplacedServerHelloExtensionObservation struct {
	TLSHelloExtensionRecordObservation
	ExtensionType uint16
	ExtensionName string
	RefusalReason string
}

func ObserveTLSMisplacedServerHelloExtensionRecord(record []byte, maxElements int) (*TLSMisplacedServerHelloExtensionObservation, error) {
	observation, err := ObserveTLSServerHelloRecord(record, maxElements)
	if err != nil {
		return nil, err
	}
	body := record[9:]
	if observation.SelectedVersion != 0x304 || bytes.Equal(body[2:34], tlsRetryRandom[:]) {
		return nil, protocolError(ErrContextRequired, "TLS extension-location observation needs a final TLS 1.3 ServerHello")
	}
	h, err := tlsHello(body, false)
	if err != nil {
		return nil, err
	}
	for _, kind := range h.order {
		reason := misplacedTLS13ServerExtensionError(kind)
		if reason == nil {
			continue
		}
		literal, err := observeTLSHelloExtensionRecord(record, maxElements, kind, "misplaced ServerHello extension")
		if err != nil {
			return nil, err
		}
		return &TLSMisplacedServerHelloExtensionObservation{TLSHelloExtensionRecordObservation: *literal,
			ExtensionType: kind, ExtensionName: misplacedTLS13ServerExtension(kind), RefusalReason: reason.Error()}, nil
	}
	return nil, protocolError(ErrContextRequired, "TLS ServerHello has no recognized misplaced extension")
}
