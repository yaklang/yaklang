package netx

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"testing"

	utls "github.com/refraction-networking/utls"
)

const chrome151JA4 = "t13d1516h2_8daaf6152771_806a8c22fdea"

func isGREASEValue(v uint16) bool {
	return v&0x0f0f == 0x0a0a
}

type parsedClientHello struct {
	ciphers     []uint16
	extensions  []uint16
	sigAlgs     []uint16
	alpn        []string
	groups      []uint16
	keyShares   []uint16
	hybridShare []byte
}

func buildClientHelloForProfile(t *testing.T, name string) ([]byte, *clientHelloPreset, *utls.UConn) {
	t.Helper()
	profile, err := GetClientHelloProfile(name)
	if err != nil {
		t.Fatal(err)
	}
	preset, err := profile.newClientHelloPreset()
	if err != nil {
		t.Fatalf("build preset: %v", err)
	}
	uConn := utls.UClient(nil, &utls.Config{ServerName: "tls.peet.ws"}, utls.HelloCustom)
	if err := uConn.ApplyPreset(preset.spec); err != nil {
		t.Fatalf("apply preset: %v", err)
	}
	if preset.applyState != nil {
		if err := preset.applyState(uConn); err != nil {
			t.Fatalf("apply state: %v", err)
		}
	}
	if err := uConn.BuildHandshakeState(); err != nil {
		t.Fatalf("build handshake state: %v", err)
	}
	return uConn.HandshakeState.Hello.Raw, preset, uConn
}

func parseClientHelloForTest(t *testing.T, raw []byte) *parsedClientHello {
	t.Helper()
	if len(raw) < 39 {
		t.Fatalf("short ClientHello: %d", len(raw))
	}
	p := raw[4+2+32:]
	sessionIDLen := int(p[0])
	p = p[1+sessionIDLen:]
	cipherLen := int(binary.BigEndian.Uint16(p[:2]))
	out := &parsedClientHello{}
	for i := 2; i < 2+cipherLen; i += 2 {
		if v := binary.BigEndian.Uint16(p[i : i+2]); !isGREASEValue(v) {
			out.ciphers = append(out.ciphers, v)
		}
	}
	p = p[2+cipherLen:]
	p = p[1+int(p[0]):]
	p = p[2:]
	for len(p) >= 4 {
		extensionType := binary.BigEndian.Uint16(p[:2])
		extensionLen := int(binary.BigEndian.Uint16(p[2:4]))
		body := p[4 : 4+extensionLen]
		p = p[4+extensionLen:]
		if isGREASEValue(extensionType) {
			continue
		}
		out.extensions = append(out.extensions, extensionType)
		switch extensionType {
		case 13:
			for i := 2; i < len(body); i += 2 {
				out.sigAlgs = append(out.sigAlgs, binary.BigEndian.Uint16(body[i:i+2]))
			}
		case 16:
			for i := 2; i < len(body); {
				length := int(body[i])
				out.alpn = append(out.alpn, string(body[i+1:i+1+length]))
				i += 1 + length
			}
		case 10:
			for i := 2; i < len(body); i += 2 {
				if v := binary.BigEndian.Uint16(body[i : i+2]); !isGREASEValue(v) {
					out.groups = append(out.groups, v)
				}
			}
		case 51:
			for i := 2; i+4 <= len(body); {
				group := binary.BigEndian.Uint16(body[i : i+2])
				length := int(binary.BigEndian.Uint16(body[i+2 : i+4]))
				if !isGREASEValue(group) {
					out.keyShares = append(out.keyShares, group)
				}
				if group == uint16(x25519MLKEM768) {
					out.hybridShare = append([]byte(nil), body[i+4:i+4+length]...)
				}
				i += 4 + length
			}
		}
	}
	return out
}

