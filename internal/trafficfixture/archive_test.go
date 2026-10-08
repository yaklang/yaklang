package trafficfixture

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestFrozenBatchIntegrity(t *testing.T) {
	c, err := load()
	if err != nil {
		t.Fatal(err)
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
	raw, err := os.ReadFile(filepath.Join(c.root, batchDir, original.File))
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
