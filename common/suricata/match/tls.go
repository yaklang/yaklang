package match

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/gopacket/gopacket"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/data/protocol"
)

type tlsProvider struct {
	PK gopacket.Packet

	// raw is the reassembled TLS record stream for one direction.
	raw []byte

	// Certificate data from the leaf certificate.
	certSubject     []byte
	certIssuer      []byte
	certSerial      []byte
	certFingerprint []byte
	certLeaf        []byte
	certs           [][]byte
	certChainLen    int

	sni         []byte
	random      []byte
	randomTime  []byte
	randomBytes []byte
	version     string
	versionRaw  uint16

	// JA3 related data.
	ja3Hash    []byte
	ja3String  []byte
	ja3sHash   []byte
	ja3sString []byte
}

// newTLSProviderFromBytes parses TLS records without relying on gopacket's
// handshake decoder, which only understands ClientHello/ClientKeyExchange and
// rejects Certificate messages. Returns nil when raw does not begin with a
// plausible TLS record stream.
func newTLSProviderFromBytes(pk gopacket.Packet, raw []byte) *tlsProvider {
	provider := &tlsProvider{PK: pk, raw: append([]byte(nil), raw...)}
	if !provider.parseTLSRecords() {
		return nil
	}
	return provider
}

func (t *tlsProvider) parseTLSRecords() bool {
	var handshake []byte
	data := t.raw
	valid := false
	handshakeClosed := false
	for len(data) >= 5 {
		contentType := data[0]
		length := int(binary.BigEndian.Uint16(data[3:5]))
		if contentType < 20 || contentType > 23 || length == 0 || 5+length > len(data) {
			break
		}
		valid = true
		switch contentType {
		case 20:
			// Handshake messages after ChangeCipherSpec are encrypted and
			// must not be interpreted as certificate/handshake plaintext.
			handshakeClosed = true
		case 22:
			if !handshakeClosed {
				handshake = append(handshake, data[5:5+length]...)
			}
		}
		data = data[5+length:]
	}
	t.parseHandshakeMessages(handshake)
	return valid
}

func (t *tlsProvider) parseHandshakeMessages(data []byte) {
	for len(data) >= 4 {
		handshakeType := data[0]
		length := int(data[1])<<16 | int(data[2])<<8 | int(data[3])
		if length == 0 || 4+length > len(data) {
			return
		}
		body := data[4 : 4+length]
		switch handshakeType {
		case 1:
			t.parseClientHello(body)
		case 2:
			t.parseServerHello(body)
		case 11:
			t.parseCertificateMessage(body)
		}
		data = data[4+length:]
	}
}

