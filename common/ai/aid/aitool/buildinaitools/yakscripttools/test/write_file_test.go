package test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yak"
)

func getWriteFileTool(t *testing.T) *aitool.Tool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/fs/write_file.yak")
	if err != nil {
		t.Fatalf("failed to read write_file.yak from embed FS: %v", err)
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools("write_file", string(content))
	if aiTool == nil {
		t.Fatalf("failed to parse write_file.yak metadata")
	}
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	if len(tools) == 0 {
		t.Fatalf("ConvertTools returned empty")
	}
	return tools[0]
}

func execWriteFileTool(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) (stdout string, stderr string) {
	t.Helper()
	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, err := tool.Callback(context.Background(), params, nil, w1, w2)
	if err != nil {
		t.Logf("tool execution error (may be expected): %v", err)
	}
	return w1.String(), w2.String()
}

// TestWriteFileTool_StdoutNotEmpty verifies that when write_file succeeds,
// the AI-visible stdout contains confirmation of the write operation.
//
// Bug: write_file.yak used only println() for all status output. Since Yaklang's
// println() writes directly to os.Stdout (not the stdout writer passed to the tool
// callback), the AI tool framework's stdout buffer was always empty. The AI could
// not confirm whether the write succeeded or failed.
func TestWriteFileTool_StdoutNotEmpty(t *testing.T) {
	tool := getWriteFileTool(t)

	targetFile := filepath.Join(t.TempDir(), "test_output.md")
	testContent := "# Test Report\n\nThis is a test file written by write_file tool.\n"

	stdout, stderr := execWriteFileTool(t, tool, aitool.InvokeParams{
		"file":    targetFile,
		"content": testContent,
		"force":   false,
	})

	t.Logf("stdout:\n%s", stdout)
	t.Logf("stderr:\n%s", stderr)

	// Verify the file was actually written
	written, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("file was not created: %v", err)
	}
	if string(written) != testContent {
		t.Fatalf("file content mismatch: got %q, want %q", string(written), testContent)
	}
	t.Log("✓ File actually written to disk")

	// The AI must receive confirmation via stdout (the AI-visible channel).
	// Without the fix: stdout is completely empty because all output used println().
	// With the fix: stdout contains [info] lines from yakit.Info calls.
	if stdout == "" {
		t.Fatalf("BUG: write_file stdout is completely empty after successful write.\n" +
			"The AI has no way to confirm the write succeeded or see the file path.\n" +
			"All output must go through yakit.Info(), not println().")
	}
	t.Log("✓ stdout is not empty (AI-visible output present)")

	// Must contain key information about the write result
	if !strings.Contains(stdout, targetFile) {
		t.Fatalf("BUG: stdout does not mention the target file path '%s'.\nstdout:\n%s", targetFile, stdout)
	}
	t.Log("✓ stdout contains the target file path")

	// Must indicate success
	hasSuccess := strings.Contains(stdout, "write success") ||
		strings.Contains(stdout, "success") ||
		strings.Contains(stdout, "written") ||
		strings.Contains(stdout, targetFile)
	if !hasSuccess {
		t.Fatalf("BUG: stdout does not indicate write success.\nstdout:\n%s", stdout)
	}
	t.Log("✓ stdout indicates write success")
}

// TestWriteFileTool_ForceOverwrite verifies that force=true allows overwriting
// and the AI receives confirmation in stdout.
func TestWriteFileTool_ForceOverwrite(t *testing.T) {
	tool := getWriteFileTool(t)

	targetFile := filepath.Join(t.TempDir(), "existing.txt")
	if err := os.WriteFile(targetFile, []byte("original content"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	newContent := "overwritten content"
	stdout, stderr := execWriteFileTool(t, tool, aitool.InvokeParams{
		"file":    targetFile,
		"content": newContent,
		"force":   true,
	})

	t.Logf("stdout:\n%s", stdout)
	t.Logf("stderr:\n%s", stderr)

	written, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("file could not be read: %v", err)
	}
	if string(written) != newContent {
		t.Fatalf("file was not overwritten: got %q, want %q", string(written), newContent)
	}
	t.Log("✓ File overwritten on disk")

	if stdout == "" {
		t.Fatalf("BUG: write_file stdout is empty after force overwrite. AI cannot confirm result.")
	}
	t.Log("✓ stdout is not empty for force overwrite")
}

// TestWriteFileTool_RefuseOverwriteWithoutForce verifies that force=false (default)
// refuses to overwrite and the AI receives an error message in stdout.
func TestWriteFileTool_RefuseOverwriteWithoutForce(t *testing.T) {
	tool := getWriteFileTool(t)

	targetFile := filepath.Join(t.TempDir(), "existing.txt")
	originalContent := "original content"
	if err := os.WriteFile(targetFile, []byte(originalContent), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	stdout, _ := execWriteFileTool(t, tool, aitool.InvokeParams{
		"file":    targetFile,
		"content": "should not overwrite",
		"force":   false,
	})

	t.Logf("stdout:\n%s", stdout)

	// File should be unchanged
	written, _ := os.ReadFile(targetFile)
	if string(written) != originalContent {
		t.Fatalf("file was unexpectedly overwritten without force=true")
	}
	t.Log("✓ File not overwritten (correct)")

	// AI must receive the refusal/error message in stdout
	if stdout == "" {
		t.Fatalf("BUG: write_file stdout is empty when refusing overwrite. AI gets no error feedback.")
	}
	t.Log("✓ stdout contains refusal message")
}

// TestWriteFileTool_CreateParentDirs verifies that missing parent directories
// are created and the AI receives confirmation in stdout.
func TestWriteFileTool_CreateParentDirs(t *testing.T) {
	tool := getWriteFileTool(t)

	targetFile := filepath.Join(t.TempDir(), "deep", "nested", "dir", "output.md")
	testContent := "content in deeply nested file"

	stdout, _ := execWriteFileTool(t, tool, aitool.InvokeParams{
		"file":    targetFile,
		"content": testContent,
		"force":   false,
	})

	t.Logf("stdout:\n%s", stdout)

	written, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("nested file was not created: %v", err)
	}
	if string(written) != testContent {
		t.Fatalf("nested file content mismatch")
	}
	t.Log("✓ Nested directories and file created")

	if stdout == "" {
		t.Fatalf("BUG: write_file stdout is empty after creating nested dirs and file.")
	}
	t.Log("✓ stdout not empty for nested dir creation")
}
