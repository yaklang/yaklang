package bin_parser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

func validateProtocolCorpusJSONSchemaFiles(corpusDir, schemaName, instanceName string) error {
	schemaData, err := os.ReadFile(filepath.Join(corpusDir, schemaName))
	if err != nil {
		return fmt.Errorf("read schema %s: %w", schemaName, err)
	}
	instanceData, err := os.ReadFile(filepath.Join(corpusDir, instanceName))
	if err != nil {
		return fmt.Errorf("read instance %s: %w", instanceName, err)
	}
	return validateProtocolCorpusJSONSchema(schemaData, instanceData)
}

func validateProtocolCorpusJSONSchema(schemaData, instanceData []byte) error {
	schemaDocument, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaData))
	if err != nil {
		return fmt.Errorf("decode schema: %w", err)
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(instanceData))
	if err != nil {
		return fmt.Errorf("decode instance: %w", err)
	}

	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	const schemaLocation = "https://yaklang.invalid/protocol-corpus.schema.json"
	if err := compiler.AddResource(schemaLocation, schemaDocument); err != nil {
		return fmt.Errorf("register schema: %w", err)
	}
	compiled, err := compiler.Compile(schemaLocation)
	if err != nil {
		return fmt.Errorf("compile schema: %w", err)
	}
	if err := compiled.Validate(instance); err != nil {
		return fmt.Errorf("schema validation: %w", err)
	}
	return nil
}

