package aispec

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMissingStructuredOutputExecutorExplainsImport(t *testing.T) {
	previous := structuredOutputExecutor.Swap(nil)
	t.Cleanup(func() { structuredOutputExecutor.Store(previous) })
	for _, unset := range []bool{true, false} {
		if !unset {
			RegisterStructuredOutputExecutor(nil)
		}
		err := RequireStructuredOutputExecutor()
		require.ErrorIs(t, err, ErrStructuredOutputExecutorNotRegistered)
		require.Contains(t, err.Error(), "import (\n\t_ \"github.com/yaklang/yaklang/common/ai/aid/liteforge\"\n)")
		called := false
		result, err := ExecuteStructuredOutput("input", map[string]any{"field": "description"}, func(string, ...AIConfigOption) (string, error) {
			called = true
			return "", nil
		})
		require.ErrorIs(t, err, ErrStructuredOutputExecutorNotRegistered)
		require.Nil(t, result)
		require.False(t, called, "unloaded runtime must fail before any model request")
	}
}
