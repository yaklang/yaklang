package minirehs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func programInvalidUTFPatterns() []Pattern {
	return []Pattern{{ID: 17, Expr: `\x{FFFD}foo(?=!)`}, {ID: 33, Expr: `bar\x{FFFD}`}, {ID: 34, Expr: `a*`}}
}

func TestProgramExportCompatibilityFixtures(t *testing.T) {
	dir := programEvidenceDir(t)
	for _, locate := range []bool{false, true} {
		db, err := Compile(programInvalidUTFPatterns(), WithBackend(BackendMVS), WithReportLocation(locate), WithLogger(silentLogger{}))
		if err != nil {
			t.Fatal(err)
		}
		image, err := db.MarshalImage()
		db.Close()
		if err != nil {
			t.Fatal(err)
		}
		name := fmt.Sprintf("image-v%d-invalid-utf-loc-%v.rehs", imageVersion, locate)
		if err := os.WriteFile(filepath.Join(dir, name), image, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

// Export an actual frontend blob for the standalone C allocation-fault probe.
// It contains both W>8 paths and enough single-word assertions to use heap
// buffers in fused assertion/combined scans.
func TestProgramExportNativeErrorFixture(t *testing.T) {
	dir := programEvidenceDir(t)
	patterns := []Pattern{{ID: 0, Expr: `[ab]{600}`}, {ID: 1, Expr: `\b[ab]{600}\b`}}
	for i := 2; i < 21; i++ {
		patterns = append(patterns, Pattern{ID: PatternID(i), Expr: `\b[a-z]{3}\b`})
	}
	db, err := Compile(patterns, WithBackend(BackendMVS), WithLogger(silentLogger{}))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m := digMVSDB(t, db)
	if m.nfas[0].nword <= 8 || m.nfas[1].nword <= 8 || !m.nfas[1].hasAssert || m.merged == nil {
		t.Fatal("fixture did not reach the required wide native paths")
	}
	if err := os.WriteFile(filepath.Join(dir, "native-error.blob"), m.blob, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("native error fixture: lean W=%d, assertion W=%d, merged W=%d, %d fused assertions",
		m.nfas[0].nword, m.nfas[1].nword, m.merged.nword, len(m.assertAlwaysOnCIdxs))
}

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