func (c *parsedClientHello) ja4() string {
	sni := "i"
	for _, extension := range c.extensions {
		if extension == 0 {
			sni = "d"
			break
		}
	}
	alpn := "00"
	if len(c.alpn) > 0 {
		first := c.alpn[0]
		alpn = string(first[0]) + string(first[len(first)-1])
	}
	hexList := func(values []uint16) []string {
		out := make([]string, 0, len(values))
		for _, value := range values {
			out = append(out, fmt.Sprintf("%04x", value))
		}
		return out
	}
	truncatedSHA := func(value string) string {
		sum := sha256.Sum256([]byte(value))
		return hex.EncodeToString(sum[:])[:12]
	}
	ciphers := hexList(c.ciphers)
	sort.Strings(ciphers)
	extensions := make([]string, 0, len(c.extensions))
	for _, extension := range c.extensions {
		if extension != 0 && extension != 16 {
			extensions = append(extensions, fmt.Sprintf("%04x", extension))
		}
	}
	sort.Strings(extensions)
	return fmt.Sprintf(
		"t13%s%02d%02d%s_%s_%s",
		sni,
		len(c.ciphers),
		len(c.extensions),
		alpn,
		truncatedSHA(strings.Join(ciphers, ",")),
		truncatedSHA(strings.Join(extensions, ",")+"_"+strings.Join(hexList(c.sigAlgs), ",")),
	)
}

func TestTLSFingerprintProfiles(t *testing.T) {
	want := []string{TLSFingerprintChrome120, TLSFingerprintChrome151}
	if got := AvailableClientHelloProfiles(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("profiles = %v, want %v", got, want)
	}
	if DefaultTLSFingerprint != TLSFingerprintChrome151 {
		t.Fatalf("default profile = %s", DefaultTLSFingerprint)
	}
}

func TestChrome151ClientHello(t *testing.T) {
	raw, preset, uConn := buildClientHelloForProfile(t, TLSFingerprintChrome151)
	parts := parseClientHelloForTest(t, raw)
	if got := parts.ja4(); got != chrome151JA4 {
		t.Fatalf("JA4 mismatch\n want %s\n  got %s", chrome151JA4, got)
	}
	if got, want := fmt.Sprint(parts.groups), "[4588 29 23 24]"; got != want {
		t.Fatalf("supported groups = %s, want %s", got, want)
	}
	if got, want := fmt.Sprint(parts.keyShares), "[4588 29]"; got != want {
		t.Fatalf("key shares = %s, want %s", got, want)
	}
	if got, want := fmt.Sprint(parts.sigAlgs), "[2308 2309 2310 1027 2052 1025 1283 2053 1281 2054 1537]"; got != want {
		t.Fatalf("signature algorithms = %s, want %s", got, want)
	}
	if !containsUint16(parts.extensions, 17613) || containsUint16(parts.extensions, 17513) {
		t.Fatalf("ALPS codepoints = %v", parts.extensions)
	}
	var hasNativeALPS17613 bool
	for _, extension := range preset.spec.Extensions {
		switch ext := extension.(type) {
		case *utls.ApplicationSettingsExtensionNew:
			hasNativeALPS17613 = true
		case *utls.GenericExtension:
			if ext.Id == 17613 {
				t.Fatal("ALPS 17613 must use the fork's handshake-aware extension, not GenericExtension")
			}
		}
	}
	if !hasNativeALPS17613 {
		t.Fatal("ALPS 17613 extension is missing from the Chrome 151 preset")
	}
	if preset.applyState == nil || uConn.HandshakeState.State13.KEMKey == nil {
		t.Fatal("ML-KEM handshake state was not injected")
	}
	if uConn.HandshakeState.State13.KEMKey.CurveID != x25519MLKEM768 {
		t.Fatalf("KEM curve = %d", uConn.HandshakeState.State13.KEMKey.CurveID)
	}
}

func TestChrome151FreshKeySharePerConnection(t *testing.T) {
	first, _, _ := buildClientHelloForProfile(t, TLSFingerprintChrome151)
	second, _, _ := buildClientHelloForProfile(t, TLSFingerprintChrome151)
	firstShare := parseClientHelloForTest(t, first).hybridShare
	secondShare := parseClientHelloForTest(t, second).hybridShare
	if len(firstShare) != 1216 || len(secondShare) != 1216 {
		t.Fatalf("hybrid share sizes = %d, %d", len(firstShare), len(secondShare))
	}
	if string(firstShare) == string(secondShare) {
		t.Fatal("two connections reused an identical hybrid key share")
	}
}

func containsUint16(values []uint16, target uint16) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
