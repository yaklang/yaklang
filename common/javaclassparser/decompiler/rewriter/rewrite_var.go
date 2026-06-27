package rewriter

import (
	"fmt"
	"maps"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
)

func RewriteVar(sts *[]statements.Statement, startVarId int, params []*values.JavaRef, traceCtx ...*class_context.ClassContext) {
	var ctx *class_context.ClassContext
	if len(traceCtx) > 0 {
		ctx = traceCtx[0]
	}
	className, methodName := "", ""
	if ctx != nil {
		className = ctx.ClassName
		methodName = ctx.FunctionName
	}
	scope := NewScope(startVarId, sts)
	for _, v := range params {
		scope.assignedMap[v.VarUid] = v.Id
		core.TraceRewriteVar(className, methodName, "param uid=%s id=%s", v.VarUid, v.Id.String())
	}
	rewriteVar(scope, className, methodName)
	var checkUndefinedVar func(scope *Scope, parentAssigned map[*utils.VariableId]struct{})
	undefined := make(map[values.JavaValue]int)
	varAssignMap := map[*utils.VariableId][]*statements.AssignStatement{}
	varAssignMapDeep := map[*utils.VariableId][]int{}
	checkUndefinedVar = func(scope *Scope, parentAssigned map[*utils.VariableId]struct{}) {
		assigned := maps.Clone(parentAssigned)
		for _, v := range scope.varMap {
			switch value := v.(type) {
			case *Scope:
				checkUndefinedVar(value, assigned)
			case *statements.AssignStatement:
				leftValue := value.LeftValue
				if ref, ok := leftValue.(*values.JavaRef); ok {
					varAssignMap[ref.Id] = append(varAssignMap[ref.Id], value)
					varAssignMapDeep[ref.Id] = append(varAssignMapDeep[ref.Id], scope.deep)
					if value.IsFirst {
						assigned[ref.Id] = struct{}{}
					} else {
						if _, ok := assigned[ref.Id]; !ok {
							undefined[value.LeftValue] = scope.deep
						}
					}
				}
			}
		}
	}
	assigned := make(map[*utils.VariableId]struct{})
	for _, v := range params {
		assigned[v.Id] = struct{}{}
	}
	checkUndefinedVar(scope, assigned)
	//for key, _ := range undefined {
	//	sts := varAssignMap[key.(*values.JavaRef).Id]
	//	if len(sts) > 0 {
	//		sts[0].IsFirst = true
	//		for _, statement := range sts[1:] {
	//			statement.IsFirst = false
	//		}
	//	}
	//}
	// Iterate undefined vars in a stable name order: `undefined` is a Go map and each iteration may
	// prepend a DeclareStatement to scope.sts (below), so a raw map range would emit the leading
	// declarations in a random order and make the same method decompile differently run to run. The
	// ref names were already assigned by rewriteVar above, so String() is a deterministic key.
	undefinedKeys := make([]values.JavaValue, 0, len(undefined))
	for key := range undefined {
		undefinedKeys = append(undefinedKeys, key)
	}
	// Sort by (name, deep): several keys can share one uid (same variable, different assignment sites)
	// but carry different deeps, and each iteration overwrites that uid's IsFirst decision, so the
	// last-processed key wins. Ordering by deep as a tie-breaker makes "which key wins" deterministic;
	// keys identical in both fields compute the same decision, so their relative order is irrelevant.
	sort.SliceStable(undefinedKeys, func(i, j int) bool {
		ni := undefinedKeys[i].(*values.JavaRef).Id.String()
		nj := undefinedKeys[j].(*values.JavaRef).Id.String()
		if ni != nj {
			return ni < nj
		}
		return undefined[undefinedKeys[i]] < undefined[undefinedKeys[j]]
	})
	for _, key := range undefinedKeys {
		undefinedVarDeep := undefined[key]
		uid := key.(*values.JavaRef).Id
		assignSts := varAssignMap[uid]
		deepMap := varAssignMapDeep[uid]
		firstIsOk := true
		if len(assignSts) > 0 {
			firstDeep := -1
			for index, deep := range deepMap {
				if deep > undefinedVarDeep {
					continue
				}
				if firstDeep == -1 {
					firstDeep = index
					continue
				}
				if deep < deepMap[firstDeep] {
					firstIsOk = false
					break
				}
			}
			if firstIsOk && firstDeep != -1 {
				assignSts[firstDeep].IsFirst = true
				for _, statement := range assignSts[firstDeep+1:] {
					statement.IsFirst = false
				}
			}
		}
		if !firstIsOk {
			for _, st := range assignSts {
				st.IsFirst = false
			}
			*scope.sts = append([]statements.Statement{statements.NewDeclareStatement(key)}, *scope.sts...)
		}
	}
	// Lift cross-case switch-local declarations out of their case bodies. Runs last so the
	// IsFirst decisions above are final and this pass's demotions are not subsequently undone.
	hoistSwitchDeclarations(sts)
	dropEmptySlotAssignments(sts)
	// General LCA declaration placement: hoist any generated local whose single (minted-id-reused)
	// variable is referenced across more than one sibling scope to the block that dominates all of
	// its uses. Runs last so it sees the final IsFirst/IsDeclare flags and any bare declarations the
	// if/switch hoisters already inserted (which it treats as authoritative and never duplicates).
	// Restricted to ids the minted path merged across scopes: only those can now have a declaration
	// that fails to dominate every use. Everything the decompiler already scoped correctly is left
	// byte-for-byte as the baseline produced it.
	placeCrossScopeDeclarations(sts, scope.reused)
}

