package main_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/internal/trafficoracle"
)

func udpOracleFixture() (trafficoracle.Evidence, string) {
	b := make([]byte, 86)
	ip := func(at, size int, src, dst []byte) {
		b[at], b[at+8], b[at+9] = 0x45, 64, 17
		binary.BigEndian.PutUint16(b[at+2:at+4], uint16(size))
		copy(b[at+12:at+16], src)
		copy(b[at+16:at+20], dst)
	}
	ip(14, 72, []byte{1, 2, 3, 4}, []byte{4, 3, 2, 1})
	ip(42, 28, []byte{5, 6, 7, 8}, []byte{8, 7, 6, 5})
	for _, at := range []int{34, 62} {
		binary.BigEndian.PutUint16(b[at:at+2], 42497)
		binary.BigEndian.PutUint16(b[at+2:at+4], 6635)
		binary.BigEndian.PutUint16(b[at+4:at+6], 8)
	}
	binary.BigEndian.PutUint16(b[38:40], 52)
	field := func(name string, at, n int, show string) string {
		value := ""
		if n != 0 {
			value = hex.EncodeToString(b[at : at+n])
		}
		return fmt.Sprintf(`<field name="%s" pos="%d" size="%d" show="%s" value="%s"/>`, name, at, n, show, value)
	}
	proto := func(at int, stream string) string {
		u := at + 20
		return fmt.Sprintf(`<proto name="ip" pos="%d" size="20">`, at) + field("ip.src", at+12, 4, "") + field("ip.dst", at+16, 4, "") + `</proto>` +
			fmt.Sprintf(`<proto name="udp" pos="%d" size="8">`, u) + field("udp.srcport", u, 2, "") + field("udp.dstport", u+2, 2, "") + field("udp.length", u+4, 2, "") + field("udp.checksum", u+6, 2, "") + field("udp.stream", u+8, 0, stream) + `</proto>`
	}
	// Same ports in two layers; deliberately put the inner layer first. An
	// occurrence-index or tuple-only matcher would select reference stream 1619.
	text := `<pdml><packet><proto name="frame" pos="0" size="86">` + field("frame.number", 0, 0, "11") + field("frame.interface_id", 0, 0, "225") + field("frame.cap_len", 0, 0, "86") + `</proto>` + proto(42, "1619") + proto(14, "931") + `</packet></pdml>`
	return trafficoracle.Evidence{Number: 11, Interface: 225, IPOffset: 14, Captured: b}, text
}

func udpOraclePacket(t *testing.T, text string) trafficoracle.Packet {
	t.Helper()
	var packets []trafficoracle.Packet
	if err := trafficoracle.ReadPDML(strings.NewReader(text), func(p trafficoracle.Packet) error { packets = append(packets, p); return nil }); err != nil || len(packets) != 1 {
		t.Fatalf("read reference: %v, packets=%d", err, len(packets))
	}
	return packets[0]
}

func TestUDPOracleMatchesCapturedLayer(t *testing.T) {
	for _, badLength := range []bool{false, true} {
		t.Run(fmt.Sprintf("invalidLength%v", badLength), func(t *testing.T) {
			e, text := udpOracleFixture()
			p := udpOraclePacket(t, text)
			// Section/encapsulation are independently frozen source metadata.
			// A per-frame projection must retain them, never coalesce domains.
			e.Section, e.Encapsulation = 2, "/vlan:100"
			if badLength {
				binary.BigEndian.PutUint16(e.Captured[38:40], 65500)
				for i := range p.Protocols[3].Fields {
					if p.Protocols[3].Fields[i].Name == "udp.length" {
						p.Protocols[3].Fields[i].Value = "ffdc"
					}
				}
			}
			o, err := trafficoracle.MatchDirectIPv4UDP(p, e)
			if err != nil || o.Stream != 931 || o.UDPOffset != 34 || o.Source != "1.2.3.4" || o.Destination != "4.3.2.1" || o.SourcePort != 42497 || o.ExtentConsistent == badLength || o.Section != e.Section || o.Encapsulation != e.Encapsulation {
				t.Fatalf("wrong outer observation: %+v, %v", o, err)
			}
			if len(p.Protocols) != 4 || p.Protocols[1].Position != 62 {
				t.Fatal("inner reference layer was discarded")
			}
		})
	}
}

