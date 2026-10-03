package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnhanceKnowledgeForkSeparatesWorkerEmittersAndCollections(t *testing.T) {
	var seen *Emitter
	parent := NewEnhanceKnowledgeManager(func(_ context.Context, emitter *Emitter, _ string) (<-chan EnhanceKnowledge, error) {
		seen = emitter
		out := make(chan EnhanceKnowledge)
		close(out)
		return out, nil
	})
	parentEmitter, childEmitter := &Emitter{}, &Emitter{}
	parent.SetEmitter(parentEmitter)
	parent.AppendKnowledge("task", &BasicEnhanceKnowledge{UUID: "existing", Content: "source"})
	child := parent.ForkForSubAgent()
	child.SetEmitter(childEmitter)
	_, err := child.FetchKnowledge(context.Background(), "query")
	require.NoError(t, err)
	require.Same(t, childEmitter, seen)
	_, err = parent.FetchKnowledge(context.Background(), "query")
	require.NoError(t, err)
	require.Same(t, parentEmitter, seen)
	child.AppendKnowledge("task", &BasicEnhanceKnowledge{UUID: "child-only"})
	child.SetKnowledgeUseless("task", "existing")
	require.Len(t, parent.GetKnowledgeByTaskID("task"), 1)
	require.Equal(t, "existing", parent.GetKnowledgeByTaskID("task")[0].GetUUID())
	require.Len(t, child.GetKnowledgeByTaskID("task"), 1)
	require.Equal(t, "child-only", child.GetKnowledgeByTaskID("task")[0].GetUUID())
}