func validateProtocolCorpusSourceManifest(source protocolCorpusSourceSpec, manifest protocolCorpusManifest) error {
	var problems []string

	sourceRepositories := make(map[string]protocolCorpusSourceRepository, len(source.Repositories))
	for _, repository := range source.Repositories {
		if _, exists := sourceRepositories[repository.ID]; exists {
			problems = append(problems, fmt.Sprintf("sources.json has duplicate repository id %q", repository.ID))
		}
		sourceRepositories[repository.ID] = repository
	}
	manifestRepositories := make(map[string]protocolCorpusRepository, len(manifest.Repositories))
	for _, repository := range manifest.Repositories {
		if _, exists := manifestRepositories[repository.ID]; exists {
			problems = append(problems, fmt.Sprintf("manifest.json has duplicate repository id %q", repository.ID))
		}
		manifestRepositories[repository.ID] = repository
	}
	for _, id := range sortedProtocolCorpusKeys(sourceRepositories, manifestRepositories) {
		sourceRepository, inSource := sourceRepositories[id]
		manifestRepository, inManifest := manifestRepositories[id]
		switch {
		case !inSource:
			problems = append(problems, fmt.Sprintf("repository %q is absent from sources.json", id))
		case !inManifest:
			problems = append(problems, fmt.Sprintf("repository %q is absent from manifest.json", id))
		default:
			problems = appendProtocolCorpusStringMismatch(problems, "repository", id, "repository", sourceRepository.Repository, manifestRepository.Repository)
			problems = appendProtocolCorpusStringMismatch(problems, "repository", id, "commit", sourceRepository.Commit, manifestRepository.Commit)
			problems = appendProtocolCorpusStringMismatch(problems, "repository", id, "license", sourceRepository.License, manifestRepository.License)
			problems = appendProtocolCorpusStringMismatch(problems, "repository", id, "license_sha256", sourceRepository.LicenseSHA256, manifestRepository.LicenseSHA256)
			problems = appendProtocolCorpusStringMismatch(problems, "repository", id, "homepage", sourceRepository.Homepage, manifestRepository.Homepage)
		}
	}

	sourceCaptures := make(map[string]protocolCorpusSourceCapture, len(source.Captures))
	for _, capture := range source.Captures {
		if _, exists := sourceCaptures[capture.ID]; exists {
			problems = append(problems, fmt.Sprintf("sources.json has duplicate capture id %q", capture.ID))
		}
		if _, exists := sourceRepositories[capture.RepositoryID]; !exists {
			problems = append(problems, fmt.Sprintf("source capture %q refers to unknown repository %q", capture.ID, capture.RepositoryID))
		}
		sourceCaptures[capture.ID] = capture
	}
	manifestCaptures := make(map[string]protocolCorpusCapture, len(manifest.Captures))
	for _, capture := range manifest.Captures {
		if _, exists := manifestCaptures[capture.ID]; exists {
			problems = append(problems, fmt.Sprintf("manifest.json has duplicate capture id %q", capture.ID))
		}
		if _, exists := manifestRepositories[capture.RepositoryID]; !exists {
			problems = append(problems, fmt.Sprintf("manifest capture %q refers to unknown repository %q", capture.ID, capture.RepositoryID))
		}
		manifestCaptures[capture.ID] = capture
	}
	for _, id := range sortedProtocolCorpusKeys(sourceCaptures, manifestCaptures) {
		sourceCapture, inSource := sourceCaptures[id]
		manifestCapture, inManifest := manifestCaptures[id]
		switch {
		case !inSource:
			problems = append(problems, fmt.Sprintf("capture %q is absent from sources.json", id))
		case !inManifest:
			problems = append(problems, fmt.Sprintf("capture %q is absent from manifest.json", id))
		default:
			problems = appendProtocolCorpusStringMismatch(problems, "capture", id, "repository_id", sourceCapture.RepositoryID, manifestCapture.RepositoryID)
			problems = appendProtocolCorpusStringMismatch(problems, "capture", id, "upstream_path", sourceCapture.UpstreamPath, manifestCapture.UpstreamPath)
			problems = appendProtocolCorpusStringMismatch(problems, "capture", id, "protocol", sourceCapture.Protocol, manifestCapture.Protocol)
			if !equalProtocolCorpusOptionalString(sourceCapture.RoadmapName, manifestCapture.RoadmapName) {
				problems = append(problems, fmt.Sprintf("capture %q field roadmap_name differs: sources=%s manifest=%s", id, formatProtocolCorpusOptionalString(sourceCapture.RoadmapName), formatProtocolCorpusOptionalString(manifestCapture.RoadmapName)))
			}
			problems = appendProtocolCorpusStringMismatch(problems, "capture", id, "display_filter", sourceCapture.DisplayFilter, manifestCapture.DisplayFilter)
			problems = appendProtocolCorpusStringMismatch(problems, "capture", id, "decode_as", strings.Join(sourceCapture.DecodeAs, "\n"), strings.Join(manifestCapture.DecodeAs, "\n"))
			problems = appendProtocolCorpusStringMismatch(problems, "capture", id, "evidence_kind", sourceCapture.EvidenceKind, manifestCapture.EvidenceKind)
			problems = appendProtocolCorpusStringMismatch(problems, "capture", id, "notes", sourceCapture.Notes, manifestCapture.Notes)
			problems = appendProtocolCorpusStringMismatch(problems, "capture", id, "source_sha256", sourceCapture.SourceSHA256, manifestCapture.SHA256)
			if repository, exists := sourceRepositories[sourceCapture.RepositoryID]; exists {
				wantSourceURL := "https://raw.githubusercontent.com/" + repository.Repository + "/" + repository.Commit + "/" + sourceCapture.UpstreamPath
				if repository.Origin == "generated" {
					wantSourceURL = "generated://scapy/" + repository.Commit + "/" + sourceCapture.ID
				}
				if manifestCapture.SourceURL != wantSourceURL {
					problems = append(problems, fmt.Sprintf("capture %q field source_url is not derived exactly from its source repository: got=%q want=%q", id, manifestCapture.SourceURL, wantSourceURL))
				}
			}
			if sourceCapture.AllowEmpty != (manifestCapture.PacketCount == 0) {
				problems = append(problems, fmt.Sprintf("capture %q field allow_empty differs from manifest packet_count: allow_empty=%t packet_count=%d", id, sourceCapture.AllowEmpty, manifestCapture.PacketCount))
			}
		}
	}

	return protocolCorpusProblems(problems)
}

