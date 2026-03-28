package workflowdag

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
)

// ToolCallNode represents a tool call node in a workflow DAG
// JSON format: {"call_id": "a1", "tool_name": "name", "call_intent": "why", "depends_on": ["b"], "allow_failed": false}
type ToolCallNode struct {
	*BaseNode

	// CallID is the unique identifier for this tool call
	CallID string `json:"call_id"`

	// ToolName is the name of the tool to be called
	ToolName string `json:"tool_name"`

	// CallIntent describes why this tool call is needed
	CallIntent string `json:"call_intent"`

	// RawDependsOn stores the raw depends_on from JSON
	RawDependsOn []string `json:"depends_on"`

	// AllowFailedFlag indicates if execution can continue when dependencies fail
	// If both allow_failed and disallow_failed are present and conflict, allow_failed wins
	AllowFailedFlag *bool `json:"allow_failed,omitempty"`

	// DisallowFailedFlag is the inverse of AllowFailedFlag
	DisallowFailedFlag *bool `json:"disallow_failed,omitempty"`

	// ExecuteFunc is the function to execute when this node runs
	ExecuteFunc func(ctx context.Context, node *ToolCallNode) error `json:"-"`

	// Result stores the execution result
	Result any `json:"-"`

	// Error stores any execution error
	Error error `json:"-"`
}

// NewToolCallNode creates a new ToolCallNode
func NewToolCallNode(callID, toolName string, deps ...string) *ToolCallNode {
	node := &ToolCallNode{
		BaseNode:     NewBaseNode(callID, deps...),
		CallID:       callID,
		ToolName:     toolName,
		RawDependsOn: deps,
	}
	return node
}

// GetID returns the call_id as the node ID
func (n *ToolCallNode) GetID() string {
	return n.CallID
}

// DependsOn returns the list of node IDs this node depends on
func (n *ToolCallNode) DependsOn() []string {
	return n.RawDependsOn
}

// AllowFailed returns whether this node allows its dependencies to fail
// If both allow_failed and disallow_failed are set and conflict, allow_failed wins (more permissive)
func (n *ToolCallNode) AllowFailed() bool {
	// If allow_failed is explicitly set, use it
	if n.AllowFailedFlag != nil {
		return *n.AllowFailedFlag
	}

	// If disallow_failed is set, use its inverse
	if n.DisallowFailedFlag != nil {
		return !(*n.DisallowFailedFlag)
	}

	// Default: don't allow failed (strict mode)
	return false
}

// Execute runs the tool call
func (n *ToolCallNode) Execute(ctx context.Context) error {
	if n.ExecuteFunc == nil {
		return nil
	}
	n.SetStatus(NodeStatusProcessing)
	err := n.ExecuteFunc(ctx, n)
	if err != nil {
		n.Error = err
		n.SetStatus(NodeStatusFailed)
		return err
	}
	n.SetStatus(NodeStatusCompleted)
	return nil
}

// DisplayName returns a human-readable name for display
func (n *ToolCallNode) DisplayName() string {
	return fmt.Sprintf("%s(%s)", n.CallID, n.ToolName)
}

// ToolCallDAG is a specialized DAG for tool call workflows
type ToolCallDAG struct {
	*WorkflowDAG[*ToolCallNode]
}

// NewToolCallDAG creates a new ToolCallDAG
func NewToolCallDAG(ctx context.Context) *ToolCallDAG {
	return &ToolCallDAG{
		WorkflowDAG: New[*ToolCallNode](ctx),
	}
}

