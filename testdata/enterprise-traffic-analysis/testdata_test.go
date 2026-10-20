package enterprisetrafficanalysis

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	root := "."
	entries, err := os.ReadDir(root)
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
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			t.Fatalf("%s: %v", manifestPath, err)
		}
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
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("%s: missing file: %v", path, err)
		}
		if file.size != 0 && info.Size() != file.size {
			t.Fatalf("%s: size=%d manifest=%d", path, info.Size(), file.size)
		}
		if file.sha256 != "" {
			actual, err := fileSHA256(path)
			if err != nil {
				t.Fatal(err)
			}
			if actual != file.sha256 {
				t.Fatalf("%s: sha256=%s manifest=%s", path, actual, file.sha256)
			}
		}
	}
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func TestTaskCardsHaveSafetyAndEvidenceBoundaries(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "public-corpus-index" {
			continue
		}
		path := filepath.Join(entry.Name(), "TASKS.md")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		text := string(data)
		for _, required := range []string{"##", "禁止", "证据"} {
			if !strings.Contains(text, required) {
				t.Errorf("%s: missing %q boundary section", path, required)
			}
		}
	}
}

func TestKeyLogFixtureMatchesManifest(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("https-keylog-dridex", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest sourceManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	actual, err := fileSHA256(filepath.Join("https-keylog-dridex", manifest.KeyLog))
	if err != nil {
		t.Fatal(err)
	}
	if actual != manifest.KeyLogSHA {
		t.Fatal(fmt.Errorf("keylog sha256=%s manifest=%s", actual, manifest.KeyLogSHA))
	}
}
