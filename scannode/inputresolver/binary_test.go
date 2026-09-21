package inputresolver

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

func binaryWorkspace(t *testing.T, data []byte) (*Workspace, string) {
	t.Helper()
	root := t.TempDir()
	name := "inputs/fixture.bin"
	if err := os.MkdirAll(filepath.Join(root, "inputs"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, name), data, 0400); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &Workspace{root: root, ctx: ctx, cancel: cancel, manifest: &aiv1.InputManifest{Resources: []*aiv1.InputResource{{ResourceId: "fixture", RelativePath: name, SizeBytes: uint64(len(data)), Sha256: hex.EncodeToString(sum[:])}}}}, name
}

func captureFixture(t *testing.T, ng bool, count int) []byte {
	t.Helper()
	var output bytes.Buffer
	// Ethernet + IPv4 + UDP: 192.0.2.1:12345 -> 198.51.100.2:9999.
	packet := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 8, 0, 0x45, 0, 0, 28, 0, 0, 0, 0, 64, 17, 0, 0, 192, 0, 2, 1, 198, 51, 100, 2, 0x30, 0x39, 0x27, 0x0f, 0, 8, 0, 0}
	ci := gopacket.CaptureInfo{CaptureLength: len(packet), Length: len(packet)}
	if ng {
		w, err := pcapgo.NewNgWriter(&output, layers.LinkTypeEthernet)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < count; i++ {
			if err := w.WritePacket(ci, packet); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.Flush(); err != nil {
			t.Fatal(err)
		}
	} else {
		w := pcapgo.NewWriter(&output)
		if err := w.WriteFileHeader(65535, layers.LinkTypeEthernet); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < count; i++ {
			if err := w.WritePacket(ci, packet); err != nil {
				t.Fatal(err)
			}
		}
	}
	return output.Bytes()
}

func TestParsePacketCaptureKnownEndpointsAndProvenance(t *testing.T) {
	for _, ng := range []bool{false, true} {
		data := captureFixture(t, ng, 1)
		w, name := binaryWorkspace(t, data)
		result, err := w.ParsePacketCapture(context.Background(), name)
		if err != nil {
			t.Fatal(err)
		}
		packets := result["packets"].([]map[string]any)
		if len(packets) != 1 || packets[0]["source_ip"] != "192.0.2.1" || packets[0]["destination_ip"] != "198.51.100.2" || packets[0]["source_port"] != "12345" || packets[0]["destination_port"] != "9999" {
			t.Fatalf("observations: %#v", packets)
		}
		if result["sha256"] != w.manifest.Resources[0].Sha256 || result["resource_id"] != "fixture" {
			t.Fatalf("provenance: %#v", result)
		}
		refs := w.MaterialReferences()
		if len(refs) != 1 || refs[0].Operations[0] != "parse_packet_capture" {
			t.Fatalf("access event missing: %#v", refs)
		}
	}
}

func TestParsePacketCaptureBoundsAndMalformed(t *testing.T) {
	w, name := binaryWorkspace(t, captureFixture(t, false, maxCapturePackets+1))
	result, err := w.ParsePacketCapture(context.Background(), name)
	if err != nil || result["truncated"] != true || result["packet_count"] != maxCapturePackets {
		t.Fatalf("bound: %#v %v", result, err)
	}
	data := captureFixture(t, false, 1)
	binary.LittleEndian.PutUint32(data[32:], 0x7fffffff)
	for _, data := range [][]byte{[]byte("not a capture"), data, captureFixture(t, true, 1)[:30]} {
		w, name := binaryWorkspace(t, data)
		if _, err := w.ParsePacketCapture(context.Background(), name); err == nil {
			t.Fatal("accepted malformed capture")
		}
	}
}

