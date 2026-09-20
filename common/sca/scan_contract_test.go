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
