package aicommon

import (
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestAttachedKnowledgeBasePreferences(t *testing.T) {
	for _, tc := range []struct{ key, value, hint string }{
		{CONTEXT_PROVIDER_KEY_NAME, "all", "Prefer the user's attached knowledge base name"},
		{CONTEXT_PROVIDER_KEY_NAME, "  项目资料\n优先中文说明。 \n", "Prefer the user's attached knowledge base name"},
		{CONTEXT_PROVIDER_KEY_SYSTEM_FLAG, CONTEXT_PROVIDER_VALUE_ALL_KNOWLEDGE_BASE, "explicitly selected all knowledge bases"},
		{CONTEXT_PROVIDER_KEY_SYSTEM_FLAG, CONTEXT_PROVIDER_VALUE_AUTO_SELECT_KNOWLEDGE_BASE + ":优先项目资料", "Do not substitute all=true"},
	} {
		t.Run(tc.value, func(t *testing.T) {
			input := NewAttachedResource(AttachedResourceTypeKnowledgeBase, tc.key, tc.value)
			resource, err := ParseAttachedResourceData(input)
			require.NoError(t, err)
			knowledge := resource.(*AttachedKnowledgeBaseResourceData)
			require.Equal(t, tc.key, knowledge.Key)
			require.Equal(t, tc.value, knowledge.Value)
			rendered := resource.ToAttachData(nil)
			require.Contains(t, rendered, tc.hint)
			require.Contains(t, rendered, "```\n"+tc.value+"\n```", "preserve exact user attachment bytes")
			require.Equal(t, tc.value, input.Value)
		})
	}
}

func TestAttachedKnowledgeBaseLongContentReadBack(t *testing.T) {
	content := " \n" + strings.Repeat("优先中文资料😀\n", 1500) + "TAIL_PREFERENCE\n "
	resource, err := ParseAttachedResourceData(NewAttachedResource(AttachedResourceTypeKnowledgeBase, "selection_notes", content))
	require.NoError(t, err)
	rendered := resource.ToAttachData(nil)
	require.True(t, utf8.ValidString(rendered))
	require.Less(t, len(rendered), 10*1024)
	_, after, found := strings.Cut(rendered, "Full content saved to file: ")
	require.True(t, found)
	path, _, _ := strings.Cut(after, "\n")
	t.Cleanup(func() { _ = os.Remove(path) })
	full, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, content, string(full))
	require.Equal(t, rendered, resource.ToAttachData(nil), "reuse the saved original instead of creating duplicate files")
}
