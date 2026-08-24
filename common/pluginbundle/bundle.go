package pluginbundle

import (
	"archive/zip"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const (
	ManifestSchemaVersion = "legion_plugin_bundle.v1"
	MemberSchemaVersion   = "legion_plugin_bundle_member.v1"
	ManifestPath          = "manifest.json"

	MaxArtifactSize = int64(256 << 20)
	MaxManifestSize = int64(8 << 20)
	MaxMemberSize   = int64(16 << 20)
	MaxMemberCount  = 10_000
)

var supportedScriptTypes = map[string]struct{}{
	"port-scan": {},
	"nuclei":    {},
}

type Expected struct {
	BundleID      string
	SchemaVersion string
	ItemCount     int
}

type Manifest struct {
	SchemaVersion string         `json:"schema_version"`
	BundleID      string         `json:"bundle_id"`
	Name          string         `json:"name"`
	Description   string         `json:"description,omitempty"`
	ItemCount     int            `json:"item_count"`
	Items         []ManifestItem `json:"items"`
}

type ManifestItem struct {
	PluginID      string `json:"plugin_id"`
	ReleaseID     string `json:"release_id"`
	Name          string `json:"name"`
	Version       string `json:"version"`
	EntryKind     string `json:"entry_kind"`
	Path          string `json:"path"`
	ContentSHA256 string `json:"content_sha256"`
	SizeBytes     int64  `json:"size_bytes"`
}

// MemberDocument mirrors the immutable v1 document emitted by Legion. Fields
// not needed by the runtime stay as RawMessage, but are still named here so a
// future producer cannot silently add unreviewed executable metadata.
type MemberDocument struct {
	SchemaVersion          string            `json:"schema_version"`
	PluginID               string            `json:"plugin_id"`
	ReleaseID              string            `json:"release_id"`
	Name                   string            `json:"name"`
	Type                   string            `json:"type"`
	Version                string            `json:"version"`
	EntryKind              string            `json:"entry_kind"`
	Content                string            `json:"content"`
	DataScope              string            `json:"data_scope"`
	OwnerUserID            string            `json:"owner_user_id,omitempty"`
	ProductKey             string            `json:"product_key"`
	DisplayName            string            `json:"display_name"`
	Category               string            `json:"category,omitempty"`
	SourceType             string            `json:"source_type"`
	DefaultReleaseID       string            `json:"default_release_id,omitempty"`
	Description            string            `json:"description,omitempty"`
	DefaultArgumentsSchema json.RawMessage   `json:"default_arguments_schema,omitempty"`
	Enabled                bool              `json:"enabled"`
	PluginLabels           map[string]string `json:"plugin_labels"`
	Status                 string            `json:"status"`
	ScriptContentSHA256    string            `json:"script_content_sha256"`
	ScriptSizeBytes        int64             `json:"script_size_bytes"`
	RuntimeMinVersion      string            `json:"runtime_min_version,omitempty"`
	RuntimeMaxVersion      string            `json:"runtime_max_version,omitempty"`
	CLIParameters          json.RawMessage   `json:"cli_parameters"`
	DispatchContract       json.RawMessage   `json:"dispatch_contract"`
	WorkloadContract       json.RawMessage   `json:"workload_contract"`
	ReportContract         json.RawMessage   `json:"report_contract"`
	Manifest               json.RawMessage   `json:"manifest,omitempty"`
	ReleaseLabels          map[string]string `json:"release_labels"`
}

type Member struct {
	Manifest ManifestItem
	Document MemberDocument
}

type Bundle struct {
	Manifest Manifest
	Members  []Member
	RootDir  string
}

func ExtractArchive(archivePath, destination string, expected Expected) (Bundle, error) {
	info, err := os.Stat(archivePath)
	if err != nil {
		return Bundle{}, fmt.Errorf("stat plugin bundle archive: %w", err)
	}
	if info.Size() <= 0 || info.Size() > MaxArtifactSize {
		return Bundle{}, fmt.Errorf("plugin bundle archive size is outside the allowed range")
	}
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return Bundle{}, fmt.Errorf("open plugin bundle archive: %w", err)
	}
	defer reader.Close()

	entries := make(map[string]*zip.File, len(reader.File))
	for _, entry := range reader.File {
		name := entry.Name
		if err := validateArchivePath(name); err != nil {
			return Bundle{}, err
		}
		if entry.FileInfo().IsDir() || !entry.Mode().IsRegular() {
			return Bundle{}, fmt.Errorf("plugin bundle entry %q is not a regular file", name)
		}
		if _, exists := entries[name]; exists {
			return Bundle{}, fmt.Errorf("plugin bundle contains duplicate entry %q", name)
		}
		entries[name] = entry
	}
	manifestEntry := entries[ManifestPath]
	if manifestEntry == nil {
		return Bundle{}, errors.New("plugin bundle manifest is missing")
	}
	manifestRaw, err := readZIPEntry(manifestEntry, MaxManifestSize)
	if err != nil {
		return Bundle{}, fmt.Errorf("read plugin bundle manifest: %w", err)
	}
	manifest, err := decodeManifest(manifestRaw, expected)
	if err != nil {
		return Bundle{}, err
	}
	if len(entries) != len(manifest.Items)+1 {
		return Bundle{}, fmt.Errorf("plugin bundle contains undeclared entries")
	}
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return Bundle{}, fmt.Errorf("create plugin bundle destination: %w", err)
	}
	if err := writeRegularFile(filepath.Join(destination, ManifestPath), manifestRaw); err != nil {
		return Bundle{}, err
	}

	totalUncompressed := int64(len(manifestRaw))
	for _, item := range manifest.Items {
		entry := entries[item.Path]
		if entry == nil {
			return Bundle{}, fmt.Errorf("plugin bundle member %q is missing", item.Path)
		}
		if int64(entry.UncompressedSize64) != item.SizeBytes {
			return Bundle{}, fmt.Errorf("plugin bundle member %q size does not match manifest", item.Path)
		}
		totalUncompressed += item.SizeBytes
		if totalUncompressed > MaxArtifactSize {
			return Bundle{}, fmt.Errorf("plugin bundle uncompressed size exceeds limit")
		}
		raw, err := readZIPEntry(entry, MaxMemberSize)
		if err != nil {
			return Bundle{}, fmt.Errorf("read plugin bundle member %q: %w", item.Path, err)
		}
		if err := validateMember(raw, item); err != nil {
			return Bundle{}, err
		}
		localPath := filepath.Join(destination, filepath.FromSlash(item.Path))
		if err := os.MkdirAll(filepath.Dir(localPath), 0o700); err != nil {
			return Bundle{}, fmt.Errorf("create plugin bundle member directory: %w", err)
		}
		if err := writeRegularFile(localPath, raw); err != nil {
			return Bundle{}, err
		}
	}
	return LoadDirectory(destination, expected)
}

