package aireact

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestScenarioRecognition_MergeQueriesDeduplicatesAndPreservesOrder(t *testing.T) {
	merged := reactloops.MergeScenarioSearchQueries(
		[]string{"query a", "query b"},
		[]string{"query b", "query c"},
		[]string{"query a", "query d"},
	)

	require.Equal(t, []string{"query a", "query b", "query c", "query d"}, merged)
}

func TestScenarioRecognition_FallbackQueriesProduceThreeGroups(t *testing.T) {
	toolQueries, knowledgeQueries, memoryQueries := reactloops.BuildFallbackScenarioQueries(&fakeScenarioGetter{values: map[string]string{
		"user_query":               "修复 Java 反编译代码",
		"upstream_intent_analysis": "重构反编译后的 Java 代码",
	}}, "Java 反编译重构场景")

	require.NotEmpty(t, toolQueries)
	require.NotEmpty(t, knowledgeQueries)
	require.NotEmpty(t, memoryQueries)
	require.Contains(t, toolQueries[0], "工具")
	require.Contains(t, knowledgeQueries[0], "知识")
	require.Contains(t, memoryQueries[0], "Java 反编译重构场景")
}

type fakeScenarioGetter struct {
	values map[string]string
}

func (f *fakeScenarioGetter) Get(key string) string {
	return f.values[key]
}
