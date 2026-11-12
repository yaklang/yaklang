package aicommon

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

type attachedCapabilityTestLoop struct {
	*attachedBrowserTestLoop
	config *Config
}

func newAttachedCapabilityTestLoop(t *testing.T, options ...ConfigOption) *attachedCapabilityTestLoop {
	t.Helper()
	options = append([]ConfigOption{WithDisableAutoSkills(true)}, options...)
	config := NewConfig(context.Background(), options...)
	return &attachedCapabilityTestLoop{
		attachedBrowserTestLoop: &attachedBrowserTestLoop{config: config},
		config:                  config,
	}
}

func TestAttachedAIToolResourceResolvesStableName(t *testing.T) {
	tool := aitool.NewWithoutCallback(
		"stable_tool_name",
		aitool.WithDescription("stable tool description"),
		aitool.WithVerboseName("Readable Tool"),
		aitool.WithVerboseNameZh("可读工具"),
		aitool.WithStringParam("target"),
	)
	loop := newAttachedCapabilityTestLoop(t, WithTools(tool))

	data := &AttachedAIToolResourceData{Key: CONTEXT_PROVIDER_KEY_NAME}
	require.NoError(t, data.Unmarshal(NewAttachedResource(
		CONTEXT_PROVIDER_TYPE_AITOOL,
		CONTEXT_PROVIDER_KEY_NAME,
		tool.GetName(),
	)))
	require.NoError(t, data.BindLoopData(loop))
	require.Nil(t, data.lookupErr)
	require.Equal(t, tool.GetName(), data.tool.GetName())
	require.True(t, loop.config.GetAiToolManager().IsRecentlyUsedTool(tool.GetName()))
	rendered := data.ToAttachData(loop)
	require.Contains(t, rendered, "stable tool description")
	require.Contains(t, rendered, `"target"`)
	require.NotContains(t, rendered, "Readable Tool")
}

func TestAttachedAIToolResourceDoesNotResolveVerboseName(t *testing.T) {
	tool := aitool.NewWithoutCallback(
		"stable_tool_name",
		aitool.WithVerboseName("Readable Tool"),
		aitool.WithVerboseNameZh("可读工具"),
	)
	loop := newAttachedCapabilityTestLoop(t, WithTools(tool))

	data := &AttachedAIToolResourceData{Key: CONTEXT_PROVIDER_KEY_NAME}
	require.NoError(t, data.Unmarshal(NewAttachedResource(
		CONTEXT_PROVIDER_TYPE_AITOOL,
		CONTEXT_PROVIDER_KEY_NAME,
		"Readable Tool",
	)))
	require.NoError(t, data.BindLoopData(loop))
	require.Error(t, data.lookupErr)
	rendered := data.ToAttachData(loop)
	require.Contains(t, rendered, "User pinned an AI tool")
	require.Contains(t, rendered, "Readable Tool")
	require.NotContains(t, rendered, "not found")
	require.NotContains(t, rendered, "Lookup Result")
}

func TestAttachedAIToolResourceKeepsUnresolvedMark(t *testing.T) {
	loop := newAttachedCapabilityTestLoop(t)
	data := &AttachedAIToolResourceData{Key: CONTEXT_PROVIDER_KEY_NAME}
	require.NoError(t, data.Unmarshal(NewAttachedResource(CONTEXT_PROVIDER_TYPE_AITOOL, CONTEXT_PROVIDER_KEY_NAME, "missing-tool-display-name")))
	require.NoError(t, data.BindLoopData(loop))
	require.Error(t, data.lookupErr)
	rendered := data.ToAttachData(loop)
	require.Contains(t, rendered, "User pinned an AI tool")
	require.Contains(t, rendered, "missing-tool-display-name")
	require.NotContains(t, rendered, "not found")
	require.NotContains(t, rendered, "Lookup Result")
}