func LoadDirectory(root string, expected Expected) (Bundle, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return Bundle{}, errors.New("plugin bundle directory is required")
	}
	manifestRaw, err := readRegularFile(filepath.Join(root, ManifestPath), MaxManifestSize)
	if err != nil {
		return Bundle{}, fmt.Errorf("read installed plugin bundle manifest: %w", err)
	}
	manifest, err := decodeManifest(manifestRaw, expected)
	if err != nil {
		return Bundle{}, err
	}
	members := make([]Member, len(manifest.Items))
	declared := map[string]struct{}{ManifestPath: {}}
	for index, item := range manifest.Items {
		declared[item.Path] = struct{}{}
		raw, err := readRegularFile(filepath.Join(root, filepath.FromSlash(item.Path)), MaxMemberSize)
		if err != nil {
			return Bundle{}, fmt.Errorf("read installed plugin bundle member %q: %w", item.Path, err)
		}
		document, err := decodeMember(raw, item)
		if err != nil {
			return Bundle{}, err
		}
		members[index] = Member{Manifest: item, Document: document}
	}
	if err := rejectUnexpectedFiles(root, declared); err != nil {
		return Bundle{}, err
	}
	return Bundle{Manifest: manifest, Members: members, RootDir: root}, nil
}

func decodeManifest(raw []byte, expected Expected) (Manifest, error) {
	var manifest Manifest
	if err := decodeStrict(raw, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode plugin bundle manifest: %w", err)
	}
	schemaVersion := strings.TrimSpace(expected.SchemaVersion)
	if schemaVersion == "" {
		schemaVersion = ManifestSchemaVersion
	}
	if manifest.SchemaVersion != schemaVersion {
		return Manifest{}, fmt.Errorf("unsupported plugin bundle schema_version %q", manifest.SchemaVersion)
	}
	if strings.TrimSpace(manifest.BundleID) == "" ||
		(strings.TrimSpace(expected.BundleID) != "" && manifest.BundleID != strings.TrimSpace(expected.BundleID)) {
		return Manifest{}, fmt.Errorf("plugin bundle id does not match expected identity")
	}
	if strings.TrimSpace(manifest.Name) == "" {
		return Manifest{}, errors.New("plugin bundle name is required")
	}
	if manifest.ItemCount <= 0 || manifest.ItemCount > MaxMemberCount || manifest.ItemCount != len(manifest.Items) {
		return Manifest{}, errors.New("plugin bundle item_count is invalid")
	}
	if expected.ItemCount > 0 && manifest.ItemCount != expected.ItemCount {
		return Manifest{}, errors.New("plugin bundle item_count does not match dispatch metadata")
	}
	seenPaths := make(map[string]struct{}, len(manifest.Items))
	seenPlugins := make(map[string]struct{}, len(manifest.Items))
	seenReleases := make(map[string]struct{}, len(manifest.Items))
	seenNames := make(map[string]struct{}, len(manifest.Items))
	previousPath := ""
	for _, item := range manifest.Items {
		if strings.TrimSpace(item.PluginID) == "" || strings.TrimSpace(item.ReleaseID) == "" ||
			strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Version) == "" || item.EntryKind != "yak_script" {
			return Manifest{}, fmt.Errorf("plugin bundle member identity is incomplete")
		}
		if err := validateMemberPath(item.Path); err != nil {
			return Manifest{}, err
		}
		if previousPath != "" && item.Path <= previousPath {
			return Manifest{}, errors.New("plugin bundle members are not in canonical path order")
		}
		previousPath = item.Path
		if _, exists := seenPaths[item.Path]; exists {
			return Manifest{}, fmt.Errorf("plugin bundle member path %q is duplicated", item.Path)
		}
		seenPaths[item.Path] = struct{}{}
		if _, exists := seenReleases[item.ReleaseID]; exists {
			return Manifest{}, fmt.Errorf("plugin bundle release %q is duplicated", item.ReleaseID)
		}
		seenReleases[item.ReleaseID] = struct{}{}
		if _, exists := seenPlugins[item.PluginID]; exists {
			return Manifest{}, fmt.Errorf("plugin bundle plugin %q selects multiple releases", item.PluginID)
		}
		seenPlugins[item.PluginID] = struct{}{}
		if _, exists := seenNames[item.Name]; exists {
			return Manifest{}, fmt.Errorf("plugin bundle script name %q is duplicated", item.Name)
		}
		seenNames[item.Name] = struct{}{}
		if item.SizeBytes <= 0 || item.SizeBytes > MaxMemberSize || !validSHA256(item.ContentSHA256) {
			return Manifest{}, fmt.Errorf("plugin bundle member %q integrity metadata is invalid", item.Path)
		}
	}
	return manifest, nil
}

