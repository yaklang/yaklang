package loop_java_decompiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils/yakgit/yakdiff"
)

// TestYakdiffBasicUsage tests basic usage of yakdiff library
func TestYakdiffBasicUsage(t *testing.T) {
	original := `public class HelloWorld {
    public static void main(String[] args) {
        System.out.println("Hello, World!");
    }
}`

	modified := `public class HelloWorld {
    public static void main(String[] args) {
        System.out.println("Hello, Yaklang!");
        System.out.println("Welcome!");
    }
}`

	diff, err := yakdiff.Diff(original, modified)
	if err != nil {
		t.Fatalf("yakdiff.Diff failed: %v", err)
	}

	t.Logf("Diff result:\n%s", diff)

	// Verify diff contains expected changes
	if !strings.Contains(diff, "-") || !strings.Contains(diff, "+") {
		t.Error("Diff should contain - and + markers")
	}

	if !strings.Contains(diff, "Hello, World!") {
		t.Error("Diff should contain removed line")
	}

	if !strings.Contains(diff, "Hello, Yaklang!") {
		t.Error("Diff should contain added line")
	}
}

// TestYakdiffIdenticalContent tests diff with identical content
func TestYakdiffIdenticalContent(t *testing.T) {
	content := `public class Test {
    private int value;
    
    public int getValue() {
        return value;
    }
}`

	diff, err := yakdiff.Diff(content, content)
	if err != nil {
		t.Fatalf("yakdiff.Diff failed: %v", err)
	}

	// Identical content should produce empty diff
	if strings.TrimSpace(diff) != "" {
		t.Errorf("Expected empty diff for identical content, got: %s", diff)
	}

	t.Log("Identical content test passed: empty diff")
}

// TestYakdiffEmptyFiles tests diff with empty files
func TestYakdiffEmptyFiles(t *testing.T) {
	tests := []struct {
		name      string
		content1  string
		content2  string
		hasAdd    bool
		hasRemove bool
	}{
		{
			name:      "Both empty",
			content1:  "",
			content2:  "",
			hasAdd:    false,
			hasRemove: false,
		},
		{
			name:      "Empty to content",
			content1:  "",
			content2:  "public class Test {}",
			hasAdd:    true,
			hasRemove: false,
		},
		{
			name:      "Content to empty",
			content1:  "public class Test {}",
			content2:  "",
			hasAdd:    false,
			hasRemove: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff, err := yakdiff.Diff(tt.content1, tt.content2)
			if err != nil {
				t.Fatalf("yakdiff.Diff failed: %v", err)
			}

			t.Logf("Test '%s' diff:\n%s", tt.name, diff)

			if tt.hasAdd && !strings.Contains(diff, "+") {
				t.Error("Expected diff to contain + (additions)")
			}

			if tt.hasRemove && !strings.Contains(diff, "-") {
				t.Error("Expected diff to contain - (removals)")
			}

			if !tt.hasAdd && !tt.hasRemove && strings.TrimSpace(diff) != "" {
				t.Error("Expected empty diff when both empty")
			}
		})
	}
}

