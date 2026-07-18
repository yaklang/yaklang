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

func TestDirectlyAnswerViaMainLoopIsInheritedByChildConfig(t *testing.T) {
	parent := NewConfig(context.Background(), WithDirectlyAnswerViaMainLoop(true))
	require.True(t, IsDirectlyAnswerViaMainLoopEnabled(parent))

	child := NewConfig(context.Background(), ConvertConfigToOptions(parent)...)
	require.True(t, IsDirectlyAnswerViaMainLoopEnabled(child),
		"nested ReAct/plan/forge configs must preserve main-loop answer routing")
}
