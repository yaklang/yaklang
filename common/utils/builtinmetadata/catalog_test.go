package builtinmetadata

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCatalogRequiresMatchingSourceAndVersion(t *testing.T) {
	const code = `cli.String("target")`
	c := Catalog{Version: Version, Entries: map[string]Entry{
		"tool:demo": {SourceSHA256: SourceHash(code), Metadata: Metadata{Parameters: `{"type":"object"}`, EnableAIOutputLog: 1}},
	}}
	for _, tc := range []struct {
		key, source string
		want        bool
	}{
		{"tool:demo", code, true},
		{"tool:demo", code + "; cli.Int(\"port\")", false},
		{"plugin:yak:demo", code, false},
		{"missing", code, false},
	} {
		got, ok := c.Lookup(tc.key, tc.source)
		if ok != tc.want {
			t.Fatalf("Lookup(%q) hit = %v, want %v", tc.key, ok, tc.want)
		}
		got.Parameters = "caller mutation"
	}
	if got, _ := c.Lookup("tool:demo", code); got.Parameters != `{"type":"object"}` {
		t.Fatal("lookup must return an independent metadata value")
	}
	c.Version++
	if _, ok := c.Lookup("tool:demo", code); ok {
		t.Fatal("unsupported metadata format must fall back to dynamic analysis")
	}
}

func TestEmbeddedCatalogIsValid(t *testing.T) {
	if !Enabled {
		t.Skip("plain builds use dynamic analysis")
	}
	var c Catalog
	if err := json.Unmarshal(embeddedCatalog, &c); err != nil {
		t.Fatal(err)
	}
	if c.Version != Version || len(c.Entries) == 0 {
		t.Fatal("builtin metadata is empty or outdated; run go generate ./common/utils/builtinmetadata")
	}
	for key, entry := range c.Entries {
		if !validMetadata(key, entry.Metadata) {
			t.Errorf("malformed metadata for %s", key)
		}
		if len(entry.SourceSHA256) != 64 {
			t.Errorf("incomplete generated entry %s", key)
		}
		if strings.HasPrefix(key, "tool:") && (entry.Metadata.Parameters == "" || entry.Metadata.EnableAIOutputLog == 0) {
			t.Errorf("incomplete tool schema %s", key)
		}
	}
}

func TestRevisionTracksExtractorChangesPerResource(t *testing.T) {
	c := Catalog{Version: Version, Entries: map[string]Entry{
		"tool:a":  {SourceSHA256: SourceHash("code"), Metadata: Metadata{Parameters: "old schema"}},
		"forge:b": {SourceSHA256: SourceHash("forge code"), Metadata: Metadata{Parameters: "forge schema"}},
	}}
	toolRevision, forgeRevision := c.revision("tool:"), c.revision("forge:")
	entry := c.Entries["tool:a"]
	entry.Metadata.Parameters = "new schema, unchanged source"
	c.Entries["tool:a"] = entry
	if c.revision("tool:") == toolRevision {
		t.Fatal("extractor changes must invalidate synchronization even with unchanged source")
	}
	if c.revision("forge:") != forgeRevision {
		t.Fatal("tool changes must not invalidate forge synchronization")
	}
}

func TestMalformedMetadataFallsBack(t *testing.T) {
	for _, tc := range []struct {
		key  string
		meta Metadata
	}{
		{"tool:demo", Metadata{Parameters: `{"type":"object"}`, EnableAIOutputLog: 0}},
		{"tool:demo", Metadata{Parameters: `[]`, EnableAIOutputLog: 1}},
		{"tool:demo", Metadata{Parameters: `{`, EnableAIOutputLog: 1}},
		{"plugin:yak:demo", Metadata{Parameters: `"not JSON"`}},
		{"plugin:yak:demo", Metadata{Parameters: `"[]"`, EnvironmentKeys: `"not JSON"`}},
		{"forge:demo", Metadata{Parameters: `{}`}},
	} {
		c := Catalog{Version: Version, Entries: map[string]Entry{tc.key: {SourceSHA256: SourceHash("source"), Metadata: tc.meta}}}
		if _, hit := c.Lookup(tc.key, "source"); hit {
			t.Errorf("invalid metadata accepted for %s: %+v", tc.key, tc.meta)
		}
	}
}

func TestPreparedResourceHashesTrackMetadata(t *testing.T) {
	c := Catalog{Version: Version, Entries: map[string]Entry{"tool:a": {SourceSHA256: SourceHash("source"), Metadata: Metadata{Parameters: `{"type":"object"}`, EnableAIOutputLog: 1}}}}
	sources := map[string]string{"tool:": "tool source hash", "forge:": "forge source hash"}
	c.PrepareResourceHashes(sources)
	oldTool, oldForge := c.ResourceHashes["tool:"], c.ResourceHashes["forge:"]
	e := c.Entries["tool:a"]
	e.Metadata.Description = "extractor changed"
	c.Entries["tool:a"] = e
	c.PrepareResourceHashes(sources)
	if c.ResourceHashes["tool:"] == oldTool {
		t.Fatal("metadata-only changes must invalidate synchronization")
	}
	if c.ResourceHashes["forge:"] != oldForge {
		t.Fatal("unrelated metadata must retain its sync hash")
	}
	if !Enabled && ResourceHash("original", "tool:") != "original" {
		t.Fatal("ordinary builds must retain their original hash")
	}
}

func TestEmbeddedSyncHashesArePreparedAndConsistent(t *testing.T) {
	if !Enabled {
		t.Skip("gzip catalog only")
	}
	var c Catalog
	if err := json.Unmarshal(embeddedCatalog, &c); err != nil {
		t.Fatal(err)
	}
	want := c.ResourceHashes
	c.PrepareResourceHashes(c.SourceHashes)
	for prefix, expected := range c.ResourceHashes {
		if want[prefix] != expected {
			t.Fatalf("inconsistent generated synchronization hash: %s", prefix)
		}
		if got := ResourceHash(c.SourceHashes[prefix], prefix); got != expected {
			t.Fatalf("runtime did not use prepared hash for %s", prefix)
		}
		if got := ResourceHash("different source", prefix); got != "different source" {
			t.Fatal("stale resource hash must not match")
		}
	}
}

func TestEmptyParameterMetadataIsValid(t *testing.T) {
	for _, key := range []string{"plugin:yak:no-cli", "forge:no-cli"} {
		c := Catalog{Version: Version, Entries: map[string]Entry{key: {SourceSHA256: SourceHash("println(1)"), Metadata: Metadata{}}}}
		if _, hit := c.Lookup(key, "println(1)"); !hit {
			t.Fatalf("empty CLI metadata is legitimate for %s", key)
		}
	}
}
