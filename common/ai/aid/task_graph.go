package aid

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/workflowdag"
)

const taskDependencyGraphNodeID = "task-dependency"

type executableTaskNode struct {
	task  *AiTask
	id    string
	deps  []string
	stage int
	order int
}

func (n *executableTaskNode) GetID() string {
	return n.id
}

func (n *executableTaskNode) DependsOn() []string {
	return n.deps
}

func (n *executableTaskNode) AllowFailed() bool {
	return false
}

func (n *executableTaskNode) IsDone() bool {
	return n.task.executed()
}

func (n *executableTaskNode) IsProcessing() bool {
	return n.task.executing()
}

func (n *executableTaskNode) IsFailed() bool {
	return n.task.GetStatus() == aicommon.AITaskState_Aborted
}

func (n *executableTaskNode) IsSkipped() bool {
	return n.task.skiped()
}

type executableTaskGraph struct {
	root             *AiTask
	nodes            []*executableTaskNode
	nodeByID         map[string]*executableTaskNode
	dependents       map[string][]string
	stages           [][]*executableTaskNode
	stageByTaskIndex map[string]int
	order            []*AiTask
	orderIndexByTask map[string]int
}

func (g *executableTaskGraph) TotalTasks() int {
	if g == nil {
		return 0
	}
	return len(g.nodes)
}

func (g *executableTaskGraph) TotalStages() int {
	if g == nil {
		return 0
	}
	return len(g.stages)
}

func (g *executableTaskGraph) OrderedTasks() []*AiTask {
	if g == nil {
		return nil
	}
	result := make([]*AiTask, 0, len(g.order))
	result = append(result, g.order...)
	return result
}

func (g *executableTaskGraph) StageOf(taskIndex string) (int, bool) {
	if g == nil {
		return 0, false
	}
	stage, ok := g.stageByTaskIndex[strings.TrimSpace(taskIndex)]
	return stage, ok
}

func (g *executableTaskGraph) Node(taskIndex string) (*executableTaskNode, bool) {
	if g == nil {
		return nil, false
	}
	node, ok := g.nodeByID[strings.TrimSpace(taskIndex)]
	return node, ok
}

