package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContextProviderManager_TaskOnlyKeepsHostAndExcludesLaterInputs(t *testing.T) {
	parent := NewContextProviderManager()
	parent.Register("host", FileContentContextProvider("HOST_POLICY"))
	end := parent.BeginTaskContext("first", FileContentContextProvider("FIRST_INPUT"))
	child := parent.WithoutTaskContext()
	end()
	end = parent.BeginTaskContext("next", FileContentContextProvider("NEXT_INPUT"))
	defer end()
	grandchild := child.snapshotForChild()
	for _, manager := range []*ContextProviderManager{child, grandchild} {
		text := manager.Execute(nil, nil)
		require.Contains(t, text, "HOST_POLICY")
		require.NotContains(t, text, "FIRST_INPUT")
		require.NotContains(t, text, "NEXT_INPUT")
	}
	require.Contains(t, parent.Execute(nil, nil), "NEXT_INPUT")
}
