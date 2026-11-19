package test

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
//
// CPP_CORPUS_PREPROCESS=1 measures the production path: include inlining and
// macro expansion before parsing. Without it the grammar is measured alone.
//
// Each top-level directory under CPP_CORPUS is treated as one project, so macro
// tables stay scoped to the headers a project actually ships. Merging unrelated
// trees (two vendored test frameworks that both define TEST_CASE) would measure
// an artifact of the checkout layout, not the frontend.
func TestCppCorpusCoverage(t *testing.T) {
	root := os.Getenv("CPP_CORPUS")
	if root == "" {
		t.Skip("CPP_CORPUS not set")
	}
	projects := map[string]*preprocess.CPreprocessProject{}
	defer func() {
		for _, project := range projects {
			project.Close()
		}
	}()
	projectFor := func(path string) (*preprocess.CPreprocessProject, string) {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return nil, ""
		}
		scope, inside := root, relative
		if index := strings.IndexRune(relative, filepath.Separator); index > 0 {
			scope = filepath.Join(root, relative[:index])
			inside = relative[index+1:]
		}
		if project, ok := projects[scope]; ok {
			return project, inside
		}
		project := preprocess.BuildProject(filesys.NewRelLocalFs(scope), cpp2ssa.DefaultPreprocessConfig())
		projects[scope] = project
		return project, inside
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
		if os.Getenv("CPP_CORPUS_PREPROCESS") != "" {
			if project, relative := projectFor(path); project != nil {
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