// TestCompareFilesWithRealFiles tests file comparison with actual files
func TestCompareFilesWithRealFiles(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Create original file
	originalPath := filepath.Join(tmpDir, "Test.java")
	originalContent := `public class Test {
    private String name;
    
    public Test(String name) {
        this.name = name;
    }
    
    public String getName() {
        return name;
    }
}`

	err := os.WriteFile(originalPath, []byte(originalContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write original file: %v", err)
	}

	// Create backup
	backupPath := originalPath + ".bak"
	err = os.WriteFile(backupPath, []byte(originalContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write backup file: %v", err)
	}

	// Modify original file
	modifiedContent := `public class Test {
    private String name;
    private int age;
    
    public Test(String name, int age) {
        this.name = name;
        this.age = age;
    }
    
    public String getName() {
        return name;
    }
    
    public int getAge() {
        return age;
    }
}`

	err = os.WriteFile(originalPath, []byte(modifiedContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write modified file: %v", err)
	}

	// Read both files
	originalBytes, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("Failed to read backup: %v", err)
	}

	modifiedBytes, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatalf("Failed to read modified: %v", err)
	}

	// Generate diff
	diff, err := yakdiff.Diff(originalBytes, modifiedBytes)
	if err != nil {
		t.Fatalf("Failed to generate diff: %v", err)
	}

	t.Logf("File comparison diff:\n%s", diff)

	// Verify diff contains expected changes
	expectedChanges := []string{
		"+    private int age;",
		"-    public Test(String name) {",
		"+    public Test(String name, int age) {",
		"+        this.age = age;",
		"+    public int getAge() {",
		"+        return age;",
	}

	for _, expected := range expectedChanges {
		if !strings.Contains(diff, strings.TrimSpace(expected)) {
			t.Errorf("Expected diff to contain: %s", expected)
		}
	}
}

// TestDiffLineCountingAccuracy tests the accuracy of added/removed line counting
func TestDiffLineCountingAccuracy(t *testing.T) {
	original := `Line 1
Line 2
Line 3
Line 4
Line 5`

	modified := `Line 1
Line 2 modified
Line 3
Line 4 modified
Line 5
Line 6 new`

	diff, err := yakdiff.Diff(original, modified)
	if err != nil {
		t.Fatalf("Failed to generate diff: %v", err)
	}

	t.Logf("Line counting test diff:\n%s", diff)

	// Count added and removed lines
	diffLines := strings.Split(diff, "\n")
	addedLines := 0
	removedLines := 0

	for _, line := range diffLines {
		if len(line) > 0 {
			if line[0] == '+' && !strings.HasPrefix(line, "+++") {
				addedLines++
				t.Logf("Added line: %s", line)
			} else if line[0] == '-' && !strings.HasPrefix(line, "---") {
				removedLines++
				t.Logf("Removed line: %s", line)
			}
		}
	}

	t.Logf("Added lines: %d, Removed lines: %d", addedLines, removedLines)

	// We should have some additions and removals
	if addedLines == 0 {
		t.Error("Expected some added lines")
	}
	if removedLines == 0 {
		t.Error("Expected some removed lines")
	}
}

// TestDiffWithLargeFiles tests diff performance with large files
func TestDiffWithLargeFiles(t *testing.T) {
	// Generate large file content
	var lines1, lines2 []string
	for i := 0; i < 1000; i++ {
		lines1 = append(lines1, "    // This is line "+string(rune('0'+i%10)))
		if i%50 == 25 {
			lines2 = append(lines2, "    // This is modified line "+string(rune('0'+i%10)))
		} else {
			lines2 = append(lines2, "    // This is line "+string(rune('0'+i%10)))
		}
	}

	content1 := strings.Join(lines1, "\n")
	content2 := strings.Join(lines2, "\n")

	diff, err := yakdiff.Diff(content1, content2)
	if err != nil {
		t.Fatalf("Failed to generate diff for large files: %v", err)
	}

	t.Logf("Large file diff length: %d bytes", len(diff))

	// Verify diff is generated
	if strings.TrimSpace(diff) == "" {
		t.Error("Expected non-empty diff for modified large files")
	}

	// Verify it contains modifications
	if !strings.Contains(diff, "modified") {
		t.Error("Expected diff to contain modifications")
	}
}

// TestDiffWithSpecialCharacters tests diff with special characters
func TestDiffWithSpecialCharacters(t *testing.T) {
	tests := []struct {
		name     string
		content1 string
		content2 string
	}{
		{
			name:     "Unicode characters",
			content1: "// 中文注释\npublic class Test {}",
			content2: "// 中文注释修改\npublic class Test {}",
		},
		{
			name:     "Special symbols",
			content1: "String regex = \"[a-z]+\";",
			content2: "String regex = \"[A-Z]+\";",
		},
		{
			name:     "Tab vs spaces",
			content1: "public void method() {\n\tSystem.out.println(\"test\");\n}",
			content2: "public void method() {\n    System.out.println(\"test\");\n}",
		},
		{
			name:     "Escape sequences in strings",
			content1: "String s = \"Hello\\nWorld\";",
			content2: "String s = \"Hello\\n\\tWorld\";",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff, err := yakdiff.Diff(tt.content1, tt.content2)
			if err != nil {
				t.Fatalf("Failed to generate diff: %v", err)
			}

			t.Logf("Test '%s' diff:\n%s", tt.name, diff)

			// Verify diff is generated for different content
			if tt.content1 != tt.content2 && strings.TrimSpace(diff) == "" {
				t.Errorf("Expected non-empty diff for different content in test '%s'", tt.name)
			}
		})
	}
}

// TestDiffOutputFormat tests that diff output follows unified diff format
func TestDiffOutputFormat(t *testing.T) {
	original := `public class Original {
    public void method1() {
        System.out.println("method1");
    }
}`

	modified := `public class Modified {
    public void method1() {
        System.out.println("method1");
    }
    
    public void method2() {
        System.out.println("method2");
    }
}`

	diff, err := yakdiff.Diff(original, modified)
	if err != nil {
		t.Fatalf("Failed to generate diff: %v", err)
	}

	t.Logf("Diff output:\n%s", diff)

	// Verify unified diff format markers
	expectedMarkers := []string{
		"---", // original file marker
		"+++", // modified file marker
		"@@",  // hunk header
	}

	for _, marker := range expectedMarkers {
		if !strings.Contains(diff, marker) {
			t.Errorf("Expected diff to contain unified diff marker: %s", marker)
		}
	}
}

