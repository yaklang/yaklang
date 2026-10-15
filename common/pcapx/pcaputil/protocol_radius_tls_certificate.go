package pcaputil

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
)

type radiusTLSCertificateLimits struct {
	depth  int
	server bool
}

// Certificate observations never verify signatures, chain trust or an EAP
// endpoint. The caller must first establish this flight's legacy wire grammar.
func radiusTLSCertificateFields(out map[string]any, w []byte, max, depth int) error {
	bad := func() error {
		return protocolError(ErrContextRequired, "EAP-TLS certificate plaintext/structure is unproven")
	}
	if len(w) < 3 || tlsU24(w) != len(w)-3 {
		return bad()
	}
	count := 0
	for b := w[3:]; len(b) > 0; {
		if len(b) < 3 {
			return bad()
		}
		n := tlsU24(b)
		b = b[3:]
		if n == 0 || n > len(b) {
			return bad()
		}
		b = b[n:]
		count++
	}
	out["Certificate Count"] = count
	if count > max {
		return protocolError(ErrResourceExceeded, "EAP-TLS certificate list count")
	}
	if count == 0 {
		return bad()
	}
	// Preflight every entry before any X.509 decoder or output-list allocation.
	// Include the DER sub-codecs that Go's parser invokes inside known extension
	// OCTET STRINGs and RSA SubjectPublicKeyInfo BIT STRINGs; unknown extensions
	// and signature bits remain opaque.
	budget := radiusCertificateBudget{max: max, depth: min(depth, 128)}
	for b := w[3:]; len(b) > 0; {
		n := tlsU24(b)
		der := b[3 : 3+n]
		b = b[3+n:]
		if err := budget.walk(der, 1); err != nil {
			return err
		}
		tag, _, rest, ok := radiusCertificateTLV(der)
		if !ok || tag != 0x30 || len(rest) != 0 {
			return bad()
		}
	}
	rows := make([]map[string]any, 0, count)
	for b := w[3:]; len(b) > 0; {
		n := tlsU24(b)
		der := b[3 : 3+n]
		b = b[3+n:]
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return bad()
		}
		sum := sha256.Sum256(der)
		bits := 0
		switch k := cert.PublicKey.(type) {
		case *rsa.PublicKey:
			bits = k.N.BitLen()
		case *ecdsa.PublicKey:
			bits = k.Curve.Params().BitSize
		case ed25519.PublicKey:
			bits = len(k) * 8
		}
		if bits == 0 {
			return protocolError(ErrUnsupportedFeature, "EAP-TLS certificate public-key algorithm profile")
		}
		exts := make([]map[string]any, 0, len(cert.Extensions))
		for _, e := range cert.Extensions {
			exts = append(exts, map[string]any{"OID": e.Id.String(), "Critical": e.Critical, "Raw": bytes.Clone(e.Value)})
		}
		dns := append([]string{}, cert.DNSNames...)
		ips := make([]string, 0, len(cert.IPAddresses))
		for _, ip := range cert.IPAddresses {
			ips = append(ips, ip.String())
		}
		rows = append(rows, map[string]any{"DER": bytes.Clone(der), "SHA256": hex.EncodeToString(sum[:]), "Version": cert.Version, "Serial Number": cert.SerialNumber.String(), "Subject": cert.Subject.String(), "Issuer": cert.Issuer.String(), "Not Before": cert.NotBefore.Unix(), "Not After": cert.NotAfter.Unix(), "Public Key Algorithm": cert.PublicKeyAlgorithm.String(), "Signature Algorithm": cert.SignatureAlgorithm.String(), "Public Key Bits": bits, "DNS Names": dns, "IP Addresses": ips, "Is CA": cert.IsCA, "Extensions": exts, "Authentication Verified": false, "Certificate Trust": "not-evaluated"})
	}
	out["Certificates"] = rows
	return nil
}