type Scope struct {
	nowId       int
	deep        int
	sts         *[]statements.Statement
	varMap      []any
	assignedMap map[string]*utils.VariableId
	// minted is the set of VariableIds this method's rewriteVar has freshly minted. It is shared
	// (same map pointer) across every scope of one method, unlike assignedMap which is copied into
	// each SubScope. The slot's JavaRef object can be shared across sibling/disjoint branches and is
	// rebound in place by ReplaceVar; without this set, a uid first bound inside one branch (recorded
	// only in that branch's copied assignedMap) is invisible to another branch reusing the same JVM
	// slot, so that branch re-binds the SAME shared ref to a SECOND id while the ref's reads still
	// carry the first id. The store and its reads then disagree, producing either an illegal
	// self-referencing declaration (`long x = x << 8 ...`) or a use of an uninitialized split id
	// (`variable var4_1 might not have been initialized`). Tracking minted ids lets the second store
	// detect "this ref already carries an id I minted" and reuse it instead of splitting.
	minted map[*utils.VariableId]int
	// reused collects the ids that the minted path above MERGED across scopes (one logical variable
	// reached by a store in a scope that did not originally declare it). These - and only these - are
	// the ids whose single declaration may now sit in a scope that does not dominate all uses, so the
	// cross-scope declaration placement pass is restricted to them. Variables the decompiler already
	// scoped correctly (catch parameters, switch locals, ordinary locals) are never in this set and
	// are therefore left exactly as the baseline produced them. Shared method-wide like minted.
	reused map[*utils.VariableId]struct{}
}

func NewScope(startId int, sts *[]statements.Statement) *Scope {
	return &Scope{
		nowId:       startId,
		sts:         sts,
		assignedMap: map[string]*utils.VariableId{},
		minted:      map[*utils.VariableId]int{},
		reused:      map[*utils.VariableId]struct{}{},
	}
}
func (s *Scope) NextId() int {
	s.nowId++
	return s.nowId
}
func (s *Scope) SubScope(sts *[]statements.Statement) *Scope {
	assignedMap := map[string]*utils.VariableId{}
	maps.Copy(assignedMap, s.assignedMap)
	newScope := &Scope{
		nowId:       s.nowId,
		sts:         sts,
		deep:        s.deep + 1,
		assignedMap: assignedMap,
		minted:      s.minted,
		reused:      s.reused,
	}
	s.varMap = append(s.varMap, newScope)
	return newScope
}

// maxStructuredContainers bounds how many nested-block container statements a single method's
// structured tree may contain. It backstops AssertStatementsAcyclic's traversal against a degenerate
// (but technically acyclic) explosion of nested blocks.
const maxStructuredContainers = 1_000_000

// PruneCyclicContainerReferences removes structurally-impossible backlinks where a container
// statement appears inside its own descendant body. Such links are artifacts of CFG structuring,
// not Java source structure, and must be removed before recursive tree walkers run.
func PruneCyclicContainerReferences(roots []statements.Statement) []statements.Statement {
	ancestors := make(map[statements.Statement]struct{})
	visited := make(map[statements.Statement]struct{})
	var pruneList func([]statements.Statement) []statements.Statement
	var pruneStatement func(statements.Statement)

	pruneList = func(list []statements.Statement) []statements.Statement {
		if len(list) == 0 {
			return list
		}
		out := list[:0]
		for _, child := range list {
			if child == nil {
				continue
			}
			if _, ok := ancestors[child]; ok {
				continue
			}
			pruneStatement(child)
			out = append(out, child)
		}
		return out
	}

	pruneStatement = func(st statements.Statement) {
		if st == nil {
			return
		}
		if _, ok := visited[st]; ok {
			return
		}
		switch s := st.(type) {
		case *statements.IfStatement:
			visited[st] = struct{}{}
			ancestors[st] = struct{}{}
			s.IfBody = pruneList(s.IfBody)
			s.ElseBody = pruneList(s.ElseBody)
			delete(ancestors, st)
		case *statements.ForStatement:
			visited[st] = struct{}{}
			ancestors[st] = struct{}{}
			s.SubStatements = pruneList(s.SubStatements)
			delete(ancestors, st)
		case *statements.WhileStatement:
			visited[st] = struct{}{}
			ancestors[st] = struct{}{}
			s.Body = pruneList(s.Body)
			delete(ancestors, st)
		case *statements.DoWhileStatement:
			visited[st] = struct{}{}
			ancestors[st] = struct{}{}
			s.Body = pruneList(s.Body)
			delete(ancestors, st)
		case *statements.SwitchStatement:
			visited[st] = struct{}{}
			ancestors[st] = struct{}{}
			for _, c := range s.Cases {
				if c != nil {
					c.Body = pruneList(c.Body)
				}
			}
			delete(ancestors, st)
		case *statements.TryCatchStatement:
			visited[st] = struct{}{}
			ancestors[st] = struct{}{}
			s.TryBody = pruneList(s.TryBody)
			for i := range s.CatchBodies {
				s.CatchBodies[i] = pruneList(s.CatchBodies[i])
			}
			delete(ancestors, st)
		case *statements.SynchronizedStatement:
			visited[st] = struct{}{}
			ancestors[st] = struct{}{}
			s.Body = pruneList(s.Body)
			delete(ancestors, st)
		}
	}

	return pruneList(roots)
}