func validateMember(raw []byte, item ManifestItem) error {
	_, err := decodeMember(raw, item)
	return err
}

func decodeMember(raw []byte, item ManifestItem) (MemberDocument, error) {
	if int64(len(raw)) != item.SizeBytes || sha256Hex(raw) != strings.ToLower(item.ContentSHA256) {
		return MemberDocument{}, fmt.Errorf("plugin bundle member %q digest or size mismatch", item.Path)
	}
	var document MemberDocument
	if err := decodeStrict(raw, &document); err != nil {
		return MemberDocument{}, fmt.Errorf("decode plugin bundle member %q: %w", item.Path, err)
	}
	if document.SchemaVersion != MemberSchemaVersion || document.PluginID != item.PluginID ||
		document.ReleaseID != item.ReleaseID || document.Name != item.Name ||
		document.Version != item.Version || document.EntryKind != item.EntryKind {
		return MemberDocument{}, fmt.Errorf("plugin bundle member %q identity does not match manifest", item.Path)
	}
	document.Type = strings.ToLower(strings.TrimSpace(document.Type))
	if _, supported := supportedScriptTypes[document.Type]; !supported {
		return MemberDocument{}, fmt.Errorf("plugin bundle member %q has unsupported script type %q", item.Path, document.Type)
	}
	if document.Status != "published" || !document.Enabled || document.Content == "" {
		return MemberDocument{}, fmt.Errorf("plugin bundle member %q is not an enabled published script", item.Path)
	}
	if document.ScriptSizeBytes != int64(len(document.Content)) ||
		!validSHA256(document.ScriptContentSHA256) ||
		sha256Hex([]byte(document.Content)) != strings.ToLower(document.ScriptContentSHA256) {
		return MemberDocument{}, fmt.Errorf("plugin bundle member %q script digest or size mismatch", item.Path)
	}
	return document, nil
}

