package ssaapi

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dot"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// FunctionCFG 表示函数的控制流图
type FunctionCFG struct {
	*dot.Graph

	Block2Node map[*ssa.BasicBlock]int // ssa.BasicBlock -> node-id
	Node2Block map[int]*ssa.BasicBlock // node-id -> ssa.BasicBlock
}

// NewFunctionCFG 创建一个新的函数控制流图
func NewFunctionCFG(function *ssa.Function) *FunctionCFG {
	if function == nil {
		return nil
	}

	graphGraph := dot.New()
	graphGraph.MakeDirected()
	graphGraph.GraphAttribute("rankdir", "TB")
	graphGraph.GraphAttribute("fontname", "Courier New")
	graphGraph.DefaultNodeAttribute("shape", "box")
	graphGraph.DefaultNodeAttribute("fontname", "Courier New")
	graphGraph.DefaultEdgeAttribute("fontname", "Courier New")

	g := &FunctionCFG{
		Graph:      graphGraph,
		Block2Node: make(map[*ssa.BasicBlock]int),
		Node2Block: make(map[int]*ssa.BasicBlock),
	}

	// 首先添加所有基本块节点
	for _, blockInst := range function.Blocks {
		block, ok := function.GetBasicBlockByID(blockInst)
		if !ok || block == nil {
			continue
		}
		g.addBlockNode(block)
	}

	// 然后添加边
	for _, blockInst := range function.Blocks {
		block, ok := function.GetBasicBlockByID(blockInst)
		if !ok || block == nil {
			continue
		}
		fromNodeId, exists := g.Block2Node[block]
		if !exists {
			continue
		}

		// 添加后继边
		for _, succInst := range block.Succs {
			succ, ok := function.GetBasicBlockByID(succInst)
			if !ok || succ == nil {
				continue
			}
			toNodeId, exists := g.Block2Node[succ]
			if !exists {
				continue
			}

			// 检查是否有条件边
			edgeLabel := ""
			g.AddEdge(fromNodeId, toNodeId, edgeLabel)
		}
	}

	return g
}

// addBlockNode 添加一个基本块节点到图中
func (g *FunctionCFG) addBlockNode(block *ssa.BasicBlock) int {
	if nodeId, exists := g.Block2Node[block]; exists {
		return nodeId
	}

	// 生成节点标签（包含基本块名称和指令）
	label := fmt.Sprintf("** %s ** \n", block.GetName())

	// 添加块内指令到标签
	for _, inst := range block.Insts {
		inst, ok := block.GetInstructionById(inst)
		if !ok || utils.IsNil(inst) {
			log.Infof("bbbbbbb")
		}
		instStr := inst.String()
		_, isConstIns := inst.(*ssa.ConstInst)
		if instStr != "" && !isConstIns {
			label += fmt.Sprintf("%s \n", instStr)
		}
	}

	// 添加节点
	nodeId := g.AddNode(label)
	g.Block2Node[block] = nodeId
	g.Node2Block[nodeId] = block

	// 为特殊块设置样式
	switch {
	case strings.HasPrefix(block.GetName(), "entry"):
		g.NodeAttribute(nodeId, "style", "filled")
		g.NodeAttribute(nodeId, "fillcolor", "lightblue")
	case strings.HasPrefix(block.GetName(), "exit") || strings.HasPrefix(block.GetName(), "return") || strings.Contains(block.GetName(), "done"):
		g.NodeAttribute(nodeId, "style", "filled")
		g.NodeAttribute(nodeId, "fillcolor", "lightgreen")
	}

	return nodeId
}

// Dot 生成DOT格式的控制流图
func (g *FunctionCFG) Dot() string {
	var buf bytes.Buffer
	g.GenerateDOT(&buf)
	return buf.String()
}

// ShowDot 打印DOT格式的控制流图
func (g *FunctionCFG) ShowDot() {
	log.Infof(g.Dot())
}

// FunctionDotGraph 为指定函数生成DOT格式的控制流图
func FunctionDotGraph(function *ssa.Function) string {
	cfg := NewFunctionCFG(function)
	if cfg == nil {
		return ""
	}
	return cfg.Dot()
}

// ShowFunctionDot 打印指定函数的DOT格式控制流图
func ShowFunctionDot(function *ssa.Function) {
	cfg := NewFunctionCFG(function)
	if cfg != nil {
		cfg.ShowDot()
	}
}