func apkFixture(t *testing.T, manifest []byte, extra string) []byte {
	t.Helper()
	var b bytes.Buffer
	writer := zip.NewWriter(&b)
	for name, content := range map[string][]byte{"AndroidManifest.xml": manifest, "classes.dex": []byte("unused bytecode")} {
		e, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if extra != "" {
		e, err := writer.Create(extra)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// A minimal Android binary XML document with a UTF-8 string pool, typed
// attributes and balanced start/end chunks; not a text XML stand-in.
func binaryManifestFixture() []byte {
	stringsTable := []string{"manifest", "package", "owned.fixture", "uses-permission", "name", "android.permission.INTERNET", androidNamespace}
	var raw []byte
	offsets := []uint32{}
	for _, s := range stringsTable {
		offsets = append(offsets, uint32(len(raw)))
		raw = append(raw, byte(len(s)), byte(len(s)))
		raw = append(raw, []byte(s)...)
		raw = append(raw, 0)
	}
	for len(raw)%4 != 0 {
		raw = append(raw, 0)
	}
	pool := make([]byte, 28+len(offsets)*4)
	binary.LittleEndian.PutUint16(pool, 1)
	binary.LittleEndian.PutUint16(pool[2:], 28)
	binary.LittleEndian.PutUint32(pool[4:], uint32(len(pool)+len(raw)))
	binary.LittleEndian.PutUint32(pool[8:], uint32(len(offsets)))
	binary.LittleEndian.PutUint32(pool[16:], 0x100)
	binary.LittleEndian.PutUint32(pool[20:], uint32(len(pool)))
	for i, o := range offsets {
		binary.LittleEndian.PutUint32(pool[28+i*4:], o)
	}
	pool = append(pool, raw...)
	element := func(tag, key, value uint32, start bool) []byte {
		size := 24
		typ := uint16(0x103)
		if start {
			size = 56
			typ = 0x102
		}
		b := make([]byte, size)
		binary.LittleEndian.PutUint16(b, typ)
		binary.LittleEndian.PutUint16(b[2:], 16)
		binary.LittleEndian.PutUint32(b[4:], uint32(size))
		binary.LittleEndian.PutUint32(b[20:], tag)
		binary.LittleEndian.PutUint32(b[16:], 0xffffffff)
		if start {
			binary.LittleEndian.PutUint16(b[24:], 20)
			binary.LittleEndian.PutUint16(b[26:], 20)
			binary.LittleEndian.PutUint16(b[28:], 1)
			binary.LittleEndian.PutUint32(b[36:], 0xffffffff)
			if key == 4 {
				binary.LittleEndian.PutUint32(b[36:], 6)
			}
			binary.LittleEndian.PutUint32(b[40:], key)
			binary.LittleEndian.PutUint32(b[44:], 0xffffffff)
			binary.LittleEndian.PutUint16(b[48:], 8)
			b[51] = 3
			binary.LittleEndian.PutUint32(b[52:], value)
		}
		return b
	}
	out := make([]byte, 8)
	out = append(out, pool...)
	out = append(out, element(0, 1, 2, true)...)
	out = append(out, element(3, 4, 5, true)...)
	out = append(out, element(3, 0, 0, false)...)
	out = append(out, element(0, 0, 0, false)...)
	binary.LittleEndian.PutUint16(out, 3)
	binary.LittleEndian.PutUint16(out[2:], 8)
	binary.LittleEndian.PutUint32(out[4:], uint32(len(out)))
	return out
}

func TestParseAndroidPackageKnownManifest(t *testing.T) {
	for _, manifest := range [][]byte{[]byte(`<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="owned.fixture"><uses-permission android:name="android.permission.INTERNET"/></manifest>`), binaryManifestFixture()} {
		w, name := binaryWorkspace(t, apkFixture(t, manifest, ""))
		result, err := w.ParseAndroidPackage(context.Background(), name)
		if err != nil {
			t.Fatal(err)
		}
		if result["manifest"].(map[string]string)["package"] != "owned.fixture" || strings.Join(result["permissions"].([]string), ",") != "android.permission.INTERNET" {
			t.Fatalf("metadata: %#v", result)
		}
		if len(w.MaterialReferences()) != 1 || result["sha256"] == "" {
			t.Fatal("missing provenance")
		}
	}
}

func TestManagedBinaryRejectsUnsafeInputs(t *testing.T) {
	for _, data := range [][]byte{[]byte("bad"), apkFixture(t, []byte(`<manifest package="x"/>`), "../escape"), apkFixture(t, []byte(strings.Repeat("x", maxAndroidManifestBytes+1)), ""), apkFixture(t, []byte(`<manifest>`), "")} {
		w, name := binaryWorkspace(t, data)
		if _, err := w.ParseAndroidPackage(context.Background(), name); err == nil {
			t.Fatal("accepted unsafe APK")
		}
	}
	for _, method := range []string{"pcap", "apk"} {
		w, name := binaryWorkspace(t, []byte("fixture"))
		call := w.ParsePacketCapture
		if method == "apk" {
			call = w.ParseAndroidPackage
		}
		if _, err := call(context.Background(), "/etc/passwd"); err == nil {
			t.Fatal("accepted host path")
		}
		if _, err := call(context.Background(), "inputs/other"); err == nil {
			t.Fatal("accepted unowned path")
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := call(ctx, name); err == nil || !strings.Contains(err.Error(), "input_cancelled") {
			t.Fatalf("cancellation: %v", err)
		}
		w.manifest.Resources[0].SizeBytes = maxBinaryInputBytes + 1
		if _, err := call(context.Background(), name); err == nil || !strings.Contains(err.Error(), "input_binary_limit") {
			t.Fatalf("size limit: %v", err)
		}
		w.manifest.Resources[0].SizeBytes = 7
		w.manifest.Resources[0].Sha256 = strings.Repeat("0", 64)
		if _, err := call(context.Background(), name); err == nil || !strings.Contains(err.Error(), "input_file_changed") {
			t.Fatalf("digest mismatch: %v", err)
		}
	}
}

func TestFailedBinaryParsingDoesNotRecordSuccessfulMaterialUse(t *testing.T) {
	for _, kind := range []string{"pcap", "apk"} {
		w, name := binaryWorkspace(t, []byte("not a valid binary fixture"))
		call := w.ParsePacketCapture
		if kind == "apk" {
			call = w.ParseAndroidPackage
		}
		if _, err := call(context.Background(), name); err == nil {
			t.Fatal("expected parse error")
		}
		if refs := w.MaterialReferences(); len(refs) != 0 {
			t.Fatalf("failed %s parsing recorded successful material usage: %#v", kind, refs)
		}
	}
}

func TestBinaryThroughTextReadsDoesNotRecordMaterialUse(t *testing.T) {
	malformedCapture := captureFixture(t, false, 1)[:23]
	for _, content := range [][]byte{malformedCapture, []byte("prefix\x00suffix"), []byte("prefix\xff"), []byte("prefix\xe4\xb8")} {
		for _, lines := range []bool{false, true} {
			w, name := binaryWorkspace(t, content)
			var err error
			if lines {
				_, err = w.ReadLines(context.Background(), name, 1, 20)
			} else {
				_, err = w.Read(context.Background(), name, 0, MaxReadBytes)
			}
			if err == nil {
				t.Fatalf("binary text read accepted: lines=%v content=%q", lines, content)
			}
			if refs := w.MaterialReferences(); len(refs) != 0 {
				t.Fatalf("failed text read recorded material use: %#v", refs)
			}
		}
	}
}

func TestTextReadPreservesUnicodePageBoundary(t *testing.T) {
	w, name := binaryWorkspace(t, []byte("中文🙂tail"))
	result, err := w.Read(context.Background(), name, 1, 4)
	if err != nil || result["content"] != "文" || result["next_offset"] != int64(6) {
		t.Fatalf("unicode page: %#v %v", result, err)
	}
	if len(w.MaterialReferences()) != 1 {
		t.Fatal("successful text read missing provenance")
	}
}

func TestAndroidArchiveExpansionAndCountLimits(t *testing.T) {
	for _, count := range []int{1, maxAndroidEntries + 1} {
		var b bytes.Buffer
		writer := zip.NewWriter(&b)
		for i := 0; i < count; i++ {
			header := &zip.FileHeader{Name: strings.Repeat("x", i+1), Method: zip.Store}
			if count == 1 {
				header.UncompressedSize64 = maxAndroidExpandedBytes + 1
			}
			if _, err := writer.CreateRaw(header); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		w, name := binaryWorkspace(t, b.Bytes())
		if _, err := w.ParseAndroidPackage(context.Background(), name); err == nil {
			t.Fatal("accepted archive bounds violation")
		}
	}
}

func TestAndroidBinaryXMLMalformedLengths(t *testing.T) {
	fixture := binaryManifestFixture()
	for i := 0; i < len(fixture); i++ {
		if _, _, err := androidElements(context.Background(), fixture[:i]); err == nil {
			t.Fatalf("accepted truncation %d", i)
		}
	}
	for i := 8; i < len(fixture); i++ {
		data := bytes.Clone(fixture)
		data[i] = 255
		_, _, _ = androidElements(context.Background(), data)
	}
}
