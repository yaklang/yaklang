package reactloops

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/schema"
)

func TestLegacyRAGOptionsAreNoOps(t *testing.T) {
	registerStubRequireToolAction(t)
	for _, option := range []ReActLoopOption{
		WithAllowRAG(), WithAllowRAG(true), WithAllowRAG(false),
		WithAllowRAGGetter(func() bool { t.Fatal("deprecated getter must not be evaluated"); return true }),
	} {
		invoker := mock.NewMockInvoker(context.Background())
		options := append(minimalNewReActLoopOptions(), option)
		loop, err := NewReActLoop("legacy-rag-disabled", invoker, options...)
		require.NoError(t, err)
		t.Cleanup(loop.Release)
		_, exists := loop.actions.Get(schema.AI_REACT_LOOP_ACTION_KNOWLEDGE_ENHANCE)
		require.False(t, exists)
		require.False(t, loop.allowPerceptionKnowledgeRefresh())
	}
}
