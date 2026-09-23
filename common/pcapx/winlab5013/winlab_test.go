package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCapturesParseTwiceAndRejectDamage(t *testing.T) {
	for _, b := range allBundles() {
		pcaps, critical, err := bundleCritical(b)
		if err != nil {
			t.Fatalf("%s: %v", b.Name, err)
		}
		again, _, err := bundleCritical(b)
		if err != nil {
			t.Fatalf("%s second: %v", b.Name, err)
		}
		second, err := verifyNamed(b.Caps[0].File, again[b.Caps[0].File])
		if err != nil {
			t.Fatal(err)
		}
		first, err := verifyNamed(b.Caps[0].File, pcaps[b.Caps[0].File])
		if err != nil || first != second {
			t.Fatalf("%s not stable", b.Name)
		}
		readme := renderREADME(b, pcaps, critical)
		if !strings.Contains(readme, "Wireshark") || !strings.Contains(readme, "BEGIN CRITICAL") || !strings.Contains(readme, critical) {
			t.Fatalf("%s readme missing critical or wireshark notes", b.Name)
		}
		for _, c := range b.Caps {
			if !strings.Contains(c.Shark, "过滤器") && !strings.Contains(c.Shark, "`") {
				t.Fatalf("%s missing shark note", c.File)
			}
			if c.Unfinished == "" || c.Gap == "" || c.Metrics == "" {
				t.Fatalf("%s incomplete doc", c.File)
			}
			if _, err := verifyNamed(c.File, nil); err == nil {
				t.Fatalf("%s accepted a missing capture", c.File)
			}
			damaged := append([]byte{}, pcaps[c.File]...)
			if len(damaged) < 32 {
				t.Fatalf("%s too small", c.File)
			}
			damaged = damaged[:len(damaged)/2]
			if _, err := verifyNamed(c.File, damaged); err == nil {
				t.Fatalf("%s accepted a truncated capture", c.File)
			}
		}
		if strings.Contains(b.Name, "ctf-") {
			for _, raw := range pcaps {
				out, err := verifyNamed(onlyName(b), raw)
				if err != nil {
					t.Fatal(err)
				}
				flag := flagOf(out)
				if flag == "" || flag == "flag{not-this}" || flag == "flag{not-the-ftp-flag}" || bytes.Contains(raw, []byte(flag)) {
					t.Fatalf("%s flag %q is missing, a decoy, or still contiguous in the file", b.Name, flag)
				}
			}
		}
	}
}

func TestCommittedArchives(t *testing.T) {
	for _, b := range allBundles() {
		raw, err := os.ReadFile(filepath.Join("zips", b.Name))
		if err != nil {
			t.Fatalf("committed %s: %v", b.Name, err)
		}
		if len(raw) >= 100*1024*1024 {
			t.Fatalf("%s is %d bytes", b.Name, len(raw))
		}
		zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			t.Fatal(err)
		}
		readme, err := readZipREADME(zr)
		if err != nil {
			t.Fatal(err)
		}
		got, err := criticalFromREADME(readme)
		if err != nil {
			t.Fatal(err)
		}
		_, critical, err := bundleCritical(b)
		if err != nil {
			t.Fatal(err)
		}
		if got != critical {
			t.Fatalf("committed %s critical does not match the parser", b.Name)
		}
		for _, c := range b.Caps {
			f, err := findZip(zr, "captures/"+c.File)
			if err != nil {
				t.Fatal(err)
			}
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			buf, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(buf, buildCapture(c)) {
				t.Fatalf("committed %s is not the generated capture", c.File)
			}
		}
	}
}

func readZipCapture(t *testing.T, zipName, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("zips", zipName))
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	f, err := findZip(zr, name)
	if err != nil {
		t.Fatal(err)
	}
	rc, err := f.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	buf, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return buf
}