func (t *tlsProvider) parseClientHello(body []byte) {
	if len(body) < 34 {
		return
	}
	version := binary.BigEndian.Uint16(body[0:2])
	t.versionRaw = version
	t.version = tlsVersionString(version)
	t.random = append([]byte(nil), body[2:34]...)
	t.randomTime = t.random[0:4]
	t.randomBytes = t.random[4:32]

	offset := 34
	if len(body) < offset+1 {
		return
	}
	sessionIDLength := int(body[offset])
	offset++
	if len(body) < offset+sessionIDLength+2 {
		return
	}
	offset += sessionIDLength
	cipherSuitesLength := int(binary.BigEndian.Uint16(body[offset : offset+2]))
	offset += 2
	if len(body) < offset+cipherSuitesLength+1 {
		return
	}
	cipherSuites := body[offset : offset+cipherSuitesLength]
	offset += cipherSuitesLength
	compressionLength := int(body[offset])
	offset++
	if len(body) < offset+compressionLength+2 {
		return
	}
	offset += compressionLength
	extensionsLength := int(binary.BigEndian.Uint16(body[offset : offset+2]))
	offset += 2
	if len(body) < offset+extensionsLength {
		extensionsLength = len(body) - offset
	}
	extensions := body[offset : offset+extensionsLength]

	var cipherIDs []string
	for index := 0; index+1 < len(cipherSuites); index += 2 {
		cipherIDs = append(cipherIDs, fmt.Sprintf("%d", binary.BigEndian.Uint16(cipherSuites[index:index+2])))
	}

	var extensionIDs, curves, pointFormats []string
	for len(extensions) >= 4 {
		extensionType := binary.BigEndian.Uint16(extensions[0:2])
		extensionLength := int(binary.BigEndian.Uint16(extensions[2:4]))
		if len(extensions) < 4+extensionLength {
			break
		}
		extensionData := extensions[4 : 4+extensionLength]
		extensionIDs = append(extensionIDs, fmt.Sprintf("%d", extensionType))
		switch extensionType {
		case 0:
			if name := tlsServerName(extensionData); len(name) > 0 {
				t.sni = append([]byte(nil), name...)
			}
		case 10:
			if len(extensionData) >= 2 {
				groupLength := int(binary.BigEndian.Uint16(extensionData[0:2]))
				groupData := extensionData[2:]
				for index := 0; index+1 < groupLength && index+1 < len(groupData); index += 2 {
					curves = append(curves, fmt.Sprintf("%d", binary.BigEndian.Uint16(groupData[index:index+2])))
				}
			}
		case 11:
			if len(extensionData) >= 1 {
				formatLength := int(extensionData[0])
				formatData := extensionData[1:]
				for index := 0; index < formatLength && index < len(formatData); index++ {
					pointFormats = append(pointFormats, fmt.Sprintf("%d", formatData[index]))
				}
			}
		}
		extensions = extensions[4+extensionLength:]
	}

	ja3 := strings.Join([]string{
		fmt.Sprintf("%d", version),
		strings.Join(cipherIDs, "-"),
		strings.Join(extensionIDs, "-"),
		strings.Join(curves, "-"),
		strings.Join(pointFormats, "-"),
	}, ",")
	t.ja3String = []byte(ja3)
	hash := md5.Sum(t.ja3String)
	t.ja3Hash = []byte(fmt.Sprintf("%x", hash))
}

func tlsServerName(data []byte) []byte {
	if len(data) < 2 {
		return nil
	}
	offset := 2
	for offset+3 <= len(data) {
		nameType := data[offset]
		nameLength := int(binary.BigEndian.Uint16(data[offset+1 : offset+3]))
		offset += 3
		if offset+nameLength > len(data) {
			return nil
		}
		if nameType == 0 {
			return data[offset : offset+nameLength]
		}
		offset += nameLength
	}
	return nil
}

func (t *tlsProvider) parseServerHello(body []byte) {
	if len(body) < 40 {
		return
	}
	legacyVersion := binary.BigEndian.Uint16(body[0:2])
	offset := 34
	sessionIDLength := int(body[offset])
	offset++
	if len(body) < offset+sessionIDLength+3 {
		return
	}
	offset += sessionIDLength
	cipherSuite := binary.BigEndian.Uint16(body[offset : offset+2])
	offset += 2
	offset++ // compression method
	if len(body) < offset+2 {
		return
	}
	extensionsLength := int(binary.BigEndian.Uint16(body[offset : offset+2]))
	offset += 2
	if len(body) < offset+extensionsLength {
		extensionsLength = len(body) - offset
	}
	extensions := body[offset : offset+extensionsLength]

	version := legacyVersion
	var extensionIDs []string
	for len(extensions) >= 4 {
		extensionType := binary.BigEndian.Uint16(extensions[0:2])
		extensionLength := int(binary.BigEndian.Uint16(extensions[2:4]))
		if len(extensions) < 4+extensionLength {
			break
		}
		extensionData := extensions[4 : 4+extensionLength]
		extensionIDs = append(extensionIDs, fmt.Sprintf("%d", extensionType))
		if extensionType == 43 && len(extensionData) >= 2 {
			version = binary.BigEndian.Uint16(extensionData[0:2])
		}
		extensions = extensions[4+extensionLength:]
	}
	t.versionRaw = version
	t.version = tlsVersionString(version)

	// JA3S uses the legacy ServerHello version field.
	ja3s := fmt.Sprintf("%d,%d,%s", legacyVersion, cipherSuite, strings.Join(extensionIDs, "-"))
	t.ja3sString = []byte(ja3s)
	hash := md5.Sum(t.ja3sString)
	t.ja3sHash = []byte(fmt.Sprintf("%x", hash))
}

