package decompiler

import (
	"slices"
	"strings"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/rewriter"
	"github.com/yaklang/yaklang/common/utils"
)

func ParseBytesCode(decompiler *core.Decompiler) (res []statements.Statement, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = utils.ErrorStack(e)
		}
	}()
	err = decompiler.ParseSourceCode()
	if err != nil {
		// The variable-fold nil-key error ("variable-fold: nil ref key in varUserMap")
		// fires at the END of ParseStatement, after CalcOpcodeStackInfo and statement-node
		// building have already succeeded. The opcode graph and statement nodes are valid;
		// only the variable-fold pass failed because a nil ref leaked into varUserMap.
		// Suppressing this error lets the rest of the pipeline (rewriting, statement collection)
		// proceed with unfolded variables, producing valid Java instead of a full-method stub.
		if !strings.Contains(err.Error(), "nil ref key in varUserMap") {
			return nil, err
		}
	}
	err = rewriter.CheckNodesIsValid(decompiler.RootNode)
	if err != nil {
		return nil, err
	}

	statementManager := rewriter.NewRootStatementManager(decompiler.RootNode)
	statementManager.SetId(decompiler.CurrentId)
	statementManager.Aggressive = decompiler.Aggressive
	statementManager.MergeIf()
	// Tail-duplicate `return cond ? A : B` whose arm computes its value through intermediate local
	// stores (ECJ pre-sized StringBuilder, lazy field init, ...). Those stores cannot be inlined into a
	// ternary arm and would dangle on a fork after the condition collapses ("multiple next"); splitting
	// the shared return into per-arm returns keeps the condition as a real if. Runs before the callback
	// collapse so split conditions keep their structure instead of being spliced into the ternary.
	statementManager.SplitTernaryReturnArms(decompiler.FunctionContext)
	allNodes := []*core.Node{}
	core.WalkGraph[*core.Node](decompiler.RootNode, func(node *core.Node) ([]*core.Node, error) {
		allNodes = append(allNodes, node)
		return node.Next, nil
	})
	slices.Reverse(allNodes)
	for _, node := range allNodes {
		if v, ok := node.Statement.(*statements.ConditionStatement); ok {
			if v.Callback != nil {
				v.Callback(v.Condition)
				allNext := slices.Clone(node.Next)
				for _, nextNode := range allNext {
					node.RemoveNext(nextNode)
				}
				for _, sourceNode := range slices.Clone(node.Source) {
					sourceNode.RemoveNext(node)
					for _, n := range allNext {
						sourceNode.AddNext(n)
					}
				}
			}
		}
	}

	err = statementManager.Rewrite()
	if err != nil {
		return nil, err
	}
	// Collapse dead-end store nodes left by a value-ternary stored to a single-use local that was
	// then inlined into its consumer (`local = a||b ? X : Y; use(local)`): the dangling store forks
	// the entry into {dead store, consumer} and would otherwise abort ToStatements with "multiple next".
	statementManager.RemoveDeadEndAssigns()
	nodes, err := statementManager.ToStatements(func(node *core.Node) bool {
		return true
	})
	nodes = funk.Filter(nodes, func(item *core.Node) bool {
		_, ok := item.Statement.(*statements.StackAssignStatement)
		return !ok
	}).([]*core.Node)
	if err != nil {
		return nil, err
	}
	sts := core.NodesToStatements(nodes)
	// Reject a structurally-broken (cyclic / pathologically huge) statement tree here, before any of the
	// recursive tree walkers (RewriteVar, Statement.ReplaceVar/String, ...) descend it. A cyclic tree
	// would otherwise drive them into Go's unrecoverable `fatal error: stack overflow`, crashing the
	// whole process; raising an ordinary panic instead lets the recover above degrade the method to a stub.
	rewriter.AssertStatementsAcyclic(sts)
	// Fold the javac `assert` guard corruption: when several asserts share/overlap throw targets,
	// the value-merge structuring can leave an orphaned `ConditionStatement(mentions
	// $assertionsDisabled)` immediately followed by its `throw new AssertionError()`, which renders
	// as the fatal `if (cond);` and stubs the whole method. Fold that pair into a real if-body so the
	// method survives post-decompile syntax validation. Runs AFTER the acyclic check so its recursive
	// walk cannot blow the stack on a pathologically deep/cyclic tree. Kill-switch: ASSERT_FOLD_OFF=1.
	sts = rewriter.FoldAssertionGuards(sts)
	params := []*values.JavaRef{}
	for _, v := range decompiler.Params {
		if ref, ok := v.(*values.JavaRef); ok {
			params = append(params, ref)
		}
	}
	rewriter.RewriteVar(&sts, decompiler.BodyStartId, params)
	// Drop statements javac would reject as unreachable (e.g. a back-edge `continue`
	// emitted after an inner infinite loop that only exits via return / labelled
	// continue). The pass is a strict subset of the JLS reachability rules, so it
	// leaves already-correct methods untouched.
	sts = rewriter.PruneUnreachableStatements(sts)
	return sts, nil
}
