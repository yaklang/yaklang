package sfvm

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type VarFlowGraph struct {
	Nodes         *omap.OrderedMap[int, *VarFlowNode]  // nodeId -> node
	Steps         *omap.OrderedMap[int, *AnalysisStep] // stepId -> step
	Edges         []*VarFlowEdge
	nodeIdCounter int
	stepIdCounter int
	*varFlowState
}

type VarFlowNode struct {
	NodeId       int
	VariableName string
	Values       ValueOperator
}

type AnalysisStep struct {
	StepId         int
	StepType       AnalysisStepType
	SFI            *SFI
	EvidenceAttach *EvidenceAttach
}

type VarFlowEdge struct {
	EdgeId     int
	FromNodeId int
	ToNodeId   int
	Steps      []int
}

type AnalysisStepType int

const (
	AnalysisStepTypeNormal AnalysisStepType = iota
	AnalysisStepTypeDataFlow
	AnalysisStepTypeSearch
	AnalysisStepTypeFilter
	AnalysisStepTypeGet
)

func (t AnalysisStepType) String() string {
	switch t {
	case AnalysisStepTypeNormal:
		return "Normal"
	case AnalysisStepTypeDataFlow:
		return "DataFlow"
	case AnalysisStepTypeSearch:
		return "Search"
	case AnalysisStepTypeFilter:
		return "Filter"
	case AnalysisStepTypeGet:
		return "Get"
	default:
		return "Unknown"
	}
}

type varFlowState struct {
	sourceNodeId      int
	steps             []int
	currentStepId     int
	evidenceStack     *utils.Stack[*EvidenceNode]
	conditionStackLen int
}

func (s *varFlowState) pushConditionStack() {
	if s == nil {
		return
	}
	s.conditionStackLen++
}

func (s *varFlowState) popConditionStack() {
	if s == nil {
		return
	}
	s.conditionStackLen--
}

func (s *varFlowState) isInConditionStack() bool {
	if s == nil {
		return false
	}
	return s.conditionStackLen > 0
}

func (s *varFlowState) appendStep(stepId int) {
	if s == nil {
		return
	}
	s.steps = append(s.steps, stepId)
	s.currentStepId = stepId
}

func (s *varFlowState) flush() {
	if s == nil {
		return
	}
	s.steps = make([]int, 0)
	s.currentStepId = 0
	s.conditionStackLen = 0
	s.evidenceStack.Free()
}

func (s *varFlowState) hasSource() bool {
	return s.sourceNodeId != 0
}

func (s *varFlowState) hasAnalysisStep() bool {
	return len(s.steps) > 0
}

func (s *varFlowState) pushEvidenceNode(node *EvidenceNode) {
	if s == nil {
		return
	}
	if s.evidenceStack == nil {
		s.evidenceStack = utils.NewStack[*EvidenceNode]()
	}
	s.evidenceStack.Push(node)
}

func (s *varFlowState) popEvidenceNode() *EvidenceNode {
	if s == nil || s.evidenceStack == nil {
		return nil
	}
	return s.evidenceStack.Pop()
}

func newVarFlowState() *varFlowState {
	return &varFlowState{
		steps:         make([]int, 0),
		evidenceStack: utils.NewStack[*EvidenceNode](),
	}
}

func NewVarFlowGraph() *VarFlowGraph {
	return &VarFlowGraph{
		Edges:        make([]*VarFlowEdge, 0),
		Nodes:        omap.NewEmptyOrderedMap[int, *VarFlowNode](),
		Steps:        omap.NewEmptyOrderedMap[int, *AnalysisStep](),
		varFlowState: newVarFlowState(),
	}
}

func (g *VarFlowGraph) EnterCondition() {
	if g == nil {
		return
	}
	g.pushConditionStack()
}

func (g *VarFlowGraph) ExitConditionWithFilter(sfi *SFI) {
	if g == nil {
		return
	}
	g.popConditionStack()
	rootNode := g.popEvidenceNode()
	g.CreateStep(AnalysisStepTypeFilter, sfi, WithEvidenceTree(rootNode))
}

func (g *VarFlowGraph) PushFilterCondition(sfi *SFI, passed, failed ValueOperator) {
	if g == nil {
		return
	}
	node := NewEvidenceLeafNode(EvidenceTypeFilterCondition, sfi)
	node.Passed = passed
	node.Failed = failed
	g.pushEvidenceNode(node)
}

func (g *VarFlowGraph) PushStringCondition(sfi *SFI, passed, failed ValueOperator) {
	if g == nil {
		return
	}
	node := NewEvidenceLeafNode(EvidenceTypeStringCondition, sfi)
	node.Passed = passed
	node.Failed = failed
	g.pushEvidenceNode(node)
}

func (g *VarFlowGraph) PushOpcodeCondition(sfi *SFI, passed, failed ValueOperator) {
	if g == nil {
		return
	}
	node := NewEvidenceLeafNode(EvidenceTypeOpcodeCondition, sfi)
	node.Passed = passed
	node.Failed = failed
	g.pushEvidenceNode(node)
}

func (g *VarFlowGraph) PushLogicAnd() {
	if g == nil {
		return
	}
	right := g.popEvidenceNode()
	left := g.popEvidenceNode()
	node := NewEvidenceLogicNode(ConditionTypeAnd, left, right)
	g.pushEvidenceNode(node)
}

func (g *VarFlowGraph) PushLogicOr() {
	if g == nil {
		return
	}
	right := g.popEvidenceNode()
	left := g.popEvidenceNode()
	node := NewEvidenceLogicNode(ConditionTypeOr, left, right)
	g.pushEvidenceNode(node)
}

