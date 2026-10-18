package minirehs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Program evidence is opt-in because it writes complete local rule manifests.
// The ordinary regression suite remains independent of external data locations.
func programEvidenceDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("MINIREHS_EVIDENCE_DIR")
	if dir == "" {
		t.Skip("set MINIREHS_EVIDENCE_DIR to export local program evidence")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeProgramJSON(t *testing.T, dir, name string, value interface{}) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestProgramCorpusManifest(t *testing.T) {
	dir := programEvidenceDir(t)
	getRequest := getRequestImagePatterns(t)
	mitm, _ := mitmPatterns(t)
	writeProgramJSON(t, dir, "getrequest-go-patterns.json", getRequest)
	writeProgramJSON(t, dir, "mitm-go-patterns.json", mitm)
	getDB, err := Compile(getRequest, WithBackend(BackendMVS), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	writeProgramJSON(t, dir, "getrequest-compile-info.json", getDB.Info())
	getDB.Close()
	type attempt struct {
		Pattern Pattern
		Error   string
		Info    *DatabaseInfo
	}
	attempts := make([]attempt, 0, len(mitm))
	for _, p := range mitm {
		a := attempt{Pattern: p}
		db, err := Compile([]Pattern{p}, WithBackend(BackendMVS), WithLogger(silentLogger{}))
		if err != nil {
			a.Error = err.Error()
		} else {
			info := db.Info()
			a.Info = &info
			db.Close()
		}
		attempts = append(attempts, a)
	}
	writeProgramJSON(t, dir, "mitm-compile-attempts.json", attempts)
	t.Logf("exported %d GetRequest and %d unfiltered MITM rules", len(getRequest), len(mitm))
}
