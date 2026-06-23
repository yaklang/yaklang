package rewriter

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	utils3 "github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/utils"
	utils2 "github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

type LoopStatement struct {
	Condition values.JavaValue
	BodyStart *core.Node
}

func RebuildLoopNode(manager *RewriteManager) error {
	for _, node := range manager.CircleEntryPoint {
		doWhileSt := statements.NewDoWhileStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), nil)
		doWhileNode := manager.NewNode(doWhileSt)
		// Redirect every edge `source -> circleNode` to `source -> doWhileNode` while preserving the
		// edge's index in source.Next. The previous remove-all-source + AddSource approach appended
		// the redirected edge to the end of source.Next; for a bottom-tested loop the loop-condition
		// node is itself a back-edge source, so its branch to circleNode was shoved to index 1,
		// swapping the if's two successors and inverting the reconstructed do-while condition (break
		// and continue landed on the wrong branch). In-place replacement keeps branch polarity.
		for _, n := range slices.Clone(node.Source) {
			replaceNextInPlace(n, node, doWhileNode)
		}
		doWhileNode.AddNext(node)
		manager.WhileNode = append(manager.WhileNode, doWhileNode)
	}
	return nil
}
// replaceNextInPlace rewires the edge node->oldNext to node->newNext while keeping the edge at
// its original index inside node.Next. Position matters: a ConditionStatement's TrueNode()/
// FalseNode() are bound to fixed Next indices computed during graph construction, so appending a
// freshly created break/continue node (the old RemoveNext + AddNext pair) would shift it to the
// other branch and silently invert the loop condition - body and exit swap, producing
// "if (i < n) break; else { body }" instead of "if (i < n) { body } else break;". Replacing in
// place preserves the branch polarity so the reconstructed loop keeps its original semantics.
func replaceNextInPlace(node, oldNext, newNext *core.Node) {
	idx := slices.Index(node.Next, oldNext)
	node.RemoveNext(oldNext)
	if idx < 0 || idx > len(node.Next) {
		node.AddNext(newNext)
		return
	}
	node.Next = slices.Insert(node.Next, idx, newNext)
	// newNext is already spliced into node.Next; AddNext only fixes the reverse Source link here.
	node.AddNext(newNext)
}

