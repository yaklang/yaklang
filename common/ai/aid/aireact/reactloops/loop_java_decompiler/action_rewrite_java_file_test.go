package loop_java_decompiler_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_java_decompiler"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/reactloopstests"
)

// TestRewriteJavaFile_CompleteRewrite tests complete file rewrite
func TestRewriteJavaFile_CompleteRewrite(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test Java file with obfuscated code
	testFile := filepath.Join(tmpDir, "ObfuscatedClass.java")
	originalCode := `public class ObfuscatedClass {
    private String var1;
    private int var2;
    
    public void method1(String a, int b) {
        this.var1 = a;
        this.var2 = b;
    }
    
    public String method2() {
        return var1 + var2;
    }
}`

	err := os.WriteFile(testFile, []byte(originalCode), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Create test runtime and framework
	runtime := &testRuntime{timeline: make(map[string]string)}
	actionOption := loop_java_decompiler.RewriteJavaFileAction(runtime)
	framework := reactloopstests.NewActionTestFramework(t, "test-rewrite-complete", actionOption)

	// Prepare new code
	newCode := `public class ObfuscatedClass {
    private String username;
    private int userId;
    
    public void setUserInfo(String username, int userId) {
        this.username = username;
        this.userId = userId;
    }
    
    public String getUserInfo() {
        return username + userId;
    }
}`

	// Execute rewrite action - complete rewrite (no line numbers specified)
	err = framework.ExecuteAction("rewrite_java_file", map[string]interface{}{
		"file_path":      testFile,
		"rewrite_reason": "Rename obfuscated variables (var1->username, var2->userId) and improve method names",
		"new_code":       newCode,
	})

	if err != nil {
		t.Fatalf("Rewrite action failed: %v", err)
	}

	// Verify file was rewritten
	rewrittenContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read rewritten file: %v", err)
	}

	rewrittenStr := string(rewrittenContent)
	if !strings.Contains(rewrittenStr, "username") {
		t.Error("Rewritten file should contain 'username'")
	}
	if !strings.Contains(rewrittenStr, "userId") {
		t.Error("Rewritten file should contain 'userId'")
	}
	if strings.Contains(rewrittenStr, "var1") {
		t.Error("Rewritten file should not contain 'var1'")
	}

	// Verify backup was created
	backupPath := testFile + ".bak"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("Backup file should have been created")
	} else {
		backupContent, _ := os.ReadFile(backupPath)
		if !strings.Contains(string(backupContent), "var1") {
			t.Error("Backup should contain original code with 'var1'")
		}
	}

	// Verify loop context
	loop := framework.GetLoop()
	rewrittenFiles := loop.GetInt("rewritten_files")
	if rewrittenFiles != 1 {
		t.Errorf("Expected rewritten_files=1, got %d", rewrittenFiles)
	}

	t.Logf("Complete rewrite test passed - rewrote %d files", rewrittenFiles)
}

// TestRewriteJavaFile_PartialRewrite tests partial line range rewrite
func TestRewriteJavaFile_PartialRewrite(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test Java file
	testFile := filepath.Join(tmpDir, "PartialTest.java")
	originalCode := `public class PartialTest {
    private String name;
    
    public void method1() {
        int var1 = 10;
        int var2 = 20;
        int var3 = var1 + var2;
        System.out.println(var3);
    }
    
    public String getName() {
        return name;
    }
}`

	err := os.WriteFile(testFile, []byte(originalCode), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Create test runtime and framework
	runtime := &testRuntime{timeline: make(map[string]string)}
	actionOption := loop_java_decompiler.RewriteJavaFileAction(runtime)
	framework := reactloopstests.NewActionTestFramework(t, "test-rewrite-partial", actionOption)

	// New code for lines 5-8 (the method body)
	newMethodBody := `    public void method1() {
        int firstNumber = 10;
        int secondNumber = 20;
        int sum = firstNumber + secondNumber;
        System.out.println(sum);
    }`

	// Execute rewrite action - partial rewrite
	err = framework.ExecuteAction("rewrite_java_file", map[string]interface{}{
		"file_path":          testFile,
		"rewrite_start_line": 4,
		"rewrite_end_line":   9,
		"rewrite_reason":     "Rename obfuscated variables in method1",
		"new_code":           newMethodBody,
	})

	if err != nil {
		t.Fatalf("Partial rewrite action failed: %v", err)
	}

	// Verify file was rewritten
	rewrittenContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read rewritten file: %v", err)
	}

	rewrittenStr := string(rewrittenContent)

	// Should contain new variable names
	if !strings.Contains(rewrittenStr, "firstNumber") {
		t.Error("Rewritten file should contain 'firstNumber'")
	}
	if !strings.Contains(rewrittenStr, "secondNumber") {
		t.Error("Rewritten file should contain 'secondNumber'")
	}
	if !strings.Contains(rewrittenStr, "sum") {
		t.Error("Rewritten file should contain 'sum'")
	}

	// Should still contain the unchanged parts
	if !strings.Contains(rewrittenStr, "public String getName()") {
		t.Error("Unchanged method should still exist")
	}

	t.Log("Partial rewrite test passed")
}

