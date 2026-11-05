package sharkcli

import (
	"bytes"
	"encoding/json"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func tlsClientKeyExchangeDetail(m map[string]any, version, suite uint64, authenticated bool) (*pcaputil.TLSClientKeyExchangeView, []byte, string) {
	auth, ok := m["Authenticated"].(bool)
	// The native profile admits the initial cleartext CKE only. A worker DTO
	// cannot turn it into authenticated renegotiation by changing both flags.
	if !ok || auth || authenticated || version != 0x303 || suite > 0xffff ||
		number(m["Type"]) != 16 || number(m["Client Key Exchange Version"]) != version || number(m["Client Key Exchange Cipher Suite"]) != suite {
		return nil, nil, ""
	}
	wire := bytesValue(m["Encoded Client Key Exchange"])
	if len(wire) < 4 || number(m["Length"]) != uint64(len(wire)-4) {
		return nil, nil, ""
	}
	view, err := pcaputil.DecodeTLS12ClientKeyExchange(wire, uint16(version), uint16(suite))
	if err != nil || textValue(m["Client Key Exchange Profile"]) != view.Profile {
		return nil, nil, ""
	}
	expected, err := json.Marshal(view.Fields)
	observed, observedErr := json.Marshal(m["Client Key Exchange"])
	if err != nil || observedErr != nil || !bytes.Equal(expected, observed) {
		return nil, nil, ""
	}
	return view, wire, "TLS Client Key Exchange · reassembled handshake bytes · frame offsets unavailable"
}

func tlsClientKeyExchangeFields(view *pcaputil.TLSClientKeyExchangeView, source, depth, limit int) ([]field, bool) {
	priority := map[string]int{"Client EC Point Length": 0, "Client EC Point": 1,
		"Client DH Public Value Length": 0, "Client DH Public Value": 1,
		"Encrypted Premaster Length": 0, "Encrypted Premaster": 1}
	return tlsHandshakeVectorFields(view.Fields, view.Ranges, "Client Key Exchange body", priority, nil, source, depth, limit, 0x303)
}