func TestUDPOracleRefusesUnqualifiedJoins(t *testing.T) {
	cases := []struct {
		name string
		edit func(*trafficoracle.Packet, *trafficoracle.Evidence)
	}{
		{"frame", func(p *trafficoracle.Packet, e *trafficoracle.Evidence) { e.Number++ }},
		{"interface", func(p *trafficoracle.Packet, e *trafficoracle.Evidence) { e.Interface++ }},
		{"capture-length", func(p *trafficoracle.Packet, e *trafficoracle.Evidence) { p.CapturedLength-- }},
		{"negative-offset", func(p *trafficoracle.Packet, e *trafficoracle.Evidence) { e.IPOffset = -1 }},
		{"huge-offset", func(p *trafficoracle.Packet, e *trafficoracle.Evidence) { e.IPOffset = int(^uint(0) >> 1) }},
		{"short-header", func(p *trafficoracle.Packet, e *trafficoracle.Evidence) {
			e.Captured = e.Captured[:18]
			p.CapturedLength = 18
		}},
		{"wrong-version", func(p *trafficoracle.Packet, e *trafficoracle.Evidence) { e.Captured[14] = 0x65 }},
		{"fragment", func(p *trafficoracle.Packet, e *trafficoracle.Evidence) { e.Captured[20] = 0x20 }},
		{"wrong-transport", func(p *trafficoracle.Packet, e *trafficoracle.Evidence) { e.Captured[23] = 6 }},
		{"source-bytes", func(p *trafficoracle.Packet, e *trafficoracle.Evidence) { e.Captured[26]++ }},
		{"port-position", func(p *trafficoracle.Packet, e *trafficoracle.Evidence) { p.Protocols[3].Fields[0].Position++ }},
		{"port-size", func(p *trafficoracle.Packet, e *trafficoracle.Evidence) { p.Protocols[3].Fields[0].Size++ }},
		{"port-bytes", func(p *trafficoracle.Packet, e *trafficoracle.Evidence) { p.Protocols[3].Fields[0].Value = "0000" }},
		{"missing-checksum", func(p *trafficoracle.Packet, e *trafficoracle.Evidence) {
			p.Protocols[3].Fields = append(p.Protocols[3].Fields[:3], p.Protocols[3].Fields[4:]...)
		}},
		{"missing-stream", func(p *trafficoracle.Packet, e *trafficoracle.Evidence) {
			p.Protocols[3].Fields = p.Protocols[3].Fields[:4]
		}},
		{"duplicate-stream", func(p *trafficoracle.Packet, e *trafficoracle.Evidence) {
			p.Protocols[3].Fields = append(p.Protocols[3].Fields, p.Protocols[3].Fields[4])
		}},
		{"duplicate-parent", func(p *trafficoracle.Packet, e *trafficoracle.Evidence) {
			p.Protocols = append(p.Protocols, p.Protocols[2], p.Protocols[3])
		}},
		{"only-inner", func(p *trafficoracle.Packet, e *trafficoracle.Evidence) { p.Protocols = p.Protocols[:2] }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e, text := udpOracleFixture()
			p := udpOraclePacket(t, text)
			c.edit(&p, &e)
			if _, err := trafficoracle.MatchDirectIPv4UDP(p, e); err == nil {
				t.Fatal("unqualified evidence joined to a reference stream")
			}
		})
	}
}

func TestUDPOracleReferenceMetadataIntegrity(t *testing.T) {
	_, good := udpOracleFixture()
	for name, text := range map[string]string{
		"truncated-XML":     strings.TrimSuffix(good, `</packet></pdml>`),
		"missing-frame":     strings.ReplaceAll(good, "frame.number", "unrelated.number"),
		"missing-interface": strings.ReplaceAll(good, "frame.interface_id", "unrelated.interface"),
		"bad-position":      strings.Replace(good, `pos="34"`, `pos="-1"`, 1),
		"oversized-field":   strings.Replace(good, `value="01020304"`, `value="`+strings.Repeat("a", 129)+`"`, 1),
		"duplicate-frame":   strings.Replace(good, `</pdml>`, strings.TrimSuffix(strings.TrimPrefix(good, `<pdml>`), `</pdml>`)+`</pdml>`, 1),
		"nested-transport":  strings.Replace(good, `<field name="udp.checksum" pos="40" size="2" show="" value="0000"/>`, `<proto name="udp" pos="40" size="8"/>`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if err := trafficoracle.ReadPDML(strings.NewReader(text), func(trafficoracle.Packet) error { return nil }); err == nil {
				t.Fatal("corrupt reference accepted")
			}
		})
	}
}