// AssertStatementsAcyclic verifies that the structured statement tree produced for one method is a
// proper tree of nested-block containers (if/for/while/do-while/switch/try) - i.e. no container is its
// own ancestor or otherwise reachable twice. A structuring defect on certain real-world classes emitted
// a self-referential container (an IfStatement whose own body contained itself), which sent the many
// recursive tree walkers (rewriteVar, Statement.ReplaceVar, Statement.String, ...) into unbounded
// recursion. Because that surfaces as Go's UNRECOVERABLE `fatal error: stack overflow`, the per-method
// recover nets could not contain it and a single class crashed the whole host process. This check runs
// ITERATIVELY (its own explicit stack, so it cannot itself overflow) once before any recursive pass; on
// a cycle or pathological size it raises an ordinary panic, which ParseBytesCode's recover converts into
// a returned error so the method degrades to a clean stub. Container nodes are never legitimately shared
// in a well-formed tree, so a repeat visit is always a defect; leaf statements are not tracked, so
// shared leaf singletons (break/continue/...) never trigger a false positive.
func AssertStatementsAcyclic(roots []statements.Statement) {
	visited := make(map[statements.Statement]struct{})
	count := 0
	// ancestors tracks container nodes on the CURRENT DFS path; visited tracks ALL expanded
	// containers. A node revisited via ancestors is a TRUE cycle (A contains A transitively) which
	// would drive recursive walkers (Statement.String, rewriteVar, ...) into unbounded recursion →
	// must panic. A node revisited but NOT on the current path is merely SHARED (two independent
	// parents reference the same Statement, a finite DAG): safe to skip its children (already
	// expanded by the first visit) and continue, letting the method decompile instead of stubbing
	// the whole body (druid TDDLHint.<init>, jackson UTF8DataInputJsonParser).
	type stackEntry struct {
		st    statements.Statement
		leave bool
	}
	ancestors := make(map[statements.Statement]struct{})
	stack := make([]stackEntry, 0, len(roots))
	for _, r := range roots {
		stack = append(stack, stackEntry{st: r, leave: false})
	}
	for len(stack) > 0 {
		entry := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if entry.leave {
			delete(ancestors, entry.st)
			continue
		}
		st := entry.st
		if st == nil {
			continue
		}
		var children [][]statements.Statement
		switch s := st.(type) {
		case *statements.IfStatement:
			children = append(children, s.IfBody, s.ElseBody)
		case *statements.ForStatement:
			children = append(children, s.SubStatements)
		case *statements.WhileStatement:
			children = append(children, s.Body)
		case *statements.DoWhileStatement:
			children = append(children, s.Body)
		case *statements.SwitchStatement:
			for _, c := range s.Cases {
				if c != nil {
					children = append(children, c.Body)
				}
			}
		case *statements.TryCatchStatement:
			children = append(children, s.TryBody)
			children = append(children, s.CatchBodies...)
		default:
			// Leaf / non-container statement: nothing to descend into.
			continue
		}
		// ancestors MUST be checked before visited: a node on the current DFS path is a TRUE cycle
		// (st is its own transitive ancestor) and drives recursive walkers into unbounded recursion,
		// so it must panic. Since a node is added to BOTH visited and ancestors on first expansion,
		// checking visited first would short-circuit a self-cycle (A whose body contains A: the child
		// A is already in visited) and misclassify it as a harmless shared DAG, letting the cycle
		// through to FoldAssertionGuards/rewriteVar and a fatal stack overflow.
		if _, ok := ancestors[st]; ok {
			panic(fmt.Errorf("cyclic container statement (%T) in structured tree; rejecting to avoid unbounded recursion", st))
		}
		if _, ok := visited[st]; ok {
			// Visited but NOT on the current path: two independent parents reference the same
			// Statement object - a finite DAG, not a cycle. Skip expanding its children (already
			// done on the first visit) and continue, so the method decompiles instead of stubbing.
			continue
		}
		visited[st] = struct{}{}
		ancestors[st] = struct{}{}
		count++
		if count > maxStructuredContainers {
			panic(fmt.Errorf("structured statement tree has more than %d container nodes; rejecting as pathological", maxStructuredContainers))
		}
		// Push a leave marker so ancestors is cleaned up after all children are processed,
		// then push the children. LIFO → children are processed first, leave marker last.
		stack = append(stack, stackEntry{st: st, leave: true})
		for _, body := range children {
			for _, child := range body {
				stack = append(stack, stackEntry{st: child, leave: false})
			}
		}
	}
}

// maxRewriteVarDepth bounds the structural recursion of rewriteVar. The walker descends one frame per
// nested block (if/for/while/do-while/switch/try); a pathological or cyclic statement tree (observed on
// machine-generated parsers, and on degenerate structuring output) can drive this past the goroutine
// stack limit, which manifests as Go's UNRECOVERABLE `fatal error: stack overflow` and crashes the whole
// host process - the recover nets cannot catch it. No hand-written or normally-generated Java nests
// thousands of blocks deep, so once depth crosses this threshold we raise an ordinary (recoverable)
// panic instead; ParseBytesCode's recover turns it into a returned error and the method degrades to a
// clean stub rather than taking the process down. The limit sits far above any legitimate nesting yet
// far below the ~250k frames it takes to overflow a 1GB stack.
const maxRewriteVarDepth = 5000

