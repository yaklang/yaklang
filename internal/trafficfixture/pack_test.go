package trafficfixture

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func deflateRecipe(t *testing.T, raw []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w, e := flate.NewWriter(&b, flate.BestSpeed)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = w.Write(raw); e != nil {
		t.Fatal(e)
	}
	if e = w.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestPackedRecipeRejectsMalformed(t *testing.T) {
	good := deflateRecipe(t, []byte{0, 2, 1})
	ids, e := decodePackedRecipe(good, 3, 2)
	if e != nil || fmt.Sprint(ids) != "[0 1 0]" {
		t.Fatalf("normal recipe %v %v", ids, e)
	}
	tests := []struct {
		name         string
		raw          []byte
		count, total int
	}{
		{"truncated-flate", good[:len(good)-1], 3, 2}, {"trailing-compressed-byte", append(append([]byte{}, good...), 0), 3, 2},
		{"negative-count", good, -1, 2}, {"count-budget", good, maxRecipeEntries + 1, 2}, {"chunk-count-budget", good, 3, maxPackedChunks + 1},
		{"declared-too-small", good, 2, 2}, {"declared-too-large", good, 4, 2}, {"zero-count-with-values", good, 0, 2},
		{"noncanonical-varint", deflateRecipe(t, []byte{0x80, 0}), 1, 2}, {"unterminated-varint", deflateRecipe(t, []byte{0x80}), 1, 2},
		{"negative-id", deflateRecipe(t, []byte{1}), 1, 2}, {"id-outside-table", deflateRecipe(t, []byte{4}), 1, 2},
		{"oversized-varint", deflateRecipe(t, []byte{0xff, 0xff, 0xff, 0xff, 0xff, 1}), 1, 2},
	}
	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			if _, e := decodePackedRecipe(c.raw, c.count, c.total); e == nil {
				t.Fatal("accepted malformed recipe")
			}
		})
	}
	if ids, e := decodePackedRecipe(deflateRecipe(t, nil), 0, 1); e != nil || len(ids) != 0 {
		t.Fatalf("empty member recipe %v %v", ids, e)
	}
}
func TestPackedManifestRejectsAmbiguousJSON(t *testing.T) {
	for _, raw := range []string{`{"version":2,"version":1}`, `{"version":2,"unknown":1}`, `{"recipes":{"x":{"bytes":1,"bytes":2}}}`, `{"version":2} {}`, `{"version":true}`, `{"version":2,"recipes":{"x":{"offset":9223372036854775808}}}`} {
		t.Run(raw, func(t *testing.T) {
			var m packedManifest
			if strictPackJSON([]byte(raw), &m) == nil {
				t.Fatal("accepted ambiguous/invalid manifest")
			}
		})
	}
	var m packedManifest
	if e := strictPackJSON([]byte(`{"version":2}`), &m); e != nil || m.Version != 2 {
		t.Fatal("ordinary JSON rejected", e)
	}
}
func packedTestSource(t *testing.T) (*corpus, *Batch) {
	t.Helper()
	c, e := load()
	if e != nil {
		t.Fatal(e)
	}
	for _, b := range c.index.Batches {
		if b.packed != nil {
			return c, b
		}
	}
	t.Fatal("no consolidated source")
	return nil, nil
}
func TestPackedCorpusPreservesFrozenContracts(t *testing.T) {
	c, b := packedTestSource(t)
	if len(c.index.Packs) != 1 || len(c.index.Batches) != 289 {
		t.Fatal("physical/logical inventory changed")
	}
	p := b.packed
	raw, e := readMember(p.physical, "source-index.json")
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != "54f3b48c6ab6417878d96523fe51955448ab2dd01fcf7c5bd69cc3d1b791fd8d" {
		t.Fatal("frozen source index changed")
	}
	direct, packed, members, aliases := 0, 0, 0, 0
	for _, b := range c.index.Batches {
		members += len(b.Members)
		aliases += len(b.Aliases)
		if b.packed == nil {
			direct++
			continue
		}
		packed++
		if e := p.bind(b); e != nil {
			t.Fatal(e)
		}
		if _, e := os.Stat(filepath.Join(c.root, batchDir, b.File)); !os.IsNotExist(e) {
			t.Fatal("old PR-added physical ZIP still present", b.File, e)
		}
	}
	if direct != 36 || packed != 253 || members != 24849 || aliases != 25197 {
		t.Fatalf("frozen contract counts %d/%d/%d/%d", direct, packed, members, aliases)
	}
	origin, e := SourceArchive("validation-sparkplug-template-device-commands.zip")
	if e != nil || origin.Bytes != 802963 || origin.SHA256 != "d922a372fa8fb9ae07cc03328599326e7833b2b1a2d7d20394d8861a462960b7" || origin.StorageFile != p.storage.File || origin.StorageSHA256 != p.storage.SHA256 {
		t.Fatal("source provenance/storage binding", origin, e)
	}
	// Source identity, data hash/size and aliases cannot be retargeted to a
	// convenient existing packed recipe, even if every new hash is valid.
	for _, kind := range []string{"file", "archive-hash", "source-sha", "member-hash", "member-size", "alias"} {
		t.Run(kind, func(t *testing.T) {
			changed := *b
			changed.packed = nil
			changed.Members = map[string]Member{}
			for k, v := range b.Members {
				changed.Members[k] = v
			}
			changed.Aliases = map[string]string{}
			for k, v := range b.Aliases {
				changed.Aliases[k] = v
			}
			switch kind {
			case "file":
				changed.File = "other.zip"
			case "archive-hash":
				changed.SHA256 = fmt.Sprintf("%064d", 0)
			case "source-sha":
				changed.SourceSHA = "other"
			case "member-hash":
				for k, v := range changed.Members {
					v.SHA256 = fmt.Sprintf("%064d", 0)
					changed.Members[k] = v
					break
				}
			case "member-size":
				for k, v := range changed.Members {
					v.Bytes++
					changed.Members[k] = v
					break
				}
			case "alias":
				changed.Aliases["invented/path"] = "other"
			}
			if p.bind(&changed) == nil {
				t.Fatal("accepted altered source contract")
			}
		})
	}
}
func TestPackedPayloadOwnershipAndCache(t *testing.T) {
	c, b := packedTestSource(t)
	var source *Batch
	var name string
	for _, candidate := range c.index.Batches {
		if candidate.packed == nil {
			continue
		}
		names := []string{}
		for n := range candidate.Members {
			if isCapture(n) {
				names = append(names, n)
			}
		}
		sort.Strings(names)
		if len(names) > 0 {
			source = candidate
			name = names[0]
			break
		}
	}
	if source == nil {
		t.Fatal("no packed capture")
	}
	want, e := readMember(source, name)
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 16; i++ {
				got, e := readMember(source, name)
				if e != nil || !bytes.Equal(got, want) {
					t.Errorf("concurrent content/ownership mismatch %v", e)
					return
				}
				for j := range got {
					got[j] ^= 255
				}
			}
		}()
	}
	wg.Wait()
	// Traverse enough real blocks to evict the bounded private cache. A returned
	// logical buffer never aliases the shared immutable cached storage.
	p := b.packed
	for _, n := range p.manifest.Blocks {
		if _, e = p.block(n); e != nil {
			t.Fatal(e)
		}
		p.mu.Lock()
		bad := p.cachedBytes > packCacheBytes
		p.mu.Unlock()
		if bad {
			t.Fatal("cache budget exceeded")
		}
	}
	got, e := readMember(source, name)
	if e != nil || !bytes.Equal(got, want) {
		t.Fatal("eviction changed logical bytes", e)
	}
	for _, m := range []Member{{Bytes: maxMember + 1}, {Bytes: -1}, {Bytes: 1, SHA256: fmt.Sprintf("%064d", 0)}} {
		if _, e := p.read(m); e == nil {
			t.Fatal("accepted invalid logical member")
		}
	}
}

