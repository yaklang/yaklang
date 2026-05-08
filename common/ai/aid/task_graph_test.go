package aid

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

func newTaskGraphTestCoordinator(t *testing.T, workdir string, handler func(*schema.AiOutputEvent)) *Coordinator {
	t.Helper()
	ctx := context.Background()
	coordinator, err := NewCoordinatorContext(
		ctx,
		"task-graph-test",
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			if handler != nil {
				handler(event)
			}
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			response := config.NewAIResponse()
			response.Close()
			return response, nil
		}),
	)
	require.NoError(t, err)
	coordinator.Workdir = workdir
	return coordinator
}

func newGraphTask(c *Coordinator, name string) *AiTask {
	return c.generateAITaskWithName(name, name+" goal")
}

func graphDeps(t *testing.T, graph *executableTaskGraph, taskIndex string) []string {
	t.Helper()
	node, ok := graph.Node(taskIndex)
	require.True(t, ok, "node %s not found", taskIndex)
	return node.deps
}

func TestBuildStrictExecutableTaskGraph_LeafNodesStayExecutableOnlyWithoutImplicitChaining(t *testing.T) {
	coordinator := newTaskGraphTestCoordinator(t, t.TempDir(), nil)

	root := newGraphTask(coordinator, "Root")
	group := newGraphTask(coordinator, "Collect")
	parse := newGraphTask(coordinator, "Parse")
	enrich := newGraphTask(coordinator, "Enrich")
	review := newGraphTask(coordinator, "Review")

	group.ParentTask = root
	parse.ParentTask = group
	enrich.ParentTask = group
	review.ParentTask = root
	group.Subtasks = []*AiTask{parse, enrich}
	root.Subtasks = []*AiTask{group, review}

	coordinator.standardizeTaskTree(root)

	graph, err := buildStrictExecutableTaskGraph(root)
	require.NoError(t, err)

	require.Equal(t, []string{parse.Index, enrich.Index, review.Index}, []string{
		graph.nodes[0].id,
		graph.nodes[1].id,
		graph.nodes[2].id,
	})
	_, exists := graph.Node(root.Index)
	assert.False(t, exists, "root structure task must not become executable")
	_, exists = graph.Node(group.Index)
	assert.False(t, exists, "non-leaf structure task must not become executable")

	assert.Empty(t, graphDeps(t, graph, parse.Index))
	assert.Empty(t, graphDeps(t, graph, enrich.Index))
	assert.Empty(t, graphDeps(t, graph, review.Index))
	assert.NotContains(t, graphDeps(t, graph, enrich.Index), group.Index)
	assert.NotContains(t, graphDeps(t, graph, review.Index), root.Index)
	assert.Equal(t, 0, graph.nodes[0].stage)
	assert.Equal(t, 0, graph.nodes[1].stage)
	assert.Equal(t, 0, graph.nodes[2].stage)
	require.Len(t, graph.stages, 1)
	require.Equal(t, []string{parse.Index, enrich.Index, review.Index}, []string{
		graph.stages[0][0].id,
		graph.stages[0][1].id,
		graph.stages[0][2].id,
	})
}

func TestBuildStrictExecutableTaskGraph_ExpandsStructuralDependencies(t *testing.T) {
	coordinator := newTaskGraphTestCoordinator(t, t.TempDir(), nil)

	root := newGraphTask(coordinator, "Root")
	prep := newGraphTask(coordinator, "Prep")
	fetch := newGraphTask(coordinator, "Fetch")
	normalize := newGraphTask(coordinator, "Normalize")
	analyze := newGraphTask(coordinator, "Analyze")
	publish := newGraphTask(coordinator, "Publish")
	draft := newGraphTask(coordinator, "Draft")
	finalize := newGraphTask(coordinator, "Finalize")

	prep.ParentTask = root
	analyze.ParentTask = root
	publish.ParentTask = root
	fetch.ParentTask = prep
	normalize.ParentTask = prep
	draft.ParentTask = publish
	finalize.ParentTask = publish

	normalize.DependsOn = []string{"Fetch"} // leaf -> leaf
	analyze.DependsOn = []string{"Prep"}    // leaf -> non-leaf
	publish.DependsOn = []string{"Analyze"} // non-leaf inheritance to subtree entry leaves
	finalize.DependsOn = []string{"Draft"}

	prep.Subtasks = []*AiTask{fetch, normalize}
	publish.Subtasks = []*AiTask{draft, finalize}
	root.Subtasks = []*AiTask{prep, analyze, publish}

	coordinator.standardizeTaskTree(root)

	graph, err := buildStrictExecutableTaskGraph(root)
	require.NoError(t, err)

	assert.Equal(t, []string{fetch.Index}, graphDeps(t, graph, normalize.Index))
	assert.ElementsMatch(t, []string{fetch.Index, normalize.Index}, graphDeps(t, graph, analyze.Index))
	assert.Equal(t, []string{analyze.Index}, graphDeps(t, graph, draft.Index))
	assert.Equal(t, []string{draft.Index}, graphDeps(t, graph, finalize.Index))
}