func rewriteVar(scope *Scope, className, methodName string) int {
	if scope.deep > maxRewriteVarDepth {
		panic(fmt.Errorf("rewriteVar: block nesting depth %d exceeds limit %d (pathological or cyclic statement tree)", scope.deep, maxRewriteVarDepth))
	}
	idReplaceMap := map[*utils.VariableId]*utils.VariableId{}
	defer func() {
		for oldId, newId := range idReplaceMap {
			for _, statement := range *scope.sts {
				statement.ReplaceVar(oldId, newId)
			}
		}
	}()
	for _, statement := range *scope.sts {
		switch statement := statement.(type) {
		case *statements.AssignStatement:
			left := core.UnpackSoltValue(statement.LeftValue)
			hasNamed := false
			if v, ok := left.(*values.JavaRef); ok {
				_, ok := scope.assignedMap[v.VarUid]
				if ok {
					hasNamed = true
				} else if mintDepth, minted := scope.minted[v.Id]; minted {
					// v.Id is an id this method already minted for the same logical variable in a
					// sibling/disjoint scope; that binding never reached THIS scope's copied
					// assignedMap, but the shared JavaRef object already carries the minted id (an
					// in-place ReplaceVar rebound it). Re-minting here would split one variable into
					// two ids and corrupt earlier uses. Reuse the already-minted id: record it for
					// this scope so later refs resolve, and keep the ref as-is. Crucially still consume
					// a name slot (nowId++): the reused id's name is var<nowId> (the minting scope
					// started at this same nowId), so without the bump the NEXT fresh bind in this
					// scope would mint that very name and collide (var1 / var1_1). Advancing nowId
					// keeps the merge while preserving clean sequential names for later siblings.
					//
					// The merge is gated on assignmentReadsLeftId: ONLY a self-referential store
					// (`word = (word << 8) | ...`, whose rhs reads the very id being assigned) is
					// hazardous to split - the split rebinds the rhs read onto a fresh,
					// never-initialized id and emits illegal `long x = x << 8`. A reuse whose rhs
					// does NOT read the slot (`b = data[i] & 0xff`, `i = 0`) is safe to bind fresh,
					// so it falls through to the else and is named exactly as the baseline did. This
					// keeps genuinely independent slot reuses - e.g. two disjoint loop indexes that
					// happen to share a JVM slot - as separate variables instead of collapsing them.
					scope.assignedMap[v.VarUid] = v.Id
					scope.reused[v.Id] = struct{}{}
					scope.varMap = append(scope.varMap, statement)
					scope.nowId++
					core.TraceRewriteVar(className, methodName, "reuse-minted depth=%d mintDepth=%d uid=%s id=%s", scope.deep, mintDepth, v.VarUid, v.Id.String())
				} else {
					oldId := v.Id
					newId := utils.NewRootVariableId()
					idReplaceMap[oldId] = newId
					newRef := *v
					newRef.Id = newId
					newRef.Id.SetName(fmt.Sprintf("var%d", scope.nowId))
					statement.LeftValue = &newRef
					scope.varMap = append(scope.varMap, statement)
					scope.nowId++
					scope.assignedMap[v.VarUid] = newId
					scope.minted[newId] = scope.deep
					core.TraceRewriteVar(className, methodName, "bind depth=%d uid=%s old=%s new=%s type=%s",
						scope.deep, v.VarUid, oldId.String(), newId.String(), v.Type().String(&class_context.ClassContext{}))
				}
			}
			if hasNamed {
				ref := statement.LeftValue.(*values.JavaRef)
				id, _ := scope.assignedMap[ref.VarUid]
				ref.Id = id
				scope.varMap = append(scope.varMap, statement)
				core.TraceRewriteVar(className, methodName, "reuse depth=%d uid=%s id=%s", scope.deep, ref.VarUid, id.String())
			}
		case *statements.IfStatement:
			subScope := scope.SubScope(&statement.IfBody)
			core.TraceRewriteVar(className, methodName, "enter if depth=%d body=%d", subScope.deep, len(statement.IfBody))
			rewriteVar(subScope, className, methodName)
			subScope = scope.SubScope(&statement.ElseBody)
			core.TraceRewriteVar(className, methodName, "enter else depth=%d body=%d", subScope.deep, len(statement.ElseBody))
			rewriteVar(subScope, className, methodName)
		case *statements.ForStatement:
			subScope := scope.SubScope(&statement.SubStatements)
			core.TraceRewriteVar(className, methodName, "enter for depth=%d body=%d", subScope.deep, len(statement.SubStatements))
			rewriteVar(subScope, className, methodName)
		case *statements.WhileStatement:
			subScope := scope.SubScope(&statement.Body)
			core.TraceRewriteVar(className, methodName, "enter while depth=%d body=%d", subScope.deep, len(statement.Body))
			rewriteVar(subScope, className, methodName)
		case *statements.DoWhileStatement:
			subScope := scope.SubScope(&statement.Body)
			core.TraceRewriteVar(className, methodName, "enter do-while depth=%d body=%d", subScope.deep, len(statement.Body))
			rewriteVar(subScope, className, methodName)
		case *statements.SwitchStatement:
			subScope := scope.SubScope(nil)
			for _, c := range statement.Cases {
				subScope.sts = &c.Body
				core.TraceRewriteVar(className, methodName, "enter switch-case depth=%d body=%d", subScope.deep, len(c.Body))
				rewriteVar(subScope, className, methodName)
			}
		case *statements.TryCatchStatement:
			subScope := scope.SubScope(&statement.TryBody)
			core.TraceRewriteVar(className, methodName, "enter try depth=%d body=%d", subScope.deep, len(statement.TryBody))
			rewriteVar(subScope, className, methodName)
			for _, c := range statement.CatchBodies {
				subScope = scope.SubScope(&c)
				core.TraceRewriteVar(className, methodName, "enter catch depth=%d body=%d", subScope.deep, len(c))
				rewriteVar(subScope, className, methodName)
			}
		}
	}
	return scope.nowId
}

// hoistSwitchDeclarations lifts the declaration of any local that is declared inside a switch
// case yet shared across more than one case out to the block that contains the switch. A switch
// body is a single lexical block, so a local first declared in one case is visible to later
// cases textually after it - but NOT to cases the decompiler reordered before it, nor to any read
// after the switch; javac then rejects those uses as "cannot find symbol". A variable assigned in
// two or more sibling cases is by construction one logical variable spanning the whole switch, so
// its `T x` declaration belongs ahead of the switch. The case assignments are demoted to plain
// `x = ...` and a single `T x;` is inserted immediately before the switch. Hoisting only widens
// scope and is always valid Java, so it never deletes or corrupts reachable code. The pass runs
// AFTER RewriteVar's declaration placement so its IsFirst decisions are final and are not undone.
func hoistSwitchDeclarations(sts *[]statements.Statement) {
	if sts == nil {
		return
	}
	list := *sts
	out := make([]statements.Statement, 0, len(list))
	for i, st := range list {
		switch s := st.(type) {
		case *statements.IfStatement:
			hoistSwitchDeclarations(&s.IfBody)
			hoistSwitchDeclarations(&s.ElseBody)
			if os.Getenv("JDEC_IF_HOIST_OFF") == "" {
				for _, decl := range ifHoistDeclarations(s, list[i+1:]) {
					out = append(out, decl)
				}
			}
		case *statements.ForStatement:
			hoistSwitchDeclarations(&s.SubStatements)
		case *statements.WhileStatement:
			hoistSwitchDeclarations(&s.Body)
		case *statements.DoWhileStatement:
			hoistSwitchDeclarations(&s.Body)
		case *statements.SynchronizedStatement:
			hoistSwitchDeclarations(&s.Body)
		case *statements.TryCatchStatement:
			hoistSwitchDeclarations(&s.TryBody)
			for i := range s.CatchBodies {
				hoistSwitchDeclarations(&s.CatchBodies[i])
			}
		case *statements.SwitchStatement:
			for _, c := range s.Cases {
				hoistSwitchDeclarations(&c.Body)
			}
			// Statements after the switch in THIS block are where an out-of-scope read would
			// occur; pass them so only variables actually read after the switch are hoisted.
			for _, decl := range switchHoistDeclarations(s, list[i+1:]) {
				out = append(out, decl)
			}
		}
		out = append(out, st)
	}
	*sts = out
}