func TestPackedLogicalMemberCountExistingAPI(t *testing.T) {
	for _, n := range []int{10000, 10001} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			members := map[string]Member{}
			empty := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
			for i := 0; i < n; i++ {
				members[fmt.Sprintf("empty/%05d", i)] = Member{SHA256: empty, Bytes: 0}
			}
			b := &Batch{ID: "count-boundary", File: "count-boundary.zip", SHA256: fmt.Sprintf("%064x", 1), Password: "bin-parser", Members: members, Aliases: map[string]string{}, PackID: "test-pack"}
			source := *b
			source.PackID = ""
			p := &packedArchive{source: map[string]*Batch{b.ID: &source}, manifest: packedManifest{ArchiveBytes: map[string]int64{b.ID: 2 << 20}, Recipes: map[string]packedRecipe{empty: {Count: 0}}}}
			err := p.bind(b)
			if n == 10000 && err != nil {
				t.Fatal("normal original maximum rejected", err)
			}
			if n == 10001 && (err == nil || err.Error() != "packed logical member count limit") {
				t.Fatal("logical10001 count boundary", err)
			}
			if n == 10001 && b.packed != nil {
				t.Fatal("rejected logical batch gained storage state")
			}
		})
	}
}

// This fixture is independent of the generator: the expected payload and both
// table layouts are written from constants, then read through the existing API.
func compactPackFixture(t *testing.T, version int, change func(*packedManifest, map[string][]byte), options ...zstd.EOption) (string, *Pack, *Batch, []byte) {
	t.Helper()
	want := append(bytes.Repeat([]byte{'q'}, 4096), 'z')
	hash := fmt.Sprintf("%x", sha256.Sum256(want))
	source := &Batch{ID: "compact-unit", File: "source.zip", SHA256: strings.Repeat("2", 64), Password: "bin-parser", Members: map[string]Member{"data.bin": {SHA256: hash, Bytes: int64(len(want))}}, Aliases: map[string]string{"validation/compact.bin": "data.bin"}}
	m := packedManifest{Version: version, SourceSHA: strings.Repeat("1", 40), Blocks: []string{"blocks/00000.bin", "blocks/00001.bin"}, Recipes: map[string]packedRecipe{}, ArchiveBytes: map[string]int64{source.ID: 17}}
	files := map[string][]byte{"blocks/00000.bin": append([]byte(nil), want[:4096]...), "blocks/00001.bin": append([]byte(nil), want[4096:]...)}
	var table, deltas []byte
	if version == 2 {
		for i := 0; i < 17; i++ {
			var entry [12]byte
			block, offset, size := uint32(0), uint32(i*256), uint32(256)
			if i == 16 {
				block, offset, size = 1, 0, 1
			}
			binary.LittleEndian.PutUint32(entry[:], block)
			binary.LittleEndian.PutUint32(entry[4:], offset)
			binary.LittleEndian.PutUint32(entry[8:], size)
			table = append(table, entry[:]...)
			if i == 0 {
				deltas = append(deltas, 0)
			} else {
				deltas = append(deltas, 2)
			}
		}
		m.ChunkCount = 17
	} else {
		table, deltas, m.ChunkCount = []byte{0, 16, 1, 0}, []byte{0, 2}, 2
	}
	files["tables/00000.bin"] = table
	files["recipes/00000.bin"] = deflateRecipe(t, deltas)
	m.Tables = []packedTable{{File: "tables/00000.bin", Count: m.ChunkCount}}
	m.Recipes[hash] = packedRecipe{File: "recipes/00000.bin", Bytes: len(files["recipes/00000.bin"]), Count: m.ChunkCount}
	if change != nil {
		change(&m, files)
	}
	var err error
	files["manifest.json"], err = json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	files["source-index.json"], err = json.Marshal(Index{SchemaVersion: 1, Batches: []*Batch{source}})
	if err != nil {
		t.Fatal(err)
	}
	p := &Pack{ID: "compact-unit-pack", File: "compact.zip", Password: "bin-parser", SourceSHA: m.SourceSHA, Members: map[string]Member{}}
	var archive bytes.Buffer
	w := zip.NewWriter(&archive)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		raw := files[name]
		compressed := raw
		method := uint16(zip.Store)
		if len(options) != 0 && strings.HasPrefix(name, "blocks/") {
			encoder, err := zstd.NewWriter(nil, append([]zstd.EOption{zstd.WithEncoderConcurrency(1)}, options...)...)
			if err != nil {
				t.Fatal(err)
			}
			compressed = encoder.EncodeAll(raw, nil)
			encoder.Close()
			method = packedZstdMethod
		}
		crc := crc32.ChecksumIEEE(raw)
		p.Members[name] = Member{SHA256: fmt.Sprintf("%x", sha256.Sum256(raw)), Bytes: int64(len(raw))}
		header := &zip.FileHeader{Name: name, Method: method, Flags: 1, CRC32: crc, CompressedSize64: uint64(len(compressed) + 12), UncompressedSize64: uint64(len(raw))}
		header.SetMode(0600)
		f, err := w.CreateRaw(header)
		if err != nil {
			t.Fatal(err)
		}
		keys := decryptReader{keys: [3]uint32{0x12345678, 0x23456789, 0x34567890}}
		for _, c := range []byte(p.Password) {
			keys.update(c)
		}
		plain := append(make([]byte, 12), compressed...)
		plain[11] = byte(crc >> 24)
		for i, c := range plain {
			v := uint16(keys.keys[2]) | 2
			plain[i] = c ^ byte((v*(v^1))>>8)
			keys.update(c)
		}
		if _, err := f.Write(plain); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	p.SHA256 = fmt.Sprintf("%x", sha256.Sum256(archive.Bytes()))
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, batchDir), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, batchDir, p.File), archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	bound := *source
	bound.PackID = p.ID
	return root, p, &bound, want
}

