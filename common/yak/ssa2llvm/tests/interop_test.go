package tests

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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

// 1. Lifecycle test
func TestInterop_Lifecycle(t *testing.T) {
	code := `
func main() {
    a = getObject(100)
    a = 0
}
`
	output := checkRunBinary(t, code, "main", map[string]string{"GCLOG": "1"}, []string{
		"[Go] Created object 100",
		"[Yak GC] Finalizer triggered",
		"[Go] Releasing handle",
	})
	require.NotEmpty(t, output)
}

// 2. Member read/write test
func TestInterop_MemberAccess(t *testing.T) {
	code := `
func main() {
    a = getObject(10)
    v1 = a.Number
    println(v1)

    a.Number = 20
    v2 = a.Number
    println(v2)
}
`
	output := checkRunBinary(t, code, "main", nil, []string{"10\n", "20\n"})
	nums := extractIntLines(output)
	require.Len(t, nums, 2)
	require.Equal(t, int64(10), nums[0])
	require.Equal(t, int64(20), nums[1])
}

// 3. Function pass test
func TestInterop_FuncPass(t *testing.T) {
	code := `
func main() {
    a = getObject(99)
    dump(a)
}
`
	checkRunBinary(t, code, "main", nil, []string{
		"[Go] Dump:",
		"Number:99",
		"Name:YakTest",
	})
}
