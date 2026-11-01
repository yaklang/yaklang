package trafficfixture

// Consolidated storage changes physical layout only. Logical batch/member and
// alias contracts remain pinned to the frozen source index. Production readers
// do not import this package. The old per-member/per-batch limits remain intact.
import (
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
)

type Pack struct {
	ID        string            `json:"id"`
	File      string            `json:"file"`
	SHA256    string            `json:"sha256"`
	Password  string            `json:"password"`
	Members   map[string]Member `json:"members"`
	SourceSHA string            `json:"source_sha"`
	data      *packedArchive
}
type packedTable struct {
	File  string `json:"file"`
	First int    `json:"first"`
	Count int    `json:"count"`
}
type packedRecipe struct {
	File   string `json:"file"`
	Offset int    `json:"offset"`
	Bytes  int    `json:"bytes"`
	Count  int    `json:"count"`
}
type packedManifest struct {
	Version      int                     `json:"version"`
	SourceSHA    string                  `json:"source_sha"`
	ChunkCount   int                     `json:"chunk_count"`
	Tables       []packedTable           `json:"tables"`
	Blocks       []string                `json:"blocks"`
	Recipes      map[string]packedRecipe `json:"recipes"`
	ArchiveBytes map[string]int64        `json:"archive_bytes"`
}
type packedChunk struct{ block, offset, bytes uint32 }
type packedArchive struct {
	storage     *Pack
	physical    *Batch
	manifest    packedManifest
	source      map[string]*Batch
	chunks      []packedChunk
	mu          sync.Mutex
	cached      map[string][]byte
	order       []string
	cachedBytes int
}

const packCacheBytes = 32 << 20
const maxPackedChunks = 1 << 21
const maxPackExpanded = 256 << 20
const maxRecipeEntries = 1 << 20
const packedZstdMethod = 93
const maxPackedStorageBlock = 4 << 20