// TestCompareBackupScenarios tests various backup comparison scenarios
func TestCompareBackupScenarios(t *testing.T) {
	tmpDir := t.TempDir()

	scenarios := []struct {
		name           string
		originalCode   string
		modifiedCode   string
		shouldHaveDiff bool
		description    string
	}{
		{
			name: "No changes",
			originalCode: `public class Test {
    public void method() {
        System.out.println("test");
    }
}`,
			modifiedCode: `public class Test {
    public void method() {
        System.out.println("test");
    }
}`,
			shouldHaveDiff: false,
			description:    "Identical files should have no diff",
		},
		{
			name: "Comment added",
			originalCode: `public class Test {
    public void method() {
        System.out.println("test");
    }
}`,
			modifiedCode: `public class Test {
    // Added comment
    public void method() {
        System.out.println("test");
    }
}`,
			shouldHaveDiff: true,
			description:    "Adding comment should show in diff",
		},
		{
			name: "Method renamed",
			originalCode: `public class Test {
    public void oldMethod() {
        System.out.println("test");
    }
}`,
			modifiedCode: `public class Test {
    public void newMethod() {
        System.out.println("test");
    }
}`,
			shouldHaveDiff: true,
			description:    "Renaming method should show in diff",
		},
		{
			name: "Whitespace only change",
			originalCode: `public class Test {
    public void method() {
        System.out.println("test");
    }
}`,
			modifiedCode: `public class Test {
    public void method() {
        System.out.println("test"); 
    }
}`,
			shouldHaveDiff: true,
			description:    "Trailing whitespace should show in diff",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Create file and backup
			filePath := filepath.Join(tmpDir, scenario.name+".java")
			backupPath := filePath + ".bak"

			err := os.WriteFile(backupPath, []byte(scenario.originalCode), 0644)
			if err != nil {
				t.Fatalf("Failed to write backup: %v", err)
			}

			err = os.WriteFile(filePath, []byte(scenario.modifiedCode), 0644)
			if err != nil {
				t.Fatalf("Failed to write file: %v", err)
			}

			// Read and compare
			originalBytes, _ := os.ReadFile(backupPath)
			modifiedBytes, _ := os.ReadFile(filePath)

			diff, err := yakdiff.Diff(originalBytes, modifiedBytes)
			if err != nil {
				t.Fatalf("Failed to generate diff: %v", err)
			}

			hasDiff := strings.TrimSpace(diff) != ""

			if hasDiff != scenario.shouldHaveDiff {
				t.Errorf("Scenario '%s': expected hasDiff=%v, got hasDiff=%v\nDescription: %s\nDiff:\n%s",
					scenario.name, scenario.shouldHaveDiff, hasDiff, scenario.description, diff)
			}

			t.Logf("Scenario '%s' passed: %s", scenario.name, scenario.description)
		})
	}
}

// TestDiffMessageFormatting tests the formatting of diff messages
func TestDiffMessageFormatting(t *testing.T) {
	original := []byte("Line 1\nLine 2\nLine 3")
	modified := []byte("Line 1\nLine 2 modified\nLine 3\nLine 4")

	diff, err := yakdiff.Diff(original, modified)
	if err != nil {
		t.Fatalf("Failed to generate diff: %v", err)
	}

	// Count added and removed lines
	diffLines := strings.Split(diff, "\n")
	addedLines := 0
	removedLines := 0

	for _, line := range diffLines {
		if len(line) > 0 {
			if line[0] == '+' && !strings.HasPrefix(line, "+++") {
				addedLines++
			} else if line[0] == '-' && !strings.HasPrefix(line, "---") {
				removedLines++
			}
		}
	}

	// Format message like in action_compare_files.go
	msg := "Comparison result:\n\n"
	msg += "Unified Diff:\n"
	msg += "```diff\n"
	msg += diff
	msg += "\n```"

	t.Logf("Formatted message:\n%s", msg)
	t.Logf("Statistics: +%d lines added, -%d lines removed", addedLines, removedLines)

	// Verify message contains key components
	if !strings.Contains(msg, "```diff") {
		t.Error("Expected formatted message to contain diff code block")
	}

	if addedLines == 0 {
		t.Error("Expected some added lines")
	}
}

// BenchmarkYakdiff benchmarks yakdiff performance
func BenchmarkYakdiff(b *testing.B) {
	original := strings.Repeat("public class Test {\n    // Line\n}\n", 100)
	modified := strings.Repeat("public class Modified {\n    // Changed line\n}\n", 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := yakdiff.Diff(original, modified)
		if err != nil {
			b.Fatal(err)
		}
	}
}