func TestPackedCompactTableExistingAPI(t *testing.T) {
	for _, version := range []int{2, 3} {
		t.Run(fmt.Sprintf("v%d", version), func(t *testing.T) {
			root, p, b, want := compactPackFixture(t, version, nil)
			a, err := openPack(root, p)
			if err != nil {
				t.Fatal("valid storage layout rejected", err)
			}
			if err := a.bind(b); err != nil {
				t.Fatal(err)
			}
			got, err := readMember(b, "data.bin")
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("complete payload differs: %v", err)
			}
			got[0] ^= 255
			again, err := readMember(b, "data.bin")
			if err != nil || !bytes.Equal(again, want) {
				t.Fatalf("result ownership differs: %v", err)
			}
		})
	}
}

func TestPackedCompactTableRejectsMalformed(t *testing.T) {
	cases := []struct {
		name, errorText string
		version         int
		change          func(*packedManifest, map[string][]byte)
	}{
		{"truncated", "chunk table width", 3, func(m *packedManifest, f map[string][]byte) { f["tables/00000.bin"] = []byte{0, 16, 1} }},
		{"zero", "chunk boundary", 3, func(m *packedManifest, f map[string][]byte) {
			f["tables/00000.bin"][0], f["tables/00000.bin"][1] = 0, 0
		}},
		{"over-4096", "chunk boundary", 3, func(m *packedManifest, f map[string][]byte) { f["tables/00000.bin"][0] = 1 }},
		{"cross-block", "chunk boundary", 3, func(m *packedManifest, f map[string][]byte) { f["tables/00000.bin"] = []byte{255, 15, 3, 0} }},
		{"missing-tail-block", "incomplete block partition", 3, func(m *packedManifest, f map[string][]byte) { f["tables/00000.bin"] = []byte{255, 15, 1, 0} }},
		{"bad-first-index", "chunk table range", 3, func(m *packedManifest, f map[string][]byte) { m.Tables[0].First = 1 }},
		{"excess-count", "chunk table range", 3, func(m *packedManifest, f map[string][]byte) { m.Tables[0].Count = 3 }},
		{"v2-length-layout", "chunk table width", 2, func(m *packedManifest, f map[string][]byte) { f["tables/00000.bin"] = []byte{0, 1} }},
		{"v2-limit-retained", "chunk boundary", 2, func(m *packedManifest, f map[string][]byte) {
			f["tables/00000.bin"][8] = 1
			f["tables/00000.bin"][9] = 1
		}},
		{"unsupported-version", "limits/version/source", 4, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root, p, _, _ := compactPackFixture(t, c.version, c.change)
			if _, err := openPack(root, p); err == nil || !strings.Contains(err.Error(), c.errorText) {
				t.Fatalf("expected %q, got %v", c.errorText, err)
			}
		})
	}
}