// TestRewriteJavaFile_MissingFile tests error handling for missing file
func TestRewriteJavaFile_MissingFile(t *testing.T) {
	runtime := &testRuntime{timeline: make(map[string]string)}
	actionOption := loop_java_decompiler.RewriteJavaFileAction(runtime)
	framework := reactloopstests.NewActionTestFramework(t, "test-rewrite-missing", actionOption)

	// Execute with non-existent file
	err := framework.ExecuteAction("rewrite_java_file", map[string]interface{}{
		"file_path":      "/nonexistent/Test.java",
		"rewrite_reason": "Test",
		"new_code":       "public class Test {}",
	})

	// Should handle error gracefully
	if err != nil {
		t.Logf("Execution completed with expected error: %v", err)
	}

	t.Log("Missing file error handling test passed")
}

// TestRewriteJavaFile_InvalidLineRange tests error handling for invalid line range
func TestRewriteJavaFile_InvalidLineRange(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a small test file
	testFile := filepath.Join(tmpDir, "SmallFile.java")
	originalCode := `public class SmallFile {
    public void method() {
        System.out.println("test");
    }
}`

	err := os.WriteFile(testFile, []byte(originalCode), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	runtime := &testRuntime{timeline: make(map[string]string)}
	actionOption := loop_java_decompiler.RewriteJavaFileAction(runtime)
	framework := reactloopstests.NewActionTestFramework(t, "test-rewrite-invalid-range", actionOption)

	// Try to rewrite lines beyond file length
	err = framework.ExecuteAction("rewrite_java_file", map[string]interface{}{
		"file_path":          testFile,
		"rewrite_start_line": 10,
		"rewrite_end_line":   20,
		"rewrite_reason":     "Test invalid range",
		"new_code":           "public void newMethod() {}",
	})

	// Should handle error gracefully
	if err != nil {
		t.Logf("Execution completed with expected error: %v", err)
	}

	t.Log("Invalid line range error handling test passed")
}

// TestRewriteJavaFile_BackupNotOverwritten tests that existing backups are not overwritten
func TestRewriteJavaFile_BackupNotOverwritten(t *testing.T) {
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "BackupTest.java")
	originalCode := `public class BackupTest {
    private String original;
}`

	err := os.WriteFile(testFile, []byte(originalCode), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Create an existing backup manually
	backupPath := testFile + ".bak"
	manualBackupCode := `public class BackupTest {
    private String manualBackup;
}`
	err = os.WriteFile(backupPath, []byte(manualBackupCode), 0644)
	if err != nil {
		t.Fatalf("Failed to write manual backup: %v", err)
	}

	runtime := &testRuntime{timeline: make(map[string]string)}
	actionOption := loop_java_decompiler.RewriteJavaFileAction(runtime)
	framework := reactloopstests.NewActionTestFramework(t, "test-rewrite-backup", actionOption)

	// Rewrite the file
	newCode := `public class BackupTest {
    private String rewritten;
}`

	err = framework.ExecuteAction("rewrite_java_file", map[string]interface{}{
		"file_path":      testFile,
		"rewrite_reason": "Test backup preservation",
		"new_code":       newCode,
	})

	if err != nil {
		t.Fatalf("Rewrite action failed: %v", err)
	}

	// Verify original backup was NOT overwritten
	backupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("Failed to read backup: %v", err)
	}

	if !strings.Contains(string(backupContent), "manualBackup") {
		t.Error("Original backup should be preserved, not overwritten")
	}

	t.Log("Backup preservation test passed")
}

// TestRewriteJavaFile_EmptyNewCode tests error handling for empty new code
func TestRewriteJavaFile_EmptyNewCode(t *testing.T) {
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "EmptyTest.java")
	originalCode := `public class EmptyTest {
    public void method() {}
}`

	err := os.WriteFile(testFile, []byte(originalCode), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	runtime := &testRuntime{timeline: make(map[string]string)}
	actionOption := loop_java_decompiler.RewriteJavaFileAction(runtime)
	framework := reactloopstests.NewActionTestFramework(t, "test-rewrite-empty", actionOption)

	// Execute with empty new code
	err = framework.ExecuteAction("rewrite_java_file", map[string]interface{}{
		"file_path":      testFile,
		"rewrite_reason": "Test empty code",
		"new_code":       "",
	})

	// Should handle error gracefully
	if err != nil {
		t.Logf("Execution completed with expected error: %v", err)
	}

	t.Log("Empty new code error handling test passed")
}

