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

func TestStandardizeTaskTree_AssignsDefaultDependenciesByDFS(t *testing.T) {
	coordinator := newTaskGraphTestCoordinator(t, t.TempDir(), nil)
	root := &AiTask{Name: "Root", Goal: "root goal"}
	childA := &AiTask{Name: "Collect", Goal: "collect data", ParentTask: root}
	childA1 := &AiTask{Name: "Parse", Goal: "parse data", ParentTask: childA}
	childB := &AiTask{Name: "Review", Goal: "review data", ParentTask: root, DependsOn: []string{}}
	childC := &AiTask{Name: "Report", Goal: "report data", ParentTask: root, DependsOn: []string{"custom_dep"}}
	childA.Subtasks = []*AiTask{childA1}
	root.Subtasks = []*AiTask{childA, childB, childC}

	coordinator.standardizeTaskTree(root)

	assert.Equal(t, "1", root.Index)
	assert.Equal(t, "1-1", childA.Index)
	assert.Equal(t, "1-1-1", childA1.Index)
	assert.Equal(t, "1-2", childB.Index)
	assert.Equal(t, "1-3", childC.Index)

	assert.Nil(t, root.DependsOn)
	assert.Equal(t, []string{"1"}, childA.DependsOn)
	assert.Equal(t, []string{"1-1"}, childA1.DependsOn)
	assert.Equal(t, []string{"1-1-1"}, childB.DependsOn)
	assert.Equal(t, []string{"custom_dep"}, childC.DependsOn)
	assert.Same(t, coordinator, childA.Coordinator)
	assert.Same(t, childA, childA1.ParentTask)
}

func TestEmitTaskDependencyGraph_WritesArtifactAndStreamsMarkdown(t *testing.T) {
	workdir := t.TempDir()
	coordinator := newTaskGraphTestCoordinator(t, workdir, nil)
	root := &AiTask{Name: "Root", Goal: "root goal"}
	child := &AiTask{Name: "Collect Data", Goal: "collect", ParentTask: root}
	root.Subtasks = []*AiTask{child}
	coordinator.standardizeTaskTree(root)

	markdown, err := coordinator.buildTaskDependencyGraphMarkdown(root, "initial graph")
	require.NoError(t, err)
	assert.Contains(t, markdown, "## 任务依赖图")
	assert.Contains(t, markdown, "更新说明：")
	assert.Contains(t, markdown, "```mermaid")
	assert.Contains(t, markdown, "flowchart TB")
	assert.Contains(t, markdown, "1 Root")
	assert.Contains(t, markdown, "1-1 Collect Data")

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
			assert.Contains(t, string(content), "flowchart TB")
		}
	}
	assert.True(t, found, "expected task graph artifact file")
}