func LoopJmpRewriter(manager *RewriteManager, circleNode *core.Node) error {
	loopEnd := searchCircleEndNode(circleNode, circleNode.Next[0])
	preWhileNodes := utils.NodeFilter(manager.WhileNode, func(node *core.Node) bool {
		return utils.IsDominate(manager.DominatorMap, node, circleNode)
	})
	preWhileNodeEnds := map[*core.Node]*core.Node{}
	for _, n := range preWhileNodes {
		preWhileNodeEnds[n] = searchCircleEndNode(n, n.Next[0])
	}
	checkNode := func(node *core.Node) ([]*core.Node, error) {
		if node.IsJmp {
			return nil, nil
		}
		if _, ok := node.Statement.(*statements.IfStatement); ok {
			return nil, nil
		}
		if !utils.IsDominate(manager.DominatorMap, circleNode, node) {
			return nil, nil
		}
		nextList := []*core.Node{}
		allNext := slices.Clone(node.Next)
		for _, next := range allNext {
			if next == circleNode {
				continueNode := manager.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
					return "continue"
				}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
				}))
				continueNode.IsJmp = true
				replaceNextInPlace(node, next, continueNode)
				continueNode.AddNext(next)
				continue
			}

			if false && !utils.IsDominate(manager.DominatorMap, node, next) && node != circleNode {
				if node != circleNode {
					breakNode := manager.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
						return "break"
					}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
					}))
					breakNode.HideNext = next
					breakNode.IsJmp = true
					node.RemoveNext(next)
					node.AddNext(breakNode)
					breakNode.AddNext(circleNode)
					circleNode.AddNext(next)
					continue
				}
				breakNode := manager.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
					return "break"
				}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
				}))
				breakNode.HideNext = next
				breakNode.IsJmp = true

				matched := utils.NodeFilter(manager.WhileNode, func(node *core.Node) bool {
					return node == next
				})
				if len(matched) > 0 {
					if utils.IsDominate(manager.DominatorMap, matched[0], circleNode) {
						loopNode := matched[0].Statement.(*statements.DoWhileStatement)
						if loopNode.Label == "" {
							label := manager.NewLoopLabel()
							loopNode.Label = label
						}
						breakNode.Statement = statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
							return "continue " + loopNode.Label
						}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
						})
					}
					//} else {
					//	return nil, errors.New("loop end node conflict")
					//}
				} else {
					//var ok bool
					for _, n := range manager.WhileNode {
						if loopEnd == next && utils.IsDominate(manager.DominatorMap, n, circleNode) {
							loopNode := n.Statement.(*statements.DoWhileStatement)
							if loopNode.Label == "" {
								label := manager.NewLoopLabel()
								loopNode.Label = label
							}
							breakNode.Statement = statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
								return "break " + loopNode.Label
							}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
							})
							//ok = true
							break
						}
					}
					//if !ok {
					//	return nil, errors.New("loop end node conflict")
					//}
				}

				node.RemoveNext(next)
				node.AddNext(breakNode)
				breakNode.AddNext(next)
				continue
			}
			if loopEnd != nil && (next == loopEnd && node != circleNode) {
				breakNode := manager.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
					return "break"
				}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
				}))
				replaceNextInPlace(node, next, breakNode)
				breakNode.AddNext(circleNode)
				circleNode.AddNext(next)
				breakNode.IsJmp = true
				continue
			}
			if node != circleNode {
				matched := utils.NodeFilter(manager.WhileNode, func(node *core.Node) bool {
					return node == next
				})
				if len(matched) > 0 {
					if utils.IsDominate(manager.DominatorMap, matched[0], circleNode) {
						loopNode := matched[0].Statement.(*statements.DoWhileStatement)
						if loopNode.Label == "" {
							label := manager.NewLoopLabel()
							loopNode.Label = label
						}
						breakNode := manager.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
							return "break"
						}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
						}))
						breakNode.Statement = statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
							return "continue " + loopNode.Label
						}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
						})
						breakNode.IsJmp = true
						replaceNextInPlace(node, next, breakNode)
						breakNode.AddNext(matched[0])
					}
					//} else {
					//	return nil, errors.New("loop end node conflict")
					//}
				} else {
					//var ok bool
					for _, n := range manager.WhileNode {
						if len(n.Next) < 2 {
							continue
						}
						endNode := preWhileNodeEnds[n]
						if endNode == next {
							loopNode := n.Statement.(*statements.DoWhileStatement)
							if loopNode.Label == "" {
								label := manager.NewLoopLabel()
								loopNode.Label = label
							}
							breakNode := manager.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
								return "break"
							}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
							}))
							breakNode.Statement = statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
								return "break " + loopNode.Label
							}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
							})
							breakNode.IsJmp = true
							replaceNextInPlace(node, next, breakNode)
							breakNode.AddNext(endNode)
							break
						}
					}
					//if !ok {
					//	return nil, errors.New("loop end node conflict")
					//}
				}
			}
			nextList = append(nextList, next)
		}
		return nextList, nil
	}
	times := 0
	err := core.WalkGraph[*core.Node](circleNode.Next[0], func(node *core.Node) ([]*core.Node, error) {
		times++
		//return node.Next, nil
		return checkNode(node)
	})
	if err != nil {
		return err
	}
	_, err = checkNode(circleNode)
	if err != nil {
		return err
	}
	return nil
}
func LoopRewriter(manager *RewriteManager, node *core.Node) error {
	circleNode := node
	loopStart := circleNode.Next[0]
	circleNode.RemoveNext(loopStart)

	body := []statements.Statement{}
	endNodes := []*core.Node{}
	circleSet := getCircleElementSet(circleNode, loopStart)
	err := core.WalkGraph[*core.Node](loopStart, func(node *core.Node) ([]*core.Node, error) {
		if !circleSet.Has(node) {
			endNodes = append(endNodes, node)
			return nil, nil
		}
		err := manager.CheckVisitedNode(node)
		if err != nil {
			return nil, err
		}
		body = append(body, node.Statement)
		var next []*core.Node
		for _, n := range node.Next {
			if slices.Contains(manager.DominatorMap[node], n) {
				next = append(next, n)
			} else {
				if n != circleNode {
					endNodes = append(endNodes, n)
				}
			}
		}
		return next, nil
	})
	if err != nil {
		return err
	}
	doWhileSt := statements.NewDoWhileStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), nil)
	doWhileSt.Label = circleNode.Statement.(*statements.DoWhileStatement).Label
	doWhileSt.Body = append(doWhileSt.Body, body...)
	//allSource := slices.Clone(node.Source)
	//node.RemoveAllSource()
	//for _, n := range allSource {
	//	n.AddNext(manager.NewNode(doWhileSt))
	//}
	loopNode := manager.NewNode(doWhileSt)
	circleNode.Replace(loopNode)
	endNodes = lo.Filter(endNodes, func(item *core.Node, index int) bool {
		return !IsEndNode(item)
	})
	for _, c := range NodeDeduplication(endNodes) {
		loopNode.AddNext(c)
	}
	return nil
}
func getCircleElementSet(circleNode *core.Node, loopStart *core.Node) *utils2.Set[*core.Node] {
	finalSet := utils2.NewSet[*core.Node]()
	walkedNodes := utils2.NewSet[*core.Node]()
	haltRoutes := map[*core.Node][][]*core.Node{}
	var walkNodes func(start *core.Node, route []*core.Node)
	walkNodes = func(start *core.Node, route []*core.Node) {
		route = slices.Clone(route)
		if walkedNodes.Has(start) {
			haltRoutes[start] = append(haltRoutes[start], route)
			return
		}
		walkedNodes.Add(start)
		if slices.Contains(route, start) {
			haltRoutes[start] = append(haltRoutes[start], route)
			return
		}
		if start == circleNode {
			finalSet.AddList(route)
			return
		}
		route = append(route, start)
		for _, n := range start.Next {
			walkNodes(n, route)
		}
		return
	}
	walkNodes(loopStart, []*core.Node{})
	finalSet.Add(circleNode)
	for {
		var hasNew bool
		newM := map[*core.Node][][]*core.Node{}
		maps.Copy(newM, haltRoutes)
		for k, v := range newM {
			if finalSet.Has(k) {
				hasNew = true
				for _, nodes := range v {
					finalSet.AddList(nodes)
				}
				delete(haltRoutes, k)
			}
		}
		if !hasNew {
			break
		}
	}
	return finalSet
}
func searchCircleEndNode(circleNode *core.Node, loopStart *core.Node) *core.Node {
	elementSet := getCircleElementSet(circleNode, loopStart)
	outNodes := []*core.Node{}
	elementSet.ForEach(func(node *core.Node) {
		for _, n := range node.Next {
			if !elementSet.Has(n) {
				outNodes = append(outNodes, n)
			}
		}
	})
	outNodes = NodeDeduplication(outNodes)
	if len(outNodes) == 0 {
		return nil
	}
	if len(outNodes) == 1 {
		return outNodes[0]
	}
	if len(outNodes) > 1 {
		edgeSet := utils2.NewSet[*core.Node]()
		core.WalkGraph[*core.Node](outNodes[0], func(node *core.Node) ([]*core.Node, error) {
			edgeSet.Add(node)
			return node.Next, nil
		})
		var mergeNode *core.Node
		core.WalkGraph[*core.Node](outNodes[1], func(node *core.Node) ([]*core.Node, error) {
			if edgeSet.Has(node) {
				mergeNode = node
				return nil, nil
			}
			return node.Next, nil
		})
		return mergeNode
	}

	return nil
}
