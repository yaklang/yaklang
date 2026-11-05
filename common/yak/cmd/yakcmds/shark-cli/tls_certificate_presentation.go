package sharkcli

import (
	"bytes"
	"encoding/json"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func tlsCertificateDetail(m map[string]any, version uint64, authenticated bool) (*pcaputil.TLSCertificateView, []byte, string) {
	if version != 0x304 || !authenticated || m["Authenticated"] != true || number(m["Type"]) != 11 || number(m["Certificate Version"]) != version {
		return nil, nil, ""
	}
	wire := bytesValue(m["Encoded Certificate"])
	if len(wire) < 4 || len(wire) > 1<<20 || number(m["Length"]) != uint64(len(wire)-4) {
		return nil, nil, ""
	}
	view, err := pcaputil.DecodeTLSCertificate(wire, uint16(version), 4096)
	if err != nil {
		return nil, nil, ""
	}
	observed, observedErr := json.Marshal(m["Certificate Body"])
	expected, expectedErr := json.Marshal(view.Fields)
	if observedErr != nil || expectedErr != nil || !bytes.Equal(observed, expected) {
		return nil, nil, ""
	}
	return view, wire, "TLS Certificate · authenticated handshake plaintext · frame offsets unavailable"
}

var tlsCertificateFieldPriority = map[string]int{
	"Context Length": 0, "Context": 1,
	"Certificate List Length": 2, "Certificate Entries": 3,
	"Certificate Length": 0, "Certificate Data": 1, "Extensions Length": 2, "Extensions": 3,
	"Type": 0, "Length": 1, "Data": 2,
}

func tlsCertificateFields(view *pcaputil.TLSCertificateView, source, depth, limit int) ([]field, bool) {
	return tlsHandshakeVectorFields(view.Fields, view.Ranges, "Certificate body", tlsCertificateFieldPriority,
		map[uint64]string{5: "status_request", 18: "signed_certificate_timestamp"}, source, depth, limit, 0x304)
}