func appendProtocolCorpusStringMismatch(problems []string, kind, id, field, sourceValue, manifestValue string) []string {
	if sourceValue != manifestValue {
		return append(problems, fmt.Sprintf("%s %q field %s differs: sources=%q manifest=%q", kind, id, field, sourceValue, manifestValue))
	}
	return problems
}

func equalProtocolCorpusOptionalString(left, right *string) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func formatProtocolCorpusOptionalString(value *string) string {
	if value == nil {
		return "null"
	}
	return fmt.Sprintf("%q", *value)
}

func sortedProtocolCorpusKeys[A, B any](left map[string]A, right map[string]B) []string {
	keys := make(map[string]struct{}, len(left)+len(right))
	for key := range left {
		keys[key] = struct{}{}
	}
	for key := range right {
		keys[key] = struct{}{}
	}
	result := make([]string, 0, len(keys))
	for key := range keys {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func validateProtocolCorpusArtifactInventory(corpusDir string, manifest protocolCorpusManifest) error {
	captureFiles, captureErr := collectProtocolCorpusArtifacts(corpusDir, "captures")
	licenseFiles, licenseErr := collectProtocolCorpusArtifacts(corpusDir, "licenses")

	var problems []string
	for _, item := range []struct {
		name string
		err  error
	}{
		{name: "capture", err: captureErr},
		{name: "license", err: licenseErr},
	} {
		if item.err != nil {
			problems = append(problems, fmt.Sprintf("enumerate %s artifacts: %v", item.name, item.err))
		}
	}

	captureReferences := make([]string, 0, len(manifest.Captures))
	for _, capture := range manifest.Captures {
		captureReferences = append(captureReferences, capture.CaptureFile)
	}
	licenseReferences := make([]string, 0, len(manifest.Repositories))
	for _, repository := range manifest.Repositories {
		licenseReferences = append(licenseReferences, repository.LicenseFile)
	}

	problems = append(problems, validateProtocolCorpusArtifactSet("capture", "captures", captureFiles, captureReferences, ".pcap", ".pcapng", ".cap")...)
	problems = append(problems, validateProtocolCorpusArtifactSet("license", "licenses", licenseFiles, licenseReferences)...)
	return protocolCorpusProblems(problems)
}

func collectProtocolCorpusArtifacts(corpusDir, subdirectory string) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	root := filepath.Join(corpusDir, subdirectory)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink is not allowed: %s", path)
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(corpusDir, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		result[relative] = struct{}{}
		return nil
	})
	return result, err
}

func validateProtocolCorpusArtifactSet(kind, subdirectory string, actual map[string]struct{}, references []string, allowedExtensions ...string) []string {
	var problems []string
	counts := make(map[string]int, len(references))
	for _, reference := range references {
		normalized, err := normalizeProtocolCorpusArtifactReference(reference, subdirectory)
		if err != nil {
			problems = append(problems, fmt.Sprintf("invalid %s reference %q: %v", kind, reference, err))
			continue
		}
		if len(allowedExtensions) > 0 && !protocolCorpusExtensionAllowed(normalized, allowedExtensions) {
			problems = append(problems, fmt.Sprintf("invalid %s reference %q: extension must be one of %s", kind, reference, strings.Join(allowedExtensions, ", ")))
			continue
		}
		counts[normalized]++
	}

	actualNames := make([]string, 0, len(actual))
	for name := range actual {
		actualNames = append(actualNames, name)
	}
	sort.Strings(actualNames)
	for _, name := range actualNames {
		switch counts[name] {
		case 0:
			problems = append(problems, fmt.Sprintf("orphan %s artifact %q is not referenced", kind, name))
		case 1:
		default:
			problems = append(problems, fmt.Sprintf("%s artifact %q is referenced %d times", kind, name, counts[name]))
		}
	}

	referenceNames := make([]string, 0, len(counts))
	for name := range counts {
		referenceNames = append(referenceNames, name)
	}
	sort.Strings(referenceNames)
	for _, name := range referenceNames {
		if _, exists := actual[name]; !exists {
			problems = append(problems, fmt.Sprintf("referenced %s artifact %q does not exist", kind, name))
		}
	}
	return problems
}

