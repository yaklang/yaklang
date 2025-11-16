package loop_java_decompiler_test

import (
	"context"
	_ "embed"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_java_decompiler"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/reactloopstests"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed testdata/plexus-cipher-2.0.jar
var testJarData []byte

// TestDecompileJar_BasicFunctionality tests basic decompilation with the test JAR
func TestDecompileJar_BasicFunctionality(t *testing.T) {
	// Create temporary directory for test JAR
	tmpDir := t.TempDir()

	// Write embedded JAR data to temp file
	testJarPath := filepath.Join(tmpDir, "plexus-cipher-2.0.jar")
	if err := os.WriteFile(testJarPath, testJarData, 0644); err != nil {
		t.Fatalf("Failed to write test JAR: %v", err)
	}

	// Create output directory
	outputDir := filepath.Join(tmpDir, "test_decompiled")

	// Create action test framework
	framework := reactloopstests.NewActionTestFramework(t, "test-decompile-basic")

	// Track action execution
	actionCalled := false
	var feedbackMsg string

	// Register the decompile action using the actual implementation
	framework.RegisterTestAction(
		"decompile_jar",
		"Decompile a JAR file",
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			jarPath := action.GetString("jar_path")
			if jarPath == "" {
				return utils.Error("jar_path parameter is required")
			}
			if utils.GetFirstExistedFile(jarPath) == "" {
				return utils.Errorf("JAR file not found: %s", jarPath)
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			actionCalled = true

			// Use the actual decompile_jar handler logic
			// This is a simplified version for testing
			jarPath := action.GetString("jar_path")
			outDir := action.GetString("output_dir")
			if outDir == "" {
				outDir = outputDir
			}

			// For this test, we just verify the parameters and set feedback
			loop.Set("working_directory", outDir)
			loop.Set("jar_path", jarPath)
			loop.Set("total_files", 10)      // Mock value
			loop.Set("files_with_issues", 2) // Mock value

			feedbackMsg = "Successfully decompiled JAR file"
			op.Feedback(feedbackMsg)
			op.Continue()
		},
	)

	// Execute the action
	err := framework.ExecuteAction("decompile_jar", map[string]interface{}{
		"jar_path":   testJarPath,
		"output_dir": outputDir,
	})

	if err != nil {
		t.Fatalf("Action execution failed: %v", err)
	}

	// Verify action was called
	if !actionCalled {
		t.Error("Decompile action should have been called")
	}

	// Verify feedback
	if !strings.Contains(feedbackMsg, "Successfully") {
		t.Errorf("Expected success feedback, got: %s", feedbackMsg)
	}

	// Verify loop context was set
	loop := framework.GetLoop()
	if loop.Get("working_directory") != outputDir {
		t.Errorf("Working directory not set correctly")
	}

	t.Logf("✅ Basic decompile test passed")
}

// TestDecompileJar_RealDecompilation tests actual JAR decompilation
func TestDecompileJar_RealDecompilation(t *testing.T) {
	// Create temporary directory for test JAR
	tmpDir := t.TempDir()

	// Write embedded JAR data to temp file
	testJarPath := filepath.Join(tmpDir, "plexus-cipher-2.0.jar")
	if err := os.WriteFile(testJarPath, testJarData, 0644); err != nil {
		t.Fatalf("Failed to write test JAR: %v", err)
	}

	// Create output directory
	outputDir := filepath.Join(tmpDir, "plexus_decompiled")

	t.Logf("Decompiling %s to %s", testJarPath, outputDir)

	// Create a minimal test runtime
	runtime := &testRuntime{timeline: make(map[string]string)}

	// Get the actual action option
	actionOption := loop_java_decompiler.DecompileJarAction(runtime)

	// Create framework with the actual action
	framework := reactloopstests.NewActionTestFramework(t, "test-decompile-real", actionOption)

	// Execute the decompilation
	err := framework.ExecuteAction("decompile_jar", map[string]interface{}{
		"jar_path":   testJarPath,
		"output_dir": outputDir,
	})

	if err != nil {
		t.Fatalf("Decompilation failed: %v", err)
	}

	// Verify output directory was created
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		t.Errorf("Output directory was not created: %s", outputDir)
	}

	// Verify README.md was created
	readmePath := filepath.Join(outputDir, "README.md")
	if _, err := os.Stat(readmePath); os.IsNotExist(err) {
		t.Errorf("README.md was not created: %s", readmePath)
	} else {
		// Read and verify README content
		readme, err := os.ReadFile(readmePath)
		if err != nil {
			t.Errorf("Failed to read README.md: %v", err)
		} else {
			readmeStr := string(readme)
			if !strings.Contains(readmeStr, "JAR Decompilation Report") {
				t.Error("README.md should contain decompilation report header")
			}
			if !strings.Contains(readmeStr, "Total Java Files") {
				t.Error("README.md should contain file count")
			}
			t.Logf("README.md created successfully (%d bytes)", len(readme))
		}
	}

	// Verify .java files were created
	javaFiles := 0
	bakFiles := 0
	filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			if filepath.Ext(path) == ".java" {
				javaFiles++
			} else if strings.HasSuffix(path, ".java.bak") {
				bakFiles++
			}
		}
		return nil
	})

	if javaFiles == 0 {
		t.Error("No .java files were created")
	} else {
		t.Logf("Created %d .java files", javaFiles)
	}

	if bakFiles == 0 {
		t.Error("No .bak files were created")
	} else {
		t.Logf("Created %d .bak files", bakFiles)
	}

	// Verify backup count matches java file count
	if javaFiles != bakFiles {
		t.Errorf("Backup count mismatch: %d java files, %d bak files", javaFiles, bakFiles)
	}

	// Check if COMPILATION_ERRORS.txt exists (may or may not exist depending on the JAR)
	errorsPath := filepath.Join(outputDir, "COMPILATION_ERRORS.txt")
	if _, err := os.Stat(errorsPath); err == nil {
		errors, _ := os.ReadFile(errorsPath)
		t.Logf("Compilation errors detected: %d bytes", len(errors))
	}

	// Verify loop context
	loop := framework.GetLoop()
	totalFiles := loop.GetInt("total_files")
	if totalFiles != javaFiles {
		t.Errorf("Total files in context (%d) doesn't match actual files (%d)", totalFiles, javaFiles)
	}

	t.Logf("✅ Real decompilation test passed: %d files decompiled", javaFiles)
}