var hoistProbeCtx = &class_context.ClassContext{}

// switchHoistDeclarations demotes the in-case declaration of any local that is declared inside a
// switch case yet read after the switch to a plain assignment, and returns the bare declarations
// to emit ahead of the switch (deterministic name order). The "read after the switch" test is the
// precise trigger: a local declared in a case is in scope for later cases (textually after it), so
// only an outside read is unsafe. afterSts are the statements following the switch in the same
// block; reference detection is by final variable NAME, which is consistent across the rendered
// output. See hoistSwitchDeclarations for why this is always safe.
func switchHoistDeclarations(sw *statements.SwitchStatement, afterSts []statements.Statement) []statements.Statement {
	declaredInside := map[string]bool{}
	assignsByUid := map[string][]*statements.AssignStatement{}
	refByUid := map[string]values.JavaValue{}
	var collect func([]statements.Statement)
	collect = func(sts []statements.Statement) {
		for _, st := range sts {
			switch s := st.(type) {
			case *statements.AssignStatement:
				if s.ArrayMember != nil {
					continue
				}
				ref, ok := core.UnpackSoltValue(s.LeftValue).(*values.JavaRef)
				if !ok || ref == nil || ref.Id == nil {
					continue
				}
				assignsByUid[ref.VarUid] = append(assignsByUid[ref.VarUid], s)
				refByUid[ref.VarUid] = s.LeftValue
				if s.IsFirst || s.IsDeclare {
					declaredInside[ref.VarUid] = true
				}
			case *statements.IfStatement:
				collect(s.IfBody)
				collect(s.ElseBody)
			case *statements.ForStatement:
				collect(s.SubStatements)
			case *statements.WhileStatement:
				collect(s.Body)
			case *statements.DoWhileStatement:
				collect(s.Body)
			case *statements.SynchronizedStatement:
				collect(s.Body)
			case *statements.TryCatchStatement:
				collect(s.TryBody)
				for i := range s.CatchBodies {
					collect(s.CatchBodies[i])
				}
			case *statements.SwitchStatement:
				for _, c := range s.Cases {
					collect(c.Body)
				}
			}
		}
	}
	for _, c := range sw.Cases {
		collect(c.Body)
	}
	uids := make([]string, 0, len(assignsByUid))
	for uid := range assignsByUid {
		uids = append(uids, uid)
	}
	sort.SliceStable(uids, func(i, j int) bool {
		return uids[i] < uids[j]
	})
	var declares []statements.Statement
	for _, uid := range uids {
		if !declaredInside[uid] {
			continue
		}
		if len(assignsByUid[uid]) < 2 {
			continue
		}
		targetRef := refByUid[uid]
		name := targetRef.String(hoistProbeCtx)
		for _, as := range assignsByUid[uid] {
			ref, ok := core.UnpackSoltValue(as.LeftValue).(*values.JavaRef)
			if !ok || ref == nil || ref.Id == nil {
				continue
			}
			candidateName := ref.String(hoistProbeCtx)
			if candidateName != "" && statementsReadName(afterSts, candidateName) {
				targetRef = as.LeftValue
				name = candidateName
				break
			}
		}
		if name == "" || !statementsReadName(afterSts, name) {
			continue
		}
		targetJavaRef, ok := core.UnpackSoltValue(targetRef).(*values.JavaRef)
		if !ok || targetJavaRef == nil || targetJavaRef.Id == nil {
			continue
		}
		targetID := targetJavaRef.Id
		for _, as := range assignsByUid[uid] {
			if ref, ok := core.UnpackSoltValue(as.LeftValue).(*values.JavaRef); ok && ref != nil && ref.Id != targetID {
				ref.Id = targetID
			}
			as.IsFirst = false
			as.IsDeclare = false
		}
		declares = append(declares, statements.NewDeclareStatement(targetRef))
		redirectPostBlockReassignments(afterSts, uid, targetID)
	}
	return declares
}

