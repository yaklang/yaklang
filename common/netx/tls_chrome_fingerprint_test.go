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

const chromeJA4 = "t13d1516h2_8daaf6152771_806a8c22fdea"

func isGREASE(v uint16) bool {
	return v&0x0f0f == 0x0a0a
}

// buildChromeClientHello 用 ChromeClientHelloSpec 生成真实的 ClientHello 报文
func buildChromeClientHello(t *testing.T) []byte {
	t.Helper()
	spec, err := ChromeClientHelloSpec()
	if err != nil {
		t.Fatalf("build chrome client hello spec: %v", err)
	}
	uconn := utls.UClient(nil, &utls.Config{ServerName: "tls.peet.ws"}, utls.HelloCustom)
	if err := uconn.ApplyPreset(spec); err != nil {
		t.Fatalf("apply preset: %v", err)
	}
	if err := uconn.BuildHandshakeState(); err != nil {
		t.Fatalf("build handshake state: %v", err)
	}
	raw := uconn.HandshakeState.Hello.Raw
	if len(raw) == 0 {
		t.Fatal("empty client hello")
	}
	return raw
}

type clientHelloParts struct {
	ciphers    []uint16
	extensions []uint16
	sigAlgs    []uint16
	alpn       []string
	versions   []uint16
	groups     []uint16
	keyShares  []uint16
}

// parseClientHello 从 ClientHello 报文体中取出指纹相关字段，GREASE 一律剔除
func parseClientHello(t *testing.T, raw []byte) *clientHelloParts {
	t.Helper()
	// handshake header(4) + legacy_version(2) + random(32)
	p := raw[4+2+32:]
	sessionIDLen := int(p[0])
	p = p[1+sessionIDLen:]

	cipherLen := int(binary.BigEndian.Uint16(p[:2]))
	out := &clientHelloParts{}
	for i := 2; i < 2+cipherLen; i += 2 {
		if v := binary.BigEndian.Uint16(p[i : i+2]); !isGREASE(v) {
			out.ciphers = append(out.ciphers, v)
		}
	}
	p = p[2+cipherLen:]
	p = p[1+int(p[0]):] // compression methods
	p = p[2:]           // extensions total length

	for len(p) >= 4 {
		extType := binary.BigEndian.Uint16(p[:2])
		extLen := int(binary.BigEndian.Uint16(p[2:4]))
		body := p[4 : 4+extLen]
		p = p[4+extLen:]
		if isGREASE(extType) {
			continue
		}
		out.extensions = append(out.extensions, extType)

		switch extType {
		case 0x000d: // signature_algorithms
			for i := 2; i < len(body); i += 2 {
				out.sigAlgs = append(out.sigAlgs, binary.BigEndian.Uint16(body[i:i+2]))
			}
		case 0x0010: // application_layer_protocol_negotiation
			for i := 2; i < len(body); {
				l := int(body[i])
				out.alpn = append(out.alpn, string(body[i+1:i+1+l]))
				i += 1 + l
			}
		case 0x002b: // supported_versions
			for i := 1; i < len(body); i += 2 {
				if v := binary.BigEndian.Uint16(body[i : i+2]); !isGREASE(v) {
					out.versions = append(out.versions, v)
				}
			}
		case 0x000a: // supported_groups
			for i := 2; i < len(body); i += 2 {
				if v := binary.BigEndian.Uint16(body[i : i+2]); !isGREASE(v) {
					out.groups = append(out.groups, v)
				}
			}
		case 0x0033: // key_share
			for i := 2; i+4 <= len(body); {
				g := binary.BigEndian.Uint16(body[i : i+2])
				l := int(binary.BigEndian.Uint16(body[i+2 : i+4]))
				if !isGREASE(g) {
					out.keyShares = append(out.keyShares, g)
				}
				i += 4 + l
			}
		}
	}
	return out
}

