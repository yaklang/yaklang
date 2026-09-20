package sca

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/sca/analyzer"
	"strings"
	"testing"
	"testing/fstest"
)

func TestMemoryFilesystemNoTemp(t *testing.T) {
	t.Setenv("TMPDIR", "/path/which/does/not/exist/sca")
	input := fstest.MapFS{"go.mod": {Data: []byte("module example.org/app\nrequire example.org/a v1.2.3\n")}}
	pkgs, err := ScanFilesystem(input, _withAnalayzers(analyzer.TypGoMod))
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 || pkgs[0].Name != "example.org/a" || pkgs[0].Evidence != "declared" {
		t.Fatalf("%#v", pkgs)
	}
}
func TestRepeatedOptionsAndWorkers(t *testing.T) {
	input := fstest.MapFS{"go.mod": {Data: []byte("module example.org/app\nrequire example.org/a v1.2.3\n")}}
	var baseline string
	for _, n := range []int{1, 2, 4, 8} {
		pkgs, err := ScanFilesystem(input, _withConcurrent(n), _withAnalayzers(analyzer.TypGoMod, analyzer.TypGoMod))
		if err != nil {
			t.Fatal(err)
		}
		if len(pkgs) != 1 {
			t.Fatalf("repeated parser: %d", len(pkgs))
		}
		raw, _ := json.Marshal(pkgs)
		if baseline != "" && baseline != string(raw) {
			t.Fatal("nondeterministic")
		}
		baseline = string(raw)
	}
}
func TestParseErrorsReachCaller(t *testing.T) {
	_, err := ScanFilesystem(fstest.MapFS{"go.mod": {Data: []byte("require (broken")}}, _withAnalayzers(analyzer.TypGoMod))
	if err == nil || !strings.Contains(err.Error(), "go.mod") {
		t.Fatalf("missing parser error: %v", err)
	}
}

func TestPipAndPipenvDefaultDiscovery(t *testing.T) {
	input := fstest.MapFS{
		"pip/requirements.txt":     {Data: []byte("flask==2.0.3\n")},
		"pipenv/Pipfile.lock":      {Data: []byte(`{"default":{"requests":{"version":"==2.31.0"}}}`)},
		"ignored/not-Pipfile.lock": {Data: []byte("not a lock file")},
	}
	pkgs, err := ScanFilesystem(input)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"flask": "2.0.3", "requests": "2.31.0"}
	if len(pkgs) != len(want) {
		t.Fatalf("expected both pip and Pipenv records, got %#v", pkgs)
	}
	for _, p := range pkgs {
		if version, ok := want[p.Name]; !ok || version != p.Version {
			t.Fatalf("unexpected package %s@%s", p.Name, p.Version)
		}
		delete(want, p.Name)
	}
}
func TestCallbackPanicDoesNotAbandonQueue(t *testing.T) {
	input := fstest.MapFS{}
	for _, n := range []string{"a", "b", "c"} {
		input[n] = &fstest.MapFile{Data: []byte("1234")}
	}
	_, err := ScanFilesystem(input, _withConcurrent(1), _withCustomAnalyzer(func(analyzer.MatchInfo) int { return 1 }, func(*analyzer.FileInfo, map[string]*analyzer.FileInfo) []*analyzer.CustomPackage {
		panic("callback failure")
	}))
	if err == nil || strings.Count(err.Error(), "internal_error") != 3 {
		t.Fatalf("lost failures: %v", err)
	}
}
func TestLocalReplaceRetainsDeclaration(t *testing.T) {
	input := fstest.MapFS{"go.mod": {Data: []byte("module example.org/app\nrequire example.org/a v1.2.3\nreplace example.org/a => ../a\n")}}
	pkgs, err := ScanFilesystem(input, _withAnalayzers(analyzer.TypGoMod))
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 || pkgs[0].Version != "" || pkgs[0].DeclaredVersion != "v1.2.3" || pkgs[0].Source != "../a" {
		t.Fatalf("local replacement lost: %#v", pkgs)
	}
}
