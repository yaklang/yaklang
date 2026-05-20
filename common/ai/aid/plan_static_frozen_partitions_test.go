package aid

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestBuildPlanStaticFrozenPartitions(t *testing.T) {
	mem := GetDefaultContextProvider()
	cod := &Coordinator{
		Config:          &aicommon.Config{Ctx: context.Background()},
		ContextProvider: mem,
		userInput:       "test",
	}
	root := cod.generateAITaskWithName("Root", "root goal")
	root.Index = "1"

	appendPlanFactsFrozenPartition(cod.Config, "## Facts\n- stable fact")
	appendPlanDocumentFrozenPartition(cod.Config, "## Document\n- stable guidance")

	partitions := BuildPlanStaticFrozenPartitions(cod.Config.GetOrCreateFrozenBlockPartitionProducer())
	require.Len(t, partitions, 2)
	require.Equal(t, "plan_facts", partitions[0].ID)
	require.Equal(t, "Plan Facts", partitions[0].Title)
	require.Equal(t, "## Facts\n- stable fact", partitions[0].Content)
	require.Equal(t, 100, partitions[0].Order)
	require.Equal(t, "plan_document", partitions[1].ID)
	require.Equal(t, "Plan Document", partitions[1].Title)
	require.Equal(t, "## Document\n- stable guidance", partitions[1].Content)
	require.Equal(t, 110, partitions[1].Order)

	again := BuildPlanStaticFrozenPartitions(cod.Config.GetOrCreateFrozenBlockPartitionProducer())
	require.Equal(t, partitions[0].Nonce, again[0].Nonce)
	require.Equal(t, partitions[1].Nonce, again[1].Nonce)
}

func TestBuildPlanStaticFrozenPartitionsEmpty(t *testing.T) {
	require.Empty(t, BuildPlanStaticFrozenPartitions(nil))
}

func TestBuildPlanStaticFrozenPartitionsDoesNotParseTaskInput(t *testing.T) {
	mem := GetDefaultContextProvider()
	cod := &Coordinator{
		Config:          &aicommon.Config{Ctx: context.Background()},
		ContextProvider: mem,
		userInput:       "test",
	}
	root := cod.generateAITaskWithName("Root", "root goal")
	root.Index = "1"
	root.SetUserInput(`<|FACTS_n1|>
## Facts
- from task
<|FACTS_END_n1|>

<|DOCUMENT_n2|>
## Document
- from task
<|DOCUMENT_END_n2|>

root input`)
	cod.rootTask = root
	mem.RootTask = root

	require.Empty(t, BuildPlanStaticFrozenPartitions(cod.Config.GetOrCreateFrozenBlockPartitionProducer()), "plan static partitions must come from explicit config append, not task user input")
}

func TestTaskProgressDoesNotPrependPlanStaticDocs(t *testing.T) {
	mem := GetDefaultContextProvider()
	cod := &Coordinator{
		Config:          &aicommon.Config{Ctx: context.Background()},
		ContextProvider: mem,
		userInput:       "test",
	}
	root := cod.generateAITaskWithName("Root", "root goal")
	root.Index = "1"
	appendPlanFactsFrozenPartition(cod.Config, "## Facts\n- stable fact")
	appendPlanDocumentFrozenPartition(cod.Config, "## Document\n- stable guidance")

	progress := root.Progress()
	require.Contains(t, progress, "Root")
	require.NotContains(t, progress, "FACTS_")
	require.NotContains(t, progress, "DOCUMENT_")
	require.NotContains(t, progress, "stable fact")
	require.NotContains(t, progress, "stable guidance")
}
