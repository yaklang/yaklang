package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDisableAIVerificationIsInheritedByChildConfig(t *testing.T) {
	parent := NewConfig(context.Background(), WithDisableAIVerification(true))
	require.True(t, IsAIVerificationDisabled(parent))

	child := NewConfig(context.Background(), ConvertConfigToOptions(parent)...)
	require.True(t, IsAIVerificationDisabled(child),
		"nested ReAct/plan/forge configs must not silently re-enable verification AI")
}
