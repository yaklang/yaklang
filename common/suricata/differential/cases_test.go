package differential

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/suricata/rule"
)

func TestBuiltinCasesAreValid(t *testing.T) {
	seen := make(map[string]struct{})
	for _, testCase := range BuiltinCases() {
		t.Run(testCase.Name, func(t *testing.T) {
			require.NotEmpty(t, testCase.Description)
			require.NotEmpty(t, testCase.Engines)
			_, duplicate := seen[testCase.Name]
			require.False(t, duplicate)
			seen[testCase.Name] = struct{}{}
			_, err := rule.ParseStrictRule(testCase.Rules)
			require.NoError(t, err)
			frames, err := testCase.Traffic.Frames()
			require.NoError(t, err)
			require.NotEmpty(t, frames)
		})
	}
}
