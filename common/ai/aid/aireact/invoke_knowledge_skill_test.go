package aireact

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

// Replaces the former automatic-RAG answer/artifact tests. Both production
// protocols expose the skill and Yak tools; the removed action stays unavailable
// even if a legacy caller explicitly enables its old switch.
func TestReAct_KnowledgeToolsReplaceLegacyRAG(t *testing.T) {
	useTempBuiltinSkillReleaseDB(t)
	useTempYakitHome(t)
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("native=%t", native), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithDisableAutoSkills(false),
				aicommon.WithEnableFunctionCallMode(native), aicommon.WithWorkdir(t.TempDir()),
				aicommon.WithDisableCreateDBRuntime(true),
				aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					t.Error("tool discovery must not trigger automatic knowledge selection or an AI call")
					return nil, fmt.Errorf("unexpected automatic AI call")
				}))
			require.NoError(t, err)
			loop, err := reactloops.CreateLoopByName("default", r, reactloops.WithAllowRAG(true),
				reactloops.WithAllowRAGGetter(func() bool { t.Error("legacy getter must stay a no-op"); return true }))
			require.NoError(t, err)
			defer loop.Release()
			require.NotContains(t, loop.GetAllActionNames(), schema.AI_REACT_LOOP_ACTION_KNOWLEDGE_ENHANCE)
			_, err = loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_KNOWLEDGE_ENHANCE)
			require.Error(t, err)
			require.True(t, loop.GetSkillsContextManager().IsAutoSkillLoadedAndUnfolded("yaklang-system"))
			r.config.GetTimeline().FreezeAll()
			for _, name := range []string{"manage_knowledge_base", "query_knowledge_base"} {
				require.Contains(t, loop.GetSkillsContextManager().Render("knowledge-test"), name)
			}
		})
	}
}