func digestValid(s string, n int) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == n && len(s) == n*2
}
func strictPackJSON(raw []byte, to any) error {
	// Reject duplicate keys as well as unknown fields; provenance must not depend
	// on a JSON parser's first/last duplicate-key policy.
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 16 {
			return fmt.Errorf("packed JSON depth limit")
		}
		v, e := d.Token()
		if e != nil {
			return e
		}
		delim, ok := v.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return e
				}
				s, ok := k.(string)
				if !ok || seen[s] {
					return fmt.Errorf("packed JSON duplicate key")
				}
				seen[s] = true
				if e = walk(depth + 1); e != nil {
					return e
				}
			}
			v, e = d.Token()
			if e != nil || v != json.Delim('}') {
				return fmt.Errorf("packed JSON object")
			}
		case '[':
			for d.More() {
				if e = walk(depth + 1); e != nil {
					return e
				}
			}
			v, e = d.Token()
			if e != nil || v != json.Delim(']') {
				return fmt.Errorf("packed JSON array")
			}
		default:
			return fmt.Errorf("packed JSON delimiter")
		}
		return nil
	}
	if e := walk(0); e != nil {
		return e
	}
	if _, e := d.Token(); e != io.EOF {
		return fmt.Errorf("packed JSON trailing content")
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(to)
}
func openPack(root string, p *Pack) (*packedArchive, error) {
	physical := &Batch{ID: p.ID, File: p.File, SHA256: p.SHA256, Password: p.Password, Members: p.Members, packStorage: true}
	raw, e := readBoundedFile(filepath.Join(root, batchDir, p.File), maxBatch)
	if e != nil {
		return nil, e
	}
	if e = openArchive(raw, physical, maxPackExpanded); e != nil {
		return nil, e
	}
	a := &packedArchive{storage: p, physical: physical, source: map[string]*Batch{}, cached: map[string][]byte{}}
	raw, e = readMember(physical, "manifest.json")
	if e != nil {
		return nil, e
	}
	if e = strictPackJSON(raw, &a.manifest); e != nil {
		return nil, e
	}
	m := &a.manifest
	if (m.Version != 2 && m.Version != 3) || m.SourceSHA != p.SourceSHA || !digestValid(p.SourceSHA, 20) || m.ChunkCount <= 0 || m.ChunkCount > maxPackedChunks || len(m.Blocks) == 0 || len(m.Recipes) == 0 || len(m.Recipes) > 100000 {
		return nil, fmt.Errorf("packed manifest limits/version/source")
	}
	chunkLimit, blockLimit, tableWidth := uint32(256), int64(1<<20), 12
	if m.Version == 3 {
		chunkLimit, blockLimit, tableWidth = 4096, maxPackedStorageBlock, 2
	}
	for _, f := range physical.files {
		if m.Version == 2 && f.Method == packedZstdMethod {
			return nil, fmt.Errorf("packed version2 compression")
		}
	}
	raw, e = readMember(physical, "source-index.json")
	if e != nil {
		return nil, e
	}
	var source Index
	if e = json.Unmarshal(raw, &source); e != nil {
		return nil, e
	}
	if source.SchemaVersion != 1 || len(source.Batches) == 0 {
		return nil, fmt.Errorf("packed source index")
	}
	for _, b := range source.Batches {
		if b == nil || b.ID == "" || a.source[b.ID] != nil || b.PackID != "" {
			return nil, fmt.Errorf("packed source batch identity")
		}
		a.source[b.ID] = b
	}
	used := map[string]bool{"manifest.json": true, "source-index.json": true}
	a.chunks = make([]packedChunk, 0, m.ChunkCount)
	for i, name := range m.Blocks {
		pin, ok := p.Members[name]
		if !ok || name != fmt.Sprintf("blocks/%05d.bin", i) || pin.Bytes <= 0 || pin.Bytes > blockLimit || used[name] {
			return nil, fmt.Errorf("packed block inventory")
		}
		used[name] = true
	}
	var compactBlock, compactOffset uint32
	for i, t := range m.Tables {
		if t.File != fmt.Sprintf("tables/%05d.bin", i) || t.First != len(a.chunks) || t.Count <= 0 || t.Count > 8192 || t.Count > m.ChunkCount-len(a.chunks) {
			return nil, fmt.Errorf("packed chunk table range")
		}
		raw, e = readMember(physical, t.File)
		if e != nil {
			return nil, e
		}
		if len(raw) != t.Count*tableWidth {
			return nil, fmt.Errorf("packed chunk table width")
		}
		used[t.File] = true
		for off := 0; off < len(raw); off += tableWidth {
			var c packedChunk
			if m.Version == 2 {
				c = packedChunk{binary.LittleEndian.Uint32(raw[off:]), binary.LittleEndian.Uint32(raw[off+4:]), binary.LittleEndian.Uint32(raw[off+8:])}
			} else {
				c = packedChunk{compactBlock, compactOffset, uint32(binary.LittleEndian.Uint16(raw[off:]))}
			}
			if int(c.block) >= len(m.Blocks) || c.bytes == 0 || c.bytes > chunkLimit || uint64(c.offset)+uint64(c.bytes) > uint64(p.Members[m.Blocks[c.block]].Bytes) {
				return nil, fmt.Errorf("packed chunk boundary")
			}
			a.chunks = append(a.chunks, c)
			if m.Version == 3 {
				compactOffset += c.bytes
				if int64(compactOffset) == p.Members[m.Blocks[compactBlock]].Bytes {
					compactBlock++
					compactOffset = 0
				}
			}
		}
	}
	if len(a.chunks) != m.ChunkCount {
		return nil, fmt.Errorf("packed missing chunk table")
	}
	// Chunks partition payload blocks exactly: no hidden bytes, gaps, overlap or
	// reordered physical ranges. Recipes may reference any already-pinned chunk.
	block, offset := uint32(0), uint32(0)
	for _, c := range a.chunks {
		if c.block != block || c.offset != offset {
			return nil, fmt.Errorf("packed chunk partition")
		}
		offset += c.bytes
		if int64(offset) == p.Members[m.Blocks[block]].Bytes {
			block++
			offset = 0
		}
	}
	if int(block) != len(m.Blocks) || offset != 0 {
		return nil, fmt.Errorf("packed incomplete block partition")
	}
	for h, r := range m.Recipes {
		pin, ok := p.Members[r.File]
		if !digestValid(h, 32) || !ok || !strings.HasPrefix(r.File, "recipes/") || !strings.HasSuffix(r.File, ".bin") || pin.Bytes <= 0 || pin.Bytes > 1<<20 || r.Offset < 0 || r.Bytes <= 0 || r.Count < 0 || r.Count > maxRecipeEntries || int64(r.Offset) > pin.Bytes || int64(r.Bytes) > pin.Bytes-int64(r.Offset) {
			return nil, fmt.Errorf("packed recipe bounds")
		}
		used[r.File] = true
	}
	for name := range p.Members {
		if !used[name] && name != "tools/packgen.go" {
			return nil, fmt.Errorf("packed unknown storage member %s", name)
		}
	}
	return a, nil
}
func (a *packedArchive) bind(b *Batch) error {
	// The physical pack aggregates batches; each original logical batch keeps
	// openBatch's independent member-count limit.
	if len(b.Members) > 10000 {
		return fmt.Errorf("packed logical member count limit")
	}

	source := a.source[b.ID]
	if source == nil {
		return fmt.Errorf("packed batch missing frozen source")
	}
	left, right := *b, *source
	left.PackID = ""
	left.packed = nil
	left.files = nil
	right.files = nil
	if !reflect.DeepEqual(left, right) {
		return fmt.Errorf("packed source contract changed: %s", b.ID)
	}
	if n, ok := a.manifest.ArchiveBytes[b.ID]; !ok || n <= 0 || n > maxBatch {
		return fmt.Errorf("packed source archive provenance")
	}
	var total int64
	for name, m := range b.Members {
		if !validName(name) || !digestValid(m.SHA256, 32) || m.Bytes < 0 || m.Bytes > maxMember {
			return fmt.Errorf("packed logical member bounds")
		}
		total += m.Bytes
		if total > maxBatch {
			return fmt.Errorf("packed logical batch bound")
		}
		if _, ok := a.manifest.Recipes[m.SHA256]; !ok {
			return fmt.Errorf("packed missing member recipe")
		}
	}
	for alias, name := range b.Aliases {
		if !validName(alias) {
			return fmt.Errorf("packed unsafe alias")
		}
		if _, ok := b.Members[name]; !ok {
			return fmt.Errorf("packed missing alias member")
		}
	}
	b.packed = a
	return nil
}
func (a *packedArchive) block(name string) ([]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if raw, ok := a.cached[name]; ok {
		return raw, nil
	}
	raw, e := readMember(a.physical, name)
	if e != nil {
		return nil, e
	}
	limit := 1 << 20
	if a.manifest.Version == 3 && strings.HasPrefix(name, "blocks/") {
		limit = maxPackedStorageBlock
	}
	if len(raw) > limit {
		return nil, fmt.Errorf("packed cache block bound")
	}
	for a.cachedBytes+len(raw) > packCacheBytes && len(a.order) > 0 {
		old := a.order[0]
		a.order = a.order[1:]
		a.cachedBytes -= len(a.cached[old])
		delete(a.cached, old)
	}
	a.cached[name] = raw
	a.order = append(a.order, name)
	a.cachedBytes += len(raw)
	return raw, nil
}
func decodePackedRecipe(raw []byte, count, chunkCount int) ([]uint32, error) {
	if count < 0 || count > maxRecipeEntries || chunkCount <= 0 || chunkCount > maxPackedChunks {
		return nil, fmt.Errorf("packed recipe count limit")
	}
	source := bytes.NewReader(raw)
	r := flate.NewReader(source)
	data, e := io.ReadAll(io.LimitReader(r, int64(count)*5+1))
	closeErr := r.Close()
	if e != nil || closeErr != nil || len(data) > count*5 || source.Len() != 0 {
		return nil, fmt.Errorf("packed recipe compressed stream")
	}
	out := make([]uint32, 0, count)
	var previous int64
	for i := 0; i < count; i++ {
		u, n := binary.Uvarint(data)
		if n <= 0 || n > 5 {
			return nil, fmt.Errorf("packed recipe varint")
		}
		var canonical [10]byte
		if binary.PutUvarint(canonical[:], u) != n {
			return nil, fmt.Errorf("packed noncanonical recipe varint")
		}
		data = data[n:]
		delta := int64(u>>1) ^ -int64(u&1)
		id := previous + delta
		if id < 0 || id >= int64(chunkCount) {
			return nil, fmt.Errorf("packed recipe chunk identity")
		}
		out = append(out, uint32(id))
		previous = id
	}
	if len(data) != 0 {
		return nil, fmt.Errorf("packed recipe trailing IDs")
	}
	return out, nil
}
func (a *packedArchive) read(m Member) ([]byte, error) {
	if m.Bytes < 0 || m.Bytes > maxMember {
		return nil, fmt.Errorf("packed member size bound")
	}
	r, ok := a.manifest.Recipes[m.SHA256]
	if !ok {
		return nil, fmt.Errorf("packed recipe missing")
	}
	raw, e := a.block(r.File)
	if e != nil {
		return nil, e
	}
	if r.Offset < 0 || r.Bytes <= 0 || r.Offset > len(raw) || r.Bytes > len(raw)-r.Offset {
		return nil, fmt.Errorf("packed recipe storage range")
	}
	ids, e := decodePackedRecipe(raw[r.Offset:r.Offset+r.Bytes], r.Count, len(a.chunks))
	if e != nil {
		return nil, e
	}
	out := make([]byte, 0, int(m.Bytes))
	for _, id := range ids {
		c := a.chunks[id]
		raw, e = a.block(a.manifest.Blocks[c.block])
		if e != nil {
			return nil, e
		}
		if int64(c.bytes) > m.Bytes-int64(len(out)) {
			return nil, fmt.Errorf("packed member expanded limit")
		}
		out = append(out, raw[int(c.offset):int(c.offset+c.bytes)]...)
	}
	if int64(len(out)) != m.Bytes || fmt.Sprintf("%x", sha256.Sum256(out)) != m.SHA256 {
		return nil, fmt.Errorf("packed member integrity mismatch")
	}
	return out, nil
}