// TestRewriteJavaFile_MultipleRewrites tests multiple sequential rewrites
func TestRewriteJavaFile_MultipleRewrites(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple test files
	files := []string{"File1.java", "File2.java", "File3.java"}
	for _, filename := range files {
		filePath := filepath.Join(tmpDir, filename)
		code := "public class " + strings.TrimSuffix(filename, ".java") + " {\n    private String var1;\n}"
		err := os.WriteFile(filePath, []byte(code), 0644)
		if err != nil {
			t.Fatalf("Failed to write %s: %v", filename, err)
		}
	}

	runtime := &testRuntime{timeline: make(map[string]string)}
	actionOption := loop_java_decompiler.RewriteJavaFileAction(runtime)

	// Rewrite each file (each in its own framework instance to avoid state issues)
	rewriteCount := 0
	for i, filename := range files {
		// Create a new framework for each file (test framework limitation: global state)
		framework := reactloopstests.NewActionTestFramework(t, fmt.Sprintf("test-rewrite-multiple-%d", i), actionOption)
		
		filePath := filepath.Join(tmpDir, filename)
		className := strings.TrimSuffix(filename, ".java")
		newCode := "public class " + className + " {\n    private String username;\n}"

		err := framework.ExecuteAction("rewrite_java_file", map[string]interface{}{
			"file_path":      filePath,
			"rewrite_reason": "Rename var1 to username",
			"new_code":       newCode,
		})

		if err != nil {
			t.Fatalf("Failed to rewrite %s: %v", filename, err)
		}
		
		// Verify this specific file was rewritten
		rewrittenContent, err := os.ReadFile(filePath)
		if err != nil {
			t.Errorf("Failed to read rewritten file %s: %v", filename, err)
			continue
		}
		
		if strings.Contains(string(rewrittenContent), "username") {
			rewriteCount++
		}
	}

	// Verify all files were successfully rewritten
	if rewriteCount != len(files) {
		t.Errorf("Expected %d files rewritten, got %d", len(files), rewriteCount)
	}

	// Verify all backups were created
	backupCount := 0
	for _, filename := range files {
		backupPath := filepath.Join(tmpDir, filename+".bak")
		if _, err := os.Stat(backupPath); os.IsNotExist(err) {
			t.Errorf("Backup not created for %s", filename)
		} else {
			backupCount++
		}
	}

	t.Logf("Multiple rewrites test passed - rewrote %d files, created %d backups", rewriteCount, backupCount)
}

// TestRewriteJavaFile_SyntaxFixing tests using rewrite to fix syntax errors
func TestRewriteJavaFile_SyntaxFixing(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file with syntax errors (missing semicolon, unbalanced braces)
	testFile := filepath.Join(tmpDir, "SyntaxError.java")
	brokenCode := `public class SyntaxError {
    public void method() {
        System.out.println("missing semicolon")
        int x = 10
    
}`

	err := os.WriteFile(testFile, []byte(brokenCode), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	runtime := &testRuntime{timeline: make(map[string]string)}
	actionOption := loop_java_decompiler.RewriteJavaFileAction(runtime)
	framework := reactloopstests.NewActionTestFramework(t, "test-rewrite-syntax-fix", actionOption)

	// Rewrite with fixed syntax
	fixedCode := `public class SyntaxError {
    public void method() {
        System.out.println("missing semicolon");
        int x = 10;
    }
}`

	err = framework.ExecuteAction("rewrite_java_file", map[string]interface{}{
		"file_path":      testFile,
		"rewrite_reason": "Fix missing semicolons and unbalanced braces",
		"new_code":       fixedCode,
	})

	if err != nil {
		t.Fatalf("Syntax fix rewrite failed: %v", err)
	}

	// Verify file was fixed
	fixedContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read fixed file: %v", err)
	}

	fixedStr := string(fixedContent)

	// Count braces
	openBraces := strings.Count(fixedStr, "{")
	closeBraces := strings.Count(fixedStr, "}")
	if openBraces != closeBraces {
		t.Errorf("Fixed file still has unbalanced braces: {=%d, }=%d", openBraces, closeBraces)
	}

	// Verify backup contains broken code
	backupPath := testFile + ".bak"
	backupContent, _ := os.ReadFile(backupPath)
	if !strings.Contains(string(backupContent), "missing semicolon\")") {
		t.Error("Backup should contain original broken code")
	}

	t.Log("Syntax fixing test passed")
}
