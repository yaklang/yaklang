package trafficfixture

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/pcapgo"
)

func TestEnterpriseTrafficAnalysisBatchAliases(t *testing.T) {
	root, err := RepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	names, err := Names()
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "testdata/enterprise-traffic-analysis/"
	count := 0
	for _, name := range names {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		count++
		data, err := ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(data) == 0 {
			t.Fatalf("%s: empty fixture", name)
		}
	}
	if count != 23 {
		t.Fatalf("enterprise fixture aliases=%d, want 23", count)
	}
	raw, err := ReadBatch("validation-enterprise-traffic-analysis-20261030", "validation/expected.json")
	if err != nil {
		t.Fatal(err)
	}
	var answers FrozenExpectations
	if err := json.Unmarshal(raw, &answers); err != nil {
		t.Fatal(err)
	}
	if len(answers.Cases) != 10 {
		t.Fatalf("enterprise capture answers=%d, want 10", len(answers.Cases))
	}
	for _, item := range answers.Cases {
		t.Run(item.ID, func(t *testing.T) {
			raw, err := ReadFile(filepath.Join(root, filepath.FromSlash(item.Input.OriginalPath)))
			if err != nil {
				t.Fatal(err)
			}
			// Capture record facts are independent of application-layer decoders.
			var next func() ([]byte, gopacket.CaptureInfo, error)
			var link string
			if strings.HasSuffix(item.Input.File, ".pcapng") {
				reader, err := pcapgo.NewNgReader(bytes.NewReader(raw), pcapgo.DefaultNgReaderOptions)
				if err != nil {
					t.Fatal(err)
				}
				next, link = reader.ReadPacketData, reader.LinkType().String()
			} else {
				reader, err := pcapgo.NewReader(bytes.NewReader(raw))
				if err != nil {
					t.Fatal(err)
				}
				next, link = reader.ReadPacketData, reader.LinkType().String()
			}
			packets := 0
			for {
				_, _, err := next()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				packets++
			}
			if packets != item.Facts.PacketCount || link != item.Facts.LinkType {
				t.Fatalf("capture facts=%d/%s, oracle=%d/%s", packets, link, item.Facts.PacketCount, item.Facts.LinkType)
			}
			if item.ExpectedStatus != "partial" || len(item.Missing) == 0 || len(item.Expectations) != 1 || item.Expectations[0].Kind != "source_manifest_constraints" || item.Expectations[0].Expected.PacketCount != packets {
				t.Fatal("source manifest facts must retain their partial semantic scope")
			}
		})
	}
}

func TestFrozenBatchIntegrity(t *testing.T) {
	t.Run("standalone-inventory-workspace", workspaceStandaloneInventory)
	c, err := load()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range c.index.Packs {
		for name := range p.Members {
			t.Run("consolidated-storage/"+name, func(t *testing.T) {
				if _, err := readMember(p.data.physical, name); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
	var captures int
	unique := map[string]bool{}
	for _, b := range c.index.Batches {
		for name, member := range b.Members {
			t.Run(b.ID+"/"+name, func(t *testing.T) {
				data, err := readMember(b, name)
				if err != nil {
					t.Fatal(err)
				}
				if int64(len(data)) != member.Bytes {
					t.Fatal("incorrect size")
				}
			})
			if b.ID == "baseline-2fb090d177c3" && len(name) > 9 && name[:9] == "captures/" {
				captures++
				unique[member.SHA256] = true
			}
		}
	}
	if captures != 694 || len(unique) != 690 {
		t.Fatalf("baseline inventory: %d captures, %d unique hashes", captures, len(unique))
	}
}

func TestArchiveRejectsTampering(t *testing.T) {
	c, err := load()
	if err != nil {
		t.Fatal(err)
	}
	original := c.index.Batches[0]
	raw, err := readBoundedFile(filepath.Join(c.root, batchDir, original.File), maxBatch)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("compressed hash", func(t *testing.T) {
		copy := append([]byte(nil), raw...)
		copy[len(copy)/2] ^= 1
		b := *original
		if openBatch(copy, &b) == nil {
			t.Fatal("accepted modified archive")
		}
	})
	t.Run("password", func(t *testing.T) {
		b := *original
		b.Password = "incorrect"
		if _, err := readMember(&b, "validation/expected.json"); err == nil {
			t.Fatal("accepted wrong password")
		}
	})
	t.Run("member hash", func(t *testing.T) {
		b := *original
		b.Members = map[string]Member{}
		for name, m := range original.Members {
			b.Members[name] = m
		}
		m := b.Members["validation/expected.json"]
		m.SHA256 = fmt.Sprintf("%064d", 0)
		b.Members["validation/expected.json"] = m
		if _, err := readMember(&b, "validation/expected.json"); err == nil {
			t.Fatal("accepted modified member hash")
		}
	})
	for _, name := range []string{"../escape", "/absolute", "a/../../escape", "a\\escape", "C:/escape"} {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			w := zip.NewWriter(&buf)
			f, err := w.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			f.Write([]byte("x"))
			w.Close()
			b := &Batch{ID: "unsafe", SHA256: fmt.Sprintf("%x", sha256.Sum256(buf.Bytes())), Members: map[string]Member{name: {Bytes: 1, SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("x")))}}}
			if openBatch(buf.Bytes(), b) == nil {
				t.Fatal("accepted unsafe member")
			}
		})
	}
}