func (g *VarFlowGraph) PushLogicNot() {
	if g == nil {
		return
	}
	child := g.popEvidenceNode()
	node := NewEvidenceLogicNode(ConditionTypeNot, child)
	g.pushEvidenceNode(node)
}

func (g *VarFlowGraph) nextNodeId() int {
	g.nodeIdCounter++
	return g.nodeIdCounter
}

func (g *VarFlowGraph) nextStepId() int {
	g.stepIdCounter++
	return g.stepIdCounter
}

func (g *VarFlowGraph) StartFlow(sourceVar string) {
	nodeId := g.CreateNode(sourceVar)
	g.varFlowState.sourceNodeId = nodeId
}

func (g *VarFlowGraph) CreateNode(name string) int {
	nodeId := g.nextNodeId()
	node := &VarFlowNode{
		NodeId:       nodeId,
		VariableName: name,
	}
	g.Nodes.Set(nodeId, node)
	return nodeId
}

func (g *VarFlowGraph) CreateStep(stepType AnalysisStepType, sfi *SFI, opts ...EvidenceAttachOption) {
	// 如果在condition filter语句里面，就不创建边
	if g.isInConditionStack() {
		return
	}
	stepId := g.nextStepId()
	step := &AnalysisStep{
		StepId:   stepId,
		StepType: stepType,
		SFI:      sfi,
	}
	if len(opts) > 0 {
		step.EvidenceAttach = NewEvidenceAttach(opts...)
	}
	g.Steps.Set(stepId, step)
	g.appendStep(stepId)
	return
}

func (g *VarFlowGraph) CreateEdge(fromNodeId, toNodeId int) *VarFlowEdge {
	edge := &VarFlowEdge{
		EdgeId:     len(g.Edges) + 1,
		FromNodeId: fromNodeId,
		ToNodeId:   toNodeId,
		Steps:      append([]int{}, g.varFlowState.steps...),
	}
	return edge
}

func (g *VarFlowGraph) CommitFlow(variableName string) error {
	defer g.varFlowState.flush()

	if !g.varFlowState.hasAnalysisStep() {
		if !g.varFlowState.hasSource() {
			return utils.Errorf("no source and no analysis step")
		}
		return utils.Errorf("no analysis step")
	}

	targetNodeId := g.CreateNode(variableName)
	sourceNodeId := g.varFlowState.sourceNodeId
	edge := g.CreateEdge(sourceNodeId, targetNodeId)
	g.Edges = append(g.Edges, edge)
	return nil
}

func (g *VarFlowGraph) GetStep(stepId int) (*AnalysisStep, bool) {
	return g.Steps.Get(stepId)
}

func (g *VarFlowGraph) GetNode(nodeId int) (*VarFlowNode, bool) {
	return g.Nodes.Get(nodeId)
}

func (g *VarFlowGraph) AttachEvidenceToStep(stepId int, opts ...EvidenceAttachOption) {
	step, ok := g.Steps.Get(stepId)
	if !ok {
		return
	}
	if step.EvidenceAttach == nil {
		step.EvidenceAttach = NewEvidenceAttach(opts...)
	} else {
		for _, opt := range opts {
			opt(step.EvidenceAttach)
		}
	}
}

func (g *VarFlowGraph) AttachEvidenceToCurrentStep(opts ...EvidenceAttachOption) {
	if g.varFlowState == nil || g.varFlowState.currentStepId == 0 {
		return
	}
	g.AttachEvidenceToStep(g.varFlowState.currentStepId, opts...)
}

func (g *VarFlowGraph) AttachValuesToNode(nodeId int, values ValueOperator) {
	node, ok := g.Nodes.Get(nodeId)
	if !ok {
		return
	}
	node.Values = values
}

func (g *VarFlowGraph) String() string {
	if g == nil {
		return "<nil graph>"
	}

	var result strings.Builder
	result.WriteString("VarFlowGraph:\n")
	result.WriteString("  Nodes:\n")
	g.Nodes.ForEach(func(key int, node *VarFlowNode) bool {
		result.WriteString("    " + node.String() + "\n")
		return true
	})
	result.WriteString("  Edges:\n")
	for _, edge := range g.Edges {
		result.WriteString("    " + edge.String() + "\n")
	}

	result.WriteString("  Steps:\n")
	g.Steps.ForEach(func(key int, step *AnalysisStep) bool {
		result.WriteString(fmt.Sprintf("    Step#%d [%s]\n", step.StepId, step.StepType))
		if step.EvidenceAttach != nil {
			evidenceStr := step.EvidenceAttach.String()
			indented := strings.ReplaceAll(evidenceStr, "\n", "\n      ")
			result.WriteString("      " + indented + "\n")
		}
		return true
	})
	return result.String()
}

func (n *VarFlowNode) String() string {
	if n == nil {
		return "<nil node>"
	}
	return fmt.Sprintf("[Node#%d] Var:%s", n.NodeId, n.VariableName)
}

func (e *VarFlowEdge) String() string {
	if e == nil {
		return "<nil edge>"
	}
	edgeId := fmt.Sprintf("Edge#%d", e.EdgeId)

	// Helper to print step flow
	var stringsBuilder strings.Builder
	for i, stepId := range e.Steps {
		// 为了演示简单，这里只打印 ID。实际可以用 g.Steps 查找详细信息
		stringsBuilder.WriteString(fmt.Sprintf("Step#%d", stepId))
		if i < len(e.Steps)-1 {
			stringsBuilder.WriteString(" -> ")
		}
	}

	if e.FromNodeId == 0 {
		return fmt.Sprintf("[%s] [Search] -> $%d steps: [%s]", edgeId, e.ToNodeId, stringsBuilder.String())
	}
	return fmt.Sprintf("[%s] $%d -> $%d steps: [%s]", edgeId, e.FromNodeId, e.ToNodeId, stringsBuilder.String())
}
