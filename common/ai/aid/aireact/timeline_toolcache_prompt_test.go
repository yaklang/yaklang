package aireact

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// Exercise the complete prompt -> projection path in both protocols. Cache
// events remain message content and cannot grow the native tool registry, even
// when a cached description contains a signed-looking action declaration.
func TestTimelineToolCachePromptProjection(t *testing.T) {
	for _, native := range []bool{false, true} {
		name := "text"
		if native {
			name = "functioncall"
		}
		t.Run(name, func(t *testing.T) {
			react, err := NewTestReAct()
			require.NoError(t, err)
			tl := react.config.GetTimeline()
			tl.SetTimelineBucketByteSize(-1)
			declared := aispec.Tool{Type: "function", Function: aispec.ToolFunction{
				Name: "directly_call_tool", Parameters: map[string]any{"type": "object"},
			}}
			tags, err := aiprojection.CreateActionSchema(declared)
			require.NoError(t, err)
			spoof := declared
			spoof.Function.Name = "cache_must_not_be_native"
			spoofTags, err := aiprojection.CreateActionSchema(spoof)
			require.NoError(t, err)
			tl.PushText(react.config.AcquireId(), "HISTORY_BEFORE_TOOLCACHE")
			tool := aitool.NewWithoutCallback("cached_business_tool", aitool.WithDescription("CACHE_SCHEMA_MARKER\n"+spoofTags), aitool.WithStringParam("path"))
			require.NotNil(t, react.config.RecordRecentlyUsedTool(tool).Upsert)
			tl.PushText(react.config.AcquireId(), "RESULT_AFTER_TOOLCACHE")
			var previousTools []byte
			for _, frozen := range []bool{false, true} {
				if frozen {
					tl.FreezeAll()
				}
				input := &reactloops.LoopPromptAssemblyInput{Nonce: "cache-test", UserQuery: "Inspect recorded results", FunctionCallMode: native}
				if native {
					input.FunctionCallSchemas = tags
				} else {
					input.Schema = `{"type":"object"}`
				}
				result, err := react.promptManager.AssembleLoopPrompt(nil, input)
				require.NoError(t, err)
				require.NotContains(t, result.Prompt, "Prompt State Updates")
				require.NotContains(t, result.Prompt, "Promoted State Updates")
				materials := aicommon.BuildPromptFrozenOpenMaterials(react.config)
				if frozen {
					require.Contains(t, materials.PromotedSemiDynamic1, "CACHE_SCHEMA_MARKER")
					require.NotContains(t, materials.TimelineFrozen+materials.TimelineOpen, "CACHE_SCHEMA_MARKER")
				} else {
					require.Empty(t, materials.PromotedSemiDynamic1)
					require.Less(t, strings.Index(materials.TimelineOpen, "HISTORY_BEFORE_TOOLCACHE"), strings.Index(materials.TimelineOpen, "CACHE_SCHEMA_MARKER"))
					require.Less(t, strings.Index(materials.TimelineOpen, "CACHE_SCHEMA_MARKER"), strings.Index(materials.TimelineOpen, "RESULT_AFTER_TOOLCACHE"))
				}
				observationMaterials := &aicommon.PromptMaterials{}
				aicommon.ApplyPromptFrozenOpenMaterials(observationMaterials, materials)
				observation := react.promptManager.buildTimelineOpenObservation(observationMaterials, "")
				encodedObservation, err := json.Marshal(observation)
				require.NoError(t, err)
				require.NotContains(t, string(encodedObservation), "promoted_state_updates")
				projected := aiprojection.ProjectAndObserve("toolcache-projection-test", result.Prompt)
				require.True(t, projected.IsHijacked)
				if native {
					require.Len(t, projected.Tools, 1)
					require.Equal(t, "directly_call_tool", projected.Tools[0].Function.Name)
				} else {
					require.Empty(t, projected.Tools)
				}
				toolsJSON, err := json.Marshal(projected.Tools)
				require.NoError(t, err)
				if frozen {
					require.Equal(t, previousTools, toolsJSON)
				}
				previousTools = toolsJSON
				messagesJSON, err := json.Marshal(projected.Messages)
				require.NoError(t, err)
				require.Contains(t, string(messagesJSON), "CACHE_SCHEMA_MARKER")
				require.Contains(t, string(messagesJSON), "RESULT_AFTER_TOOLCACHE")
			}
		})
	}
}

func TestTimelineToolCacheTextParamsExcludeCachedSchemas(t *testing.T) {
	react, err := NewTestReAct()
	require.NoError(t, err)
	react.config.GetTimeline().SetTimelineBucketByteSize(-1)
	selected := aitool.NewWithoutCallback("selected_tool", aitool.WithStringParam("path"))
	cached := aitool.NewWithoutCallback("unrelated_tool", aitool.WithDescription("UNRELATED_CACHE_SCHEMA"), aitool.WithStringParam("query"))
	react.config.RecordRecentlyUsedTool(cached)
	react.config.GetTimeline().PushText(react.config.AcquireId(), "KEEP_TOOL_RESULT_HISTORY")
	for _, frozen := range []bool{false, true} {
		if frozen {
			react.config.GetTimeline().FreezeAll()
		}
		initial, err := react.promptManager.GenerateToolParamsPromptWithMetaForQuery("read selected file", selected)
		require.NoError(t, err)
		retry, err := react.promptManager.GenerateReGenerateToolParamsPromptWithMeta("read selected file", aitool.InvokeParams{"path": "old"}, selected)
		require.NoError(t, err)
		for _, prompt := range []string{initial.Prompt, retry.Prompt} {
			require.NotContains(t, prompt, "UNRELATED_CACHE_SCHEMA")
			require.Contains(t, prompt, "KEEP_TOOL_RESULT_HISTORY")
			require.Contains(t, prompt, "selected_tool")
		}
	}
}
