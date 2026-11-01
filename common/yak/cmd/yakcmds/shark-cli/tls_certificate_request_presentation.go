package sharkcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// Revalidate the owned encoding and all published literals before attaching
// ranges. Metadata passing through a worker/JSON boundary is not itself proof
// that a selected field occupies a byte in the encrypted record.
func tlsCertificateRequestDetail(m map[string]any, version uint64, authenticated bool) (*pcaputil.TLSCertificateRequestView, []byte, string) {
	if version < 0x301 || version > 0x304 || number(m["Type"]) != 13 || number(m["Certificate Request Version"]) != version {
		return nil, nil, ""
	}
	if version == 0x304 && (!authenticated || m["Authenticated"] != true) || m["Authenticated"] == true && !authenticated {
		return nil, nil, ""
	}
	wire := bytesValue(m["Encoded Certificate Request"])
	if len(wire) < 4 || number(m["Length"]) != uint64(len(wire)-4) {
		return nil, nil, ""
	}
	view, err := pcaputil.DecodeTLSCertificateRequest(wire, uint16(version), 4096)
	if err != nil {
		return nil, nil, ""
	}
	observed, err := json.Marshal(m["Certificate Request"])
	expected, expectedErr := json.Marshal(view.Fields)
	if err != nil || expectedErr != nil || !bytes.Equal(observed, expected) {
		return nil, nil, ""
	}
	name := "TLS Certificate Request · reassembled handshake bytes · frame offsets unavailable"
	if m["Authenticated"] == true {
		name = "TLS Certificate Request · authenticated handshake plaintext · frame offsets unavailable"
	}
	return view, wire, name
}

// A message name or connection label is insufficient for row promotion. Require
// the same body/authentication checks that qualify its owned detail source.
func tlsDetailedHandshakeRank(e *pcaputil.ProtocolEvent) int {
	if e == nil || e.Protocol != "tls" || e.Status != "decoded" && e.Status != "deferred" {
		return 0
	}
	v := eventValues(e)
	version, authenticated := number(v["Negotiated Version"]), v["Authentication Verified"] == true
	rank := 0
	for _, h := range mapList(v["Handshake Messages"]) {
		if tlsKeyUpdatePlaintext(h, authenticated && version == 0x304) != nil {
			return 2
		}
		if view, _, _ := tlsCertificateRequestDetail(h, version, authenticated); view != nil {
			rank = 1
		}
	}
	return rank
}

var tlsCertificateRequestFieldPriority = map[string]int{
	"Certificate Types Length": 0, "Certificate Types": 1,
	"Context Length": 0, "Context": 1,
	"Signature Algorithms Length": 2, "Signature Algorithms": 3,
	"Certificate Authorities Length": 4, "Certificate Authorities": 5,
	"Extensions Length": 6, "Extensions": 7,
	"Type": 0, "Length": 1, "Algorithm": 0, "DER": 2, "Distinguished Name": 3,
	"Data": 8, "Interpretation": 9,
}

func tlsCertificateRequestFields(view *pcaputil.TLSCertificateRequestView, source, depth, limit int, version uint16) ([]field, bool) {
	ranges := map[string]pcaputil.TLSCertificateRequestFieldRange{}
	for _, span := range view.Ranges {
		ranges[strings.Join(span.Path, "\x00")] = span
	}
	var out []field
	limited := false
	var walk func(string, any, []string, int)
	walk = func(name string, value any, path []string, level int) {
		if len(out) >= limit {
			limited = true
			return
		}
		f := field{depth: level}
		switch v := value.(type) {
		case map[string]any:
			f.text, f.branch = name, true
			out = append(out, f)
			keys := make([]string, 0, len(v))
			for k := range v {
				keys = append(keys, k)
			}
			sort.Slice(keys, func(i, j int) bool {
				a, aok := tlsCertificateRequestFieldPriority[keys[i]]
				b, bok := tlsCertificateRequestFieldPriority[keys[j]]
				if !aok {
					a = 100
				}
				if !bok {
					b = 100
				}
				if a == b {
					return keys[i] < keys[j]
				}
				return a < b
			})
			for _, key := range keys {
				walk(key, v[key], append(append([]string(nil), path...), key), level+1)
			}
			return
		case []map[string]any:
			f.text, f.branch = fmt.Sprintf("%s · %d entries", name, len(v)), true
			out = append(out, f)
			for i, entry := range v {
				label := fmt.Sprintf("%s[%d]", name, i)
				if name == "Extensions" {
					if known := map[uint64]string{13: "signature_algorithms", 47: "certificate_authorities", 50: "signature_algorithms_cert"}[number(entry["Type"])]; known != "" {
						label += " · " + known
					}
				}
				walk(label, entry, append(append([]string(nil), path...), strconv.Itoa(i)), level+1)
			}
			return
		default:
			f.text = clipText(name+": "+formatFieldValue(name, value), 2048)
			if name == "Algorithm" {
				f.text = name + ": " + tlsCertificateRequestAlgorithm(uint16(number(value)), version)
			}
			if name == "Type" && len(path) == 3 && path[0] == "Certificate Types" {
				if label := map[uint64]string{1: "rsa_sign", 2: "dss_sign", 64: "ecdsa_sign"}[number(value)]; label != "" {
					f.text = fmt.Sprintf("Type: %s (%d)", label, number(value))
				}
			}
			if span, ok := ranges[strings.Join(path, "\x00")]; source > 0 && ok {
				f.source, f.start, f.end, f.hasRange = source, span.Offset, span.Offset+span.Length, true
			}
			out = append(out, f)
		}
	}
	walk("Certificate Request body", view.Fields, nil, depth)
	return out, limited
}

// Labels interpret an observed identifier, never claim that it was selected or
// is valid in CertificateVerify. TLS 1.2 ECDSA pairs do not select a curve.
// Names are pinned to the IANA TLS SignatureScheme registry, 2026-10-28.
func tlsCertificateRequestAlgorithm(code, version uint16) string {
	if version < 0x304 {
		hash := map[byte]string{1: "md5", 2: "sha1", 3: "sha224", 4: "sha256", 5: "sha384", 6: "sha512"}[byte(code>>8)]
		signature := map[byte]string{1: "rsa", 2: "dsa", 3: "ecdsa"}[byte(code)]
		if hash != "" && signature != "" {
			return fmt.Sprintf("%s / %s (0x%04x)", hash, signature, code)
		}
	}
	label := map[uint16]string{
		0x0401: "rsa_pkcs1_sha256", 0x0501: "rsa_pkcs1_sha384", 0x0601: "rsa_pkcs1_sha512",
		0x0403: "ecdsa_secp256r1_sha256", 0x0503: "ecdsa_secp384r1_sha384", 0x0603: "ecdsa_secp521r1_sha512",
		0x0804: "rsa_pss_rsae_sha256", 0x0805: "rsa_pss_rsae_sha384", 0x0806: "rsa_pss_rsae_sha512",
		0x0807: "ed25519", 0x0808: "ed448",
		0x0809: "rsa_pss_pss_sha256", 0x080a: "rsa_pss_pss_sha384", 0x080b: "rsa_pss_pss_sha512",
		0x081a: "ecdsa_brainpoolP256r1tls13_sha256", 0x081b: "ecdsa_brainpoolP384r1tls13_sha384", 0x081c: "ecdsa_brainpoolP512r1tls13_sha512",
		0x0904: "mldsa44", 0x0905: "mldsa65", 0x0906: "mldsa87",
	}[code]
	if label == "" {
		return fmt.Sprintf("0x%04x (name unavailable)", code)
	}
	return fmt.Sprintf("%s (0x%04x)", label, code)
}
