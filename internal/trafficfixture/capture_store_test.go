package trafficfixture

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestCentralCaptureStorePreservesEveryPhysicalSource(t *testing.T) {
	c, err := load()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(c.root, captureStoreManifest))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(c.root, captureStoreArchive))
	if err != nil {
		t.Fatal(err)
	}
	s, err := parseCaptureStore(manifest, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.files) != 38 {
		t.Fatalf("physical source inventory changed: %d", len(s.files))
	}
	for name, member := range s.index.Members {
		t.Run(name, func(t *testing.T) {
			data, found, err := s.read(name, maxCaptureStoreBytes)
			if err != nil || !found || int64(len(data)) != member.Bytes || fmt.Sprintf("%x", sha256.Sum256(data)) != member.SHA256 {
				t.Fatalf("source bytes changed: found=%t err=%v", found, err)
			}
			if _, err := os.Stat(filepath.Join(c.root, filepath.FromSlash(name))); !os.IsNotExist(err) {
				t.Fatalf("archived source still checked out: %s (%v)", name, err)
			}
			if _, _, err := s.read(name, member.Bytes-1); err == nil {
				t.Fatal("caller limit was ignored")
			}
		})
	}
	if err := Verify(); err != nil {
		t.Fatal(err)
	}
}

func captureStoreControl(t *testing.T, names []string, edit func(*captureStoreIndex)) ([]byte, []byte) {
	t.Helper()
	var buffer bytes.Buffer
	w := zip.NewWriter(&buffer)
	index := captureStoreIndex{Version: 1, Archive: captureStoreArchive, SourceHEAD: "92620d72b0c8a1d62ce923e4e69cc1fad5d4c66d", Members: map[string]Member{}}
	for _, name := range names {
		h := &zip.FileHeader{Name: name, Method: zip.Store}
		h.SetMode(0644)
		f, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte("wire")); err != nil {
			t.Fatal(err)
		}
		index.Members[name] = Member{Bytes: 4, SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("wire")))}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	index.ArchiveBytes, index.ArchiveSHA256 = int64(buffer.Len()), fmt.Sprintf("%x", sha256.Sum256(buffer.Bytes()))
	if edit != nil {
		edit(&index)
	}
	manifest, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	return manifest, buffer.Bytes()
}

func TestCentralCaptureStoreRejectsAmbiguousOrModifiedSources(t *testing.T) {
	for _, names := range [][]string{{"../escape.pcap"}, {"C:/escape.pcap"}, {"a\\b.pcap"}, {"/a.pcap"}, {"b.pcap", "a.pcap"}, {"a.pcap", "a.pcap"}, {"A.pcap", "a.pcap"}} {
		manifest, raw := captureStoreControl(t, names, nil)
		if _, err := parseCaptureStore(manifest, raw); err == nil {
			t.Fatalf("accepted ambiguous names %q", names)
		}
	}
	for _, edit := range []func(*captureStoreIndex){
		func(i *captureStoreIndex) { i.Version = 2 },
		func(i *captureStoreIndex) { i.ArchiveSHA256 = fmt.Sprintf("%064d", 0) },
		func(i *captureStoreIndex) { i.ArchiveBytes++ },
		func(i *captureStoreIndex) { i.Members["missing.pcap"] = i.Members["a.pcap"] },
		func(i *captureStoreIndex) { m := i.Members["a.pcap"]; m.Bytes++; i.Members["a.pcap"] = m },
	} {
		manifest, raw := captureStoreControl(t, []string{"a.pcap"}, edit)
		if _, err := parseCaptureStore(manifest, raw); err == nil {
			t.Fatal("accepted altered manifest")
		}
	}
	manifest, raw := captureStoreControl(t, []string{"a.pcap"}, func(i *captureStoreIndex) {
		m := i.Members["a.pcap"]
		m.SHA256 = fmt.Sprintf("%064d", 0)
		i.Members["a.pcap"] = m
	})
	s, err := parseCaptureStore(manifest, raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.read("a.pcap", 100); err == nil {
		t.Fatal("accepted member hash mismatch")
	}
}

func TestCentralCaptureConcurrentReadsAndPrivateMaterialization(t *testing.T) {
	root, err := RepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(root, "common/ai/aismoking/live/testdata/pcap_analysis.pcap")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, err := ReadFile(name)
			if err != nil || len(data) != 709 {
				t.Errorf("concurrent source read: bytes=%d error=%v", len(data), err)
				return
			}
			clear(data)
		}()
	}
	wg.Wait()
	dir := t.TempDir()
	filename, err := Materialize(name, dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(filename) != dir {
		t.Fatal("capture was extracted outside its private directory")
	}
	data, err := os.ReadFile(filename)
	if err != nil || len(data) != 709 || bytes.Equal(data, make([]byte, 709)) {
		t.Fatalf("private source ownership lost: %v", err)
	}
}