func TestBuildStrictExecutableTaskGraph_CalculatesStages(t *testing.T) {
	coordinator := newTaskGraphTestCoordinator(t, t.TempDir(), nil)

	root := newGraphTask(coordinator, "Root")
	a := newGraphTask(coordinator, "A")
	b := newGraphTask(coordinator, "B")
	c := newGraphTask(coordinator, "C")

	a.ParentTask = root
	b.ParentTask = root
	c.ParentTask = root
	b.DependsOn = []string{"A"}
	c.DependsOn = []string{"A", "B"}
	root.Subtasks = []*AiTask{a, b, c}

	coordinator.standardizeTaskTree(root)

	graph, err := buildStrictExecutableTaskGraph(root)
	require.NoError(t, err)

	stageA, ok := graph.StageOf(a.Index)
	require.True(t, ok)
	stageB, ok := graph.StageOf(b.Index)
	require.True(t, ok)
	stageC, ok := graph.StageOf(c.Index)
	require.True(t, ok)

	assert.Equal(t, 0, stageA)
	assert.Equal(t, 1, stageB)
	assert.Equal(t, 2, stageC)
	require.Len(t, graph.stages, 3)
}

func TestBuildStrictExecutableTaskGraph_UnknownDependencyWarnsAndIgnores(t *testing.T) {
	coordinator := newTaskGraphTestCoordinator(t, t.TempDir(), nil)
	root := newGraphTask(coordinator, "Root")
	a := newGraphTask(coordinator, "A")
	b := newGraphTask(coordinator, "B")
	a.ParentTask = root
	b.ParentTask = root
	b.DependsOn = []string{"A", "missing"}
	root.Subtasks = []*AiTask{a, b}
	coordinator.standardizeTaskTree(root)

	graph, err := buildStrictExecutableTaskGraph(root)
	require.NoError(t, err)
	assert.Equal(t, []string{a.Index}, graphDeps(t, graph, b.Index))
	stageA, _ := graph.StageOf(a.Index)
	stageB, _ := graph.StageOf(b.Index)
	assert.Equal(t, 0, stageA)
	assert.Equal(t, 1, stageB)
}

func TestBuildStrictExecutableTaskGraph_CycleStillFails(t *testing.T) {
	t.Run("cycle", func(t *testing.T) {
		coordinator := newTaskGraphTestCoordinator(t, t.TempDir(), nil)
		root := newGraphTask(coordinator, "Root")
		a := newGraphTask(coordinator, "A")
		b := newGraphTask(coordinator, "B")
		a.ParentTask = root
		b.ParentTask = root
		a.DependsOn = []string{"B"}
		b.DependsOn = []string{"A"}
		root.Subtasks = []*AiTask{a, b}
		coordinator.standardizeTaskTree(root)

		_, err := buildStrictExecutableTaskGraph(root)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "cycle")
	})
}

func TestEmitTaskDependencyGraph_WritesExecutableGraphArtifact(t *testing.T) {
	workdir := t.TempDir()
	coordinator := newTaskGraphTestCoordinator(t, workdir, nil)
	root := newGraphTask(coordinator, "Root")
	group := newGraphTask(coordinator, "Collect Data")
	child := newGraphTask(coordinator, "Fetch Data")
	group.ParentTask = root
	child.ParentTask = group
	group.Subtasks = []*AiTask{child}
	root.Subtasks = []*AiTask{group}
	coordinator.standardizeTaskTree(root)

	markdown, err := coordinator.buildTaskDependencyGraphMarkdown(root, "initial graph")
	require.NoError(t, err)
	assert.Contains(t, markdown, "## 任务依赖图")
	assert.Contains(t, markdown, "说明：图中仅展示会真正执行的叶子任务节点")
	assert.Contains(t, markdown, "更新说明：")
	assert.Contains(t, markdown, "```mermaid")
	assert.Contains(t, markdown, "flowchart TB")
	assert.Contains(t, markdown, child.Index+" Fetch Data")
	assert.NotContains(t, markdown, root.Index+" Root")
	assert.NotContains(t, markdown, group.Index+" Collect Data")

	coordinator.standardizeTaskTreeAndNotify(root, "initial graph")

	entries, err := os.ReadDir(workdir)
	require.NoError(t, err)
	found := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), "task_dependency_graph_1_") && strings.HasSuffix(entry.Name(), ".md") {
			found = true
			content, readErr := os.ReadFile(filepath.Join(workdir, entry.Name()))
			require.NoError(t, readErr)
			assert.Contains(t, string(content), "```mermaid")
			assert.Contains(t, string(content), child.Index+" Fetch Data")
		}
	}
	assert.True(t, found, "expected task graph artifact file")
}
