package sfanalysis

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCheckSearchIndirection covers the callee round trip: jump to the function
// value, filter the name there, walk back to the call sites, intersect. The
// direct form filters the call itself.
func TestCheckSearchIndirection(t *testing.T) {
	indirect := `
*?{opcode: If} as $stmt
$stmt<cfgConditionValues> as $cond
$cond #> ?{opcode: call} as $calls
$calls #> ?{opcode: Function} as $callees
$callees?{*strcmp*} as $callee
$callee -> ?{opcode: call} as $callers
$calls & $callers as $call
`
	direct := `
*?{opcode: If} as $stmt
$stmt<cfgConditionValues> as $cond
$cond #> ?{opcode: call} as $calls
$calls?{*strcmp*} as $call
`

	t.Run("round trip is reported", func(t *testing.T) {
		report := Analyze(context.Background(), indirect, WithProfile(ProfileQuality))
		require.NotNil(t, report.Quality)
		found := false
		for _, p := range report.Quality.Problems {
			if p.Type == ProblemTypeIndirectCalleeSearch {
				found = true
				require.Equal(t, Info, p.Severity)
				require.Contains(t, p.Suggestion, "$calls?{*strcmp*}")
			}
		}
		require.True(t, found, "the callee round trip must be reported")
	})

	t.Run("direct form is not reported", func(t *testing.T) {
		report := Analyze(context.Background(), direct, WithProfile(ProfileQuality))
		require.NotNil(t, report.Quality)
		for _, p := range report.Quality.Problems {
			require.NotEqual(t, ProblemTypeIndirectCalleeSearch, p.Type)
		}
	})

	t.Run("comments do not trigger it", func(t *testing.T) {
		commented := direct + `
// 反例：$calls #> ?{opcode: Function} as $f
// $f -> ?{opcode: call} as $callers
// $calls & $callers as $bad
`
		report := Analyze(context.Background(), commented, WithProfile(ProfileQuality))
		for _, p := range report.Quality.Problems {
			require.NotEqual(t, ProblemTypeIndirectCalleeSearch, p.Type)
		}
	})

	t.Run("no target value condition is not enough", func(t *testing.T) {
		// Missing `& $callers` or the return edge: not the pattern.
		report := Analyze(context.Background(), `
*?{opcode: If} as $stmt
$stmt<cfgConditionValues> as $cond
$cond #> ?{opcode: call} as $calls
$calls #> ?{opcode: Function} as $callees
$callees?{*strcmp*} as $callee
`, WithProfile(ProfileQuality))
		for _, p := range report.Quality.Problems {
			require.NotEqual(t, ProblemTypeIndirectCalleeSearch, p.Type)
		}
	})
}