// Definite, minimally encoded lengths, with no input-driven allocation.
func radiusCertificateTLV(w []byte) (byte, []byte, []byte, bool) {
	if len(w) < 2 || w[0]&31 == 31 || w[0] == 0 {
		return 0, nil, nil, false
	}
	at, n := 2, int(w[1])
	if n&128 != 0 {
		k := n & 127
		if k == 0 || k > 4 || len(w) < 2+k || w[2] == 0 {
			return 0, nil, nil, false
		}
		at += k
		n = 0
		for _, b := range w[2:at] {
			if n > len(w)/256 {
				return 0, nil, nil, false
			}
			n = n*256 + int(b)
		}
		if n < 128 {
			return 0, nil, nil, false
		}
	}
	if n > len(w)-at {
		return 0, nil, nil, false
	}
	return w[0], w[at : at+n], w[at+n:], true
}

type radiusCertificateBudget struct{ nodes, max, depth int }

func (b *radiusCertificateBudget) walk(w []byte, depth int) error {
	bad := func() error {
		return protocolError(ErrContextRequired, "EAP-TLS certificate DER plaintext is unproven")
	}
	for len(w) > 0 {
		tag, value, rest, ok := radiusCertificateTLV(w)
		if !ok {
			return bad()
		}
		w = rest
		b.nodes++
		if depth > b.depth || b.nodes > b.max {
			return protocolError(ErrResourceExceeded, "EAP-TLS certificate DER node/depth budget")
		}
		if tag == 6 {
			components, first := 0, true
			for at := 0; at < len(value); {
				if value[at] == 0x80 {
					return bad()
				}
				for {
					if at >= len(value) {
						return bad()
					}
					v := value[at]
					at++
					if v&128 == 0 {
						break
					}
				}
				if first {
					components += 2
					first = false
				} else {
					components++
				}
				if components > b.max {
					return protocolError(ErrResourceExceeded, "EAP-TLS certificate OID component budget")
				}
			}
			if first {
				return bad()
			}
		}
		if tag&32 != 0 {
			if err := b.walk(value, depth+1); err != nil {
				return err
			}
		}
		if tag == 0x30 {
			if inner, ok := radiusCertificateExtensionDER(value); ok {
				if err := b.walk(inner, depth+2); err != nil {
					return err
				}
			}
			if inner, ok := radiusCertificateRSADER(value); ok {
				if err := b.walk(inner, depth+2); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
func radiusCertificateExtensionDER(w []byte) ([]byte, bool) {
	tag, oid, rest, ok := radiusCertificateTLV(w)
	if !ok || tag != 6 {
		return nil, false
	}
	known := false
	if len(oid) == 3 && oid[0] == 0x55 && oid[1] == 0x1d {
		switch oid[2] {
		case 15, 19, 17, 30, 31, 35, 37, 14, 32:
			known = true
		}
	} else if bytes.Equal(oid, []byte{0x2b, 6, 1, 5, 5, 7, 1, 1}) {
		known = true
	}
	if !known {
		return nil, false
	}
	tag, v, rest, ok := radiusCertificateTLV(rest)
	if !ok {
		return nil, false
	}
	if tag == 1 {
		tag, v, rest, ok = radiusCertificateTLV(rest)
	}
	return v, ok && tag == 4 && len(rest) == 0
}
func radiusCertificateRSADER(w []byte) ([]byte, bool) {
	tag, alg, rest, ok := radiusCertificateTLV(w)
	if !ok || tag != 0x30 {
		return nil, false
	}
	tag, oid, _, ok := radiusCertificateTLV(alg)
	if !ok || tag != 6 || !bytes.Equal(oid, []byte{0x2a, 0x86, 0x48, 0x86, 0xf7, 13, 1, 1, 1}) {
		return nil, false
	}
	tag, bits, rest, ok := radiusCertificateTLV(rest)
	if !ok || tag != 3 || len(bits) < 2 || bits[0] != 0 || len(rest) != 0 {
		return nil, false
	}
	return bits[1:], true
}