func validateArchivePath(value string) error {
	if value == ManifestPath {
		return nil
	}
	return validateMemberPath(value)
}

func validateMemberPath(value string) error {
	if value == "" || strings.Contains(value, "\\") || strings.Contains(value, ":") ||
		strings.HasPrefix(value, "/") || path.Clean(value) != value ||
		!strings.HasPrefix(value, "plugins/") || !strings.HasSuffix(value, "/plugin.json") {
		return fmt.Errorf("unsafe plugin bundle member path %q", value)
	}
	return nil
}

func readZIPEntry(entry *zip.File, limit int64) ([]byte, error) {
	if int64(entry.UncompressedSize64) <= 0 || int64(entry.UncompressedSize64) > limit {
		return nil, errors.New("entry size exceeds limit")
	}
	reader, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return readLimited(reader, limit)
}

func readRegularFile(filename string, limit int64) ([]byte, error) {
	info, err := os.Lstat(filename)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > limit {
		return nil, errors.New("file is not a bounded regular file")
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return readLimited(file, limit)
}

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	limited := io.LimitReader(reader, limit+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, errors.New("content exceeds limit")
	}
	return raw, nil
}

func writeRegularFile(filename string, content []byte) error {
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create installed plugin bundle file: %w", err)
	}
	writer := bufio.NewWriter(file)
	if _, err := writer.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	if err := writer.Flush(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return nil
}

func rejectUnexpectedFiles(root string, declared map[string]struct{}) error {
	var files []string
	err := filepath.WalkDir(root, func(filename string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filename == root {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("installed plugin bundle contains a symbolic link")
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("installed plugin bundle contains a non-regular file")
		}
		relative, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(files)
	if len(files) != len(declared) {
		return errors.New("installed plugin bundle contains undeclared files")
	}
	for _, filename := range files {
		if _, ok := declared[filename]; !ok {
			return fmt.Errorf("installed plugin bundle contains undeclared file %q", filename)
		}
	}
	return nil
}

func decodeStrict(raw []byte, output any) error {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("unexpected trailing JSON content")
	}
	return nil
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(strings.TrimSpace(value))
	return err == nil && len(decoded) == sha256.Size
}

func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