func protocolCorpusExtensionAllowed(path string, allowed []string) bool {
	extension := strings.ToLower(filepath.Ext(path))
	for _, candidate := range allowed {
		if extension == candidate {
			return true
		}
	}
	return false
}

func normalizeProtocolCorpusArtifactReference(reference, subdirectory string) (string, error) {
	if reference == "" {
		return "", fmt.Errorf("path is empty")
	}
	if strings.Contains(reference, "\\") {
		return "", fmt.Errorf("path must use forward slashes")
	}
	clean := filepath.Clean(filepath.FromSlash(reference))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes the corpus directory")
	}
	normalized := filepath.ToSlash(clean)
	if normalized != reference {
		return "", fmt.Errorf("path is not canonical (want %q)", normalized)
	}
	if !strings.HasPrefix(normalized, subdirectory+"/") {
		return "", fmt.Errorf("path is outside %s/", subdirectory)
	}
	return normalized, nil
}

func protocolCorpusProblems(problems []string) error {
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(problems, "; "))
}

func TestProtocolCorpusArtifactInventoryFailsClosed(t *testing.T) {
	type mutation func(t *testing.T, corpusDir string, manifest *protocolCorpusManifest)
	tests := []struct {
		name     string
		want     string
		mutation mutation
	}{
		{
			name: "orphan capture",
			want: "orphan capture artifact",
			mutation: func(t *testing.T, corpusDir string, _ *protocolCorpusManifest) {
				writeProtocolCorpusTestArtifact(t, corpusDir, "captures/repo/orphan.pcap")
			},
		},
		{
			name: "stray capture extension",
			want: "orphan capture artifact",
			mutation: func(t *testing.T, corpusDir string, _ *protocolCorpusManifest) {
				writeProtocolCorpusTestArtifact(t, corpusDir, "captures/repo/stray.bak")
			},
		},
		{
			name: "orphan license",
			want: "orphan license artifact",
			mutation: func(t *testing.T, corpusDir string, _ *protocolCorpusManifest) {
				writeProtocolCorpusTestArtifact(t, corpusDir, "licenses/orphan.txt")
			},
		},
		{
			name: "duplicate capture reference",
			want: "capture artifact \"captures/repo/sample.pcap\" is referenced 2 times",
			mutation: func(_ *testing.T, _ string, manifest *protocolCorpusManifest) {
				manifest.Captures = append(manifest.Captures, protocolCorpusCapture{ID: "duplicate", CaptureFile: "captures/repo/sample.pcap"})
			},
		},
		{
			name: "duplicate license reference",
			want: "license artifact \"licenses/repo.txt\" is referenced 2 times",
			mutation: func(_ *testing.T, _ string, manifest *protocolCorpusManifest) {
				manifest.Repositories = append(manifest.Repositories, protocolCorpusRepository{ID: "duplicate", LicenseFile: "licenses/repo.txt"})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			corpusDir, manifest := newProtocolCorpusInventoryFixture(t)
			if err := validateProtocolCorpusArtifactInventory(corpusDir, manifest); err != nil {
				t.Fatalf("valid fixture rejected before mutation: %v", err)
			}
			test.mutation(t, corpusDir, &manifest)
			err := validateProtocolCorpusArtifactInventory(corpusDir, manifest)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("mutation was not rejected with %q: %v", test.want, err)
			}
		})
	}
}

