package yaktest

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/yak"
)

func TestFileLineRunnerExamples(t *testing.T) {
	os.Setenv("YAKMODE", "vm")

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to locate current file")
	}

	baseDir := filepath.Join(filepath.Dir(filename), "testdata", "file_line")
	payloadFile := normalizeYakPath(filepath.Join(baseDir, "payloads-basic.txt"))
	outputFile := normalizeYakPath(filepath.Join(t.TempDir(), "payloads-out.txt"))
	missingFile := normalizeYakPath(filepath.Join(t.TempDir(), "missing.txt"))

	mutateResult, err := mutate.FuzzTagExec(fmt.Sprintf("{{file:line(%s)}}", payloadFile))
	if err != nil {
		t.Fatalf("mutate.FuzzTagExec failed: %v", err)
	}
	t.Logf("mutate.FuzzTagExec => %#v", mutateResult)

	cases := []struct {
		name   string
		script string
	}{
		{name: "direct_file_line", script: "direct_file_line.yak"},
		{name: "double_colon_file_line", script: "double_colon_file_line.yak"},
		{name: "string_fuzz_file_line", script: "string_fuzz_file_line.yak"},
		{name: "workaround_file_readlines", script: "workaround_file_readlines.yak"},
		{name: "write_output_file_line", script: "write_output_file_line.yak"},
		{name: "missing_file_returns_empty", script: "missing_file_returns_empty.yak"},
	}

	replacements := map[string]string{
		"__PAYLOAD_FILE__": payloadFile,
		"__OUTPUT_FILE__":  outputFile,
		"__MISSING_FILE__": missingFile,
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			scriptPath := filepath.Join(baseDir, tc.script)
			raw, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatalf("read script %s failed: %v", scriptPath, err)
			}

			code := string(raw)
			for from, to := range replacements {
				code = strings.ReplaceAll(code, from, to)
			}

			t.Logf("running %s", scriptPath)
			engine, err := yak.Execute(code)
			if err != nil {
				t.Fatalf("run script %s failed: %v", scriptPath, err)
			}
			result, _ := engine.GetVar("result")
			t.Logf("%s => %#v", tc.name, result)
		})
	}

	outputRaw, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("read output file failed: %v", err)
	}
	t.Logf("output file => %q", string(outputRaw))
}

func normalizeYakPath(path string) string {
	absPath, err := filepath.Abs(path)
	if err == nil {
		path = absPath
	}
	return strings.ReplaceAll(path, "\\", "/")
}
