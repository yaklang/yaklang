package aid

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/workflowdag"
)

const taskDependencyGraphNodeID = "task-dependency"

type taskGraphNode struct {
	task *AiTask
	id   string
	deps []string
}

func (n *taskGraphNode) GetID() string {
	return n.id
}

func (n *taskGraphNode) DependsOn() []string {
	return n.deps
}

func (n *taskGraphNode) AllowFailed() bool {
	return false
}

func (n *taskGraphNode) IsDone() bool {
	return n.task.executed()
}

func (n *taskGraphNode) IsProcessing() bool {
	return n.task.executing()
}

func (n *taskGraphNode) IsFailed() bool {
	return n.task.GetStatus() == aicommon.AITaskState_Aborted
}

func (n *taskGraphNode) IsSkipped() bool {
	return n.task.skiped()
}

func findTaskTreeRoot(task *AiTask) *AiTask {
	if task == nil {
		return nil
	}
	root := task
	for i := 0; i < 1000 && root.ParentTask != nil; i++ {
		root = root.ParentTask
	}
	return root
}

func normalizeDependencyRefs(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	result := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, dep := range raw {
		trimmed := strings.TrimSpace(dep)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func buildTaskReferenceMap(root *AiTask) map[string]string {
	references := make(map[string]string)
	order := DFSOrderAiTask(root)
	for i := 0; i < order.Len(); i++ {
		task, ok := order.Get(i)
		if !ok || task == nil || task.Index == "" {
			continue
		}
		references[task.Index] = task.Index
		if name := strings.TrimSpace(task.Name); name != "" {
			if _, exists := references[name]; !exists {
				references[name] = task.Index
			}
		}
		if semanticID := strings.TrimSpace(task.GetSemanticIdentifier()); semanticID != "" {
			if _, exists := references[semanticID]; !exists {
				references[semanticID] = task.Index
			}
		}
	}
	return references
}

func applyDefaultDependsOn(root *AiTask) {
	if root == nil {
		return
	}
	order := DFSOrderAiTask(root)
	previousIndex := ""
	for i := 0; i < order.Len(); i++ {
		task, ok := order.Get(i)
		if !ok || task == nil {
			continue
		}
		task.DependsOn = normalizeDependencyRefs(task.DependsOn)
		if len(task.DependsOn) == 0 && previousIndex != "" {
			task.DependsOn = []string{previousIndex}
		}
		previousIndex = task.Index
	}
}

func (c *Coordinator) standardizeTaskTree(task *AiTask) *AiTask {
	root := findTaskTreeRoot(task)
	if root == nil {
		return nil
	}
	c.ensureTaskTreeInitialized(root)
	root.GenerateIndex()
	applyDefaultDependsOn(root)
	if root.ParentTask == nil {
		c.rootTask = root
	}
	return root
}

func (c *Coordinator) standardizeTaskTreeAndNotify(task *AiTask, reason string) *AiTask {
	root := c.standardizeTaskTree(task)
	if root == nil {
		return nil
	}
	if reason != "" {
		if err := c.emitTaskDependencyGraph(root, reason); err != nil {
			log.Warnf("emit task dependency graph failed: %v", err)
		}
	}
	return root
}

func (c *Coordinator) emitTaskGraphArtifact(identifier string, payload string) string {
	workdir := c.GetOrCreateWorkDir()
	if workdir == "" {
		return ""
	}
	if err := os.MkdirAll(workdir, 0755); err != nil {
		log.Warnf("create task graph workdir failed: %v", err)
		return ""
	}
	if c.Emitter != nil && !c.IsArtifactsPinned() {
		c.Emitter.EmitPinDirectory(workdir)
		c.SetArtifactsPinned()
	}
	if !strings.HasSuffix(identifier, "_") {
		identifier += "_"
	}
	filename := identifier + utils.DatetimePretty2() + ".md"
	fullpath := filepath.Join(workdir, filename)
	if err := os.WriteFile(fullpath, []byte(payload), 0644); err != nil {
		log.Warnf("write task graph artifact failed: %v", err)
		return ""
	}
	if c.Emitter != nil {
		c.Emitter.EmitPinFilename(fullpath)
	}
	return fullpath
}

func (c *Coordinator) buildTaskDependencyMermaid(root *AiTask) (string, error) {
	if root == nil {
		return "", workflowdag.ErrEmptyDAG
	}
	ctx := c.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	dag := workflowdag.New[*taskGraphNode](ctx)
	order := DFSOrderAiTask(root)
	references := buildTaskReferenceMap(root)
	for i := 0; i < order.Len(); i++ {
		task, ok := order.Get(i)
		if !ok || task == nil || task.Index == "" {
			continue
		}
		resolvedDeps := make([]string, 0, len(task.DependsOn))
		normalizedDeps := normalizeDependencyRefs(task.DependsOn)
		for _, dep := range normalizedDeps {
			if depIndex, exists := references[dep]; exists {
				resolvedDeps = append(resolvedDeps, depIndex)
			}
		}
		if err := dag.AddNode(&taskGraphNode{task: task, id: task.Index, deps: resolvedDeps}); err != nil {
			return "", err
		}
	}
	if err := dag.Build(); err != nil {
		return "", err
	}
	options := workflowdag.DefaultMermaidOptions()
	options.Direction = workflowdag.MermaidDirectionTB
	options.ShowEdgeLabels = false
	options.Title = "任务依赖图"
	options.NodeLabelFunc = func(node workflowdag.DAGNode) string {
		graphNode, ok := node.(*taskGraphNode)
		if !ok || graphNode == nil || graphNode.task == nil {
			return node.GetID()
		}
		label := strings.TrimSpace(graphNode.task.Name)
		if label == "" {
			label = graphNode.task.Goal
		}
		if label == "" {
			return graphNode.task.Index
		}
		return fmt.Sprintf("%s %s", graphNode.task.Index, label)
	}
	return dag.GenerateMermaidFlowChartWithOptions(options)
}

func localizeTaskGraphReason(reason string) string {
	switch strings.TrimSpace(reason) {
	case "":
		return ""
	case "mock plan initialized":
		return "使用 Mock 计划初始化任务树"
	case "initial plan generated":
		return "初始任务规划已生成"
	case "plan review added subtasks":
		return "任务审查后新增了子任务"
	case "task delta replace_all applied":
		return "任务变更已整体替换后续任务"
	case "task delta applied":
		return "任务变更已应用"
	case "deep think subtasks updated":
		return "深入思考后子任务已更新"
	case "dynamic plan updated":
		return "动态规划已更新"
	case "task appended":
		return "已追加新任务"
	default:
		return reason
	}
}

func (c *Coordinator) buildTaskDependencyGraphMarkdown(root *AiTask, reason string) (string, error) {
	mermaid, err := c.buildTaskDependencyMermaid(root)
	if err != nil {
		return "", err
	}
	var builder strings.Builder
	builder.WriteString("## 任务依赖图\n\n")
	localizedReason := localizeTaskGraphReason(reason)
	if localizedReason != "" {
		builder.WriteString(fmt.Sprintf("更新说明：%s\n\n", localizedReason))
	}
	builder.WriteString("```mermaid\n")
	builder.WriteString(mermaid)
	if !strings.HasSuffix(mermaid, "\n") {
		builder.WriteRune('\n')
	}
	builder.WriteString("```\n")
	return builder.String(), nil
}

func (c *Coordinator) emitTaskDependencyGraph(root *AiTask, reason string) error {
	if c == nil || root == nil || c.Emitter == nil {
		return nil
	}

	markdown, err := c.buildTaskDependencyGraphMarkdown(root, reason)
	if err != nil {
		return err
	}

	updated := false
	if reason != "" {
		updated = true
		event, err := c.Emitter.EmitDefaultStreamEvent(taskDependencyGraphNodeID, strings.NewReader(reason), root.Index)
		if err != nil {
			return err
		}
		event.GetStreamEventWriterId()
		_, _ = c.Emitter.EmitTextReferenceMaterial("task-dependency-graph", markdown)
	}

	artifactName := fmt.Sprintf("task_dependency_graph_%s", strings.ReplaceAll(root.Index, "-", "_"))
	artifactPath := c.emitTaskGraphArtifact(artifactName, markdown)
	if artifactPath != "" {
		markdown = markdown + fmt.Sprintf("\n图文件：%s\n", artifactPath)
	}

	if updated {
		return nil
	}
	_, err = c.Emitter.EmitDefaultStreamEvent(taskDependencyGraphNodeID, strings.NewReader(markdown), root.Index)
	if err != nil {
		return err
	}
	return nil
}
