package loop_yaklangcode

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractErrorLineNumbers(t *testing.T) {
	fb := `[Error]: ... in [63:8 -- 63:9] from compiler
   63 >         + "line"
------------------------[Error]: ... in [65:81 -- 65:82]`
	lines := extractErrorLineNumbers(fb)
	require.Equal(t, []int{63, 65}, lines)
}

func TestFormatReactiveCurrentCode_FocusWindow(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 120; i++ {
		b.WriteString("println(")
		b.WriteString(strings.Repeat("x", 8))
		b.WriteString(")\n")
	}
	code := b.String()
	feedback := `in [60:1 -- 60:2] from compiler
   60 > println(xxxxxxxx)`
	out := formatReactiveCurrentCode(code, 0, feedback, true)
	assert.Contains(t, out, "省略未报错区域")
	assert.Contains(t, out, "60 |")
	// Avoid substring false positive: "11 |" / "21 |" also contain "1 |"
	assert.False(t, strings.Contains(out, "\n1 |") || strings.HasPrefix(out, "1 |"),
		"line 1 should be omitted from focus window")
	assert.Less(t, len(out), len(code))
}

func TestShrinkReactiveSamplesAfterCodeExists(t *testing.T) {
	raw := strings.Repeat("sample\n", 2000)
	out := shrinkReactiveSamplesAfterCodeExists(raw, true)
	assert.Less(t, len(out), len(raw))
	assert.Equal(t, raw, shrinkReactiveSamplesAfterCodeExists(raw, false))
}