func (g *executableTaskGraph) OrderOf(taskIndex string) (int, bool) {
	if g == nil {
		return 0, false
	}
	idx, ok := g.orderIndexByTask[strings.TrimSpace(taskIndex)]
	return idx, ok
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

func normalizeTaskDependencyRefs(root *AiTask) {
	if root == nil {
		return
	}
	order := DFSOrderAiTask(root)
	for i := 0; i < order.Len(); i++ {
		task, ok := order.Get(i)
		if !ok || task == nil {
			continue
		}
		task.DependsOn = normalizeDependencyRefs(task.DependsOn)
	}
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

func executableLeafTasks(root *AiTask) []*AiTask {
	result := make([]*AiTask, 0)
	if root == nil {
		return result
	}
	order := DFSOrderAiTask(root)
	for i := 0; i < order.Len(); i++ {
		task, ok := order.Get(i)
		if !ok || task == nil || len(task.Subtasks) > 0 {
			continue
		}
		result = append(result, task)
	}
	return result
}

func buildExecutableLeavesByTask(root *AiTask) map[string][]*AiTask {
	result := make(map[string][]*AiTask)
	var walk func(task *AiTask) []*AiTask
	walk = func(task *AiTask) []*AiTask {
		if task == nil {
			return nil
		}
		if len(task.Subtasks) == 0 {
			result[task.Index] = []*AiTask{task}
			return result[task.Index]
		}
		var leaves []*AiTask
		for _, child := range task.Subtasks {
			leaves = append(leaves, walk(child)...)
		}
		result[task.Index] = leaves
		return leaves
	}
	walk(root)
	return result
}

func appendUniqueStrings(target []string, seen map[string]struct{}, values ...string) []string {
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		target = append(target, value)
	}
	return target
}

func addDependency(target map[string]struct{}, depID string) {
	if depID == "" {
		return
	}
	target[depID] = struct{}{}
}

func collectSubtreeEntryLeaves(subtreeLeaves []*AiTask, leafDeps map[string]map[string]struct{}) []*AiTask {
	if len(subtreeLeaves) == 0 {
		return nil
	}
	subtreeSet := make(map[string]struct{}, len(subtreeLeaves))
	for _, leaf := range subtreeLeaves {
		if leaf == nil || leaf.Index == "" {
			continue
		}
		subtreeSet[leaf.Index] = struct{}{}
	}
	var entries []*AiTask
	for _, leaf := range subtreeLeaves {
		if leaf == nil || leaf.Index == "" {
			continue
		}
		hasInternalDep := false
		for depID := range leafDeps[leaf.Index] {
			if _, ok := subtreeSet[depID]; ok {
				hasInternalDep = true
				break
			}
		}
		if !hasInternalDep {
			entries = append(entries, leaf)
		}
	}
	return entries
}

func resolveDependencyLeafTargets(task *AiTask, refs []string, references map[string]string, leavesByTask map[string][]*AiTask) ([]*AiTask, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	resolved := make([]*AiTask, 0, len(refs))
	seen := make(map[string]struct{})
	for _, ref := range refs {
		canonicalRef := strings.TrimSpace(ref)
		if canonicalRef == "" {
			continue
		}
		targetIndex, ok := references[canonicalRef]
		if !ok {
			if task != nil {
				log.Warnf("task executable DAG ignored unknown dependency: task=%q ref=%q", task.Index, canonicalRef)
			} else {
				log.Warnf("task executable DAG ignored unknown dependency ref=%q", canonicalRef)
			}
			continue
		}
		targetLeaves := leavesByTask[targetIndex]
		if len(targetLeaves) == 0 {
			if task != nil {
				log.Warnf("task executable DAG ignored dependency without executable leaves: task=%q ref=%q target=%q", task.Index, canonicalRef, targetIndex)
			} else {
				log.Warnf("task executable DAG ignored dependency without executable leaves: ref=%q target=%q", canonicalRef, targetIndex)
			}
			continue
		}
		for _, target := range targetLeaves {
			if target == nil || target.Index == "" {
				continue
			}
			if _, exists := seen[target.Index]; exists {
				continue
			}
			seen[target.Index] = struct{}{}
			resolved = append(resolved, target)
		}
	}
	return resolved, nil
}

func propagateExecutableLeafDependencies(task *AiTask, leafDeps map[string]map[string]struct{}, references map[string]string, leavesByTask map[string][]*AiTask) ([]*AiTask, error) {
	if task == nil {
		return nil, nil
	}
	if len(task.Subtasks) == 0 {
		if leafDeps[task.Index] == nil {
			leafDeps[task.Index] = make(map[string]struct{})
		}
		resolvedDeps, err := resolveDependencyLeafTargets(task, normalizeDependencyRefs(task.DependsOn), references, leavesByTask)
		if err != nil {
			return nil, err
		}
		for _, dep := range resolvedDeps {
			addDependency(leafDeps[task.Index], dep.Index)
		}
		return []*AiTask{task}, nil
	}

	subtreeLeaves := make([]*AiTask, 0)
	for _, child := range task.Subtasks {
		childLeaves, err := propagateExecutableLeafDependencies(child, leafDeps, references, leavesByTask)
		if err != nil {
			return nil, err
		}
		subtreeLeaves = append(subtreeLeaves, childLeaves...)
	}

	entryLeaves := collectSubtreeEntryLeaves(subtreeLeaves, leafDeps)
	resolvedDeps, err := resolveDependencyLeafTargets(task, normalizeDependencyRefs(task.DependsOn), references, leavesByTask)
	if err != nil {
		return nil, err
	}
	if len(resolvedDeps) > 0 {
		for _, entry := range entryLeaves {
			if entry == nil || entry.Index == "" {
				continue
			}
			if leafDeps[entry.Index] == nil {
				leafDeps[entry.Index] = make(map[string]struct{})
			}
			for _, dep := range resolvedDeps {
				addDependency(leafDeps[entry.Index], dep.Index)
			}
		}
	}
	return subtreeLeaves, nil
}

func buildStrictExecutableTaskGraph(root *AiTask) (*executableTaskGraph, error) {
	if root == nil {
		return nil, workflowdag.ErrEmptyDAG
	}

	leafOrder := executableLeafTasks(root)
	if len(leafOrder) == 0 {
		return nil, workflowdag.ErrEmptyDAG
	}

	references := buildTaskReferenceMap(root)
	leavesByTask := buildExecutableLeavesByTask(root)
	leafDeps := make(map[string]map[string]struct{}, len(leafOrder))
	for _, leaf := range leafOrder {
		if leaf == nil || leaf.Index == "" {
			continue
		}
		leafDeps[leaf.Index] = make(map[string]struct{})
	}
	if _, err := propagateExecutableLeafDependencies(root, leafDeps, references, leavesByTask); err != nil {
		return nil, err
	}

	nodes := make([]*executableTaskNode, 0, len(leafOrder))
	nodeByID := make(map[string]*executableTaskNode, len(leafOrder))
	orderIndexByTask := make(map[string]int, len(leafOrder))
	for idx, leaf := range leafOrder {
		if leaf == nil || leaf.Index == "" {
			continue
		}
		orderIndexByTask[leaf.Index] = idx
		deps := make([]string, 0, len(leafDeps[leaf.Index]))
		for depID := range leafDeps[leaf.Index] {
			if depID == leaf.Index {
				return nil, utils.Errorf("task executable DAG contains self dependency on %q", leaf.Index)
			}
			deps = append(deps, depID)
		}
		sort.Slice(deps, func(i, j int) bool {
			return orderIndexByTask[deps[i]] < orderIndexByTask[deps[j]]
		})
		node := &executableTaskNode{
			task:  leaf,
			id:    leaf.Index,
			deps:  deps,
			order: idx,
		}
		nodes = append(nodes, node)
		nodeByID[node.id] = node
	}

	dependents := make(map[string][]string, len(nodes))
	stageByTaskIndex, stages, err := calculateStrictExecutableStages(nodes)
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		for _, depID := range node.deps {
			dependents[depID] = append(dependents[depID], node.id)
		}
		if stage, ok := stageByTaskIndex[node.id]; ok {
			node.stage = stage
		}
	}

	return &executableTaskGraph{
		root:             root,
		nodes:            nodes,
		nodeByID:         nodeByID,
		dependents:       dependents,
		stages:           stages,
		stageByTaskIndex: stageByTaskIndex,
		order:            leafOrder,
		orderIndexByTask: orderIndexByTask,
	}, nil
}

