package sharkcli

import (
	"bytes"
	"encoding/json"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// Revalidate the owned encoding and every published literal before assigning
// ranges. Record-authentication metadata is consumed, not independently proved
// at the worker boundary; the body view is not a full negotiation verdict.
func tlsEncryptedExtensionsDetail(m map[string]any, version uint64, authenticated bool) (*pcaputil.TLSEncryptedExtensionsView, []byte, string) {
	if version != 0x304 || !authenticated || m["Authenticated"] != true || number(m["Type"]) != 8 || number(m["Encrypted Extensions Version"]) != version {
		return nil, nil, ""
	}
	wire := bytesValue(m["Encoded Encrypted Extensions"])
	if len(wire) < 6 || len(wire) > 65541 || number(m["Length"]) != uint64(len(wire)-4) {
		return nil, nil, ""
	}
	view, err := pcaputil.DecodeTLSEncryptedExtensions(wire, uint16(version), 4096)
	if err != nil {
		return nil, nil, ""
	}
	observed, err := json.Marshal(m["Encrypted Extensions"])
	expected, expectedErr := json.Marshal(view.Fields)
	if err != nil || expectedErr != nil || !bytes.Equal(observed, expected) {
		return nil, nil, ""
	}
	return view, wire, "TLS Encrypted Extensions · authenticated handshake plaintext · frame offsets unavailable"
}

func tlsEncryptedExtensionsFields(view *pcaputil.TLSEncryptedExtensionsView, source, depth, limit int) ([]field, bool) {
	return tlsHandshakeVectorFields(view.Fields, view.Ranges, "Encrypted Extensions body", map[string]int{
		"Extensions Length": 0, "Extensions": 1, "Type": 0, "Length": 1,
		"ALPN Length": 2, "Protocol Length": 3, "Protocol": 4, "Data": 5,
	}, map[uint64]string{0: "server_name", 1: "max_fragment_length", 10: "supported_groups", 14: "use_srtp", 15: "heartbeat", 16: "application_layer_protocol_negotiation", 19: "client_certificate_type", 20: "server_certificate_type", 28: "record_size_limit", 42: "early_data", 57: "quic_transport_parameters"}, source, depth, limit, 0x304)
}
