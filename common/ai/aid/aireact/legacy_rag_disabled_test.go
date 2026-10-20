package aireact

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

func TestLegacyRAGRetrievalDisabled(t *testing.T) {
	// No config or database: retired entry points must fail before doing any work.
	runtime := &ReAct{}
	_, enhanceErr := runtime.EnhanceKnowledgeGetterEx(context.Background(), "question", nil, "docs")
	_, quickErr := runtime.QuickKnowledgeSearch(context.Background(), "question", nil, "docs")
	_, selectErr := runtime.SelectKnowledgeBase(context.Background(), "question")
	for _, err := range []error{enhanceErr, quickErr, selectErr} {
		require.Error(t, err)
		for _, guidance := range []string{"yaklang-system", "manage_knowledge_base(op=list)", "query_knowledge_base", "token_limit"} {
			require.Contains(t, err.Error(), guidance)
		}
	}
}

func TestKnowledgeCallersDoNotExposeLegacyActions(t *testing.T) {
	runtime, err := NewTestReAct(aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableCreateDBRuntime(true))
	require.NoError(t, err)
	for _, name := range []string{schema.AI_REACT_LOOP_NAME_DEFAULT, schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE,
		schema.AI_REACT_LOOP_NAME_REPORT_GENERATING, schema.AI_REACT_LOOP_NAME_INFOSEC_RECON} {
		t.Run(name, func(t *testing.T) {
			loop, err := reactloops.CreateLoopByName(name, runtime, reactloops.WithAllowRAGGetter(func() bool {
				t.Fatal("deprecated RAG option must not be evaluated")
				return true
			}))
			require.NoError(t, err)
			defer loop.Release()
			for _, action := range []string{schema.AI_REACT_LOOP_ACTION_KNOWLEDGE_ENHANCE, "search_knowledge", "search_knowledge_infosec"} {
				require.False(t, loop.Actions().Have(action), action)
			}
		})
	}
}
