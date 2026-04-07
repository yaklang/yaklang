package plugin

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKindString(t *testing.T) {
	require.Equal(t, "new-pm", KindNewPM.String())
	require.Equal(t, "legacy", KindLegacy.String())
	require.Equal(t, "tool", KindTool.String())
	require.Equal(t, "unknown", Kind(99).String())
}

func TestDescriptorValidate_Valid(t *testing.T) {
	d := &Descriptor{
		Name: "test-plugin",
		Kind: KindNewPM,
		Path: "/usr/lib/LLVMTestPlugin.so",
	}
	require.NoError(t, d.Validate())
}

func TestDescriptorValidate_EmptyName(t *testing.T) {
	d := &Descriptor{
		Kind: KindNewPM,
		Path: "/usr/lib/LLVMTestPlugin.so",
	}
	err := d.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Name")
}

func TestDescriptorValidate_EmptyPath(t *testing.T) {
	d := &Descriptor{
		Name: "test-plugin",
		Kind: KindNewPM,
	}
	err := d.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Path")
}