func TestPackedZstdStorageBoundaries(t *testing.T) {
	open := func(t *testing.T, options ...zstd.EOption) (*Pack, *packedArchive, *Batch, []byte) {
		t.Helper()
		root, p, b, want := compactPackFixture(t, 3, nil, options...)
		a, err := openPack(root, p)
		if err != nil {
			t.Fatal(err)
		}
		if err := a.bind(b); err != nil {
			t.Fatal(err)
		}
		return p, a, b, want
	}
	t.Run("complete-owned-output", func(t *testing.T) {
		_, _, b, want := open(t, zstd.WithWindowSize(maxPackedStorageBlock))
		got, err := readMember(b, "data.bin")
		if err != nil || !bytes.Equal(got, want) {
			t.Fatal("zstd content", err)
		}
		got[0] ^= 255
		again, err := readMember(b, "data.bin")
		if err != nil || !bytes.Equal(again, want) {
			t.Fatal("zstd ownership", err)
		}
	})
	t.Run("decoded-size-bound", func(t *testing.T) {
		_, a, _, _ := open(t, zstd.WithWindowSize(maxPackedStorageBlock))
		name := "blocks/00000.bin"
		pin := a.physical.Members[name]
		pin.Bytes--
		a.physical.Members[name] = pin
		if raw, err := a.block(name); err == nil || raw != nil {
			t.Fatal("accepted expanded block", err)
		}
		if _, ok := a.cached[name]; ok {
			t.Fatal("cached failed block")
		}
	})
	t.Run("window-bound", func(t *testing.T) {
		root, p, b, _ := compactPackFixture(t, 3, nil, zstd.WithWindowSize(8<<20), zstd.WithSingleSegment(false))
		path := filepath.Join(root, batchDir, p.File)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range zr.File {
			if f.Name != "blocks/00000.bin" {
				continue
			}
			encrypted, err := f.OpenRaw()
			if err != nil {
				t.Fatal(err)
			}
			keys := decryptReader{r: encrypted, keys: [3]uint32{0x12345678, 0x23456789, 0x34567890}}
			for _, c := range []byte(p.Password) {
				keys.update(c)
			}
			plain, err := io.ReadAll(&keys)
			if err != nil || len(plain) < 18 || plain[16]&0x20 != 0 {
				t.Fatalf("non-single-segment fixture required: %v", err)
			}
			// The encoder reduces small inputs' window. Set an explicit valid
			// 8MiB window descriptor; payload and its checksum remain unchanged.
			plain[17] = 13 << 3
			var header zstd.Header
			if err := header.Decode(plain[12:]); err != nil || header.SingleSegment || header.WindowSize != 8<<20 {
				t.Fatalf("actual declared window: %+v %v", header, err)
			}
			reference, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxWindow(8<<20), zstd.WithDecoderMaxMemory(8<<20))
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := reference.DecodeAll(plain[12:], nil)
			reference.Close()
			if err != nil || !bytes.Equal(decoded, bytes.Repeat([]byte{'q'}, 4096)) {
				t.Fatal("over-window fixture must remain valid", err)
			}
			keys = decryptReader{keys: [3]uint32{0x12345678, 0x23456789, 0x34567890}}
			for _, c := range []byte(p.Password) {
				keys.update(c)
			}
			for i, c := range plain {
				v := uint16(keys.keys[2]) | 2
				plain[i] = c ^ byte((v*(v^1))>>8)
				keys.update(c)
			}
			offset, err := f.DataOffset()
			if err != nil {
				t.Fatal(err)
			}
			copy(raw[int(offset):int(offset)+len(plain)], plain)
		}
		p.SHA256 = fmt.Sprintf("%x", sha256.Sum256(raw))
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		a, err := openPack(root, p)
		if err != nil {
			t.Fatal(err)
		}
		if err := a.bind(b); err != nil {
			t.Fatal(err)
		}
		if _, err := readMember(b, "data.bin"); !errors.Is(err, zstd.ErrWindowSizeExceeded) {
			t.Fatalf("expected window refusal, got %v", err)
		}
	})
	t.Run("v2-remains-legacy", func(t *testing.T) {
		root, p, _, _ := compactPackFixture(t, 2, nil, zstd.WithWindowSize(maxPackedStorageBlock))
		if _, err := openPack(root, p); err == nil || !strings.Contains(err.Error(), "version2 compression") {
			t.Fatalf("v2 codec drift: %v", err)
		}
	})
	t.Run("corrupt-frame", func(t *testing.T) {
		root, p, b, _ := compactPackFixture(t, 3, nil, zstd.WithWindowSize(maxPackedStorageBlock))
		path := filepath.Join(root, batchDir, p.File)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range zr.File {
			if f.Name == "blocks/00000.bin" {
				offset, err := f.DataOffset()
				if err != nil {
					t.Fatal(err)
				}
				raw[int(offset+int64(f.CompressedSize64))-1] ^= 1
			}
		}
		p.SHA256 = fmt.Sprintf("%x", sha256.Sum256(raw))
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		a, err := openPack(root, p)
		if err != nil {
			t.Fatal(err)
		}
		if err := a.bind(b); err != nil {
			t.Fatal(err)
		}
		if got, err := readMember(b, "data.bin"); err == nil || got != nil {
			t.Fatal("corrupt zstd accepted", err)
		}
	})
	for _, tc := range []struct {
		name string
		size uint64
	}{
		{"manifest.json", 1}, {"tables/00000.bin", 1}, {"blocks/00000.bin", maxPackedStorageBlock + 1},
	} {
		t.Run("physical-codec-scope/"+tc.name, func(t *testing.T) {
			var raw bytes.Buffer
			zw := zip.NewWriter(&raw)
			h := &zip.FileHeader{Name: tc.name, Method: packedZstdMethod, Flags: 1, CompressedSize64: 12, UncompressedSize64: tc.size}
			h.SetMode(0600)
			f, err := zw.CreateRaw(h)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.Write(make([]byte, 12)); err != nil {
				t.Fatal(err)
			}
			if err = zw.Close(); err != nil {
				t.Fatal(err)
			}
			b := &Batch{ID: "scope", SHA256: fmt.Sprintf("%x", sha256.Sum256(raw.Bytes())), Members: map[string]Member{tc.name: {SHA256: strings.Repeat("0", 64), Bytes: int64(tc.size)}}, packStorage: true}
			if err := openArchive(raw.Bytes(), b, maxPackExpanded); err == nil || !strings.Contains(err.Error(), "unsupported encryption or compression") {
				t.Fatalf("packed codec scope failed: %v", err)
			}
		})
	}
}
