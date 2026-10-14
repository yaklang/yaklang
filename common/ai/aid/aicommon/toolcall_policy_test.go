package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDisableToolUseBlocksInvocationAndPropagates(t *testing.T) {
	parent := NewConfig(context.Background(), WithDisableAutoSkills(true), WithDisableToolUse(true))
	child := NewConfig(context.Background(), ConvertConfigToOptions(parent)...)
	for _, config := range []*Config{parent, child} {
		require.True(t, config.IsToolUseDisabled())
		caller := &ToolCaller{config: config}
		_, err := caller.invoke(nil, nil, nil, nil, nil, nil, nil, nil)
		require.EqualError(t, err, "tool execution is disabled")
	}
}
