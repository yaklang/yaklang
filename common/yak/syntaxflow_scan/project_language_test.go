package syntaxflow_scan_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/coreplugin"
	_ "github.com/yaklang/yaklang/common/yak/ssa_compile"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
)

// Detection runs the "SSA 项目探测" core plugin, which production registers at
// startup. The isolated test home needs the same builtin plugin rows.
var languageDetectionDB sync.Once

func ensureLanguageDetectionPlugin(t *testing.T) {
	t.Helper()
	languageDetectionDB.Do(coreplugin.InitDBForTest)
}

// A project scan must compile with a language even when the caller omits
// --language. The detection step is part of the product pipeline, not a CLI
// convenience: without a builder the compiler warns "no language builder
// specified", disables extra-file analysis, records no program language, and
// every language-typed rule is skipped as a language mismatch.
func TestScanProject_DetectsProjectLanguageWhenNoneGiven(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  ssaconfig.Language
	}{
		{
			name: "go module",
			files: map[string]string{
				"go.mod":  "module example.com/demo\n\ngo 1.21\n",
				"main.go": "package main\n\nfunc main() {}\n",
			},
			want: ssaconfig.GO,
		},
		{
			name: "java source",
			files: map[string]string{
				"src/Main.java": "public class Main {\n    public static void main(String[] args) {}\n}\n",
			},
			want: ssaconfig.JAVA,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ensureLanguageDetectionPlugin(t)
			dir := t.TempDir()
			for name, content := range tc.files {
				path := filepath.Join(dir, name)
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
			}

			origCompile := syntaxflow_scan.CompileProject
			require.NotNil(t, origCompile, "the production compiler must be registered")
			t.Cleanup(func() { syntaxflow_scan.CompileProject = origCompile })

			var compileLanguage ssaconfig.Language
			var compiledFiles int
			syntaxflow_scan.CompileProject = func(ctx context.Context, cfg *ssaconfig.Config, extra ...ssaconfig.Option) (*ssaapi.Program, error) {
				prog, err := origCompile(ctx, cfg, extra...)
				// The product compile resolves a missing language into the
				// config it was given, so the stage filters and the compiler
				// agree on one language.
				compileLanguage = cfg.GetLanguage()
				if prog != nil && prog.Program != nil {
					compiledFiles = len(prog.Program.FileList)
				}
				return prog, err
			}

			// Review mode compiles in-process with struct options, exactly like
			// code-scan. That in-process path is the one that used to miss the
			// detected language; compile-only went through the compile plugin.
			result, err := syntaxflow_scan.ScanProject(context.Background(),
				ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
				ssaconfig.WithCodeSourceLocalFile(dir),
				ssaconfig.WithSetProgramName(fmt.Sprintf("%s-%s", t.Name(), uuid.NewString())),
				syntaxflow_scan.WithMode(syntaxflow_scan.StructMode),
				ssaconfig.WithScanIgnoreLanguage(true),
			)
			require.NoError(t, err)
			require.True(t, result.Succeeded)
			require.Equal(t, tc.want, compileLanguage,
				"a scan without --language must hand the detected language to the compiler")
			require.Greater(t, compiledFiles, 0,
				"the detected language must actually analyze the project files")
		})
	}
}

// An explicit language is an input fact and must never be overridden by
// detection.
func TestScanProject_KeepsExplicitLanguage(t *testing.T) {
	ensureLanguageDetectionPlugin(t)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644))

	origCompile := syntaxflow_scan.CompileProject
	require.NotNil(t, origCompile, "the production compiler must be registered")
	t.Cleanup(func() { syntaxflow_scan.CompileProject = origCompile })

	var compileLanguage ssaconfig.Language
	syntaxflow_scan.CompileProject = func(ctx context.Context, cfg *ssaconfig.Config, extra ...ssaconfig.Option) (*ssaapi.Program, error) {
		compileLanguage = cfg.GetLanguage()
		return origCompile(ctx, cfg, extra...)
	}

	_, err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("go"),
		ssaconfig.WithSetProgramName(fmt.Sprintf("%s-%s", t.Name(), uuid.NewString())),
		syntaxflow_scan.WithMode(syntaxflow_scan.StructMode),
		ssaconfig.WithScanIgnoreLanguage(true),
	)
	require.NoError(t, err)
	require.Equal(t, ssaconfig.GO, compileLanguage)
}
