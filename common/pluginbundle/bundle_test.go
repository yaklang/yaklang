package pluginbundle

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractArchiveAndLoadDirectory(t *testing.T) {
	archive := testBundleArchive(t, "bundle-1", false)
	archivePath := filepath.Join(t.TempDir(), "bundle.zip")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "installed")
	bundle, err := ExtractArchive(archivePath, destination, Expected{
		BundleID:      "bundle-1",
		SchemaVersion: ManifestSchemaVersion,
		ItemCount:     1,
	})
	if err != nil {
		t.Fatalf("ExtractArchive() error = %v", err)
	}
	if len(bundle.Members) != 1 || bundle.Members[0].Document.Name != "probe-a" ||
		bundle.Members[0].Document.Type != "port-scan" {
		t.Fatalf("unexpected bundle = %#v", bundle)
	}
	if _, err := LoadDirectory(destination, Expected{BundleID: "other", ItemCount: 1}); err == nil {
		t.Fatal("LoadDirectory() accepted a mismatched bundle id")
	}
}

func TestExtractArchiveRejectsTraversal(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create("../escape")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte("escape"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "bundle.zip")
	if err := os.WriteFile(archivePath, buffer.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = ExtractArchive(archivePath, filepath.Join(t.TempDir(), "installed"), Expected{})
	if err == nil || !strings.Contains(err.Error(), "unsafe plugin bundle member path") {
		t.Fatalf("error = %v, want traversal rejection", err)
	}
}

func TestExtractArchiveRejectsUnknownMemberField(t *testing.T) {
	archive := testBundleArchive(t, "bundle-1", true)
	archivePath := filepath.Join(t.TempDir(), "bundle.zip")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ExtractArchive(archivePath, filepath.Join(t.TempDir(), "installed"), Expected{
		BundleID: "bundle-1", ItemCount: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v, want strict schema rejection", err)
	}
}

func TestExtractArchiveRejectsNASLWithoutSelfContainedDependencies(t *testing.T) {
	archive := testBundleArchiveWithType(t, "bundle-1", false, "nasl")
	archivePath := filepath.Join(t.TempDir(), "bundle.zip")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ExtractArchive(archivePath, filepath.Join(t.TempDir(), "installed"), Expected{
		BundleID: "bundle-1", ItemCount: 1,
	})
	if err == nil || !strings.Contains(err.Error(), `unsupported script type "nasl"`) {
		t.Fatalf("error = %v, want explicit NASL rejection", err)
	}
}

func TestDecodeManifestRejectsMultipleReleasesOfOnePlugin(t *testing.T) {
	raw, err := json.Marshal(Manifest{
		SchemaVersion: ManifestSchemaVersion,
		BundleID:      "bundle-1",
		Name:          "Bundle 1",
		ItemCount:     2,
		Items: []ManifestItem{
			{
				PluginID: "plugin-1", ReleaseID: "release-1", Name: "probe-1", Version: "1.0.0",
				EntryKind: "yak_script", Path: "plugins/release-1/plugin.json",
				ContentSHA256: strings.Repeat("a", 64), SizeBytes: 1,
			},
			{
				PluginID: "plugin-1", ReleaseID: "release-2", Name: "probe-2", Version: "2.0.0",
				EntryKind: "yak_script", Path: "plugins/release-2/plugin.json",
				ContentSHA256: strings.Repeat("b", 64), SizeBytes: 1,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = decodeManifest(raw, Expected{BundleID: "bundle-1", ItemCount: 2})
	if err == nil || !strings.Contains(err.Error(), "selects multiple releases") {
		t.Fatalf("error = %v, want duplicate plugin rejection", err)
	}
}

func testBundleArchive(t *testing.T, bundleID string, unknownMemberField bool) []byte {
	return testBundleArchiveWithType(t, bundleID, unknownMemberField, "port-scan")
}

func testBundleArchiveWithType(t *testing.T, bundleID string, unknownMemberField bool, scriptType string) []byte {
	t.Helper()
	content := "handle = func(result) { return }"
	contentSum := sha256.Sum256([]byte(content))
	member := map[string]any{
		"schema_version":        MemberSchemaVersion,
		"plugin_id":             "plugin-1",
		"release_id":            "release-1",
		"name":                  "probe-a",
		"type":                  scriptType,
		"version":               "1.0.0",
		"entry_kind":            "yak_script",
		"content":               content,
		"enabled":               true,
		"status":                "published",
		"script_content_sha256": hex.EncodeToString(contentSum[:]),
		"script_size_bytes":     len(content),
	}
	if unknownMemberField {
		member["unreviewed_executable_field"] = true
	}
	memberRaw, err := json.Marshal(member)
	if err != nil {
		t.Fatal(err)
	}
	memberSum := sha256.Sum256(memberRaw)
	manifestRaw, err := json.Marshal(Manifest{
		SchemaVersion: ManifestSchemaVersion,
		BundleID:      bundleID,
		Name:          "Bundle 1",
		ItemCount:     1,
		Items: []ManifestItem{{
			PluginID:      "plugin-1",
			ReleaseID:     "release-1",
			Name:          "probe-a",
			Version:       "1.0.0",
			EntryKind:     "yak_script",
			Path:          "plugins/release-1/plugin.json",
			ContentSHA256: hex.EncodeToString(memberSum[:]),
			SizeBytes:     int64(len(memberRaw)),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, file := range []struct {
		name string
		raw  []byte
	}{{ManifestPath, manifestRaw}, {"plugins/release-1/plugin.json", memberRaw}} {
		header := &zip.FileHeader{Name: file.name, Method: zip.Store}
		header.SetMode(0o600)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(file.raw); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
