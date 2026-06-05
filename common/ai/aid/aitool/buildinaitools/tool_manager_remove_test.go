package buildinaitools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestRemoveToolByName(t *testing.T) {
	keep := aitool.NewWithoutCallback("keep_tool")
	remove := aitool.NewWithoutCallback("remove_me")
	mgr := NewToolManagerByToolGetter(func() []*aitool.Tool {
		return []*aitool.Tool{keep, remove}
	}, WithExtendTools([]*aitool.Tool{keep, remove}, true))

	mgr.RemoveToolByName("remove_me")

	tools, err := mgr.GetEnableTools()
	require.NoError(t, err)
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	assert.Contains(t, names, "keep_tool")
	assert.NotContains(t, names, "remove_me")
}