func calculateStrictExecutableStages(nodes []*executableTaskNode) (map[string]int, [][]*executableTaskNode, error) {
	if len(nodes) == 0 {
		return nil, nil, workflowdag.ErrEmptyDAG
	}

	nodeByID := make(map[string]*executableTaskNode, len(nodes))
	indegree := make(map[string]int, len(nodes))
	dependents := make(map[string][]string, len(nodes))
	for _, node := range nodes {
		if node == nil || node.id == "" {
			continue
		}
		nodeByID[node.id] = node
		indegree[node.id] = len(node.deps)
	}
	for _, node := range nodes {
		if node == nil {
			continue
		}
		for _, depID := range node.deps {
			if _, ok := nodeByID[depID]; !ok {
				return nil, nil, utils.Errorf("task executable DAG contains unresolved dependency %q -> %q", node.id, depID)
			}
			dependents[depID] = append(dependents[depID], node.id)
		}
	}

	ready := make([]*executableTaskNode, 0, len(nodes))
	for _, node := range nodes {
		if node == nil || indegree[node.id] != 0 {
			continue
		}
		ready = append(ready, node)
	}
	sort.Slice(ready, func(i, j int) bool {
		return ready[i].order < ready[j].order
	})

	topoOrder := make([]*executableTaskNode, 0, len(nodes))
	for len(ready) > 0 {
		node := ready[0]
		ready = ready[1:]
		topoOrder = append(topoOrder, node)
		for _, dependentID := range dependents[node.id] {
			indegree[dependentID]--
			if indegree[dependentID] == 0 {
				ready = append(ready, nodeByID[dependentID])
				sort.Slice(ready, func(i, j int) bool {
					return ready[i].order < ready[j].order
				})
			}
		}
	}
	if len(topoOrder) != len(nodes) {
		return nil, nil, utils.Errorf("task executable DAG contains cycle")
	}

	stageByTaskIndex := make(map[string]int, len(nodes))
	maxStage := 0
	for _, node := range topoOrder {
		stage := 0
		for _, depID := range node.deps {
			if depStage := stageByTaskIndex[depID] + 1; depStage > stage {
				stage = depStage
			}
		}
		stageByTaskIndex[node.id] = stage
		if stage > maxStage {
			maxStage = stage
		}
	}

	stages := make([][]*executableTaskNode, maxStage+1)
	for _, node := range nodes {
		stage := stageByTaskIndex[node.id]
		stages[stage] = append(stages[stage], node)
	}
	for _, stage := range stages {
		sort.Slice(stage, func(i, j int) bool {
			return stage[i].order < stage[j].order
		})
	}
	return stageByTaskIndex, stages, nil
}

func (c *Coordinator) standardizeTaskTree(task *AiTask) *AiTask {
	root := findTaskTreeRoot(task)
	if root == nil {
		return nil
	}
	c.ensureTaskTreeInitialized(root)
	root.GenerateIndex()
	normalizeTaskDependencyRefs(root)
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
	graph, err := buildStrictExecutableTaskGraph(root)
	if err != nil {
		return "", err
	}
	ctx := c.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	dag := workflowdag.New[*executableTaskNode](ctx)
	for _, node := range graph.nodes {
		if node == nil {
			continue
		}
		if err := dag.AddNode(node); err != nil {
			return "", err
		}
	}
	if err := dag.Build(); err != nil {
		return "", err
	}
	options := workflowdag.DefaultMermaidOptions()
	options.Direction = workflowdag.MermaidDirectionTB
	options.ShowEdgeLabels = false
	options.Title = "可执行任务依赖图"
	options.NodeLabelFunc = func(node workflowdag.DAGNode) string {
		graphNode, ok := node.(*executableTaskNode)
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
	builder.WriteString("说明：图中仅展示会真正执行的叶子任务节点；结构性父任务不会作为执行节点进入 DAG。\n\n")
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
