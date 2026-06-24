package tests

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
)

// determinismTargets are classes that historically decompiled non-deterministically: the same input
// produced different output (and sometimes intermittent "multiple next" stubs) run to run because
// several CFG/variable-naming passes iterated Go maps (utils.Set.List, maps.Keys/Values) in random
// order. The antlr lexers exercise the if/switch exit-node ordering (NodeDeduplication); the druid
// classes exercise the loop/try ordering and the rewrite_var undefined-variable declaration ordering.
//
// We assert RAW decompiler determinism (post-decompile syntax validation disabled), because the
// validation safety net carries a wall-clock time budget and can intermittently stub a slow-to-parse
// (but otherwise valid and deterministic) method - that is a validation-timing artifact, independent
// of the decompiler output determinism this test guards.
var determinismTargets = [][2]string{
	{"antlr-2.7.7.jar", "antlr/preprocessor/PreprocessorLexer.class"},
	{"antlr-2.7.7.jar", "antlr/ANTLRLexer.class"},
	{"druid-1.2.14.jar", "com/alibaba/druid/sql/repository/SchemaResolveVisitorFactory.class"},
	{"druid-1.2.16.jar", "com/alibaba/druid/sql/parser/SQLExprParser.class"},
}

func findM2Jar(base string) string {
	home, _ := os.UserHomeDir()
	var found string
	_ = filepath.Walk(filepath.Join(home, ".m2"), func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if filepath.Base(p) == base {
			found = p
		}
		return nil
	})
	return found
}

func extractClass(jar, entry string) []byte {
	zr, err := zip.OpenReader(jar)
	if err != nil {
		return nil
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name == entry {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			return b
		}
	}
	return nil
}

// TestDecompileDeterminism guards against regressions in the decompiler's output determinism: each
// target class must decompile to a single, byte-identical result across many runs.
func TestDecompileDeterminism(t *testing.T) {
	// Opt-in: this scans the developer's ~/.m2 for specific jars and is both slow
	// and machine-specific. The portable determinism guarantee is covered by
	// TestCorpusDeterminism (javac-backed corpus, runs by default).
	if os.Getenv("M2_DETERMINISM") == "" {
		t.Skip("set M2_DETERMINISM=1 to run the ~/.m2 determinism check (opt-in)")
	}
	javaclassparser.EnableDecompileSyntaxValidation = false
	defer func() { javaclassparser.EnableDecompileSyntaxValidation = true }()

	const N = 24
	tested := 0
	for _, tgt := range determinismTargets {
		jar := findM2Jar(tgt[0])
		if jar == "" {
			t.Logf("skip %s (jar not in ~/.m2)", tgt[0])
			continue
		}
		raw := extractClass(jar, tgt[1])
		if raw == nil {
			t.Logf("skip %s!%s (class not found)", tgt[0], tgt[1])
			continue
		}
		tested++
		distinct := map[string]int{}
		stubHist := map[int]int{}
		for i := 0; i < N; i++ {
			dec, err := javaclassparser.Decompile(raw)
			if err != nil {
				t.Fatalf("%s!%s run %d: decompile error: %v", tgt[0], filepath.Base(tgt[1]), i, err)
			}
			distinct[dec]++
			stubHist[strings.Count(dec, javaclassparser.DecompileStubMarker)]++
		}
		if len(distinct) != 1 {
			t.Errorf("%s!%s: non-deterministic output, %d distinct results across %d runs (stubHist=%v)",
				tgt[0], filepath.Base(tgt[1]), len(distinct), N, stubHist)
			continue
		}
		t.Logf("%s!%s: deterministic across %d runs (stubHist=%v)", tgt[0], filepath.Base(tgt[1]), N, stubHist)
	}
	if tested == 0 {
		t.Skip("no determinism target jars available under ~/.m2")
	}
}