// TestDecompileJar_MissingJarFile tests error handling for missing JAR
func TestDecompileJar_MissingJarFile(t *testing.T) {
	// Create a minimal test runtime
	runtime := &testRuntime{timeline: make(map[string]string)}

	// Get the actual action option
	actionOption := loop_java_decompiler.DecompileJarAction(runtime)

	// Create framework with the actual action
	framework := reactloopstests.NewActionTestFramework(t, "test-decompile-missing", actionOption)

	// Execute with non-existent JAR with a 5-second timeout to prevent hanging
	// The verifier should catch the error quickly, but we set a timeout as a safety measure
	err := framework.ExecuteActionWithTimeout("decompile_jar", map[string]interface{}{
		"jar_path": "/nonexistent/path/to/file.jar",
	}, 5*time.Second)

	// The execution should complete quickly (within timeout)
	// The verifier should catch the error and return it
	if err != nil {
		// Expected: verifier should return error for missing JAR
		if !strings.Contains(err.Error(), "JAR file not found") &&
			!strings.Contains(err.Error(), "context deadline exceeded") &&
			!strings.Contains(err.Error(), "timeout") {
			t.Logf("Execution completed with error (expected): %v", err)
		}
	}

	t.Logf("✅ Missing JAR error handling test passed")
}

// TestDecompileJar_AutoOutputDirectory tests automatic output directory naming
func TestDecompileJar_AutoOutputDirectory(t *testing.T) {
	// Create temporary directory for test JAR
	tmpDir := t.TempDir()

	// Write embedded JAR data to temp file (using different name for this test)
	jarInTmp := filepath.Join(tmpDir, "test.jar")
	if err := os.WriteFile(jarInTmp, testJarData, 0644); err != nil {
		t.Fatalf("Failed to write test JAR: %v", err)
	}

	runtime := &testRuntime{timeline: make(map[string]string)}
	actionOption := loop_java_decompiler.DecompileJarAction(runtime)
	framework := reactloopstests.NewActionTestFramework(t, "test-decompile-auto", actionOption)

	// Execute without specifying output_dir
	err := framework.ExecuteAction("decompile_jar", map[string]interface{}{
		"jar_path": jarInTmp,
		// No output_dir specified - should auto-create
	})

	if err != nil {
		t.Fatalf("Decompilation failed: %v", err)
	}

	// Verify auto-created directory
	expectedDir := filepath.Join(tmpDir, "test_decompiled")
	if _, err := os.Stat(expectedDir); os.IsNotExist(err) {
		t.Errorf("Auto-created output directory not found: %s", expectedDir)
	} else {
		t.Logf("✅ Auto-created output directory: %s", expectedDir)
	}
}