// redirectPostBlockReassignments repairs the read-before-write that arises when a local is
// assigned ONLY inside branch bodies (if/else arms or switch cases) and is then RE-assigned in the
// enclosing block right after the branch via `x = f(x)`. RewriteVar walks block scopes and never
// records the slot as defined in the enclosing scope (the branch bindings live in child scopes), so
// it mints a fresh VariableId for the post-block reassignment and rebinds BOTH its left side and the
// self-read on its right side onto that new id. The dumper then renames the colliding declaration to
// `x_1` and, because the right-side read shares the same id, emits `int x_1 = x_1 + ...`, which javac
// rejects with "variable x_1 might not have been initialized" (observed in md5() / xxHash32()).
//
// Once the branch assignments have been unified onto targetID and a single `T x;` declaration is
// hoisted ahead of the branch, every post-block reference of the SAME logical variable must point at
// targetID too, and the first post-block reassignment must be demoted from a declaration to a plain
// assignment so it reuses the hoisted declaration. The match is by VarUid, which is stable per
// variable across all of its refs, so an unrelated later variable that merely reuses the JVM slot
// (a different VarUid) is never touched.
func redirectPostBlockReassignments(afterSts []statements.Statement, uid string, targetID *utils.VariableId) {
	if targetID == nil || uid == "" || len(afterSts) == 0 {
		return
	}
	oldIds := map[*utils.VariableId]struct{}{}
	demotedFirst := false
	var walk func([]statements.Statement)
	walk = func(sts []statements.Statement) {
		for _, st := range sts {
			switch s := st.(type) {
			case *statements.AssignStatement:
				if s.ArrayMember != nil {
					continue
				}
				ref, ok := core.UnpackSoltValue(s.LeftValue).(*values.JavaRef)
				if !ok || ref == nil || ref.Id == nil || ref.VarUid != uid {
					continue
				}
				if ref.Id != targetID {
					oldIds[ref.Id] = struct{}{}
				}
				// Demote only the first declaration-form reassignment: it is the one the dumper
				// would otherwise emit as `int x = x + ...`; later reassignments are already plain.
				if !demotedFirst && (s.IsFirst || s.IsDeclare) {
					s.IsFirst = false
					s.IsDeclare = false
					demotedFirst = true
				}
			case *statements.IfStatement:
				walk(s.IfBody)
				walk(s.ElseBody)
			case *statements.ForStatement:
				walk(s.SubStatements)
			case *statements.WhileStatement:
				walk(s.Body)
			case *statements.DoWhileStatement:
				walk(s.Body)
			case *statements.SynchronizedStatement:
				walk(s.Body)
			case *statements.TryCatchStatement:
				walk(s.TryBody)
				for i := range s.CatchBodies {
					walk(s.CatchBodies[i])
				}
			case *statements.SwitchStatement:
				for _, c := range s.Cases {
					if c != nil {
						walk(c.Body)
					}
				}
			}
		}
	}
	walk(afterSts)
	for oldId := range oldIds {
		for _, st := range afterSts {
			st.ReplaceVar(oldId, targetID)
		}
	}
}

func dropEmptySlotAssignments(sts *[]statements.Statement) {
	if sts == nil {
		return
	}
	list := *sts
	out := make([]statements.Statement, 0, len(list))
	for _, st := range list {
		switch s := st.(type) {
		case *statements.AssignStatement:
			if !s.IsDeclare && s.ArrayMember == nil && assignmentValueIsEmptySlot(s.JavaValue) {
				continue
			}
		case *statements.IfStatement:
			dropEmptySlotAssignments(&s.IfBody)
			dropEmptySlotAssignments(&s.ElseBody)
		case *statements.ForStatement:
			dropEmptySlotAssignments(&s.SubStatements)
		case *statements.WhileStatement:
			dropEmptySlotAssignments(&s.Body)
		case *statements.DoWhileStatement:
			dropEmptySlotAssignments(&s.Body)
		case *statements.SynchronizedStatement:
			dropEmptySlotAssignments(&s.Body)
		case *statements.TryCatchStatement:
			dropEmptySlotAssignments(&s.TryBody)
			for i := range s.CatchBodies {
				dropEmptySlotAssignments(&s.CatchBodies[i])
			}
		case *statements.SwitchStatement:
			for i := range s.Cases {
				dropEmptySlotAssignments(&s.Cases[i].Body)
			}
		}
		out = append(out, st)
	}
	*sts = out
}

func assignmentValueIsEmptySlot(value values.JavaValue) (ok bool) {
	if value == nil {
		return true
	}
	defer func() {
		if recover() != nil {
			ok = true
		}
	}()
	return strings.Contains(value.String(hoistProbeCtx), values.EmptySlotValuePlaceholder)
}

// ifHoistDeclarations demotes declarations inside an if/else tree when the local is read after the
// if in the containing block. A branch-local declaration cannot be referenced after the if; bytecode
// patterns like `if (...) x = A; else x = B; return x;` therefore need a single `T x;` before the if.
func ifHoistDeclarations(ifst *statements.IfStatement, afterSts []statements.Statement) []statements.Statement {
	declaredInside := map[string]bool{}
	assignsByUid := map[string][]*statements.AssignStatement{}
	refByUid := map[string]values.JavaValue{}
	var collect func([]statements.Statement)
	collect = func(sts []statements.Statement) {
		for _, st := range sts {
			switch s := st.(type) {
			case *statements.AssignStatement:
				if s.ArrayMember != nil {
					continue
				}
				ref, ok := core.UnpackSoltValue(s.LeftValue).(*values.JavaRef)
				if !ok || ref == nil || ref.Id == nil {
					continue
				}
				assignsByUid[ref.VarUid] = append(assignsByUid[ref.VarUid], s)
				refByUid[ref.VarUid] = s.LeftValue
				if s.IsFirst || s.IsDeclare {
					declaredInside[ref.VarUid] = true
				}
			case *statements.IfStatement:
				collect(s.IfBody)
				collect(s.ElseBody)
			case *statements.ForStatement:
				collect(s.SubStatements)
			case *statements.WhileStatement:
				collect(s.Body)
			case *statements.DoWhileStatement:
				collect(s.Body)
			case *statements.SynchronizedStatement:
				collect(s.Body)
			case *statements.TryCatchStatement:
				collect(s.TryBody)
				for i := range s.CatchBodies {
					collect(s.CatchBodies[i])
				}
			case *statements.SwitchStatement:
				for _, c := range s.Cases {
					collect(c.Body)
				}
			}
		}
	}
	collect(ifst.IfBody)
	collect(ifst.ElseBody)

	uids := make([]string, 0, len(assignsByUid))
	for uid := range assignsByUid {
		uids = append(uids, uid)
	}
	sort.SliceStable(uids, func(i, j int) bool {
		return uids[i] < uids[j]
	})
	var declares []statements.Statement
	for _, uid := range uids {
		if !declaredInside[uid] {
			continue
		}
		if len(assignsByUid[uid]) < 2 {
			continue
		}
		canDemote := true
		for _, as := range assignsByUid[uid] {
			if !assignRendersAsPlain(as) {
				canDemote = false
				break
			}
		}
		if !canDemote {
			continue
		}
		targetRef := refByUid[uid]
		name := targetRef.String(hoistProbeCtx)
		for _, as := range assignsByUid[uid] {
			ref, ok := core.UnpackSoltValue(as.LeftValue).(*values.JavaRef)
			if !ok || ref == nil || ref.Id == nil {
				continue
			}
			candidateName := ref.String(hoistProbeCtx)
			if candidateName != "" && statementsReadName(afterSts, candidateName) {
				targetRef = as.LeftValue
				name = candidateName
				break
			}
		}
		if name == "" || !statementsReadName(afterSts, name) {
			continue
		}
		targetJavaRef, ok := core.UnpackSoltValue(targetRef).(*values.JavaRef)
		if !ok || targetJavaRef == nil || targetJavaRef.Id == nil {
			continue
		}
		targetID := targetJavaRef.Id
		for _, as := range assignsByUid[uid] {
			if ref, ok := core.UnpackSoltValue(as.LeftValue).(*values.JavaRef); ok && ref != nil && ref.Id != targetID {
				ref.Id = targetID
			}
			as.IsFirst = false
			as.IsDeclare = false
		}
		declares = append(declares, statements.NewDeclareStatement(targetRef))
		redirectPostBlockReassignments(afterSts, uid, targetID)
	}
	return declares
}