func TestReadsOwnTheirBytes(t *testing.T) {
	name := "validation/expected.json"
	first, err := ReadBatch("baseline-2fb090d177c3", name)
	if err != nil {
		t.Fatal(err)
	}
	want := first[0]
	first[0] ^= 255
	second, err := ReadBatch("baseline-2fb090d177c3", name)
	if err != nil {
		t.Fatal(err)
	}
	if second[0] != want {
		t.Fatal("read shared mutable bytes")
	}
}

func TestArchiveDirectoryLimits(t *testing.T) {
	for _, tc := range []struct {
		name      string
		size      uint64
		flags     uint16
		method    uint16
		mode      fs.FileMode
		duplicate bool
	}{
		{name: "oversized", size: maxMember + 1, flags: 1},
		{name: "symlink", size: 1, flags: 1, mode: fs.ModeSymlink | 0777},
		{name: "unencrypted", size: 1},
		{name: "strong-encryption", size: 1, flags: 0x41},
		{name: "unsupported-method", size: 1, flags: 1, method: 99},
		{name: "packed-codec-not-ordinary-archive", size: 1, flags: 1, method: 93},
		{name: "duplicate", size: 1, flags: 1, duplicate: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var raw bytes.Buffer
			w := zip.NewWriter(&raw)
			h := &zip.FileHeader{Name: "captures/input.pcap", Flags: tc.flags, Method: tc.method, UncompressedSize64: tc.size, CompressedSize64: 12}
			h.SetMode(tc.mode)
			f, err := w.CreateRaw(h)
			if err != nil {
				t.Fatal(err)
			}
			f.Write(make([]byte, 12))
			if tc.duplicate {
				copy := *h
				f, err = w.CreateRaw(&copy)
				if err != nil {
					t.Fatal(err)
				}
				f.Write(make([]byte, 12))
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			b := &Batch{ID: tc.name, SHA256: fmt.Sprintf("%x", sha256.Sum256(raw.Bytes())), Members: map[string]Member{h.Name: {Bytes: int64(tc.size), SHA256: fmt.Sprintf("%064d", 0)}}}
			if openBatch(raw.Bytes(), b) == nil {
				t.Fatal("accepted invalid archive directory")
			}
		})
	}
}

func TestMemberExpansionIsBounded(t *testing.T) {
	c, err := load()
	if err != nil {
		t.Fatal(err)
	}
	b := *c.index.Batches[0]
	b.Members = map[string]Member{}
	for name, m := range c.index.Batches[0].Members {
		b.Members[name] = m
	}
	m := b.Members["validation/expected.json"]
	m.Bytes = 1
	b.Members["validation/expected.json"] = m
	if _, err := readMember(&b, "validation/expected.json"); err == nil {
		t.Fatal("accepted expanded bytes beyond the indexed size")
	}
}

func TestMissingCaptureCannotFallBack(t *testing.T) {
	c, err := load()
	if err != nil {
		t.Fatal(err)
	}
	// A loose repository capture must not become a new, unversioned test input.
	name := filepath.Join(c.root, "internal/trafficfixture/unindexed.pcap")
	if err := os.WriteFile(name, []byte("loose"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(name) })
	if _, err := ReadFile(name); err == nil {
		t.Fatal("loaded unindexed capture")
	}
	outside := filepath.Join(t.TempDir(), "control.pcap")
	if err := os.WriteFile(outside, []byte("temporary control"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(outside); err != nil {
		t.Fatal(err)
	}
}
