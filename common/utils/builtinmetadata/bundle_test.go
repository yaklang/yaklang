package builtinmetadata

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func fixture() *Bundle {
	const code = "cli.String(\"target\")"
	files := map[string]File{".": {Mode: uint32(fs.ModeDir | 0755)}, "demo.yak": {Mode: 0644, Size: int64(len(code)), SHA256: SourceHash(code)}}
	return &Bundle{Version: Version, Kind: "aitool", Recipe: "fixture", Files: files, SourceHash: Fingerprint(files, "fixture"), Entries: map[string]Entry{"tool:demo": {Script: "demo.yak", Metadata: Metadata{Parameters: json.RawMessage(`{"type":"object"}`), EnableAIOutputLog: 1}}}}
}

func TestBundleBindsKindPathSourceAndVersion(t *testing.T) {
	b := fixture()
	if _, hit := b.lookup("tool:demo", `cli.String("target")`); !hit {
		t.Fatal("matching entry missed")
	}
	for _, tc := range [][2]string{{"tool:demo", `cli.String("changed")`}, {"plugin:yak:demo", `cli.String("target")`}} {
		if _, hit := b.lookup(tc[0], tc[1]); hit {
			t.Fatal("wrong source or resource kind hit")
		}
	}
	data, err := Encode(b)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := Encode(b)
	if !bytes.Equal(data, again) {
		t.Fatal("compressed metadata must be deterministic")
	}
	if _, err := Decode(data, "aitool", Digest(data)); err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(data, "forge", Digest(data)); err == nil {
		t.Fatal("wrong kind accepted")
	}
	if _, err := Decode(append(data, 0), "aitool", Digest(data)); err == nil {
		t.Fatal("corruption accepted")
	}
	old := Digest(data)
	e := b.Entries["tool:demo"]
	e.Metadata.Description = "new metadata"
	b.Entries["tool:demo"] = e
	data, _ = Encode(b)
	if Digest(data) == old {
		t.Fatal("hash must identify metadata itself")
	}
	e.Script = "missing.yak"
	b.Entries["tool:demo"] = e
	data, _ = Encode(b)
	if _, err := Decode(data, "aitool", ""); err == nil {
		t.Fatal("invalid source mapping accepted")
	}
	b = fixture()
	b.Version++
	data, _ = Encode(b)
	if _, err := Decode(data, "aitool", ""); err == nil {
		t.Fatal("unsupported format accepted")
	}
}

func TestMalformedMetadataAndEmptyCLI(t *testing.T) {
	for _, tc := range []struct {
		key string
		m   Metadata
	}{
		{"tool:demo", Metadata{Parameters: json.RawMessage(`[]`), EnableAIOutputLog: 1}},
		{"tool:demo", Metadata{Parameters: json.RawMessage(`{}`), EnableAIOutputLog: 0}},
		{"plugin:yak:demo", Metadata{Parameters: json.RawMessage(`"invalid"`)}},
		{"plugin:yak:demo", Metadata{EnvironmentKeys: json.RawMessage(`"invalid"`)}},
		{"sf:demo.sf", Metadata{SyntaxFlowRule: json.RawMessage(`broken`)}},
	} {
		if validMetadata(tc.key, tc.m) {
			t.Errorf("invalid metadata accepted: %s", tc.key)
		}
	}
	for _, key := range []string{"plugin:yak:no-cli", "forge:no-cli"} {
		if !validMetadata(key, Metadata{}) {
			t.Fatal("empty CLI metadata is legitimate")
		}
	}
}

func TestReadableMetadataPreservesParameterDefaults(t *testing.T) {
	b := fixture()
	e := b.Entries["tool:demo"]
	e.Metadata.Parameters = json.RawMessage(`{"type":"object","properties":{"flag":{"default":false},"count":{"default":0},"name":{"default":""},"optional":{"default":null}}}`)
	b.Entries["tool:demo"] = e
	text, err := Text(b)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(text, []byte(`\"default\"`)) || bytes.Contains(text, []byte("source_sha256")) {
		t.Fatal("nested JSON or redundant script digest")
	}
	parsed, err := Parse(text, "aitool")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(parsed.Entries["tool:demo"].Metadata.Parameters, e.Metadata.Parameters) {
		t.Fatal("parameter defaults changed")
	}
	a, err := Encode(b)
	if err != nil {
		t.Fatal(err)
	}
	binary, err := Encode(parsed)
	if err != nil || !bytes.Equal(a, binary) {
		t.Fatal("text round trip changed the published hash", err)
	}
}