// ParseToolCallNodes parses various input formats into ToolCallNodes
// Supported formats:
//   - Single JSON object: {"call_id": "a", ...}
//   - JSON array: [{"call_id": "a", ...}, {"call_id": "b", ...}]
//   - JSON object with call_id as keys: {"a": {"tool_name": "...", ...}, "b": {...}}
//   - Line-separated JSON: {"call_id": "a", ...}\n{"call_id": "b", ...}
func ParseToolCallNodes(input any) ([]*ToolCallNode, error) {
	var rawData string

	switch v := input.(type) {
	case string:
		rawData = strings.TrimSpace(v)
	case []byte:
		rawData = strings.TrimSpace(string(v))
	default:
		// Try to marshal and re-parse
		data, err := json.Marshal(input)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal input: %w", err)
		}
		rawData = string(data)
	}

	if rawData == "" {
		return nil, fmt.Errorf("empty input")
	}

	var nodes []*ToolCallNode

	// Try parsing as JSON array first
	if strings.HasPrefix(rawData, "[") {
		var arr []json.RawMessage
		if err := json.Unmarshal([]byte(rawData), &arr); err == nil {
			for _, item := range arr {
				node, err := parseToolCallNode(item)
				if err != nil {
					log.Warnf("failed to parse tool call node: %v", err)
					continue
				}
				nodes = append(nodes, node)
			}
			if len(nodes) > 0 {
				return nodes, nil
			}
		}
	}

	// Try parsing as JSON object (single node or map)
	if strings.HasPrefix(rawData, "{") {
		// First try as single node
		node, err := parseToolCallNode([]byte(rawData))
		if err == nil && node.CallID != "" {
			return []*ToolCallNode{node}, nil
		}

		// Try as map with call_id as keys
		var nodeMap map[string]json.RawMessage
		if err := json.Unmarshal([]byte(rawData), &nodeMap); err == nil {
			for callID, nodeData := range nodeMap {
				node, err := parseToolCallNodeWithID(callID, nodeData)
				if err != nil {
					log.Warnf("failed to parse tool call node %s: %v", callID, err)
					continue
				}
				nodes = append(nodes, node)
			}
			if len(nodes) > 0 {
				return nodes, nil
			}
		}
	}

	// Try line-separated JSON
	lines := strings.Split(rawData, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		node, err := parseToolCallNode([]byte(line))
		if err != nil {
			log.Warnf("failed to parse tool call node from line: %v", err)
			continue
		}
		nodes = append(nodes, node)
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("no valid tool call nodes found in input")
	}

	return nodes, nil
}

// parseToolCallNode parses a single JSON node
func parseToolCallNode(data []byte) (*ToolCallNode, error) {
	node := &ToolCallNode{
		BaseNode: NewBaseNode("", nil...),
	}
	if err := json.Unmarshal(data, node); err != nil {
		return nil, err
	}

	// Validate required fields
	if node.CallID == "" {
		return nil, fmt.Errorf("call_id is required")
	}

	// Update BaseNode
	node.BaseNode.ID = node.CallID
	node.BaseNode.Dependencies = node.RawDependsOn
	node.BaseNode.AllowFail = node.AllowFailed()

	return node, nil
}

// parseToolCallNodeWithID parses a node where call_id is provided separately
func parseToolCallNodeWithID(callID string, data []byte) (*ToolCallNode, error) {
	node := &ToolCallNode{
		BaseNode: NewBaseNode(callID),
		CallID:   callID,
	}
	if err := json.Unmarshal(data, node); err != nil {
		return nil, err
	}

	// Use provided callID if node doesn't have one
	if node.CallID == "" {
		node.CallID = callID
	}

	// Update BaseNode
	node.BaseNode.ID = node.CallID
	node.BaseNode.Dependencies = node.RawDependsOn
	node.BaseNode.AllowFail = node.AllowFailed()

	return node, nil
}

// BuildToolCallDAG creates a ToolCallDAG from various input formats
func BuildToolCallDAG(ctx context.Context, input any) (*ToolCallDAG, error) {
	nodes, err := ParseToolCallNodes(input)
	if err != nil {
		return nil, err
	}

	dag := NewToolCallDAG(ctx)
	for _, node := range nodes {
		if err := dag.AddNode(node); err != nil {
			return nil, fmt.Errorf("failed to add node %s: %w", node.CallID, err)
		}
	}

	if err := dag.Build(); err != nil {
		return nil, fmt.Errorf("failed to build DAG: %w", err)
	}

	return dag, nil
}