func newProtocolCorpusInventoryFixture(t *testing.T) (string, protocolCorpusManifest) {
	t.Helper()
	corpusDir := t.TempDir()
	writeProtocolCorpusTestArtifact(t, corpusDir, "captures/repo/sample.pcap")
	writeProtocolCorpusTestArtifact(t, corpusDir, "licenses/repo.txt")
	return corpusDir, protocolCorpusManifest{
		Repositories: []protocolCorpusRepository{{ID: "repo", LicenseFile: "licenses/repo.txt"}},
		Captures: []protocolCorpusCapture{{
			ID:          "sample",
			CaptureFile: "captures/repo/sample.pcap",
		}},
	}
}

func writeProtocolCorpusTestArtifact(t *testing.T, corpusDir, relativeName string) {
	t.Helper()
	name := filepath.Join(corpusDir, filepath.FromSlash(relativeName))
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProtocolCorpusSourceManifestMetadataFailsClosed(t *testing.T) {
	roadmapName := "DNS"
	newFixture := func() (protocolCorpusSourceSpec, protocolCorpusManifest) {
		return protocolCorpusSourceSpec{
				Repositories: []protocolCorpusSourceRepository{{
					ID: "repo", Repository: "owner/repo", Commit: strings.Repeat("a", 40),
					License: "BSD-3-Clause", LicensePath: "LICENSE", LicenseSHA256: strings.Repeat("b", 64), Homepage: "https://example.test/",
				}},
				Captures: []protocolCorpusSourceCapture{{
					ID: "sample", RepositoryID: "repo", UpstreamPath: "captures/sample.pcap",
					Protocol: "DNS", RoadmapName: &roadmapName, DisplayFilter: "dns",
					EvidenceKind: "upstream-positive", Notes: "representative sample", SourceSHA256: strings.Repeat("c", 64),
				}},
			}, protocolCorpusManifest{
				Repositories: []protocolCorpusRepository{{
					ID: "repo", Repository: "owner/repo", Commit: strings.Repeat("a", 40),
					License: "BSD-3-Clause", Homepage: "https://example.test/",
					LicenseFile: "licenses/repo.txt", LicenseSHA256: strings.Repeat("b", 64),
				}},
				Captures: []protocolCorpusCapture{{
					ID: "sample", RepositoryID: "repo", UpstreamPath: "captures/sample.pcap",
					Protocol: "DNS", RoadmapName: &roadmapName, DisplayFilter: "dns",
					EvidenceKind: "upstream-positive", Notes: "representative sample", SHA256: strings.Repeat("c", 64), PacketCount: 1,
					SourceURL: "https://raw.githubusercontent.com/owner/repo/" + strings.Repeat("a", 40) + "/captures/sample.pcap",
				}},
			}
	}

	source, manifest := newFixture()
	if err := validateProtocolCorpusSourceManifest(source, manifest); err != nil {
		t.Fatalf("valid metadata fixture rejected: %v", err)
	}

	tests := []struct {
		name     string
		want     string
		mutation func(*protocolCorpusSourceSpec)
	}{
		{name: "repository metadata", want: "field commit differs", mutation: func(source *protocolCorpusSourceSpec) { source.Repositories[0].Commit = strings.Repeat("c", 40) }},
		{name: "license hash", want: "field license_sha256 differs", mutation: func(source *protocolCorpusSourceSpec) { source.Repositories[0].LicenseSHA256 = strings.Repeat("d", 64) }},
		{name: "repository id", want: "absent from manifest.json", mutation: func(source *protocolCorpusSourceSpec) { source.Repositories[0].ID = "other" }},
		{name: "capture id", want: "absent from manifest.json", mutation: func(source *protocolCorpusSourceSpec) { source.Captures[0].ID = "other" }},
		{name: "repository_id", want: "field repository_id differs", mutation: func(source *protocolCorpusSourceSpec) { source.Captures[0].RepositoryID = "other" }},
		{name: "upstream_path", want: "field upstream_path differs", mutation: func(source *protocolCorpusSourceSpec) { source.Captures[0].UpstreamPath = "other.pcap" }},
		{name: "protocol", want: "field protocol differs", mutation: func(source *protocolCorpusSourceSpec) { source.Captures[0].Protocol = "other" }},
		{name: "roadmap_name", want: "field roadmap_name differs", mutation: func(source *protocolCorpusSourceSpec) { source.Captures[0].RoadmapName = nil }},
		{name: "display_filter", want: "field display_filter differs", mutation: func(source *protocolCorpusSourceSpec) { source.Captures[0].DisplayFilter = "other" }},
		{name: "evidence_kind", want: "field evidence_kind differs", mutation: func(source *protocolCorpusSourceSpec) { source.Captures[0].EvidenceKind = "upstream-negative" }},
		{name: "notes", want: "field notes differs", mutation: func(source *protocolCorpusSourceSpec) { source.Captures[0].Notes = "other" }},
		{name: "source hash", want: "field source_sha256 differs", mutation: func(source *protocolCorpusSourceSpec) { source.Captures[0].SourceSHA256 = strings.Repeat("d", 64) }},
		{name: "allow_empty", want: "field allow_empty differs", mutation: func(source *protocolCorpusSourceSpec) { source.Captures[0].AllowEmpty = true }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source, manifest := newFixture()
			test.mutation(&source)
			err := validateProtocolCorpusSourceManifest(source, manifest)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("metadata drift was not rejected with %q: %v", test.want, err)
			}
		})
	}
	t.Run("source_url", func(t *testing.T) {
		source, manifest := newFixture()
		manifest.Captures[0].SourceURL = "https://raw.githubusercontent.com/owner/repo/wrong/captures/sample.pcap"
		err := validateProtocolCorpusSourceManifest(source, manifest)
		if err == nil || !strings.Contains(err.Error(), "field source_url is not derived exactly") {
			t.Fatalf("source URL drift was not rejected: %v", err)
		}
	})
}

