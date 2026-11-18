package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/c2ssa/preprocess"
	"github.com/yaklang/yaklang/common/yak/cpp/cpp2ssa"
)

// TestCppCorpusCoverage parses a real-world C++ tree and reports one RESULT line
// per file. The corpus is not committed; point CPP_CORPUS at a local checkout.
func TestCppCorpusCoverage(t *testing.T) {
	root := os.Getenv("CPP_CORPUS")
	if root == "" {
		t.Skip("CPP_CORPUS not set")
	}
	// CPP_CORPUS_PREPROCESS=1 measures the production path: include inlining and
	// macro expansion before parsing. Without it the grammar is measured alone.
	var project *preprocess.CPreprocessProject
	if os.Getenv("CPP_CORPUS_PREPROCESS") != "" {
		project = preprocess.BuildProject(filesys.NewRelLocalFs(root), cpp2ssa.DefaultPreprocessConfig())
		defer project.Close()
	}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		extension := strings.ToLower(filepath.Ext(path))
		if extension != ".cpp" && extension != ".cc" && extension != ".cxx" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Logf("RESULT\tFAIL\t%s\tread: %v", path, err)
			return nil
		}
		source := string(raw)
		if project != nil {
			relative, relErr := filepath.Rel(root, path)
			if relErr == nil {
				if expanded, expandErr := project.PreprocessTU(relative, source); expandErr == nil {
					source = expanded
				}
			}
		}
		start := time.Now()
		_, parseErr := cpp2ssa.Frontend(source)
		elapsedMicros := time.Since(start).Microseconds()
		if parseErr != nil {
			message := strings.ReplaceAll(parseErr.Error(), "\n", " ")
			if len(message) > 300 {
				message = message[:300]
			}
			t.Logf("RESULT\tFAIL\t%s\t%d\t%s", path, elapsedMicros, message)
			return nil
		}
		t.Logf("RESULT\tPASS\t%s\t%d\t", path, elapsedMicros)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