func TestAttachedAIForgeResourceRecommendsRunnableForge(t *testing.T) {
	forge := &schema.AIForge{
		ForgeName:        "stable_forge_name",
		ForgeVerboseName: "Readable Forge",
		ForgeType:        schema.FORGE_TYPE_YAK,
		Description:      "forge description",
		Params:           `cli.String("target")`,
	}
	loop := newAttachedCapabilityTestLoop(t, WithForges(forge))

	data := &AttachedAIForgeResourceData{Key: CONTEXT_PROVIDER_KEY_NAME}
	require.NoError(t, data.Unmarshal(NewAttachedResource(
		CONTEXT_PROVIDER_TYPE_AIFORGE,
		CONTEXT_PROVIDER_KEY_NAME,
		forge.ForgeName,
	)))
	require.NoError(t, data.BindLoopData(loop))
	require.Nil(t, data.lookupErr)
	require.Equal(t, forge.ForgeName, data.forge.ForgeName)
	rendered := data.ToAttachData(loop)
	require.Contains(t, rendered, "forge description")
	require.Contains(t, rendered, `cli.String("target")`)
	require.Contains(t, rendered, `"blueprint_payload":"stable_forge_name"`)
	require.NotContains(t, rendered, "Readable Forge")
}

func TestAttachedAIForgeResourceDoesNotResolveVerboseName(t *testing.T) {
	forge := &schema.AIForge{
		ForgeName:        "stable_forge_name",
		ForgeVerboseName: "Readable Forge",
		ForgeType:        schema.FORGE_TYPE_YAK,
	}
	loop := newAttachedCapabilityTestLoop(t, WithForges(forge))

	data := &AttachedAIForgeResourceData{Key: CONTEXT_PROVIDER_KEY_NAME}
	require.NoError(t, data.Unmarshal(NewAttachedResource(
		CONTEXT_PROVIDER_TYPE_AIFORGE,
		CONTEXT_PROVIDER_KEY_NAME,
		"Readable Forge",
	)))
	require.NoError(t, data.BindLoopData(loop))
	require.Error(t, data.lookupErr)
	rendered := data.ToAttachData(loop)
	require.Contains(t, rendered, "User pinned an AI forge/skill")
	require.Contains(t, rendered, "Readable Forge")
	require.NotContains(t, rendered, "not found")
	require.NotContains(t, rendered, "Lookup Result")
}

func TestAttachedAIForgeResourceKeepsUnresolvedMark(t *testing.T) {
	loop := newAttachedCapabilityTestLoop(t)
	data := &AttachedAIForgeResourceData{Key: CONTEXT_PROVIDER_KEY_NAME}
	require.NoError(t, data.Unmarshal(NewAttachedResource(CONTEXT_PROVIDER_TYPE_AIFORGE, CONTEXT_PROVIDER_KEY_NAME, "missing-forge-display-name")))
	require.NoError(t, data.BindLoopData(loop))
	require.Error(t, data.lookupErr)
	rendered := data.ToAttachData(loop)
	require.Contains(t, rendered, "User pinned an AI forge/skill")
	require.Contains(t, rendered, "missing-forge-display-name")
	require.NotContains(t, rendered, "not found")
	require.NotContains(t, rendered, "Lookup Result")
	require.NotContains(t, rendered, "AI Forge lookup failed")
	require.NotContains(t, rendered, "AI Skill lookup/load failed")
}

func TestAttachedAIForgeResourceLoadsSkillMD(t *testing.T) {
	loop := newAttachedCapabilityTestLoop(t, WithSkillsFS(buildSkillsVFS()))
	data := &AttachedAIForgeResourceData{Key: CONTEXT_PROVIDER_KEY_NAME}
	require.NoError(t, data.Unmarshal(NewAttachedResource(CONTEXT_PROVIDER_TYPE_AIFORGE, CONTEXT_PROVIDER_KEY_NAME, "code-review")))
	require.NoError(t, data.BindLoopData(loop))
	require.Nil(t, data.lookupErr)
	require.True(t, loop.config.GetSkillsContextManager().IsSkillLoaded("code-review"))
	rendered := data.ToAttachData(loop)
	require.Empty(t, rendered, "loaded Skill content is already written by LoadForcedSkill")
	timeline, err := MarshalTimeline(loop.config.GetTimeline())
	require.NoError(t, err)
	require.True(t, strings.Contains(string(timeline), "code-review"))
}

func TestLoadTaskSkillWithoutManagerDoesNotMarkLoaded(t *testing.T) {
	err := loadTaskSkill(nil, &aiskillloader.SkillMeta{Name: "code-review"})
	require.ErrorContains(t, err, "context manager is not configured")
}