func TestUDPOracleRetainsNestedReferenceDiagnostics(t *testing.T) {
	e, text := udpOracleFixture()
	needle := `<field name="udp.checksum" pos="40" size="2" show="" value="0000"/>`
	text = strings.Replace(text, needle, strings.TrimSuffix(needle, `/>`)+`><proto name="_ws.malformed"><field name="udp.srcport" pos="34" size="2" value="dead"/></proto></field>`, 1)
	p := udpOraclePacket(t, text)
	o, err := trafficoracle.MatchDirectIPv4UDP(p, e)
	if err != nil || o.Stream != 931 || len(o.ReferenceDiagnostics) != 1 || o.ReferenceDiagnostics[0] != "_ws.malformed" {
		t.Fatalf("diagnostic discarded or used as header bytes: %+v %v", o, err)
	}
}

func TestUDPOracleCommandRequiresCompleteScope(t *testing.T) {
	dir := t.TempDir()
	goExe := filepath.Join(runtime.GOROOT(), "bin", "go")
	exe := filepath.Join(dir, "udp-projection")
	if runtime.GOOS == "windows" {
		goExe += ".exe"
		exe += ".exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, goExe, "build", "-p", "1", "-o", exe, "../../internal/trafficoracle/cmd/udp-projection")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build actual comparator: %v\n%s", err, output)
	}
	e, pdml := udpOracleFixture()
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"valid", "missing-reference", "duplicate-evidence", "trailing-JSON", "existing-output"} {
		t.Run(name, func(t *testing.T) {
			d := t.TempDir()
			evidence, reference, output := filepath.Join(d, "evidence.jsonl"), filepath.Join(d, "reference.pdml"), filepath.Join(d, "result.jsonl")
			input := append(bytes.Clone(raw), '\n')
			if name == "missing-reference" {
				other := e
				other.Number++
				b, _ := json.Marshal(other)
				input = append(input, append(b, '\n')...)
			} else if name == "duplicate-evidence" {
				input = append(input, input...)
			} else if name == "trailing-JSON" {
				input = append(bytes.Clone(raw), []byte(" {}\n")...)
			}
			if err := os.WriteFile(evidence, input, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(reference, []byte(pdml), 0600); err != nil {
				t.Fatal(err)
			}
			if name == "existing-output" {
				if err := os.WriteFile(output, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.CommandContext(ctx, exe, "-evidence", evidence, "-pdml", reference, "-output", output, "-capture-sha256", strings.Repeat("a", 64))
			result, err := cmd.CombinedOutput()
			if name == "valid" {
				var summary struct {
					Matched int `json:"matched_frames"`
				}
				if err != nil || json.Unmarshal(result, &summary) != nil || summary.Matched != 1 {
					t.Fatalf("complete comparison: %v %s", err, result)
				}
				b, err := os.ReadFile(output)
				var o trafficoracle.UDPObservation
				if err != nil || json.Unmarshal(b, &o) != nil || o.Stream != 931 {
					t.Fatalf("wrong actual command output: %s %v", b, err)
				}
			} else {
				if err == nil {
					t.Fatalf("incomplete or unsafe comparison succeeded: %s", result)
				}
				b, err := os.ReadFile(output)
				if name == "existing-output" {
					if err != nil || string(b) != "keep" {
						t.Fatal("existing output changed")
					}
				} else if !os.IsNotExist(err) {
					t.Fatal("failed comparison left a partial success artifact")
				}
			}
		})
	}
}

// Every address/header octet is required evidence. A neighboring reference
// layer with identical ports must never rescue a corrupted source join.
func FuzzUDPOracleCapturedHeaderJoin(f *testing.F) {
	for _, seed := range [][2]byte{{0, 1}, {7, 255}, {15, 1}, {8, 0}} {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, selected, delta byte) {
		e, text := udpOracleFixture()
		p := udpOraclePacket(t, text)
		at := []int{26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41}[int(selected)%16]
		e.Captured[at] ^= delta
		o, err := trafficoracle.MatchDirectIPv4UDP(p, e)
		if delta != 0 && err == nil {
			t.Fatalf("changed source octet %d matched reference stream %d", at, o.Stream)
		}
		if delta == 0 && (err != nil || o.Stream != 931) {
			t.Fatalf("unchanged complete source rejected: %+v %v", o, err)
		}
	})
}