func TestShippedWireContracts(t *testing.T) {
	bacFrames, err := parsePcapng(readZipCapture(t, "topic-ics.zip", "captures/ics-02-bacnet.pcapng"))
	if err != nil {
		t.Fatal(err)
	}
	var readInv, writeInv int
	var sawRead, sawWrite, sawReadAck, sawWriteAck bool
	for _, fr := range bacFrames {
		if fr.IPProto != 17 || len(fr.L4) < 8 || fr.L4[0] != 0x81 {
			continue
		}
		apdu := fr.L4[6:]
		if len(apdu) >= 11 && apdu[0] == 0x00 {
			if apdu[4] != 0x0C || apdu[9] != 0x19 || apdu[10] != 0x55 {
				t.Fatalf("BACnet confirmed request missing context tag 0 or property 85: %x", apdu)
			}
			switch int(apdu[3]) {
			case 12:
				readInv = int(apdu[2])
				sawRead = true
			case 0x0F:
				writeInv = int(apdu[2])
				sawWrite = true
			default:
				t.Fatalf("BACnet service %d", apdu[3])
			}
		}
		if len(apdu) >= 3 && apdu[0] == 0x30 {
			if int(apdu[1]) != readInv || int(apdu[2]) != 12 {
				t.Fatalf("ReadProperty ACK invoke/service = %d/%d, request invoke %d", apdu[1], apdu[2], readInv)
			}
			sawReadAck = true
		}
		if len(apdu) >= 3 && apdu[0] == 0x20 {
			if int(apdu[1]) != writeInv || int(apdu[2]) != 0x0F {
				t.Fatalf("WriteProperty ACK invoke/service = %d/%d, request invoke %d", apdu[1], apdu[2], writeInv)
			}
			sawWriteAck = true
		}
	}
	if !sawRead || !sawWrite || !sawReadAck || !sawWriteAck || readInv != 5 || writeInv != 6 {
		t.Fatalf("BACnet contract readInv=%d writeInv=%d flags %v %v %v %v", readInv, writeInv, sawRead, sawWrite, sawReadAck, sawWriteAck)
	}

	c37Frames, err := parsePcapng(readZipCapture(t, "topic-power.zip", "captures/power-03-c37118.pcapng"))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := tcpConns(c37Frames, 4712)
	if err != nil || len(cs) != 1 {
		t.Fatal(err)
	}
	var format uint16
	var nominal int
	sawData := false
	for _, side := range [][]byte{cs[0].c2s, cs[0].s2c} {
		b := side
		for len(b) > 0 {
			if b[0] != 0xAA || len(b) < 4 {
				t.Fatalf("c37 sync")
			}
			n := int(binary.BigEndian.Uint16(b[2:4]))
			if n > len(b) {
				t.Fatal("c37 size")
			}
			frame := b[:n]
			switch frame[1] & 0x70 {
			case 0x30:
				format = binary.BigEndian.Uint16(frame[38:40])
				if format&0x0002 == 0 || format&0x0008 == 0 || format&0x0001 != 0 {
					t.Fatalf("FORMAT %#x is not float-phasor|float-freq rectangular", format)
				}
				nph := int(binary.BigEndian.Uint16(frame[40:42]))
				nan := int(binary.BigEndian.Uint16(frame[42:44]))
				ndg := int(binary.BigEndian.Uint16(frame[44:46]))
				fnomAt := 62 + nph*4 + nan*4 + ndg*4
				if binary.BigEndian.Uint16(frame[fnomAt:fnomAt+2])&1 != 0 {
					nominal = 50
				} else {
					nominal = 60
				}
			case 0x00:
				ph, frw := 4, 4
				if format&0x0002 != 0 {
					ph = 8
				}
				if format&0x0008 != 0 {
					frw = 8
				}
				nph := int(binary.BigEndian.Uint16(mustConfig(cs[0].s2c, 40)))
				meas := 2 + nph*ph + frw
				if n != 14+meas+2 {
					t.Fatalf("data width %d, config implies %d", n, 14+meas+2)
				}
				sawData = true
			}
			b = b[n:]
		}
	}
	if !sawData || nominal == 0 {
		t.Fatalf("c37 data=%v nominal=%d format=%#x", sawData, nominal, format)
	}
	if _, err := parseC37(c37Frames); err != nil {
		t.Fatal(err)
	}

	dhtFrames, err := parsePcapng(readZipCapture(t, "win-protocols-20.zip", "captures/16-bittorrent-dht.pcapng"))
	if err != nil {
		t.Fatal(err)
	}
	_, replies, err := udpByPort(dhtFrames, 6881)
	if err != nil {
		t.Fatal(err)
	}
	sawNodes := false
	for _, p := range replies {
		v, rest, err := bdecode(string(p))
		if err != nil || rest != "" {
			t.Fatal(err)
		}
		m := v.(map[string]any)
		body := m["r"].(map[string]any)
		if nodes, ok := body["nodes"].(string); ok {
			if len(nodes)%26 != 0 || len(nodes) == 0 {
				t.Fatalf("nodes len %d", len(nodes))
			}
			sawNodes = true
		}
	}
	if !sawNodes {
		t.Fatal("committed DHT reply has no compact nodes")
	}
	if _, err := parseDHT(dhtFrames); err != nil {
		t.Fatal(err)
	}
}

func mustConfig(stream []byte, off int) []byte {
	b := stream
	for len(b) > 4 && b[0] == 0xAA {
		n := int(binary.BigEndian.Uint16(b[2:4]))
		if b[1]&0x70 == 0x30 {
			return b[off : off+2]
		}
		b = b[n:]
	}
	return []byte{0, 0}
}

func onlyName(b zipBundle) string { return b.Caps[0].File }

func flagOf(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "flag=") {
			return strings.TrimPrefix(line, "flag=")
		}
	}
	return ""
}

func TestZipVerifierMatchesCapture(t *testing.T) {
	for _, b := range allBundles() {
		raw, err := zipBytes(b)
		if err != nil {
			t.Fatalf("pack %s: %v", b.Name, err)
		}
		if len(raw) >= 100*1024*1024 {
			t.Fatalf("%s too big", b.Name)
		}
		zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			t.Fatal(err)
		}
		readme, err := readZipREADME(zr)
		if err != nil {
			t.Fatal(err)
		}
		got, err := criticalFromREADME(readme)
		if err != nil {
			t.Fatal(err)
		}
		_, critical, err := bundleCritical(b)
		if err != nil {
			t.Fatal(err)
		}
		if got != critical {
			t.Fatalf("%s readme critical drifted", b.Name)
		}
		dir := t.TempDir()
		for _, f := range zr.File {
			target := filepath.Join(dir, filepath.FromSlash(f.Name))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatal(err)
			}
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			buf, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, buf, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		goBin := filepath.Join(runtime.GOROOT(), "bin", "go.exe")
		if _, err := os.Stat(goBin); err != nil {
			goBin = filepath.Join(runtime.GOROOT(), "bin", "go")
		}
		var last string
		for i := 0; i < 2; i++ {
			cmd := exec.Command(goBin, "run", ".")
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s run %d: %v\n%s", b.Name, i+1, err, out)
			}
			if i == 1 && string(out) != last {
				t.Fatalf("%s runs differ", b.Name)
			}
			last = string(out)
		}
		if last != critical {
			t.Fatalf("%s go run output != parsed critical\n%s", b.Name, last)
		}
		capture := filepath.Join(dir, "captures", b.Caps[0].File)
		cut, err := os.ReadFile(capture)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(capture, cut[:len(cut)/2], 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(goBin, "run", ".")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err == nil {
			t.Fatalf("%s still succeeded after truncation:\n%s", b.Name, out)
		}
	}
}
