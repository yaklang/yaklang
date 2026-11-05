package sharkcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// Worker metadata must agree with the independently reparsed complete owned
// encoding before it can authorize any selectable handshake plaintext range.
func tlsServerKeyExchangeDetail(m map[string]any, version, suite uint64, authenticated bool) (*pcaputil.TLSServerKeyExchangeView, []byte, string) {
	auth, ok := m["Authenticated"].(bool)
	if !ok || auth != authenticated || version != 0x303 || suite > 0xffff ||
		number(m["Type"]) != 12 || number(m["Server Key Exchange Version"]) != version || number(m["Server Key Exchange Cipher Suite"]) != suite {
		return nil, nil, ""
	}
	wire := bytesValue(m["Encoded Server Key Exchange"])
	if len(wire) < 4 || number(m["Length"]) != uint64(len(wire)-4) {
		return nil, nil, ""
	}
	view, err := pcaputil.DecodeTLS12ServerKeyExchange(wire, uint16(version), uint16(suite))
	if err != nil {
		return nil, nil, ""
	}
	expected, err := json.Marshal(view.Fields)
	observed, observedErr := json.Marshal(m["Server Key Exchange"])
	if err != nil || observedErr != nil || !bytes.Equal(expected, observed) {
		return nil, nil, ""
	}
	name := "TLS Server Key Exchange · reassembled handshake bytes · frame offsets unavailable"
	if authenticated {
		name = "TLS Server Key Exchange · authenticated handshake plaintext · frame offsets unavailable"
	}
	return view, wire, name
}

func tlsServerKeyExchangeGroup(group uint16) string {
	name := map[uint16]string{23: "secp256r1", 24: "secp384r1", 25: "secp521r1", 29: "x25519", 30: "x448"}[group]
	if name == "" {
		return fmt.Sprintf("0x%04x (name unavailable)", group)
	}
	return fmt.Sprintf("%s (0x%04x)", name, group)
}

func tlsServerKeyExchangeFields(view *pcaputil.TLSServerKeyExchangeView, source, depth, limit int) ([]field, bool) {
	priority := map[string]int{"Curve Type": 0, "Named Group": 1, "Server EC Point Length": 2, "Server EC Point": 3,
		"DH Modulus Length": 0, "DH Modulus": 1, "DH Generator Length": 2, "DH Generator": 3,
		"Server DH Public Value Length": 4, "Server DH Public Value": 5, "Signature Algorithm": 6, "Signature Length": 7, "Signature": 8}
	rows, limited := tlsHandshakeVectorFields(view.Fields, view.Ranges, "Server Key Exchange body", priority, nil, source, depth, limit, 0x303)
	for i := range rows {
		if strings.HasPrefix(rows[i].text, "Signature Algorithm:") {
			rows[i].text = "Signature Algorithm: " + tlsCertificateRequestAlgorithm(uint16(number(view.Fields["Signature Algorithm"])), 0x303)
		}
		if strings.HasPrefix(rows[i].text, "Named Group:") {
			rows[i].text = "Named Group: " + tlsServerKeyExchangeGroup(uint16(number(view.Fields["Named Group"])))
		}
	}
	return rows, limited
}
