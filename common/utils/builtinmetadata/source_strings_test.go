package builtinmetadata

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
)

func TestSourceStringsRestoreExactMetadata(t *testing.T) {
	value := "这是一段很长的说明，含有 \"引号\"、反斜杠 \\、换行\n以及 <HTML> 和 emoji 🦀。"
	source := "prefix\n" + value + "\nsuffix"
	metadata := Metadata{Description: value, Usage: value, Parameters: json.RawMessage(`{"type":"object","properties":{"flag":{"default":false},"number":{"default":9223372036854775807},"empty":{"default":""},"null":{"default":null}}}`), EnableAIOutputLog: 1}
	e, err := NewEntry("demo.yak", metadata, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(e.SourceStrings) != 2 || bytes.Contains(e.Metadata, []byte("这是一段")) {
		t.Fatal("source text duplicated in metadata")
	}
	resolved, err := e.resolve(source)
	if err != nil || !reflect.DeepEqual(resolved, metadata) {
		t.Fatalf("metadata changed: %v", err)
	}
	files := map[string]File{"demo.yak": {Mode: 0644, Size: int64(len(source)), SHA256: SourceHash(source)}}
	b := &Bundle{Version: Version, Kind: "aitool", Recipe: "test", Files: files, SourceHash: Fingerprint(files, "test"), Entries: map[string]Entry{"tool:demo": e}}
	text, err := Text(b)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(text, "aitool")
	if err != nil {
		t.Fatal(err)
	}
	got, hit := parsed.lookup("tool:demo", source)
	if !hit || !reflect.DeepEqual(got, metadata) {
		t.Fatal("text round trip broke source references")
	}
	if _, hit := parsed.lookup("tool:demo", source+"changed"); hit {
		t.Fatal("changed source reused stale offsets")
	}
	for _, ref := range []SourceString{{-1, 0, 1}, {0, -1, 1}, {len(e.Metadata), 0, 1}, {e.SourceStrings[0][0], len(source), 1}, {e.SourceStrings[0][0], 0, -1}} {
		bad := e
		bad.SourceStrings = []SourceString{ref}
		if _, err := bad.resolve(source); err == nil {
			t.Fatal("invalid range accepted", ref)
		}
	}
}

func TestPackedHashIndexDoesNotLoadEntries(t *testing.T) {
	b := fixture()
	data, err := Encode(b)
	if err != nil {
		t.Fatal(err)
	}
	index, entries, err := decodeIndex(data, "aitool", Digest(data))
	if err != nil {
		t.Fatal(err)
	}
	if index.Entries != nil || len(entries) == 0 {
		t.Fatal("hash validation eagerly loaded entries")
	}
	if err := index.decodeEntries(entries); err != nil {
		t.Fatal(err)
	}
	if _, hit := index.lookup("tool:demo", `cli.String("target")`); !hit {
		t.Fatal("lazy entry missed")
	}
	broken := bytes.Clone(data)
	broken[len(broken)-1] ^= 0xff
	if _, _, err := decodeIndex(broken, "aitool", Digest(data)); err == nil {
		t.Fatal("data corruption escaped the lazy hash check")
	}
}

func TestLazyMetadataFailureInvalidatesSyncHash(t *testing.T) {
	if !Enabled {
		t.Skip("gzip runtime")
	}
	const kind = "aitool"
	previous, had := resources.Load(kind)
	defer func() {
		if had {
			resources.Store(kind, previous)
		} else {
			resources.Delete(kind)
		}
	}()
	b := fixture()
	e := b.Entries["tool:demo"]
	e.Metadata = json.RawMessage(`{"parameters":[],"enable_ai_output_log":1}`)
	b.Entries["tool:demo"] = e
	data, err := Encode(b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(data, kind, Digest(data)); err == nil {
		t.Fatal("build validator accepted invalid metadata")
	}
	index, entries, err := decodeIndex(data, kind, Digest(data))
	if err != nil {
		t.Fatal(err)
	}
	r := &resource{bundle: index, packedEntries: entries, hash: Digest(data)}
	r.once.Do(func() {}) // The archive/index validation has already completed.
	resources.Store(kind, r)
	if hash, err := Hash(kind); err != nil || hash == "" || r.bundle.Entries != nil {
		t.Fatal("hash check eagerly decoded metadata", err)
	}
	if _, hit := Lookup("tool:demo", `cli.String("target")`); hit {
		t.Fatal("invalid lazy entry accepted")
	}
	if hash, err := Hash(kind); err == nil || hash != "" {
		t.Fatal("invalid lazy entry retained a sync hash")
	}
}

func TestConcurrentHashAndLazyLookup(t *testing.T) {
	if !Enabled {
		t.Skip("gzip runtime")
	}
	const kind = "aitool"
	previous, had := resources.Load(kind)
	defer func() {
		if had {
			resources.Store(kind, previous)
		} else {
			resources.Delete(kind)
		}
	}()
	b := fixture()
	data, err := Encode(b)
	if err != nil {
		t.Fatal(err)
	}
	index, entries, err := decodeIndex(data, kind, Digest(data))
	if err != nil {
		t.Fatal(err)
	}
	r := &resource{bundle: index, packedEntries: entries, hash: Digest(data)}
	r.once.Do(func() {})
	resources.Store(kind, r)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 10; n++ {
				if hash, err := Hash(kind); err != nil || hash != r.hash {
					t.Error("concurrent hash failed", err)
					return
				}
				if _, hit := Lookup("tool:demo", `cli.String("target")`); !hit {
					t.Error("concurrent lazy lookup failed")
					return
				}
			}
		}()
	}
	wg.Wait()
	Invalidate(kind, errors.New("domain decoder rejected the cache"))
	if hash, err := Hash(kind); hash != "" || err == nil {
		t.Fatal("domain failure retained a sync marker")
	}
}
