package tests

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func extractIntLines(output string) []int64 {
	lines := strings.Split(output, "\n")
	nums := make([]int64, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if v, err := strconv.ParseInt(line, 10, 64); err == nil {
			nums = append(nums, v)
		}
	}
	return nums
}

func loadTestdata(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", path, err)
	}
	return string(data)
}

// 1. Lifecycle test
func TestInterop_Lifecycle(t *testing.T) {
	code := loadTestdata(t, "interop_lifecycle.yak")
	output := runBinary(t, code, "main")
	if !strings.Contains(output, "[Go] Created object 100") {
		t.Fatalf("Expected creation log not found. Output:\n%s", output)
	}
}

// 2. Member read/write test
func TestInterop_MemberAccess(t *testing.T) {
	code := loadTestdata(t, "interop_member_access.yak")
	output := runBinary(t, code, "main")
	nums := extractIntLines(output)
	if len(nums) != 2 || nums[0] != 10 || nums[1] != 20 {
		t.Fatalf("Expected printed values [10 20], got %v. Output:\n%s", nums, output)
	}
}

// 3. Function pass test
func TestInterop_FuncPass(t *testing.T) {
	code := loadTestdata(t, "interop_func_pass.yak")
	output := runBinary(t, code, "main")
	if !strings.Contains(output, "[Go] Dump:") {
		t.Fatalf("Expected dump log not found. Output:\n%s", output)
	}
}
