package sharkcli

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// Reparse the complete owned handshake and compare every body literal across
// the worker JSON boundary before publishing a selectable plaintext range.
func tlsCertificateVerifyDetail(m map[string]any, version uint64, authenticated bool) (*pcaputil.TLSCertificateVerifyView, []byte, string) {
	messageAuthenticated, ok := m["Authenticated"].(bool)
	if !ok || messageAuthenticated != authenticated || version < 0x301 || version > 0x304 ||
		number(m["Type"]) != 15 || number(m["Certificate Verify Version"]) != version || version == 0x304 && !authenticated {
		return nil, nil, ""
	}
	phase := textValue(m["Certificate Verify Phase"])
	if phase != "handshake" && (phase != "application" || version != 0x304) {
		return nil, nil, ""
	}
	wire := bytesValue(m["Encoded Certificate Verify"])
	if len(wire) < 4 || number(m["Length"]) != uint64(len(wire)-4) {
		return nil, nil, ""
	}
	view, err := pcaputil.DecodeTLSCertificateVerify(wire, uint16(version))
	if err != nil {
		return nil, nil, ""
	}
	expected, err := json.Marshal(view.Fields)
	observed, observedErr := json.Marshal(m["Certificate Verify"])
	if err != nil || observedErr != nil || !bytes.Equal(expected, observed) {
		return nil, nil, ""
	}
	name := "TLS Certificate Verify · reassembled handshake bytes · frame offsets unavailable"
	if authenticated {
		name = "TLS Certificate Verify · authenticated handshake plaintext · frame offsets unavailable"
	}
	return view, wire, name
}

func tlsCertificateVerifyFields(view *pcaputil.TLSCertificateVerifyView, source, depth, limit int, version uint16) ([]field, bool) {
	rows := []field{{text: "Certificate Verify body", depth: depth, branch: true}}
	if algorithm, ok := view.Fields["Signature Algorithm"]; ok {
		rows = append(rows, field{text: "Signature Algorithm: " + tlsCertificateRequestAlgorithm(uint16(number(algorithm)), version), depth: depth + 1,
			source: source, start: 4, end: 6, hasRange: source > 0})
	}
	rows = append(rows, field{text: fmt.Sprintf("Signature Length: %d", number(view.Fields["Signature Length"])), depth: depth + 1,
		source: source, start: view.SignatureOffset - 2, end: view.SignatureOffset, hasRange: source > 0})
	signature := bytesValue(view.Fields["Signature"])
	rows = append(rows, field{text: "Signature: " + formatFieldValue("Signature", signature), depth: depth + 1,
		source: source, start: view.SignatureOffset, end: view.SignatureOffset + len(signature), hasRange: source > 0 && len(signature) > 0})
	if len(rows) > limit {
		return rows[:max(0, limit)], true
	}
	return rows, false
}