// ExecuteWithHandler executes all nodes in the DAG using the provided handler
func (dag *ToolCallDAG) ExecuteWithHandler(handler func(ctx context.Context, node *ToolCallNode) error) error {
	entries, err := dag.Entries()
	if err != nil {
		return err
	}

	for chain := range entries {
		if err := chain.Execute(func(node *ToolCallNode) error {
			node.SetStatus(NodeStatusProcessing)
			if err := handler(dag.ctx, node); err != nil {
				node.Error = err
				node.SetStatus(NodeStatusFailed)
				if !node.AllowFailed() {
					return err
				}
				return nil
			}
			node.SetStatus(NodeStatusCompleted)
			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}

type toolCallExecutionResult struct {
	nodeID string
	err    error
}

// ExecuteWithHandlerConcurrent executes ready nodes concurrently while preserving DAG dependencies.
// A node becomes runnable only after all of its dependencies reach a terminal state.
// If the node does not allow failed dependencies and any dependency failed/skipped, it is marked skipped.
func (dag *ToolCallDAG) ExecuteWithHandlerConcurrent(concurrency int, handler func(ctx context.Context, node *ToolCallNode) error) error {
	if concurrency <= 0 {
		concurrency = 1
	}

	nodes := dag.GetAllNodes()
	if len(nodes) == 0 {
		return nil
	}

	ctx := dag.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type schedulerState struct {
		remainingDeps map[string]int
		queued        map[string]bool
		terminal      map[string]bool
	}

	state := &schedulerState{
		remainingDeps: make(map[string]int, len(nodes)),
		queued:        make(map[string]bool, len(nodes)),
		terminal:      make(map[string]bool, len(nodes)),
	}

	nodeMap := make(map[string]*ToolCallNode, len(nodes))
	ready := make(chan *ToolCallNode, len(nodes))
	results := make(chan toolCallExecutionResult, len(nodes))

	for _, node := range nodes {
		nodeMap[node.CallID] = node
	}

	for _, node := range nodes {
		deps := 0
		for _, depID := range node.DependsOn() {
			if _, ok := nodeMap[depID]; ok {
				deps++
			}
		}
		state.remainingDeps[node.CallID] = deps
	}

	var workers sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for node := range ready {
				if node == nil {
					continue
				}
				node.SetStatus(NodeStatusProcessing)
				err := handler(ctx, node)
				if err != nil {
					node.Error = err
					node.SetStatus(NodeStatusFailed)
				} else {
					node.SetStatus(NodeStatusCompleted)
				}
				results <- toolCallExecutionResult{nodeID: node.CallID, err: err}
			}
		}()
	}

	markQueued := func(node *ToolCallNode) {
		if node == nil {
			return
		}
		if state.queued[node.CallID] || state.terminal[node.CallID] {
			return
		}
		state.queued[node.CallID] = true
		ready <- node
	}

	markSkipped := func(startID string) {
		queue := []string{startID}
		for len(queue) > 0 {
			nodeID := queue[0]
			queue = queue[1:]

			if state.terminal[nodeID] {
				continue
			}
			node := nodeMap[nodeID]
			if node == nil {
				continue
			}
			state.terminal[nodeID] = true
			node.SetStatus(NodeStatusSkipped)

			for _, dependentID := range dag.GetDependents(nodeID) {
				dependent := nodeMap[dependentID]
				if dependent == nil || state.terminal[dependentID] || state.queued[dependentID] {
					continue
				}
				state.remainingDeps[dependentID]--
				if state.remainingDeps[dependentID] > 0 {
					continue
				}
				if dependent.AllowFailed() {
					markQueued(dependent)
					continue
				}
				queue = append(queue, dependentID)
			}
		}
	}

	seeded := 0
	for _, node := range nodes {
		if state.remainingDeps[node.CallID] == 0 {
			markQueued(node)
			seeded++
		}
	}

	// Break cycle-only components the same way the sequential executor does: pick one entry per component.
	if seeded == 0 {
		entries, err := dag.GetEntries()
		if err != nil {
			close(ready)
			workers.Wait()
			close(results)
			return err
		}
		for _, entry := range entries {
			markQueued(entry)
		}
	}

	inFlight := 0
	for _, queued := range state.queued {
		if queued {
			inFlight++
		}
	}
	completed := len(state.terminal)
	var finalErr error

	for completed < len(nodes) {
		if inFlight == 0 {
			break
		}

		result := <-results
		inFlight--
		state.terminal[result.nodeID] = true
		completed = len(state.terminal)

		node := nodeMap[result.nodeID]
		if node == nil {
			continue
		}

		if result.err != nil && !node.AllowFailed() && finalErr == nil {
			finalErr = result.err
		}

		for _, dependentID := range dag.GetDependents(result.nodeID) {
			dependent := nodeMap[dependentID]
			if dependent == nil || state.terminal[dependentID] || state.queued[dependentID] {
				continue
			}

			state.remainingDeps[dependentID]--
			if state.remainingDeps[dependentID] > 0 {
				continue
			}

			dependencyFailed := false
			for _, depID := range dependent.DependsOn() {
				depNode := nodeMap[depID]
				if depNode == nil {
					continue
				}
				if depNode.IsFailed() || depNode.IsSkipped() {
					dependencyFailed = true
					break
				}
			}

			if dependencyFailed && !dependent.AllowFailed() {
				markSkipped(dependentID)
				completed = len(state.terminal)
				continue
			}

			markQueued(dependent)
			inFlight++
		}

		// If the remaining pending nodes are only cycles, release one component entry at a time.
		if inFlight == 0 && completed < len(nodes) {
			for _, entry := range dag.entries {
				if entry == nil || state.terminal[entry.CallID] || state.queued[entry.CallID] {
					continue
				}
				markQueued(entry)
				inFlight++
			}
		}
	}

	close(ready)
	workers.Wait()
	close(results)

	return finalErr
}

// GetDOT generates a DOT graph representation
// Node names are formatted as: call_id(tool_name)
func (dag *ToolCallDAG) GetDOT() string {
	var sb strings.Builder
	sb.WriteString("digraph ToolCallDAG {\n")
	sb.WriteString("  rankdir=TB;\n")
	sb.WriteString("  node [shape=box, style=rounded];\n\n")

	nodes := dag.GetAllNodes()

	// Write nodes with status-based styling
	for _, node := range nodes {
		label := escapeQuotes(node.DisplayName())
		style := getNodeDOTStyle(node.GetStatus())
		sb.WriteString(fmt.Sprintf("  \"%s\" [label=\"%s\"%s];\n", node.CallID, label, style))
	}

	sb.WriteString("\n")

	// Write edges (dependency direction: depends_on means edge from dependency to dependent)
	for _, node := range nodes {
		for _, depID := range node.DependsOn() {
			// Edge from dependency to this node (execution order)
			sb.WriteString(fmt.Sprintf("  \"%s\" -> \"%s\";\n", depID, node.CallID))
		}
	}

	sb.WriteString("}\n")
	return sb.String()
}

// getNodeDOTStyle returns DOT styling based on node status
func getNodeDOTStyle(status NodeStatus) string {
	switch status {
	case NodeStatusPending:
		return ", fillcolor=white, style=\"rounded,filled\""
	case NodeStatusProcessing:
		return ", fillcolor=yellow, style=\"rounded,filled\""
	case NodeStatusCompleted:
		return ", fillcolor=lightgreen, style=\"rounded,filled\""
	case NodeStatusFailed:
		return ", fillcolor=lightcoral, style=\"rounded,filled\""
	case NodeStatusSkipped:
		return ", fillcolor=lightgray, style=\"rounded,filled\""
	default:
		return ""
	}
}

// escapeQuotes escapes double quotes for DOT labels
func escapeQuotes(s string) string {
	return strings.ReplaceAll(s, "\"", "\\\"")
}

// GraphNode represents a node in the graph JSON output
type GraphNode struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	ToolName   string   `json:"tool_name"`
	CallIntent string   `json:"call_intent"`
	Status     string   `json:"status"`
	Stage      int      `json:"stage"`
	Category   int      `json:"category"` // for echarts node coloring
	DependsOn  []string `json:"depends_on"`
	Error      string   `json:"error,omitempty"`
}