func archive(t *testing.T, files map[string]File, contents map[string]string) []byte {
	t.Helper()
	var out bytes.Buffer
	w := gzip.NewWriter(&out)
	tw := tar.NewWriter(w)
	for name, item := range files {
		if name == "." {
			continue
		}
		kind := byte(tar.TypeReg)
		if fs.FileMode(item.Mode).IsDir() {
			kind = tar.TypeDir
		}
		h := &tar.Header{Name: name, Mode: int64(fs.FileMode(item.Mode).Perm()), Size: item.Size, Typeflag: kind}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if kind == tar.TypeReg {
			if _, err := tw.Write([]byte(contents[name])); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestArchiveVerificationDetectsEverySetChange(t *testing.T) {
	b := fixture()
	contents := map[string]string{"demo.yak": `cli.String("target")`}
	b.Files["empty"] = File{Mode: uint32(fs.ModeDir | 0755)}
	if err := b.ValidateArchive(archive(t, b.Files, contents), ""); err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"add", "delete", "rename", "chmod", "directory-mode", "remove-empty-directory", "same-size-content"} {
		t.Run(change, func(t *testing.T) {
			files := map[string]File{}
			for k, v := range b.Files {
				files[k] = v
			}
			text := map[string]string{}
			for k, v := range contents {
				text[k] = v
			}
			switch change {
			case "add":
				files["extra.yak"] = files["demo.yak"]
				text["extra.yak"] = text["demo.yak"]
			case "delete":
				delete(files, "demo.yak")
			case "rename":
				files["other.yak"] = files["demo.yak"]
				delete(files, "demo.yak")
				text["other.yak"] = text["demo.yak"]
			case "chmod":
				f := files["demo.yak"]
				f.Mode = 0755
				files["demo.yak"] = f
			case "directory-mode":
				f := files["empty"]
				f.Mode = uint32(fs.ModeDir | 0700)
				files["empty"] = f
			case "remove-empty-directory":
				delete(files, "empty")
			case "same-size-content":
				text["demo.yak"] = `cli.String("change")`
			}
			if err := b.ValidateArchive(archive(t, files, text), ""); err == nil {
				t.Fatal("changed script set accepted")
			}
		})
	}
}

func TestScanDetectsPermissionsPathsAndContent(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "demo.yak")
	if err := os.WriteFile(file, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	before, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(file)
	if err := os.WriteFile(file, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(file, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	after, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if Fingerprint(before, "recipe") == Fingerprint(after, "recipe") {
		t.Fatal("same-size edit with preserved mtime was missed")
	}
	if Fingerprint(after, "recipe") == Fingerprint(after, "new extraction marker") {
		t.Fatal("extraction marker was ignored")
	}
}

func TestRuntimeArchiveValidationIsCached(t *testing.T) {
	if !Enabled {
		t.Skip("gzip runtime")
	}
	const kind = "aitool"
	old, had := resources.Load(kind)
	resources.Delete(kind)
	defer func() {
		if had {
			resources.Store(kind, old)
		} else {
			resources.Delete(kind)
		}
	}()
	calls := 0
	register := func() { Register(kind, "", func() ([]byte, error) { calls++; return nil, nil }) }
	for i := 0; i < 3; i++ {
		register()
		if hash, err := Hash(kind); err == nil || hash != "" {
			t.Fatal("invalid archive must never get a metadata sync hash")
		}
		if _, hit := Lookup("tool:demo", "anything"); hit {
			t.Fatal("invalid archive must not use metadata")
		}
	}
	if calls != 1 {
		t.Fatal("immutable embedded archives must only be verified once")
	}
}

func TestLocalHashCannotConfuseMetadataAndScripts(t *testing.T) {
	root := t.TempDir()
	b := fixture()
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.yak"), []byte(`cli.String("target")`), 0644); err != nil {
		t.Fatal(err)
	}
	files, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	b.Files = files
	b.SourceHash = Fingerprint(files, b.Recipe)
	data, err := Encode(b)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "aitool-metadata.json")
	text, err := Text(b)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, text, 0644); err != nil {
		t.Fatal(err)
	}
	hash, err := LocalHash(root, file, "aitool")
	if err != nil {
		t.Fatal(err)
	}
	if hash != Digest(data) || hash == b.SourceHash {
		t.Fatal("published hash must identify compressed metadata")
	}
	if err := os.WriteFile(filepath.Join(root, "extra.yak"), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LocalHash(root, file, "aitool"); err == nil {
		t.Fatal("stale metadata must not be published as current")
	}
}