func (t *tlsProvider) parseCertificateMessage(body []byte) {
	certificates := tls12Certificates(body)
	if len(certificates) == 0 {
		certificates = tls13Certificates(body)
	}
	if len(certificates) == 0 {
		return
	}
	t.certs = certificates
	t.certChainLen = len(certificates)
	t.certLeaf = certificates[0]
	for _, raw := range certificates {
		certificate, err := x509.ParseCertificate(raw)
		if err != nil {
			continue
		}
		t.certSubject = formatCertificateName(certificate.Subject)
		t.certIssuer = formatCertificateName(certificate.Issuer)
		t.certSerial = formatCertificateSerial(certificate)
		t.certFingerprint = formatCertificateFingerprint(certificate)
		return
	}
}

func tls12Certificates(body []byte) [][]byte {
	if len(body) < 3 {
		return nil
	}
	listLength := readUint24(body[0:3])
	if listLength == 0 || 3+listLength > len(body) {
		return nil
	}
	data := body[3 : 3+listLength]
	var certificates [][]byte
	for len(data) >= 3 {
		length := readUint24(data[0:3])
		data = data[3:]
		if length == 0 || length > len(data) || !looksLikeCertificate(data[:length]) {
			return nil
		}
		certificates = append(certificates, data[:length])
		data = data[length:]
	}
	if len(data) != 0 {
		return nil
	}
	return certificates
}

func tls13Certificates(body []byte) [][]byte {
	if len(body) < 4 {
		return nil
	}
	contextLength := int(body[0])
	offset := 1 + contextLength
	if len(body) < offset+3 {
		return nil
	}
	listLength := readUint24(body[offset : offset+3])
	offset += 3
	if listLength == 0 || offset+listLength > len(body) {
		return nil
	}
	end := offset + listLength
	var certificates [][]byte
	for offset+3 <= end {
		length := readUint24(body[offset : offset+3])
		offset += 3
		if length == 0 || offset+length > end || !looksLikeCertificate(body[offset:offset+length]) {
			return nil
		}
		certificates = append(certificates, body[offset:offset+length])
		offset += length
		if offset+2 > end {
			return nil
		}
		extensionLength := int(binary.BigEndian.Uint16(body[offset : offset+2]))
		offset += 2
		if offset+extensionLength > end {
			return nil
		}
		offset += extensionLength
	}
	if offset != end {
		return nil
	}
	return certificates
}

func looksLikeCertificate(data []byte) bool {
	return len(data) >= 3 && data[0] == 0x30
}

func readUint24(data []byte) int {
	return int(data[0])<<16 | int(data[1])<<8 | int(data[2])
}

func formatCertificateName(name pkix.Name) []byte {
	var parts []string
	for _, value := range name.Country {
		parts = append(parts, "C="+value)
	}
	for _, value := range name.Province {
		parts = append(parts, "ST="+value)
	}
	for _, value := range name.Locality {
		parts = append(parts, "L="+value)
	}
	for _, value := range name.Organization {
		parts = append(parts, "O="+value)
	}
	for _, value := range name.OrganizationalUnit {
		parts = append(parts, "OU="+value)
	}
	if name.CommonName != "" {
		parts = append(parts, "CN="+name.CommonName)
	}
	if name.SerialNumber != "" {
		parts = append(parts, "serialNumber="+name.SerialNumber)
	}
	return []byte(strings.Join(parts, ", "))
}

func formatCertificateSerial(certificate *x509.Certificate) []byte {
	serial := certificate.SerialNumber.Bytes()
	if len(serial) == 0 {
		return []byte("00")
	}
	// DER encodes a positive integer with a leading 0x00 when the high bit is
	// set. big.Int.Bytes strips it; Suricata's serial buffer does not.
	if serial[0]&0x80 != 0 {
		serial = append([]byte{0}, serial...)
	}
	encoded := hex.EncodeToString(serial)
	encoded = strings.ToUpper(encoded)
	var parts []string
	for index := 0; index+2 <= len(encoded); index += 2 {
		parts = append(parts, encoded[index:index+2])
	}
	return []byte(strings.Join(parts, ":"))
}

