package sharkcli

import (
	"bytes"
	"encoding/json"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func tlsCertificateStatusDetail(m map[string]any, version uint64, authenticated bool) (*pcaputil.TLSCertificateStatusView, []byte, string) {
	auth, ok := m["Authenticated"].(bool)
	if !ok || auth || authenticated || version < 0x301 || version > 0x303 ||
		number(m["Type"]) != 22 || number(m["Certificate Status Version"]) != version {
		return nil, nil, ""
	}
	wire := bytesValue(m["Encoded Certificate Status"])
	if len(wire) < 4 || number(m["Length"]) != uint64(len(wire)-4) {
		return nil, nil, ""
	}
	view, err := pcaputil.DecodeTLSCertificateStatus(wire, uint16(version))
	if err != nil {
		return nil, nil, ""
	}
	a, err := json.Marshal(view.Fields)
	b, other := json.Marshal(m["Certificate Status"])
	if err != nil || other != nil || !bytes.Equal(a, b) {
		return nil, nil, ""
	}
	return view, wire, "TLS Certificate Status · reassembled handshake bytes · OCSP unvalidated · frame offsets unavailable"
}

func tlsCertificateStatusFields(view *pcaputil.TLSCertificateStatusView, source, depth, limit int, version uint16) ([]field, bool) {
	priority := map[string]int{"Status Type": 0, "Status Type Name": 1, "OCSP Response Length": 2, "OCSP Response": 3,
		"OCSP DER Decoded": 4, "OCSP Response Validated": 5, "Certificate Trust Evaluated": 6}
	return tlsHandshakeVectorFields(view.Fields, view.Ranges, "Certificate Status body", priority, nil, source, depth, limit, version)
}