// GraphEdge represents an edge in the graph JSON output
type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// GraphJSON represents the complete graph in JSON format for echarts
type GraphJSON struct {
	Nodes      []GraphNode     `json:"nodes"`
	Edges      []GraphEdge     `json:"edges"`
	Categories []GraphCategory `json:"categories"`
}

// GraphCategory represents a category for echarts styling
type GraphCategory struct {
	Name string `json:"name"`
}

// GetGraphJSON returns a JSON representation suitable for echarts visualization
func (dag *ToolCallDAG) GetGraphJSON() *GraphJSON {
	nodes := dag.GetAllNodes()

	graph := &GraphJSON{
		Nodes: make([]GraphNode, 0, len(nodes)),
		Edges: make([]GraphEdge, 0),
		Categories: []GraphCategory{
			{Name: "pending"},
			{Name: "processing"},
			{Name: "completed"},
			{Name: "failed"},
			{Name: "skipped"},
		},
	}

	for _, node := range nodes {
		stage, _ := dag.GetStage(node.CallID)

		graphNode := GraphNode{
			ID:         node.CallID,
			Name:       node.DisplayName(),
			ToolName:   node.ToolName,
			CallIntent: node.CallIntent,
			Status:     node.GetStatus().String(),
			Stage:      stage,
			Category:   int(node.GetStatus()),
			DependsOn:  node.DependsOn(),
		}

		if node.Error != nil {
			graphNode.Error = node.Error.Error()
		}

		graph.Nodes = append(graph.Nodes, graphNode)

		// Add edges
		for _, depID := range node.DependsOn() {
			graph.Edges = append(graph.Edges, GraphEdge{
				Source: depID,
				Target: node.CallID,
			})
		}
	}

	return graph
}