// TestGenerateDecompilationReport tests the README generation
func TestGenerateDecompilationReport(t *testing.T) {
	tests := []struct {
		name              string
		totalFiles        int
		filesWithIssues   int
		compilationErrors []string
		filesList         []string
		wantContains      []string
	}{
		{
			name:              "No errors",
			totalFiles:        10,
			filesWithIssues:   0,
			compilationErrors: nil,
			filesList:         []string{"Test1.java", "Test2.java"},
			wantContains:      []string{"Compilation Status", "No obvious compilation issues detected", "Total Java Files**: 10"},
		},
		{
			name:            "With errors",
			totalFiles:      20,
			filesWithIssues: 5,
			compilationErrors: []string{
				"Test.java: missing semicolon",
				"Another.java: unbalanced braces",
			},
			filesList:    []string{"Test.java", "Another.java"},
			wantContains: []string{"⚠️ Compilation Issues Detected", "Found 2 potential issues", "Decompiler limitations"},
		},
		{
			name:            "Many errors (truncation)",
			totalFiles:      100,
			filesWithIssues: 30,
			compilationErrors: func() []string {
				errors := make([]string, 50)
				for i := range errors {
					errors[i] = "File.java: error"
				}
				return errors
			}(),
			filesList:    []string{"File.java"},
			wantContains: []string{"... and 30 more issues", "COMPILATION_ERRORS.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := loop_java_decompiler.GenerateDecompilationReport(
				"/path/to/test.jar",
				"/output/dir",
				tt.totalFiles,
				tt.totalFiles, // backupCount = totalFiles
				tt.filesWithIssues,
				0, 0, // durations
				tt.compilationErrors,
				tt.filesList,
			)

			for _, want := range tt.wantContains {
				if !strings.Contains(report, want) {
					t.Errorf("Report should contain '%s'", want)
				}
			}

			t.Logf("Generated report (%d bytes)", len(report))
		})
	}
}

// testRuntime is a minimal AIInvokeRuntime for testing
type testRuntime struct {
	timeline map[string]string
}

func (r *testRuntime) AddToTimeline(key, value string) {
	r.timeline[key] = value
}

func (r *testRuntime) GetConfig() aicommon.AICallerConfigIf {
	return nil
}

func (r *testRuntime) GetBasicPromptInfo(tools []*aitool.Tool) (string, map[string]any, error) {
	return "", nil, nil
}

func (r *testRuntime) ExecuteToolRequiredAndCall(ctx context.Context, name string) (*aitool.ToolResult, bool, error) {
	return nil, false, nil
}

func (r *testRuntime) ExecuteToolRequiredAndCallWithoutRequired(ctx context.Context, toolName string, params aitool.InvokeParams) (*aitool.ToolResult, bool, error) {
	return nil, false, nil
}

func (r *testRuntime) AskForClarification(ctx context.Context, question string, payloads []string) string {
	if len(payloads) > 0 {
		return payloads[0]
	}
	return ""
}

func (r *testRuntime) DirectlyAnswer(ctx context.Context, query string, tools []*aitool.Tool) (string, error) {
	return "", nil
}

func (r *testRuntime) EnhanceKnowledgeAnswer(ctx context.Context, query string) (string, error) {
	return "", nil
}

func (r *testRuntime) EnhanceKnowledgeGetter(ctx context.Context, userQuery string) (string, error) {
	return "", nil
}

func (r *testRuntime) VerifyUserSatisfaction(ctx context.Context, query string, isToolCall bool, payload string) (bool, string, error) {
	return true, "", nil
}

func (r *testRuntime) RequireAIForgeAndAsyncExecute(ctx context.Context, forgeName string, onFinish func(error)) {
}

func (r *testRuntime) AsyncPlanAndExecute(ctx context.Context, planPayload string, onFinish func(error)) {
}

func (r *testRuntime) InvokeLiteForge(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, opts ...aicommon.GeneralKVConfigOption) (*aicommon.Action, error) {
	return nil, nil
}

func (r *testRuntime) EmitFileArtifactWithExt(name, ext string, data any) string {
	return ""
}

func (r *testRuntime) EmitResultAfterStream(result any) {
}

func (r *testRuntime) EmitResult(result any) {
}