func assignRendersAsPlain(as *statements.AssignStatement) (ok bool) {
	if as == nil {
		return false
	}
	oldFirst, oldDeclare := as.IsFirst, as.IsDeclare
	defer func() {
		as.IsFirst, as.IsDeclare = oldFirst, oldDeclare
		if recover() != nil {
			ok = false
		}
	}()
	as.IsFirst, as.IsDeclare = false, false
	_ = as.String(hoistProbeCtx)
	return true
}

// statementsReadName is the if-hoist trigger: unlike switch hoisting, a later assignment target
// `x = ...` should not force an unrelated branch-local `x` declaration outward. Only reads in the
// statements after the if make the branch declaration unsafe.
func statementsReadName(sts []statements.Statement, name string) (res bool) {
	defer func() {
		if recover() != nil {
			res = true
		}
	}()
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
	for _, st := range sts {
		if as, ok := st.(*statements.AssignStatement); ok {
			if as.JavaValue != nil && re.MatchString(as.JavaValue.String(hoistProbeCtx)) {
				return true
			}
			if as.ArrayMember != nil && re.MatchString(as.ArrayMember.String(hoistProbeCtx)) {
				return true
			}
			if ref, ok := core.UnpackSoltValue(as.LeftValue).(*values.JavaRef); ok && ref.String(hoistProbeCtx) == name && (as.IsFirst || as.IsDeclare) {
				return false
			}
			continue
		}
		if statementReadTextMatches(st, re) {
			return true
		}
	}
	return false
}

func statementReadTextMatches(st statements.Statement, re *regexp.Regexp) (res bool) {
	defer func() {
		if recover() != nil {
			res = true
		}
	}()
	return re.MatchString(st.String(hoistProbeCtx))
}

// statementsReferenceName reports whether any of the statements textually reference the variable
// name as a whole token (so "var2" does not match "var20"). Rendering uses an empty class context;
// a render that panics is treated as a reference (conservative: hoisting is always valid Java, so
// an unnecessary hoist never breaks compilation while a missed one would).
func statementsReferenceName(sts []statements.Statement, name string) bool {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
	for _, st := range sts {
		if statementTextMatches(st, re) {
			return true
		}
	}
	return false
}

func statementTextMatches(st statements.Statement, re *regexp.Regexp) (res bool) {
	defer func() {
		if recover() != nil {
			res = true
		}
	}()
	return re.MatchString(st.String(hoistProbeCtx))
}

// generatedLocalNameRe matches a decompiler-generated local name (var0, var1, var2_1, ...). Only
// these synthetic locals are candidates for cross-scope declaration hoisting; named parameters,
// fields and `this` are never moved.
var generatedLocalNameRe = regexp.MustCompile(`^var\d+(?:_\d+)?$`)

// childStatementLists returns pointers to every nested statement list of a container statement, so a
// single traversal/mutation routine can recurse into all block kinds. The pointers allow in-place
// rewriting of the child lists (e.g. stripping a relocated bare declaration).
func childStatementLists(st statements.Statement) []*[]statements.Statement {
	switch s := st.(type) {
	case *statements.IfStatement:
		return []*[]statements.Statement{&s.IfBody, &s.ElseBody}
	case *statements.ForStatement:
		return []*[]statements.Statement{&s.SubStatements}
	case *statements.WhileStatement:
		return []*[]statements.Statement{&s.Body}
	case *statements.DoWhileStatement:
		return []*[]statements.Statement{&s.Body}
	case *statements.SynchronizedStatement:
		return []*[]statements.Statement{&s.Body}
	case *statements.SwitchStatement:
		var out []*[]statements.Statement
		for i := range s.Cases {
			if s.Cases[i] != nil {
				out = append(out, &s.Cases[i].Body)
			}
		}
		return out
	case *statements.TryCatchStatement:
		out := []*[]statements.Statement{&s.TryBody}
		for i := range s.CatchBodies {
			out = append(out, &s.CatchBodies[i])
		}
		return out
	}
	return nil
}

func safeRenderStatement(st statements.Statement) (text string, ok bool) {
	ok = true
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return st.String(hoistProbeCtx), true
}

// collectGeneratedLocalDeclIDs returns, in deterministic discovery order, the ids of every generated
// local declared (IsFirst/IsDeclare) anywhere in the subtree whose id is in the `reused` set, plus a
// representative LeftValue per id (used to synthesize the bare `T x;` declaration with the right
// type/name). Restricting to `reused` ids keeps the placement pass focused on exactly the variables
// the minted merge joined across scopes; everything else the decompiler already scoped is untouched.
func collectGeneratedLocalDeclIDs(list []statements.Statement, reused map[*utils.VariableId]struct{}) ([]*utils.VariableId, map[*utils.VariableId]values.JavaValue) {
	order := []*utils.VariableId{}
	refByID := map[*utils.VariableId]values.JavaValue{}
	var walk func([]statements.Statement)
	walk = func(sts []statements.Statement) {
		for _, st := range sts {
			if as, ok := st.(*statements.AssignStatement); ok && as.ArrayMember == nil && (as.IsFirst || as.IsDeclare) {
				if ref, ok2 := core.UnpackSoltValue(as.LeftValue).(*values.JavaRef); ok2 && ref != nil && ref.Id != nil && !ref.IsThis {
					if _, isReused := reused[ref.Id]; isReused {
						if _, seen := refByID[ref.Id]; !seen {
							name := ref.String(hoistProbeCtx)
							if generatedLocalNameRe.MatchString(name) {
								refByID[ref.Id] = as.LeftValue
								order = append(order, ref.Id)
							}
						}
					}
				}
			}
			for _, cl := range childStatementLists(st) {
				walk(*cl)
			}
		}
	}
	walk(list)
	return order, refByID
}

