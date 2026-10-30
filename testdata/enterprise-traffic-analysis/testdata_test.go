package enterprisetrafficanalysis

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type sourceManifest struct {
	ID        string `json:"id"`
	Capture   string `json:"capture"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
	KeyLog    string `json:"keylog"`
	KeyLogSHA string `json:"keylog_sha256"`
	Tasks     string `json:"tasks"`
	Safety    string `json:"safety"`
}

type multiManifest struct {
	ID       string `json:"id"`
	Captures []struct {
		File      string `json:"file"`
		SizeBytes int64  `json:"size_bytes"`
		SHA256    string `json:"sha256"`
	} `json:"captures"`
	Tasks  string `json:"tasks"`
	Safety string `json:"safety"`
}

// This package intentionally has no production code. Its tests validate the
// public-corpus scenario fixtures and the instructions that explain how to use
// them, without loading or executing any capture content.
func TestEnterpriseTrafficAnalysisManifests(t *testing.T) {
	root := enterpriseTrafficRoot(t)
	entries, err := trafficfixture.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	evaluated := 0
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "public-corpus-index" {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		manifestPath := filepath.Join(dir, "manifest.json")
		data := readFixture(t, manifestPath)
		var generic map[string]any
		if err := json.Unmarshal(data, &generic); err != nil {
			t.Fatalf("%s: invalid JSON: %v", manifestPath, err)
		}
		var single sourceManifest
		if err := json.Unmarshal(data, &single); err == nil && single.Capture != "" {
			checkSingleManifest(t, dir, single)
			evaluated++
			continue
		}
		var multi multiManifest
		if err := json.Unmarshal(data, &multi); err != nil || len(multi.Captures) == 0 {
			t.Fatalf("%s: manifest has no capture list", manifestPath)
		}
		checkMultiManifest(t, dir, multi)
		evaluated++
	}
	if evaluated < 6 {
		t.Fatalf("evaluated %d manifests; expected at least six enterprise scenarios", evaluated)
	}
}

func checkSingleManifest(t *testing.T, dir string, manifest sourceManifest) {
	t.Helper()
	if manifest.ID == "" || manifest.Tasks == "" || manifest.Safety == "" {
		t.Fatalf("%s: id, tasks and safety are required", dir)
	}
	checkFiles(t, dir, []fileCheck{
		{name: manifest.Capture, size: manifest.SizeBytes, sha256: manifest.SHA256},
		{name: manifest.KeyLog, size: 0, sha256: manifest.KeyLogSHA},
		{name: manifest.Tasks},
	})
}

func checkMultiManifest(t *testing.T, dir string, manifest multiManifest) {
	t.Helper()
	if manifest.ID == "" || manifest.Tasks == "" || manifest.Safety == "" {
		t.Fatalf("%s: id, tasks and safety are required", dir)
	}
	files := []fileCheck{{name: manifest.Tasks}}
	for _, capture := range manifest.Captures {
		files = append(files, fileCheck{name: capture.File, size: capture.SizeBytes, sha256: capture.SHA256})
	}
	checkFiles(t, dir, files)
}

type fileCheck struct {
	name   string
	size   int64
	sha256 string
}

func checkFiles(t *testing.T, dir string, files []fileCheck) {
	t.Helper()
	for _, file := range files {
		if file.name == "" {
			continue
		}
		path := filepath.Join(dir, filepath.FromSlash(file.name))
		if strings.Contains(file.name, "..") || filepath.IsAbs(file.name) {
			t.Fatalf("%s: unsafe manifest path %q", dir, file.name)
		}
		data := readFixture(t, path)
		if file.size != 0 && int64(len(data)) != file.size {
			t.Fatalf("%s: size=%d manifest=%d", path, len(data), file.size)
		}
		if file.sha256 != "" {
			actual := sha256Hex(data)
			if actual != file.sha256 {
				t.Fatalf("%s: sha256=%s manifest=%s", path, actual, file.sha256)
			}
		}
	}
}

func sha256Hex(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func TestTaskCardsHaveSafetyAndEvidenceBoundaries(t *testing.T) {
	root := enterpriseTrafficRoot(t)
	entries, err := trafficfixture.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "public-corpus-index" {
			continue
		}
		path := filepath.Join(root, entry.Name(), "TASKS.md")
		data := readFixture(t, path)
		text := string(data)
		for _, required := range []string{"##", "禁止", "证据"} {
			if !strings.Contains(text, required) {
				t.Errorf("%s: missing %q boundary section", path, required)
			}
		}
	}
}

func TestKeyLogFixtureMatchesManifest(t *testing.T) {
	root := enterpriseTrafficRoot(t)
	data := readFixture(t, filepath.Join(root, "https-keylog-dridex", "manifest.json"))
	var manifest sourceManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	actual := sha256Hex(readFixture(t, filepath.Join(root, "https-keylog-dridex", manifest.KeyLog)))
	if actual != manifest.KeyLogSHA {
		t.Fatal(fmt.Errorf("keylog sha256=%s manifest=%s", actual, manifest.KeyLogSHA))
	}
}

func enterpriseTrafficRoot(t *testing.T) string {
	t.Helper()
	repositoryRoot, err := trafficfixture.RepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(repositoryRoot, "testdata", "enterprise-traffic-analysis")
}

func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	data, err := trafficfixture.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return data
}