func TestProtocolCorpusSchemasRejectMissingRequiredFields(t *testing.T) {
	const corpusDir = "testdata/protocol-corpus"
	tests := []struct {
		name       string
		schemaFile string
		dataFile   string
		section    string
		field      string
	}{
		{name: "source repository commit", schemaFile: "source-spec.schema.json", dataFile: "sources.json", section: "repositories", field: "commit"},
		{name: "source repository license hash", schemaFile: "source-spec.schema.json", dataFile: "sources.json", section: "repositories", field: "license_sha256"},
		{name: "source capture protocol", schemaFile: "source-spec.schema.json", dataFile: "sources.json", section: "captures", field: "protocol"},
		{name: "source capture hash", schemaFile: "source-spec.schema.json", dataFile: "sources.json", section: "captures", field: "source_sha256"},
		{name: "manifest repository license hash", schemaFile: "manifest.schema.json", dataFile: "manifest.json", section: "repositories", field: "license_sha256"},
		{name: "manifest capture file", schemaFile: "manifest.schema.json", dataFile: "manifest.json", section: "captures", field: "capture_file"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schemaData, err := os.ReadFile(filepath.Join(corpusDir, test.schemaFile))
			if err != nil {
				t.Fatal(err)
			}
			instanceData, err := os.ReadFile(filepath.Join(corpusDir, test.dataFile))
			if err != nil {
				t.Fatal(err)
			}
			var instance map[string]any
			if err := json.Unmarshal(instanceData, &instance); err != nil {
				t.Fatal(err)
			}
			entries, ok := instance[test.section].([]any)
			if !ok || len(entries) == 0 {
				t.Fatalf("%s has no %s fixture", test.dataFile, test.section)
			}
			entry, ok := entries[0].(map[string]any)
			if !ok {
				t.Fatalf("%s[0] is not an object", test.section)
			}
			delete(entry, test.field)
			mutated, err := json.Marshal(instance)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateProtocolCorpusJSONSchema(schemaData, mutated); err == nil {
				t.Fatalf("schema accepted missing required field %s.%s", test.section, test.field)
			}
		})
	}
}
