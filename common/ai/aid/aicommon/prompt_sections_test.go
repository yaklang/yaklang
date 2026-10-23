package aicommon

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
)

func TestPromptSectionRoles(t *testing.T) {
	for _, tc := range []struct {
		role PromptSectionRole
		zh   PromptSectionRoleZH
	}{
		{PromptSectionRoleHighStatic, PromptSectionRoleZHHighStatic},
		{PromptSectionRoleFrozenBlock, PromptSectionRoleZHFrozenBlock},
		{PromptSectionRoleSemiDynamic1, PromptSectionRoleZHSemiDynamic1},
		{PromptSectionRoleSemiDynamic2, PromptSectionRoleZHSemiDynamic2},
		{PromptSectionRoleTimelineOpen, PromptSectionRoleZHTimelineOpen},
		{PromptSectionRoleDynamic, PromptSectionRoleZHDynamic},
		{"unknown", ""},
	} {
		t.Run(string(tc.role), func(t *testing.T) {
			require.Equal(t, tc.zh, tc.role.Zh())
			section := NewPromptSectionObservation("key", "label", tc.role, true, "content")
			require.Equal(t, tc.role, section.Role)
			require.Equal(t, tc.zh, section.RoleZh)
		})
	}
}

func TestPromptSectionObservationMetrics(t *testing.T) {
	leaf := NewPromptSectionObservation("leaf", "Leaf", PromptSectionRoleDynamic, true, "alpha\nbeta")
	require.True(t, leaf.IsIncluded())
	require.True(t, leaf.Compressible)
	require.Equal(t, len(leaf.Content), leaf.ContentBytes())
	require.Equal(t, 2, leaf.LineCount())
	require.Equal(t, MeasureTokens(leaf.Content), leaf.EstimatedTokens)
	blank := NewPromptSectionObservation("blank", "Blank", PromptSectionRoleDynamic, false, " \t")
	require.False(t, blank.IsIncluded())
	branch := NewPromptContainerSection("branch", "Branch", PromptSectionRoleDynamic)
	branch.Children = []*PromptSectionObservation{nil, leaf, blank}
	root := NewPromptContainerSection("root", "Root", PromptSectionRoleDynamic)
	root.Children = []*PromptSectionObservation{branch}
	require.Same(t, root, FinalizePromptContainerSection(root))
	require.True(t, root.IsIncluded())
	require.Equal(t, leaf.ContentBytes()+blank.ContentBytes(), root.ContentBytes())
	require.Equal(t, leaf.LineCount()+blank.LineCount(), root.LineCount())
	require.Equal(t, leaf.EstimatedTokens+blank.EstimatedTokens, root.EstimatedTokens)
	// Re-finalizing recomputes every child, without accumulating old metrics.
	leaf.Content = ""
	FinalizePromptContainerSection(root)
	require.False(t, root.IsIncluded())
	require.Equal(t, blank.ContentBytes(), root.ContentBytes())
	require.Equal(t, blank.LineCount(), root.LineCount())
	require.Equal(t, blank.EstimatedTokens, root.EstimatedTokens)
}

func TestPromptSectionObservationNilSafety(t *testing.T) {
	var section *PromptSectionObservation
	require.False(t, section.IsIncluded())
	require.Zero(t, section.ContentBytes())
	require.Zero(t, section.LineCount())
	require.Nil(t, FinalizePromptContainerSection(section))
}

func TestPromptAssemblyTypedSectionsJSON(t *testing.T) {
	content := "sensitive boundary " + aiprojection.Nonce() + "\nordinary content"
	leaf := NewPromptSectionObservation("leaf", "Leaf", PromptSectionRoleTimelineOpen, true, content)
	require.NotContains(t, leaf.Content, aiprojection.Nonce())
	require.Contains(t, leaf.Content, "ordinary content")
	root := NewPromptContainerSection("timeline", "Timeline", PromptSectionRoleTimelineOpen)
	root.Children = []*PromptSectionObservation{leaf}
	FinalizePromptContainerSection(root)
	result := &LoopPromptAssemblyResult{Prompt: "assembled prompt", Sections: []*PromptSectionObservation{root}}
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	var restored LoopPromptAssemblyResult
	require.NoError(t, json.Unmarshal(raw, &restored))
	require.Equal(t, result, &restored)
	require.Equal(t, PromptSectionRoleZHTimelineOpen, restored.Sections[0].RoleZh)
	require.Equal(t, leaf.Content, restored.Sections[0].Children[0].Content)
	require.Equal(t, strings.Count(leaf.Content, "\n")+1, restored.Sections[0].Lines)
	// The serialized section contract remains the original snake_case fields.
	var decoded struct {
		Sections []struct {
			Role            string `json:"role"`
			RoleZh          string `json:"role_zh"`
			EstimatedTokens int    `json:"estimated_tokens"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Len(t, decoded.Sections, 1)
	require.Equal(t, "timelineOpen", decoded.Sections[0].Role)
	require.Equal(t, "时间线开放段", decoded.Sections[0].RoleZh)
	require.Equal(t, root.EstimatedTokens, decoded.Sections[0].EstimatedTokens)
}
