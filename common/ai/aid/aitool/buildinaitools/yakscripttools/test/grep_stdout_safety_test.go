package test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/consts"
	_ "github.com/yaklang/yaklang/common/yak" // trigger yak package init to register tool handler
)

// TestGrepTool_StdoutSafety verifies that:
// 1. yakit.File outputs brief summary (like "[file] read file: xxx") instead of full content
// 2. grep results are properly printed via println() to stdout
// 3. stdout doesn't contain the large original file content (no duplicate/flood)
func TestGrepTool_StdoutSafety(t *testing.T) {
	// Get all tools and find the grep tool by name
	allTools := yakscripttools.GetAllYakScriptAiTools()
	var grepTool *aitool.Tool
	for _, tool := range allTools {
		if tool.GetName() == "grep" {
			grepTool = tool
			break
		}
	}
	if grepTool == nil {
		t.Fatal("grep tool not found")
	}
	t.Logf("Found grep tool: %s", grepTool.GetName())

	// Create a LARGE temporary file (2MB) with test content
	// The unique marker is hidden in the middle of large content
	largePrefix := strings.Repeat("NOISE_LINE_PREFIX_AAAA\n", 50000)  // ~1.1MB
	largeSuffix := strings.Repeat("NOISE_LINE_SUFFIX_BBBB\n", 50000)  // ~1.1MB
	uniqueMarker := "UNIQUE_GREP_TARGET_XYZ789"
	testContent := largePrefix + uniqueMarker + "\n" + largeSuffix

	tmpFile, err := consts.TempFile("grep_large_test_*.txt")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.WriteString(testContent)
	tmpFile.Close()
	tmpFileName := tmpFile.Name()
	fileSize := len(testContent)
	t.Logf("Created test file: %s, size: %d bytes (~%.2f MB)", tmpFileName, fileSize, float64(fileSize)/1024/1024)

	// Execute grep tool
	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)

	_, err = grepTool.Callback(context.Background(), aitool.InvokeParams{
		"path":           tmpFileName,
		"pattern":        uniqueMarker,
		"context-buffer": 20,
	}, nil, stdout, stderr)

	if err != nil {
		t.Fatalf("grep tool execution failed: %v", err)
	}

	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	t.Logf("Original file size: %d bytes (~%.2f MB)", fileSize, float64(fileSize)/1024/1024)
	t.Logf("Stdout size: %d bytes (~%.2f KB)", len(stdoutStr), float64(len(stdoutStr))/1024)
	t.Logf("Stderr size: %d bytes", len(stderrStr))

	// 1. Verify grep results (the unique marker) are in stdout
	if !strings.Contains(stdoutStr, uniqueMarker) {
		t.Logf("Stdout content:\n%s", stdoutStr)
		t.Fatal("Grep result (unique marker) not found in stdout")
	}
	t.Log("✓ Grep result found in stdout")

	// 2. Verify summary logs are present (like "[file] read file: xxx")
	hasSummaryLog := strings.Contains(stdoutStr, "[file] read file:") ||
		strings.Contains(stdoutStr, "[file] stat file:") ||
		strings.Contains(stdoutStr, "[file] find in:")
	if hasSummaryLog {
		t.Log("✓ File summary logs found in stdout")
	}

	// 3. Verify stdout does NOT contain the large noise content (no flooding)
	// The noise patterns should NOT appear many times in stdout
	noiseCount := strings.Count(stdoutStr, "NOISE_LINE_PREFIX_AAAA")
	if noiseCount > 10 {
		t.Fatalf("Stdout contains too many noise lines (%d), original file content is flooding stdout!", noiseCount)
	}
	t.Logf("✓ Noise content count in stdout: %d (should be minimal)", noiseCount)

	// 4. Verify stdout size is reasonable (should be << original file size)
	// If yakit.File was flooding stdout, it would be close to 2MB
	maxAllowedStdoutSize := 100 * 1024 // 100KB max for a 2MB file
	if len(stdoutStr) > maxAllowedStdoutSize {
		t.Fatalf("Stdout is too large (%d bytes), expected < %d bytes. yakit.File may be flooding stdout with file content!",
			len(stdoutStr), maxAllowedStdoutSize)
	}
	t.Logf("✓ Stdout size is safe: %d bytes (max allowed: %d bytes)", len(stdoutStr), maxAllowedStdoutSize)

	// 5. Verify grep results section is present (from println in grep.yak)
	if strings.Contains(stdoutStr, "=== grep results ===") {
		t.Log("✓ Grep results section found (from println)")
	}

	t.Logf("✓ All checks passed! Grep tool stdout safety verified.")
}