// GetGraphJSONString returns the JSON string representation
func (dag *ToolCallDAG) GetGraphJSONString() string {
	graph := dag.GetGraphJSON()
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

// GetNodeByCallID returns a node by its call_id
func (dag *ToolCallDAG) GetNodeByCallID(callID string) (*ToolCallNode, bool) {
	return dag.GetNode(callID)
}

// SetExecuteFunc sets the execute function for a specific node
func (dag *ToolCallDAG) SetExecuteFunc(callID string, fn func(ctx context.Context, node *ToolCallNode) error) error {
	node, ok := dag.GetNode(callID)
	if !ok {
		return fmt.Errorf("node %s not found", callID)
	}
	node.ExecuteFunc = fn
	return nil
}

// SetGlobalExecuteFunc sets the execute function for all nodes
func (dag *ToolCallDAG) SetGlobalExecuteFunc(fn func(ctx context.Context, node *ToolCallNode) error) {
	for _, node := range dag.GetAllNodes() {
		node.ExecuteFunc = fn
	}
}

// MarshalJSON implements json.Marshaler for ToolCallNode
func (n *ToolCallNode) MarshalJSON() ([]byte, error) {
	type Alias ToolCallNode

	var errStr string
	if n.Error != nil {
		errStr = n.Error.Error()
	}

	return json.Marshal(&struct {
		Status string `json:"status"`
		Error  string `json:"error,omitempty"`
		*Alias
	}{
		Status: n.GetStatus().String(),
		Error:  errStr,
		Alias:  (*Alias)(n),
	})
}