func formatCertificateFingerprint(certificate *x509.Certificate) []byte {
	sum := sha1.Sum(certificate.Raw)
	encoded := hex.EncodeToString(sum[:])
	var parts []string
	for index := 0; index+2 <= len(encoded); index += 2 {
		parts = append(parts, encoded[index:index+2])
	}
	return []byte(strings.Join(parts, ":"))
}

func tlsVersionString(version uint16) string {
	switch version {
	case protocol.TLSVersion10:
		return "1.0"
	case protocol.TLSVersion11:
		return "1.1"
	case protocol.TLSVersion12:
		return "1.2"
	case protocol.TLSVersion13:
		return "1.3"
	default:
		return ""
	}
}

func (t *tlsProvider) Get(mdf modifier.Modifier) []byte {
	switch mdf {
	case modifier.TLSCertSubject:
		return t.certSubject
	case modifier.TLSCertIssuer:
		return t.certIssuer
	case modifier.TLSCertSerial:
		return t.certSerial
	case modifier.TLSCertFingerprint:
		return t.certFingerprint
	case modifier.TLSCerts:
		// Suricata evaluates tls.certs per certificate in the chain. The
		// matcher currently exposes the leaf certificate, which covers the
		// certificate identity rules used by ET.
		return t.certLeaf
	case modifier.TLSSNI:
		return t.sni
	case modifier.TLSRandom:
		return t.random
	case modifier.TLSRandomTime:
		return t.randomTime
	case modifier.TLSRandomBytes:
		return t.randomBytes
	case modifier.JA3Hash:
		return t.ja3Hash
	case modifier.JA3String:
		return t.ja3String
	case modifier.JA3SHash:
		return t.ja3sHash
	case modifier.JA3SString:
		return t.ja3sString
	case modifier.Default:
		return t.raw
	}
	return nil
}

func tlsParser(c *matchContext) error {
	if !c.Must(c.Rule.ContentRuleConfig != nil) {
		return fmt.Errorf("content rule config is nil")
	}
	raw := c.overridePayload
	if len(raw) == 0 {
		if c.PK == nil || c.PK.ApplicationLayer() == nil {
			c.Reject()
			return nil
		}
		raw = c.PK.ApplicationLayer().LayerContents()
	}
	provider := newTLSProviderFromBytes(c.PK, raw)
	if !c.Must(provider != nil) {
		return nil
	}
	c.SetBufferProvider(provider.Get)
	c.Value["tls"] = provider
	return nil
}

func tlsMatcher(c *matchContext) error {
	if c.Rule.ContentRuleConfig.TLSConfig == nil {
		return fmt.Errorf("TLS config is nil")
	}

	provider, ok := c.Value["tls"].(*tlsProvider)
	if !ok || provider == nil {
		return fmt.Errorf("TLS provider not found or invalid")
	}

	cfg := c.Rule.ContentRuleConfig.TLSConfig

	if cfg.Version != "" {
		if provider.version == "" {
			c.Reject()
			return nil
		}
		if !c.Must(cfg.Version == provider.version) {
			return nil
		}
	}

	if cfg.CertChainLen != nil {
		if !c.Must(cfg.CertChainLen.Match(provider.certChainLen)) {
			return nil
		}
	}

	if cfg.JA3Hash != "" {
		if len(provider.ja3Hash) == 0 {
			c.Reject()
			return nil
		}
		if !c.Must(string(provider.ja3Hash) == cfg.JA3Hash) {
			return nil
		}
	}

	if cfg.JA3String != "" {
		if len(provider.ja3String) == 0 {
			c.Reject()
			return nil
		}
		if !c.Must(string(provider.ja3String) == cfg.JA3String) {
			return nil
		}
	}

	if cfg.JA3SHash != "" {
		if len(provider.ja3sHash) == 0 {
			c.Reject()
			return nil
		}
		if !c.Must(string(provider.ja3sHash) == cfg.JA3SHash) {
			return nil
		}
	}

	if cfg.JA3SString != "" {
		if len(provider.ja3sString) == 0 {
			c.Reject()
			return nil
		}
		if !c.Must(string(provider.ja3sString) == cfg.JA3SString) {
			return nil
		}
	}

	if cfg.CertExpired != nil {
		return fmt.Errorf("certificate expiration check not implemented")
	}

	return nil
}