// isDeclaredAtTopLevel reports whether id already has a declaration (a bare `T x;` or an inline
// `T x = ...`) directly among this block's own statements. If so the declaration already dominates
// everything in the block, so the placement pass must leave it alone: re-hoisting a correctly-placed
// inline declaration only churns the tree (bare decl + demoted assignment) and can trip the dumper's
// collision renamer into renaming the declaration but not its uses. The pass therefore acts only on
// declarations that are nested BELOW this block, which are the ones that can leave sibling uses out of
// scope. This also makes the pass idempotent w.r.t. the bare declarations the if/switch hoisters emit.
func isDeclaredAtTopLevel(list []statements.Statement, id *utils.VariableId) bool {
	for _, st := range list {
		if as, ok := st.(*statements.AssignStatement); ok && as.ArrayMember == nil && (as.IsFirst || as.IsDeclare) {
			if ref, ok2 := core.UnpackSoltValue(as.LeftValue).(*values.JavaRef); ok2 && ref != nil && ref.Id == id {
				return true
			}
		}
	}
	return false
}

// relocateDeclarations prepares a block subtree for a hoisted declaration of id: it demotes every
// in-place declaration (`T x = ...`) of id to a plain assignment (`x = ...`) and drops any bare
// `T x;` declaration that a deeper hoister had inserted, so exactly one declaration (the one being
// prepended by the caller) survives.
func relocateDeclarations(block *[]statements.Statement, id *utils.VariableId) {
	if block == nil {
		return
	}
	list := *block
	out := list[:0]
	for _, st := range list {
		if as, ok := st.(*statements.AssignStatement); ok && as.ArrayMember == nil {
			if ref, ok2 := core.UnpackSoltValue(as.LeftValue).(*values.JavaRef); ok2 && ref != nil && ref.Id == id {
				if as.IsDeclare && as.JavaValue == nil {
					// A bare declaration of this id inserted by a deeper hoist; drop it because the
					// caller is about to declare the variable in a dominating block.
					continue
				}
				if as.IsFirst || as.IsDeclare {
					as.IsFirst = false
					as.IsDeclare = false
				}
			}
		}
		for _, cl := range childStatementLists(st) {
			relocateDeclarations(cl, id)
		}
		out = append(out, st)
	}
	*block = out
}

// placeCrossScopeDeclarations hoists each generated local's declaration to the lowest block that
// dominates all of its references. rewriteVar reuses one *VariableId for a JVM slot shared by several
// source variables (minted-id reuse), so one variable can be assigned/read across multiple SIBLING
// scopes - sequential loops that reuse a slot, an if's two arms, a switch's cases, a value declared
// in a loop and read afterwards. Its declaration stays in whichever sibling rewriteVar processed
// first, so uses in the other siblings are out of scope ("cannot find symbol"). This is the general
// lowest-common-ancestor declaration placement: for each block it hoists locals referenced across
// more than one of the block's child scopes, emitting a single `T x;` at the block top and demoting
// the in-place declarations to plain assignments. Hoisting only widens scope and is always valid Java.
func placeCrossScopeDeclarations(block *[]statements.Statement, reused map[*utils.VariableId]struct{}) {
	if block == nil || len(*block) == 0 || len(reused) == 0 {
		return
	}
	list := *block
	ids, refByID := collectGeneratedLocalDeclIDs(list, reused)
	if len(ids) > 0 {
		texts := make([]string, len(list))
		allMatch := make([]bool, len(list))
		for i, st := range list {
			t, ok := safeRenderStatement(st)
			texts[i] = t
			allMatch[i] = !ok
		}
		sort.SliceStable(ids, func(i, j int) bool {
			return refByID[ids[i]].String(hoistProbeCtx) < refByID[ids[j]].String(hoistProbeCtx)
		})
		var hoisted []statements.Statement
		for _, id := range ids {
			ref := refByID[id]
			name := ref.String(hoistProbeCtx)
			if name == "" || !generatedLocalNameRe.MatchString(name) {
				continue
			}
			if isDeclaredAtTopLevel(list, id) {
				continue
			}
			re := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
			cnt := 0
			singleIdx := -1
			for i := range list {
				if allMatch[i] || re.MatchString(texts[i]) {
					cnt++
					singleIdx = i
				}
			}
			belongs := false
			if cnt >= 2 {
				// Referenced from two or more sibling top-level statements of this block: the lowest
				// common ancestor of all uses is exactly this block.
				belongs = true
			} else if cnt == 1 {
				// Referenced from a single child container: it belongs here only if that container
				// uses it in two or more of its OWN child scopes (both if-arms, >=2 switch cases,
				// try+catch); otherwise the true home is deeper and recursion will place it.
				refChildren := 0
				for _, cl := range childStatementLists(list[singleIdx]) {
					if statementsReferenceName(*cl, name) {
						refChildren++
					}
				}
				belongs = refChildren >= 2
			}
			if !belongs {
				continue
			}
			relocateDeclarations(block, id)
			hoisted = append(hoisted, statements.NewDeclareStatement(ref))
		}
		if len(hoisted) > 0 {
			*block = append(hoisted, (*block)...)
		}
	}
	for _, st := range *block {
		for _, cl := range childStatementLists(st) {
			placeCrossScopeDeclarations(cl, reused)
		}
	}
}