func (c *clientHelloParts) ja4() string {
	version := "13"
	sni := "i"
	for _, v := range c.extensions {
		if v == 0x0000 {
			sni = "d"
		}
	}
	alpn := "00"
	if len(c.alpn) > 0 {
		first := c.alpn[0]
		alpn = string(first[0]) + string(first[len(first)-1])
	}

	hexList := func(vs []uint16) []string {
		out := make([]string, 0, len(vs))
		for _, v := range vs {
			out = append(out, fmt.Sprintf("%04x", v))
		}
		return out
	}
	truncSHA := func(s string) string {
		sum := sha256.Sum256([]byte(s))
		return hex.EncodeToString(sum[:])[:12]
	}

	ciphers := hexList(c.ciphers)
	sort.Strings(ciphers)

	exts := make([]string, 0, len(c.extensions))
	for _, v := range c.extensions {
		if v == 0x0000 || v == 0x0010 { // JA4 排除 SNI 与 ALPN
			continue
		}
		exts = append(exts, fmt.Sprintf("%04x", v))
	}
	sort.Strings(exts)

	a := fmt.Sprintf("t%sd%02d%02d%s", version, len(c.ciphers), len(c.extensions), alpn)
	if sni == "i" {
		a = fmt.Sprintf("t%si%02d%02d%s", version, len(c.ciphers), len(c.extensions), alpn)
	}
	b := truncSHA(strings.Join(ciphers, ","))
	cc := truncSHA(strings.Join(exts, ",") + "_" + strings.Join(hexList(c.sigAlgs), ","))
	return a + "_" + b + "_" + cc
}

func TestChromeClientHelloSpec_JA4(t *testing.T) {
	got := parseClientHello(t, buildChromeClientHello(t)).ja4()
	if got != chromeJA4 {
		t.Fatalf("JA4 mismatch\n want %s\n  got %s", chromeJA4, got)
	}
}

func TestChromeClientHelloSpec_PostQuantum(t *testing.T) {
	parts := parseClientHello(t, buildChromeClientHello(t))

	wantGroups := []uint16{uint16(utls.X25519MLKEM768), uint16(utls.X25519), uint16(utls.CurveP256), uint16(utls.CurveP384)}
	if fmt.Sprint(parts.groups) != fmt.Sprint(wantGroups) {
		t.Errorf("supported_groups = %v, want %v", parts.groups, wantGroups)
	}

	wantKeyShares := []uint16{uint16(utls.X25519MLKEM768), uint16(utls.X25519)}
	if fmt.Sprint(parts.keyShares) != fmt.Sprint(wantKeyShares) {
		t.Errorf("key_share = %v, want %v", parts.keyShares, wantKeyShares)
	}

	wantSigAlgs := []uint16{0x0904, 0x0905, 0x0906, 0x0403, 0x0804, 0x0401, 0x0503, 0x0805, 0x0501, 0x0806, 0x0601}
	if fmt.Sprint(parts.sigAlgs) != fmt.Sprint(wantSigAlgs) {
		t.Errorf("signature_algorithms = %v, want %v", parts.sigAlgs, wantSigAlgs)
	}
}

func TestChromeClientHelloSpec_ApplicationSettingsCodepoint(t *testing.T) {
	parts := parseClientHello(t, buildChromeClientHello(t))
	var hasNew, hasOld bool
	for _, ext := range parts.extensions {
		switch ext {
		case 17613:
			hasNew = true
		case 17513:
			hasOld = true
		}
	}
	if !hasNew {
		t.Error("application_settings 新码点 17613 缺失")
	}
	if hasOld {
		t.Error("application_settings 老码点 17513 不应出现")
	}
}

// 扩展顺序与 GREASE 取值应逐次随机，JA4 则保持稳定
func TestChromeClientHelloSpec_ShuffledPerCall(t *testing.T) {
	seen := make(map[string]struct{})
	for i := 0; i < 16; i++ {
		parts := parseClientHello(t, buildChromeClientHello(t))
		if got := parts.ja4(); got != chromeJA4 {
			t.Fatalf("JA4 不稳定: want %s, got %s", chromeJA4, got)
		}
		seen[fmt.Sprint(parts.extensions)] = struct{}{}
	}
	if len(seen) < 2 {
		t.Error("扩展顺序未随机化")
	}
}