// ArchiveOrigin describes an original compressed source, not a claim that its
// old ZIP serialization still exists. For consolidated sources the old member
// contract is bound to source-index.json inside the hash-verified storage ZIP.
// Physical storage and each reconstructed logical member are verified normally.
// StorageFile remains the historical storage member name for compatibility; in
// a normal checkout that member now lives inside testdata/captures.zip. Exported
// tool workspaces materialize the same old path in their private directory.
type ArchiveOrigin struct {
	File, SHA256               string
	Bytes                      int64
	StorageFile, StorageSHA256 string
}

func SourceArchive(filename string) (ArchiveOrigin, error) {
	c, e := load()
	if e != nil {
		return ArchiveOrigin{}, e
	}
	for _, b := range c.index.Batches {
		if b.File != filename {
			continue
		}
		if b.packed != nil {
			p := b.packed
			return ArchiveOrigin{b.File, b.SHA256, p.manifest.ArchiveBytes[b.ID], p.storage.File, p.storage.SHA256}, nil
		}
		raw, e := readBoundedFile(filepath.Join(c.root, batchDir, b.File), maxBatch)
		if e != nil {
			return ArchiveOrigin{}, e
		}
		if fmt.Sprintf("%x", sha256.Sum256(raw)) != b.SHA256 {
			return ArchiveOrigin{}, fmt.Errorf("source ZIP changed")
		}
		return ArchiveOrigin{b.File, b.SHA256, int64(len(raw)), b.File, b.SHA256}, nil
	}
	return ArchiveOrigin{}, fmt.Errorf("unknown source archive %s", filename)
}
func physicalArchives(c *corpus) []*Batch {
	out := []*Batch{}
	for _, b := range c.index.Batches {
		if b.packed == nil {
			out = append(out, b)
		}
	}
	for _, p := range c.index.Packs {
		out = append(out, p.data.physical)
	}
	return out
}
