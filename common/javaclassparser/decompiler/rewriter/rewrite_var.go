package rewriter

import (
	"fmt"
	"maps"
	"os"
	"regexp"
	"sort"
	"strconv"
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
	// last-processed key wins. Ordering by deep as a tie-breaker makes "which key wins" deterministic.
	// Keys identical in name AND deep still need a stable final tie-breaker: each such key PREPENDS its
	// own DeclareStatement below, so their relative order decides which same-named local is declared
	// first and therefore keeps the bare `var2` name while the other is renamed `var2_1`. Without a
	// tie-breaker that order came from `for key := range undefined` (Go map iteration), so the two
	// declarations - and the whole method's variable naming - flipped run to run. VarUid is a stable
	// per-decompile creation counter; compare it NUMERICALLY (see varUidLess) since its string form is
	// unstable across runs (global counter magnitude shifts).
	sort.SliceStable(undefinedKeys, func(i, j int) bool {
		ni := undefinedKeys[i].(*values.JavaRef).Id.String()
		nj := undefinedKeys[j].(*values.JavaRef).Id.String()
		if ni != nj {
			return ni < nj
		}
		if undefined[undefinedKeys[i]] != undefined[undefinedKeys[j]] {
			return undefined[undefinedKeys[i]] < undefined[undefinedKeys[j]]
		}
		return varUidLess(undefinedKeys[i].(*values.JavaRef).VarUid, undefinedKeys[j].(*values.JavaRef).VarUid)
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
	// switchHoistDeclarations (keyed by VarUid) and placeCrossScopeDeclarations (keyed by
	// *VariableId) can independently emit a bare `T x;` for the SAME logical local when both
	// hoisters fire on it (nested switches whose shared slot is also read after the switch). Both
	// declarations reference the same *VariableId, so the dumper's collision renamer keeps them both
	// as `var2` and produces a duplicate-variable compile error. Drop the redundant repeats here as
	// the final step: two bare declarations of the identical *VariableId in one block are never valid
	// Java, and matching on the id pointer leaves genuinely distinct same-named locals untouched.
	dropDuplicateDeclarations(sts)
}

// dropDuplicateDeclarations removes redundant bare `T x;` declarations that name a *VariableId
// already declared earlier in the SAME block list. Re-declaring the same id in a sibling/nested
// block is legal Java (and intentional after hoisting), so each block list is scoped with its own
// seen-set rather than a single global one.
func dropDuplicateDeclarations(sts *[]statements.Statement) {
	if sts == nil {
		return
	}
	list := *sts
	seen := map[*utils.VariableId]struct{}{}
	out := list[:0]
	for _, st := range list {
		if as, ok := st.(*statements.AssignStatement); ok && as.IsDeclare && as.JavaValue == nil && as.ArrayMember == nil {
			if ref, ok2 := core.UnpackSoltValue(as.LeftValue).(*values.JavaRef); ok2 && ref != nil && ref.Id != nil {
				if _, dup := seen[ref.Id]; dup {
					continue
				}
				seen[ref.Id] = struct{}{}
			}
		}
		for _, cl := range childStatementLists(st) {
			dropDuplicateDeclarations(cl)
		}
		out = append(out, st)
	}
	*sts = out
}

// embeddedDeclInfo records one in-tree declaration's identity and rendered type for the
// embedded-assignment collision check below.
type embeddedDeclInfo struct {
	id      *utils.VariableId
	typeStr string
}

// collectEmbeddedDeclInfos walks the subtree and records every declaration (IsFirst/IsDeclare
// AssignStatement) into byID (VariableId set) and byName (rendered slot name -> declarations with
// their rendered type). Used by SynthesizeUndeclaredEmbeddedAssignDecls to tell a genuine
// cross-variable name collision (different variable, incompatible type) from the legitimate
// chained-assignment reuse of an already-declared slot (same name, same type).
func collectEmbeddedDeclInfos(sts []statements.Statement, byID map[*utils.VariableId]struct{}, byName map[string][]embeddedDeclInfo) {
	for _, st := range sts {
		if as, ok := st.(*statements.AssignStatement); ok && as.ArrayMember == nil && (as.IsFirst || as.IsDeclare) {
			if ref, ok2 := core.UnpackSoltValue(as.LeftValue).(*values.JavaRef); ok2 && ref != nil && ref.Id != nil {
				byID[ref.Id] = struct{}{}
				name := ref.String(hoistProbeCtx)
				ts := ""
				if t := ref.Type(); t != nil {
					ts = t.String(hoistProbeCtx)
				}
				byName[name] = append(byName[name], embeddedDeclInfo{id: ref.Id, typeStr: ts})
			}
		}
		for _, cl := range childStatementLists(st) {
			collectEmbeddedDeclInfos(*cl, byID, byName)
		}
	}
}

// SynthesizeUndeclaredEmbeddedAssignDecls fixes Bug Y residual C: a local whose ONLY definition is an
// embedded assignment baked into an opaque CustomValue (`s == null || (n = s.length()) == 0`, javac
// `... length; dup; istore; ifne`). The dup-collapse in code_analyser.go consumes such a local's
// standalone AssignStatement into the value stream, so the local keeps no DeclareStatement; it is then
// invisible to the identity-based collision renamer (dumper.resolveLocalNameCollisions, which keys on
// declarations) and to the textual missing-decl safety net (addMissingGeneratedLocalDecls treats a
// colliding later `T varN` as already declaring varN), so it renders undeclared AND duplicate-named
// against a later sibling-scope local of the same slot-derived varN -> `cannot find symbol`
// (commons-codec Metaphone.metaphone: the `txtLength` embedded assignment collides with a later
// `StringBuilder` local).
//
// This is DELIBERATELY narrow. It synthesizes a bare `T varN;` at method top ONLY when the orphan's
// slot-derived name collides with a declaration of a DIFFERENT variable whose rendered type is
// INCOMPATIBLE (the residual-C signature: an `int` n vs a `StringBuilder`). That declaration makes the
// orphan a real declaration the identity collision renamer can see, so it renames the colliding sibling
// to varN_1 and rebinds its uses. Two cases are intentionally left untouched:
//   - same name + SAME rendered type: the legitimate chained assignment `int b = a = 1`, where the
//     embedded `(a = 1)` reuses the already-declared slot `a`; minting a second `int a;` would make the
//     renamer split one variable into two and break the program (TestDecompiler/ContinuousAssign).
//   - name with NO declaration at all: an ordinary orphan whose type the textual safety net
//     (addMissingGeneratedLocalDecls / inferGeneratedLocalRefType, kill-switches JDEC_NO_EMBED_ASSIGN_*)
//     already recovers; staying out of its way keeps those fixes load-bearing.
//
// A bare declaration at method top is legal Java and semantically inert (the embedded assignment still
// assigns before any read on every reaching path, so definite-assignment holds). Kill-switch:
// JDEC_EMBED_ASSIGN_DECL_OFF=1.
func SynthesizeUndeclaredEmbeddedAssignDecls(sts *[]statements.Statement, targets []*values.JavaRef) {
	if sts == nil || len(targets) == 0 || os.Getenv("JDEC_EMBED_ASSIGN_DECL_OFF") == "1" {
		return
	}
	declaredID := map[*utils.VariableId]struct{}{}
	declByName := map[string][]embeddedDeclInfo{}
	collectEmbeddedDeclInfos(*sts, declaredID, declByName)
	seen := map[*utils.VariableId]struct{}{}
	var prepend []statements.Statement
	for _, ref := range targets {
		if ref == nil || ref.Id == nil || ref.IsThis || ref.IsParam {
			continue
		}
		if _, done := seen[ref.Id]; done {
			continue
		}
		if _, isDeclared := declaredID[ref.Id]; isDeclared {
			continue
		}
		// Only synthetic var<slot> locals are hoist candidates; a value carrying no static type
		// cannot render a `T x;` declaration, so leave it to the textual safety net.
		name := ref.String(hoistProbeCtx)
		if !generatedLocalNameRe.MatchString(name) || ref.Type() == nil {
			continue
		}
		targetType := ref.Type().String(hoistProbeCtx)
		hasIncompatibleCollider := false
		hasCompatibleSameName := false
		for _, d := range declByName[name] {
			if d.id == ref.Id {
				continue
			}
			if d.typeStr == targetType {
				hasCompatibleSameName = true
			} else {
				hasIncompatibleCollider = true
			}
		}
		if !hasIncompatibleCollider || hasCompatibleSameName {
			continue
		}
		seen[ref.Id] = struct{}{}
		prepend = append(prepend, statements.NewDeclareStatement(ref))
	}
	if len(prepend) > 0 {
		*sts = append(prepend, *sts...)
	}
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
	stsSnapshot := *scope.sts
	for stmtIdx := 0; stmtIdx < len(stsSnapshot); stmtIdx++ {
		statement := stsSnapshot[stmtIdx]
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
			if os.Getenv("JDEC_IFELSE_PREBIND_OFF") == "" {
				prebindEscapingIfElseSlots(scope, statement, stsSnapshot[stmtIdx+1:], idReplaceMap, className, methodName)
			}
			if os.Getenv("JDEC_IFELSE_PARALLEL_PREBIND_OFF") == "" {
				prebindParallelTypedIfElseDefs(scope, statement, stsSnapshot[stmtIdx+1:], idReplaceMap, className, methodName)
			}
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
			if os.Getenv("JDEC_SWITCH_PREBIND_OFF") == "" {
				prebindEscapingSwitchSlots(scope, statement, stsSnapshot[stmtIdx+1:], idReplaceMap, className, methodName)
			}
			subScope := scope.SubScope(nil)
			for _, c := range statement.Cases {
				subScope.sts = &c.Body
				core.TraceRewriteVar(className, methodName, "enter switch-case depth=%d body=%d", subScope.deep, len(c.Body))
				rewriteVar(subScope, className, methodName)
			}
		case *statements.TryCatchStatement:
			// Bug: a local assigned in BOTH the try arm and a catch arm (and read after the try) was
			// re-minted INDEPENDENTLY by each arm. rewriteVar descends into the try and each catch as
			// disjoint sub-scopes; the try arm's deferred ReplaceVar(old->new) rewrites only the try
			// list, never the catch list, so the catch arm still carries the slot's original id and
			// mints a SECOND fresh id, while the post-try read keeps the original id entirely. The three
			// then disagree (`int var1 = ...` in try, `int var1_1 = -1` in catch, `return var2` after),
			// which javac rejects ("cannot find symbol"). The if/else lowering has the same structure
			// but happens to survive because the minted name equals the original name; try/catch breaks
			// that coincidence because the catch exception parameter consumes the intervening name slot.
			// Pre-bind any slot assigned in two or more arms to its existing id BEFORE descending, so
			// every arm reuses that id (hasNamed path) instead of re-minting, and the post-try read -
			// which already carries that same id - stays consistent. Marking the id `reused` lets the
			// existing cross-scope declaration placement hoist the single `T x;` to the enclosing block.
			prebindSharedTryCatchSlots(scope, statement, className, methodName)
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

// prebindSharedTryCatchSlots unifies a local that is assigned in two or more arms of a try/catch
// (the try body and/or any catch body) onto a single VariableId, BEFORE rewriteVar descends into the
// arms. Without this, each disjoint arm re-mints its own id for the shared slot (the try arm's
// in-scope ReplaceVar never reaches the catch arm), and the post-try read keeps the slot's original
// id, so the three references disagree and javac rejects the recompiled source. By recording the
// slot's existing id in this scope's assignedMap, every arm takes the hasNamed reuse path and keeps
// that id; the post-try read already carries it, so all uses converge. The id is also added to the
// `reused` set so placeCrossScopeDeclarations hoists the single declaration to the enclosing block.
// Catch exception parameters are excluded: each is genuinely scoped to its own catch clause and must
// never be merged with an ordinary local.
func prebindSharedTryCatchSlots(scope *Scope, statement *statements.TryCatchStatement, className, methodName string) {
	exceptionUids := map[string]struct{}{}
	for _, ex := range statement.Exception {
		if ex != nil {
			exceptionUids[ex.VarUid] = struct{}{}
		}
	}
	armCount := map[string]int{}
	firstId := map[string]*utils.VariableId{}
	collectArm := func(arm []statements.Statement) {
		seen := map[string]struct{}{}
		var walk func([]statements.Statement)
		walk = func(sts []statements.Statement) {
			for _, st := range sts {
				switch s := st.(type) {
				case *statements.AssignStatement:
					if s.ArrayMember != nil {
						continue
					}
					ref, ok := core.UnpackSoltValue(s.LeftValue).(*values.JavaRef)
					if !ok || ref == nil || ref.Id == nil || ref.IsThis {
						continue
					}
					if _, isEx := exceptionUids[ref.VarUid]; isEx {
						continue
					}
					if _, dup := seen[ref.VarUid]; !dup {
						seen[ref.VarUid] = struct{}{}
						armCount[ref.VarUid]++
					}
					if _, has := firstId[ref.VarUid]; !has {
						firstId[ref.VarUid] = ref.Id
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
				case *statements.SwitchStatement:
					for _, c := range s.Cases {
						if c != nil {
							walk(c.Body)
						}
					}
				case *statements.TryCatchStatement:
					walk(s.TryBody)
					for i := range s.CatchBodies {
						walk(s.CatchBodies[i])
					}
				}
			}
		}
		walk(arm)
	}
	collectArm(statement.TryBody)
	for i := range statement.CatchBodies {
		collectArm(statement.CatchBodies[i])
	}
	for uid, cnt := range armCount {
		if cnt < 2 {
			continue
		}
		if _, already := scope.assignedMap[uid]; already {
			continue
		}
		id := firstId[uid]
		if id == nil {
			continue
		}
		scope.assignedMap[uid] = id
		scope.reused[id] = struct{}{}
		core.TraceRewriteVar(className, methodName, "prebind try/catch shared slot uid=%s id=%s arms=%d", uid, id.String(), cnt)
	}
}

// prebindEscapingIfElseSlots unifies a local that is written in BOTH arms of an if/else and then
// READ in the enclosing block after the if onto a single freshly-minted parent-scope VariableId,
// BEFORE rewriteVar descends into the arms. Without this each arm mints its OWN id for the shared JVM
// slot and the post-if reads keep the slot's original (un-minted) id, so one logical variable becomes
// three ids that happen to render the same varN; worse, because the arm mints run in child scopes that
// never advance the PARENT name counter, the NEXT sibling local reuses that very varN. The code still
// compiles (all the same primitive type) but silently reads the wrong variable - the commons-codec
// Nysiis/Metaphone shape `int kind` written in both branches and read in a following loop+return
// collapsed into the loop counter `i`, truncating every encode. Minting one parent-scope id here (a)
// reserves its var<n> name so the next sibling cannot collide, (b) makes both arm writes reuse it via
// the hasNamed path, and (c) records origId->newId so this scope's deferred ReplaceVar redirects every
// post-if read onto it. The id is marked `reused` so the declaration hoisters lift the single `T x;`
// ahead of the if. Strictly gated: both arms must assign the slot with the SAME rendered type, and the
// slot must actually be read after the if (probed by VariableId identity, not rendered name) - so
// genuinely independent per-arm slot reuses are left exactly as the baseline produced them.
// Kill-switch: JDEC_IFELSE_PREBIND_OFF=1.
func prebindEscapingIfElseSlots(scope *Scope, ifst *statements.IfStatement, afterSts []statements.Statement, idReplaceMap map[*utils.VariableId]*utils.VariableId, className, methodName string) {
	if ifst == nil || len(afterSts) == 0 || len(ifst.IfBody) == 0 || len(ifst.ElseBody) == 0 {
		return
	}
	armWrites := func(arm []statements.Statement) map[string]*values.JavaRef {
		res := map[string]*values.JavaRef{}
		var walk func([]statements.Statement)
		walk = func(sts []statements.Statement) {
			for _, st := range sts {
				switch s := st.(type) {
				case *statements.AssignStatement:
					if s.ArrayMember != nil {
						continue
					}
					ref, ok := core.UnpackSoltValue(s.LeftValue).(*values.JavaRef)
					if !ok || ref == nil || ref.Id == nil || ref.IsThis {
						continue
					}
					if _, dup := res[ref.VarUid]; !dup {
						res[ref.VarUid] = ref
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
				case *statements.SwitchStatement:
					for _, c := range s.Cases {
						if c != nil {
							walk(c.Body)
						}
					}
				case *statements.TryCatchStatement:
					walk(s.TryBody)
					for i := range s.CatchBodies {
						walk(s.CatchBodies[i])
					}
				}
			}
		}
		walk(arm)
		return res
	}
	ifWrites := armWrites(ifst.IfBody)
	if len(ifWrites) == 0 {
		return
	}
	elseWrites := armWrites(ifst.ElseBody)
	if len(elseWrites) == 0 {
		return
	}
	uids := make([]string, 0, len(ifWrites))
	for uid := range ifWrites {
		if _, ok := elseWrites[uid]; ok {
			uids = append(uids, uid)
		}
	}
	sort.SliceStable(uids, func(i, j int) bool { return varUidLess(uids[i], uids[j]) })
	for _, uid := range uids {
		if _, already := scope.assignedMap[uid]; already {
			continue
		}
		ifRef := ifWrites[uid]
		elseRef := elseWrites[uid]
		// Same rendered type in both arms: a slot reused across arms for DIFFERENT types must never
		// be merged into one declaration (that would be an uncompilable type clash); guarding on the
		// rendered type keeps such genuine slot reuses split.
		it, et := ifRef.Type(), elseRef.Type()
		if it == nil || et == nil || it.String(hoistProbeCtx) != et.String(hoistProbeCtx) {
			continue
		}
		origId := ifRef.Id
		// Must actually be read after the if (escapes the branches). Probe by IDENTITY: temporarily
		// rename origId to a sentinel and render afterSts; only a genuine reference to this id renders
		// the sentinel, so an unrelated later local sharing the same varN is excluded.
		const probe = "__jdec_ifelse_prebind_probe__"
		saved := origId.Name
		origId.SetName(probe)
		escapes := statementsReferenceName(afterSts, probe)
		origId.SetName(saved)
		if !escapes {
			continue
		}
		newId := utils.NewRootVariableId()
		newId.SetName(fmt.Sprintf("var%d", scope.nowId))
		scope.nowId++
		scope.assignedMap[uid] = newId
		scope.reused[newId] = struct{}{}
		idReplaceMap[origId] = newId
		if elseRef.Id != nil && elseRef.Id != origId {
			idReplaceMap[elseRef.Id] = newId
		}
		core.TraceRewriteVar(className, methodName, "prebind if/else escaping slot uid=%s old=%s new=%s", uid, origId.String(), newId.String())
	}
}

// prebindParallelTypedIfElseDefs unifies the dominant Bug AL fastjson2 shape that escapes
// prebindEscapingIfElseSlots: an if/else whose two arms each DECLARE a local of the SAME rendered
// type into the same JVM slot (a phi at the merge), read after the if, but whose two arm defs were
// minted with DIFFERENT VariableIds. The split is a simulation-order artifact: the DFS explores one
// arm's fall-through - and a LATER same-slot reuse of a DIFFERENT type (`long[] x` ... `long x`) -
// before backtracking to the other arm, so the slot table is clobbered to the foreign type and the
// second arm mints a fresh ref instead of reusing the first arm's. prebindEscapingIfElseSlots keys on
// a SHARED VarUid, so this cross-uid phi slips past it: both arms keep their own `T x = ...`
// declaration and the post-if read binds to only one arm's id, leaving the other arm's value unbound
// and the read out of scope ("cannot find symbol: variable varN" - fastjson2 ObjectReaderProvider's
// long[] acceptHashCodes; 357 such symbols dominate the fastjson2 whole-tree recompile).
//
// Provably-safe phi signature (all required):
//   - both arms are non-empty and EACH declares (IsFirst/IsDeclare), at its TOP level, exactly one
//     local of the candidate rendered type T (an unambiguous 1:1 pairing);
//   - the two arm defs carry DIFFERENT VarUids (a shared VarUid is already handled above);
//   - exactly ONE of them is referenced after the if (probed by VariableId identity); the other is
//     not. Two same-typed branch-EXCLUSIVE defs flowing into a common later read are, by the verifier's
//     single-type-at-merge guarantee, one source variable, so merging the non-escaping def onto the
//     escaping one is type-safe; and even were they distinct, the loser is unread after the if and
//     used only on its own exclusive arm, so the merge stays semantically inert.
//
// On a match both arm defs are bound to one freshly-minted parent-scope id (mirroring
// prebindEscapingIfElseSlots) and recorded `reused`, so placeCrossScopeDeclarations lifts the single
// `T x;` ahead of the if and the post-if reads converge on it. Kill-switch:
// JDEC_IFELSE_PARALLEL_PREBIND_OFF=1.
//
// KNOWN GAP (see CODEC_TODO Bug AL "if/else parallel-phi orphan read"): when the post-if read binds to
// a THIRD id (neither arm def's, an unconstructed phi result - fastjson2 FieldWriterListFunc.writeValue
// var10, ObjectWriters.fieldWriterList var3, JSONStreamReader{UTF8,UTF16} var2), the identity escape
// probe finds NEITHER arm escaping and this pass correctly declines (firing would orphan the read into
// var<nowId> while the read still renders var<slot>). Closing it needs name-based escape via
// statementsReadName plus a join-type for the hoisted declaration; deferred because there is no type-LUB
// facility to pick a common supertype when the two arms inferred different types.
func prebindParallelTypedIfElseDefs(scope *Scope, ifst *statements.IfStatement, afterSts []statements.Statement, idReplaceMap map[*utils.VariableId]*utils.VariableId, className, methodName string) {
	if ifst == nil || len(afterSts) == 0 || len(ifst.IfBody) == 0 || len(ifst.ElseBody) == 0 {
		return
	}
	// Collect each arm's TOP-LEVEL declarations grouped by rendered type. Only direct children are
	// considered: a declaration nested inside a sub-block of the arm (e.g. a loop) is scoped to that
	// sub-block and must never be hoisted past the if.
	collectTopDecls := func(arm []statements.Statement) map[string][]*values.JavaRef {
		res := map[string][]*values.JavaRef{}
		seen := map[string]struct{}{}
		for _, st := range arm {
			as, ok := st.(*statements.AssignStatement)
			if !ok || as.ArrayMember != nil || !(as.IsFirst || as.IsDeclare) {
				continue
			}
			ref, ok := core.UnpackSoltValue(as.LeftValue).(*values.JavaRef)
			if !ok || ref == nil || ref.Id == nil || ref.IsThis || ref.IsParam || ref.Type() == nil {
				continue
			}
			if _, dup := seen[ref.VarUid]; dup {
				continue
			}
			seen[ref.VarUid] = struct{}{}
			t := ref.Type().String(hoistProbeCtx)
			res[t] = append(res[t], ref)
		}
		return res
	}
	ifDecls := collectTopDecls(ifst.IfBody)
	if len(ifDecls) == 0 {
		return
	}
	elseDecls := collectTopDecls(ifst.ElseBody)
	if len(elseDecls) == 0 {
		return
	}
	escapesAfter := func(ref *values.JavaRef) bool {
		const probe = "__jdec_parallel_prebind_probe__"
		saved := ref.Id.Name
		ref.Id.SetName(probe)
		hit := statementsReferenceName(afterSts, probe)
		ref.Id.SetName(saved)
		return hit
	}
	typeKeys := make([]string, 0, len(ifDecls))
	for t := range ifDecls {
		typeKeys = append(typeKeys, t)
	}
	sort.Strings(typeKeys)
	for _, t := range typeKeys {
		ifs := ifDecls[t]
		els := elseDecls[t]
		// Require an unambiguous 1:1 same-type pairing across the two arms.
		if len(ifs) != 1 || len(els) != 1 {
			continue
		}
		ifRef, elseRef := ifs[0], els[0]
		if ifRef.VarUid == elseRef.VarUid {
			continue // shared VarUid is prebindEscapingIfElseSlots' job
		}
		ifEsc, elseEsc := escapesAfter(ifRef), escapesAfter(elseRef)
		if ifEsc == elseEsc {
			continue // need exactly one escaping def
		}
		winner, loser := ifRef, elseRef
		if elseEsc {
			winner, loser = elseRef, ifRef
		}
		if _, already := scope.assignedMap[winner.VarUid]; already {
			continue
		}
		if _, already := scope.assignedMap[loser.VarUid]; already {
			continue
		}
		newId := utils.NewRootVariableId()
		newId.SetName(fmt.Sprintf("var%d", scope.nowId))
		scope.nowId++
		scope.assignedMap[winner.VarUid] = newId
		scope.assignedMap[loser.VarUid] = newId
		scope.reused[newId] = struct{}{}
		idReplaceMap[winner.Id] = newId
		idReplaceMap[loser.Id] = newId
		core.TraceRewriteVar(className, methodName, "prebind parallel if/else typed def type=%s winner=%s/%s loser=%s/%s new=%s",
			t, winner.VarUid, winner.Id.String(), loser.VarUid, loser.Id.String(), newId.String())
	}
}

// prebindEscapingSwitchSlots unifies a local that is WRITTEN inside one or more switch cases and
// READ after the switch onto a single freshly-minted parent-scope VariableId, BEFORE rewriteVar
// descends into the cases. This is the switch analogue of prebindEscapingIfElseSlots /
// prebindSharedTryCatchSlots and fixes Bug AL's dominant fastjson2 shape (DateUtils' hand-unrolled
// date parser: each pattern case copies the canonical digit chars into a set of locals that are then
// validated AFTER the switch).
//
// rewriteVar processes all cases through ONE shared sub-scope, so the case stores already converge on
// a single id; the post-switch READS, however, live in the parent scope and keep the slot's original
// (pre-mint) id. The case stores' minted id and the reads' original id then disagree, so neither
// switchHoistDeclarations (identity "read after switch" probe) nor placeCrossScopeDeclarations sees
// the read - the in-case `T x = ...` declarations stay scoped to their case and the post-switch read
// is out of scope ("cannot find symbol: variable varN"). Pre-binding the slot to a parent-scope id
// here makes every case store reuse it (hasNamed path mutates the shared ref in place) and records
// origId->newId so the parent's deferred ReplaceVar redirects the reads, converging all references
// onto one id. switchHoistDeclarations / placeCrossScopeDeclarations then correctly lift the single
// `T x;` ahead of the switch.
//
// Strictly gated by slot identity: the candidate set is keyed by VarUid (one logical variable - the
// stack simulator mints a fresh ref, hence a fresh VarUid, whenever a JVM slot is reused for a
// different type), so a genuine disjoint slot reuse across cases is never merged; the slot must
// actually be referenced after the switch (probed by id identity via a sentinel rename, so an
// unrelated later local sharing the rendered varN is excluded); and a slot already bound by the
// enclosing scope before the switch (an ordinary local merely reassigned in the cases) is skipped.
// Widening a declaration's scope is always valid Java. Kill-switch: JDEC_SWITCH_PREBIND_OFF=1.
func prebindEscapingSwitchSlots(scope *Scope, sw *statements.SwitchStatement, afterSts []statements.Statement, idReplaceMap map[*utils.VariableId]*utils.VariableId, className, methodName string) {
	if sw == nil || len(afterSts) == 0 || len(sw.Cases) == 0 {
		return
	}
	caseWrites := map[string]*values.JavaRef{}
	var walk func([]statements.Statement)
	walk = func(sts []statements.Statement) {
		for _, st := range sts {
			switch s := st.(type) {
			case *statements.AssignStatement:
				if s.ArrayMember != nil {
					continue
				}
				ref, ok := core.UnpackSoltValue(s.LeftValue).(*values.JavaRef)
				if !ok || ref == nil || ref.Id == nil || ref.IsThis || ref.IsParam {
					continue
				}
				if _, dup := caseWrites[ref.VarUid]; !dup {
					caseWrites[ref.VarUid] = ref
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
			case *statements.SwitchStatement:
				for _, c := range s.Cases {
					if c != nil {
						walk(c.Body)
					}
				}
			case *statements.TryCatchStatement:
				walk(s.TryBody)
				for i := range s.CatchBodies {
					walk(s.CatchBodies[i])
				}
			}
		}
	}
	for _, c := range sw.Cases {
		if c != nil {
			walk(c.Body)
		}
	}
	if len(caseWrites) == 0 {
		return
	}
	uids := make([]string, 0, len(caseWrites))
	for uid := range caseWrites {
		uids = append(uids, uid)
	}
	sort.SliceStable(uids, func(i, j int) bool { return varUidLess(uids[i], uids[j]) })
	for _, uid := range uids {
		if _, already := scope.assignedMap[uid]; already {
			continue
		}
		ref := caseWrites[uid]
		if ref.Type() == nil {
			continue
		}
		origId := ref.Id
		// Must actually be referenced after the switch (escapes the cases). Probe by IDENTITY via a
		// sentinel rename + the normal text render: only a reference that carries origId renders the
		// sentinel, so an unrelated later local sharing the same varN is excluded.
		const probe = "__jdec_switch_prebind_probe__"
		saved := origId.Name
		origId.SetName(probe)
		escapes := statementsReferenceName(afterSts, probe)
		origId.SetName(saved)
		if !escapes {
			continue
		}
		newId := utils.NewRootVariableId()
		newId.SetName(fmt.Sprintf("var%d", scope.nowId))
		scope.nowId++
		scope.assignedMap[uid] = newId
		scope.reused[newId] = struct{}{}
		idReplaceMap[origId] = newId
		core.TraceRewriteVar(className, methodName, "prebind switch escaping slot uid=%s old=%s new=%s", uid, origId.String(), newId.String())
	}
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
			if os.Getenv("JDEC_PARALLEL_ARM_HOIST_OFF") == "" {
				for _, decl := range parallelArmDeclHoist(s, list[:i], list[i+1:]) {
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
			// Statements after the synchronized block in THIS block are where an out-of-scope read
			// would occur; pass them so only locals actually read after the block are hoisted.
			for _, decl := range syncHoistDeclarations(s, list[i+1:]) {
				out = append(out, decl)
			}
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
		// Numeric (not lexical) VarUid order: the suffix is a process-global counter whose magnitude
		// shifts every decompile, so a string compare is unstable across runs (see varUidLess).
		return varUidLess(uids[i], uids[j])
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
		// Decide "read after the switch" by VARIABLE IDENTITY, not by rendered name. JVM slot reuse
		// and rewriteVar's per-scope name counter (a switch-case sub-scope consumes a varN that the
		// parent counter never advances past) can give an UNRELATED later local the same varN. A
		// name-based test then both (a) falsely sees that later local as a read of this variable and
		// (b) trips statementsReadName's "redeclared here" short-circuit on the later local's own
		// declaration, wrongly suppressing this hoist (observed on String-switch temp slots reused
		// for an int after the switch). assignsReadAfterByIdentity probes by the assignment's own
		// VariableId, so only genuine references to THIS variable count.
		if !assignsReadAfterByIdentity(afterSts, assignsByUid[uid]) {
			continue
		}
		if name == "" {
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

// syncHoistDeclarations lifts the declaration of any local that is DECLARED inside a synchronized
// block yet READ after the block to the enclosing block. javac compiles
// `synchronized (lock) { ...; return v; }` as `monitorenter; ...; load v; monitorexit; areturn` -
// the value is computed inside the protected region, but the load/use sits between monitorexit and
// areturn, so the structurer correctly emits the trailing use as a sibling statement AFTER the
// synchronized block (SynchronizeRewriter splits the try body at monitor_exit: everything before is
// the block body, everything after - the `return v` - follows the block). The local, however, was
// declared inside the block and is now out of scope at that trailing use ("cannot find symbol").
// This is the single root cause behind guava `base`'s entire cannot-find-symbol recompile cascade
// (Enums.getEnumConstants and the ~66 units that transitively depend on it). A local declared inside
// the block and read after it is by construction one logical variable spanning both, so its `T x;`
// declaration belongs ahead of the block: the in-block declaration is demoted to a plain assignment
// and a single bare declaration is inserted before the synchronized statement. Unlike the if/switch
// hoisters this needs no >=2-assignment guard - a synchronized body is a single linear scope, so even
// one declaration read afterwards must be hoisted. Widening scope is always valid Java. Kill-switch:
// JDEC_SYNC_HOIST_OFF=1.
func syncHoistDeclarations(sync *statements.SynchronizedStatement, afterSts []statements.Statement) []statements.Statement {
	if os.Getenv("JDEC_SYNC_HOIST_OFF") != "" {
		return nil
	}
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
	collect(sync.Body)

	uids := make([]string, 0, len(assignsByUid))
	for uid := range assignsByUid {
		uids = append(uids, uid)
	}
	sort.SliceStable(uids, func(i, j int) bool {
		return varUidLess(uids[i], uids[j])
	})
	var declares []statements.Statement
	for _, uid := range uids {
		if !declaredInside[uid] {
			continue
		}
		if !assignsReadAfterByIdentity(afterSts, assignsByUid[uid]) {
			continue
		}
		targetRef := refByUid[uid]
		name := targetRef.String(hoistProbeCtx)
		if name == "" {
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
		// Numeric (not lexical) VarUid order: the suffix is a process-global counter whose magnitude
		// shifts every decompile, so a string compare is unstable across runs (see varUidLess).
		return varUidLess(uids[i], uids[j])
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

// parallelArmDeclHoist closes the "if/else parallel-phi orphan read" gap that ifHoistDeclarations
// cannot: a JVM slot reused for two DISTINCT logical variables (different VarUid) that are each
// first-declared at the top level of the two arms of one if/else, then read after the join. Because
// the two arms carry different VarUids, ifHoistDeclarations (which groups by VarUid) never pairs
// them, so each arm keeps its own `T varN = ...` declaration; the post-if read - bound by the
// decompiler to a THIRD merge id but rendered as the same slot name `varN` - then has no dominating
// declaration and javac rejects it as "cannot find symbol".
//
// The fix is purely name-based and id-free, which is sound because the dumper binds locals by their
// RENDERED NAME end to end (addMissingGeneratedLocalDecls keys on the rendered `varN` token, javac
// itself binds by name): emit ONE bare `T varN;` ahead of the if and demote both arms to plain
// `varN = ...`. The single surviving declaration makes the slot name declared, both arm assignments
// and the orphan read all render `varN` and bind to it, and the missing-decl safety net sees the
// name as declared so it injects nothing. No VariableId is touched.
//
// Declaration type T (the join / least-upper-bound problem): only merge when BOTH arms RENDER the
// same declaration type token (compared via renderedArmDeclType, NOT ref.Type() - this pass runs
// before the dumper's final RHS-driven type refinement, so an arm whose stale ref.Type() is Object
// can still render `ObjectWriter varN = objectWriterValued`). That rendered agreement is the
// decompiler's own proof that one shared `T varN;` accepts both stores and, being the slot's settled
// type, the post-if uses (e.g. ObjectWriter/ObjectWriter where the orphan read `varN.write(..)` needs
// exactly that type). The bare `T varN;` is built from whichever arm ref natively carries T so it
// renders the concrete type rather than `Object`. Genuinely different rendered types - a real
// least-upper-bound case (ParameterizedType vs ParameterizedTypeImpl, Long vs BigDecimal) - are left
// untouched: widening them to Object would break type-specific uses INSIDE the arms (a regression
// measured at fastjson2 +10), so that subfamily needs a real common-supertype facility (see CODEC_TODO).
//
// beforeSts/afterSts are the sibling statements of the if in its block; a name already declared at
// this block's top level (before or after by re-declaration) is left alone to avoid a duplicate
// declaration. Kill-switch: JDEC_PARALLEL_ARM_HOIST_OFF.
func parallelArmDeclHoist(ifst *statements.IfStatement, beforeSts, afterSts []statements.Statement) []statements.Statement {
	if ifst == nil || len(ifst.IfBody) == 0 || len(ifst.ElseBody) == 0 || len(afterSts) == 0 {
		return nil
	}
	// Top-level declares of one arm, keyed by rendered slot name. A name seen more than once in the
	// same arm is ambiguous and excluded (value nil).
	collectArm := func(arm []statements.Statement) map[string]*statements.AssignStatement {
		out := map[string]*statements.AssignStatement{}
		seen := map[string]bool{}
		for _, st := range arm {
			as, ok := st.(*statements.AssignStatement)
			if !ok || as.ArrayMember != nil || !(as.IsFirst || as.IsDeclare) {
				continue
			}
			ref, ok := core.UnpackSoltValue(as.LeftValue).(*values.JavaRef)
			if !ok || ref == nil || ref.Id == nil || ref.IsThis || ref.IsParam {
				continue
			}
			name := ref.String(hoistProbeCtx)
			if !generatedLocalNameRe.MatchString(name) {
				continue
			}
			if seen[name] {
				out[name] = nil
				continue
			}
			seen[name] = true
			out[name] = as
		}
		return out
	}
	ifDecls := collectArm(ifst.IfBody)
	elseDecls := collectArm(ifst.ElseBody)
	if len(ifDecls) == 0 || len(elseDecls) == 0 {
		return nil
	}
	names := make([]string, 0, len(ifDecls))
	for name, as := range ifDecls {
		if as == nil {
			continue
		}
		if other, ok := elseDecls[name]; ok && other != nil {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var declares []statements.Statement
	for _, name := range names {
		// A name already declared at this block's top level must not be re-declared.
		if statementsDeclareNameTopLevel(beforeSts, name) || statementsDeclareNameTopLevel(afterSts, name) {
			continue
		}
		if !statementsReadName(afterSts, name) {
			continue
		}
		ifAs, elseAs := ifDecls[name], elseDecls[name]
		ifRef, ok1 := core.UnpackSoltValue(ifAs.LeftValue).(*values.JavaRef)
		elseRef, ok2 := core.UnpackSoltValue(elseAs.LeftValue).(*values.JavaRef)
		if !ok1 || !ok2 || ifRef == nil || elseRef == nil {
			continue
		}
		// Same VarUid is ifHoistDeclarations' job; only handle the cross-variable slot reuse here.
		if ifRef.VarUid == elseRef.VarUid {
			continue
		}
		if !assignRendersAsPlain(ifAs) || !assignRendersAsPlain(elseAs) {
			continue
		}
		// The DECLARATION type must come from the RENDERED declaration, not ref.Type(): this pass runs
		// before the dumper's final RHS-driven type refinement, so a ref whose stale Type() is Object
		// can still render `ObjectWriter varN = objectWriterValued` (the dumper takes the RHS type). Only
		// merge when BOTH arms render the SAME declaration type token; that agreement is the decompiler's
		// own evidence that one shared `T varN;` is valid for both stores (and, being the slot's settled
		// type, for the post-if uses). Genuinely different rendered types (a real least-upper-bound case)
		// are left untouched - widening them to Object would break type-specific uses inside the arms.
		ifTok := renderedArmDeclType(ifAs, name)
		elseTok := renderedArmDeclType(elseAs, name)
		if ifTok == "" || ifTok != elseTok {
			continue
		}
		// Build the bare declaration from the arm ref that natively carries the agreed type, so the
		// emitted `T varN;` renders exactly T (a ref whose stale Type() is Object would render `Object varN;`).
		var declRef values.JavaValue
		if ifRef.Type().String(hoistProbeCtx) == ifTok {
			declRef = ifAs.LeftValue
		} else if elseRef.Type().String(hoistProbeCtx) == elseTok {
			declRef = elseAs.LeftValue
		} else {
			continue
		}
		ifAs.IsFirst, ifAs.IsDeclare = false, false
		elseAs.IsFirst, elseAs.IsDeclare = false, false
		declares = append(declares, statements.NewDeclareStatement(declRef))
	}
	return declares
}

// renderedArmDeclType renders the arm's declaration statement and extracts the leading type token
// (everything before " <name>"). This reflects the dumper's RHS-driven type inference, which a ref's
// pre-refinement Type() does not. Returns "" if the type cannot be isolated.
func renderedArmDeclType(as *statements.AssignStatement, name string) (tok string) {
	if as == nil {
		return ""
	}
	defer func() {
		if recover() != nil {
			tok = ""
		}
	}()
	s := as.String(hoistProbeCtx)
	idx := strings.Index(s, " "+name)
	if idx <= 0 {
		return ""
	}
	return strings.TrimSpace(s[:idx])
}

// statementsDeclareNameTopLevel reports whether name is first-declared (`T name = ...` / `T name;`)
// by a top-level statement of sts. Used to avoid emitting a second declaration for a slot already
// declared in the enclosing block.
func statementsDeclareNameTopLevel(sts []statements.Statement, name string) bool {
	for _, st := range sts {
		as, ok := st.(*statements.AssignStatement)
		if !ok || !(as.IsFirst || as.IsDeclare) {
			continue
		}
		ref, ok := core.UnpackSoltValue(as.LeftValue).(*values.JavaRef)
		if !ok || ref == nil {
			continue
		}
		if ref.String(hoistProbeCtx) == name {
			return true
		}
	}
	return false
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
// assignsReadAfterByIdentity reports whether any of the given assignment targets (all the same
// logical variable) is referenced in afterSts, comparing by VariableId IDENTITY rather than the
// rendered varN name. It temporarily renames each candidate id to a unique sentinel and reuses the
// normal text-render path: only references that actually carry that id render the sentinel, so a
// different variable sharing the same depth/scope-derived name is correctly excluded. This is the
// precise signal switchHoistDeclarations needs - "is THIS variable read after the switch" - free of
// the name-collision false positives/negatives that statementsReadName suffers. Kill-switch:
// JDEC_SWITCH_HOIST_IDENTITY_OFF=1 falls back to the legacy name-based test.
func assignsReadAfterByIdentity(afterSts []statements.Statement, assigns []*statements.AssignStatement) bool {
	if os.Getenv("JDEC_SWITCH_HOIST_IDENTITY_OFF") == "1" {
		for _, as := range assigns {
			if ref, ok := core.UnpackSoltValue(as.LeftValue).(*values.JavaRef); ok && ref != nil && ref.Id != nil {
				if statementsReadName(afterSts, ref.String(hoistProbeCtx)) {
					return true
				}
			}
		}
		return false
	}
	const probe = "__jdec_hoist_probe__"
	seen := map[*utils.VariableId]bool{}
	for _, as := range assigns {
		ref, ok := core.UnpackSoltValue(as.LeftValue).(*values.JavaRef)
		if !ok || ref == nil || ref.Id == nil || seen[ref.Id] {
			continue
		}
		seen[ref.Id] = true
		id := ref.Id
		saved := id.Name
		id.SetName(probe)
		hit := statementsReferenceName(afterSts, probe)
		id.SetName(saved)
		if hit {
			return true
		}
	}
	return false
}

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
// crossScopeRefUid returns a JavaRef's stable VarUid (a deterministic parse-order counter) for use
// as a sort tie-breaker; non-ref values fall back to the empty string.
func crossScopeRefUid(v values.JavaValue) string {
	if ref, ok := core.UnpackSoltValue(v).(*values.JavaRef); ok && ref != nil {
		return ref.VarUid
	}
	return ""
}

// varUidNum extracts the numeric suffix of a VarUid ("ref-N"); unparseable uids sort first as -1.
func varUidNum(u string) int64 {
	if i := strings.LastIndexByte(u, '-'); i >= 0 {
		if n, err := strconv.ParseInt(u[i+1:], 10, 64); err == nil {
			return n
		}
	}
	return -1
}

// varUidLess orders two VarUids by their NUMERIC suffix, not lexically. VarUid is "ref-N" where N is
// a process-global monotonic counter, so a lexical compare is both wrong ("ref-9" > "ref-10") and -
// critically - unstable: because the counter is shared across every decompile, the absolute N values
// of two refs shift run to run, and a lexical compare flips whenever the counter crosses a digit-width
// boundary (e.g. 998 vs 1004) between their creation. That flipped which of two same-named locals was
// declared first and so swapped their var2 / var2_1 names nondeterministically. The numeric suffix is
// invariant to the base and reflects the stable per-decompile creation order.
func varUidLess(a, b string) bool {
	na, nb := varUidNum(a), varUidNum(b)
	if na != nb {
		return na < nb
	}
	return a < b
}

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

// topLevelDeclDominatesAllUses reports whether the CURRENT declaration placement of id keeps every
// reference to id lexically in scope, i.e. each use is preceded by a declaration of id in its own
// block or an enclosing one. isDeclaredAtTopLevel only asks whether SOME top-level declaration
// EXISTS, which is too weak: a reused JVM slot can have one source local declared inside an if's
// else-arm (range B) and a disjoint later one re-declared `int var4 = ntz;` at the block top level
// (range C, AFTER the if). The later declaration does not dominate the earlier sibling-arm use, so
// that use is out of scope ("cannot find symbol") and the variable must still be hoisted. The check
// must equally NOT hoist the dual shape where each scope already declares id for itself (e.g. VarFold:
// `if(...){int var1=1; ...} int var1=2; ...` -- two disjoint scopes, both legal Java); a naive "is id
// referenced before its top-level decl" test wrongly fires there because the if-arm references var1,
// even though that arm self-declares it. blockHasUncoveredRef threads a scope-aware "declared so far"
// flag and recurses, so a self-declaring child scope is covered while a bare sibling-arm use is not.
// References are matched by id IDENTITY (a sentinel rename reusing the normal render path) so a
// same-named but distinct reused slot can never produce a false positive. Kill-switch:
// JDEC_NO_CROSS_SCOPE_DOMINATE restores the existence-only test.
func topLevelDeclDominatesAllUses(list []statements.Statement, id *utils.VariableId) bool {
	if id == nil {
		return true
	}
	const probe = "__jdec_dom_probe__"
	saved := id.Name
	id.SetName(probe)
	defer id.SetName(saved)
	return !blockHasUncoveredRef(list, id, probe, false)
}

// blockHasUncoveredRef reports whether `list` contains a reference to id that is lexically OUT OF
// SCOPE: not preceded by a declaration of id in this list or an enclosing block. `declaredOut` says
// id is already declared by an enclosing block before this list begins. The walk threads a per-scope
// "declared so far" flag: a simple `T id [= ...]` declaration turns it on for the rest of THIS list,
// and every nested child block inherits the flag value as of the point it appears -- so a sibling-arm
// use whose only declaration sits in the OTHER arm stays uncovered (must hoist), while a use inside a
// child that declares id itself is covered (must NOT hoist; the VarFold dual-scope shape). Declaration
// statements are matched by id identity and never counted as uses. Compound statements are recursed
// into instead of name-matched whole, so an inner self-declaring scope never makes its enclosing
// if/loop look like an uncovered use. A compound statement's own head (condition/selector) is not
// separately inspected here: that conservative miss can only fail to hoist (matching the prior
// existence-only behaviour, never a new over-hoist), so it cannot regress already-valid output.
func blockHasUncoveredRef(list []statements.Statement, id *utils.VariableId, name string, declaredOut bool) bool {
	declared := declaredOut
	for _, st := range list {
		decl := false
		if as, ok := st.(*statements.AssignStatement); ok && as.ArrayMember == nil && (as.IsFirst || as.IsDeclare) {
			if ref, ok2 := core.UnpackSoltValue(as.LeftValue).(*values.JavaRef); ok2 && ref != nil && ref.Id == id {
				decl = true
			}
		}
		if decl {
			declared = true
			continue
		}
		children := childStatementLists(st)
		if len(children) == 0 {
			if !declared && statementsReferenceName([]statements.Statement{st}, name) {
				return true
			}
			continue
		}
		for _, cl := range children {
			if blockHasUncoveredRef(*cl, id, name, declared) {
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
			ni := refByID[ids[i]].String(hoistProbeCtx)
			nj := refByID[ids[j]].String(hoistProbeCtx)
			if ni != nj {
				return ni < nj
			}
			// Two distinct reused slots can collapse to the SAME generated name (e.g. both render
			// `var2` because they occupy the same JVM slot depth in sibling switch cases). The name
			// tie left the relative order to collectGeneratedLocalDeclIDs's lexical walk, but the
			// dumper's collision renamer keeps whichever declaration is emitted FIRST as `var2` and
			// renames the other to `var2_1`. To make WHICH local keeps the bare name deterministic,
			// break the tie on the stable per-slot VarUid (a parse-order counter) compared NUMERICALLY
			// (see varUidLess) instead of relying on iteration-sensitive discovery order. Distinct-name
			// locals never reach this branch, so byte-for-byte output for the common case is unchanged.
			return varUidLess(crossScopeRefUid(refByID[ids[i]]), crossScopeRefUid(refByID[ids[j]]))
		})
		var hoisted []statements.Statement
		for _, id := range ids {
			ref := refByID[id]
			name := ref.String(hoistProbeCtx)
			if name == "" || !generatedLocalNameRe.MatchString(name) {
				continue
			}
			if isDeclaredAtTopLevel(list, id) {
				// Only leave it alone when that top-level declaration actually dominates every use;
				// a later disjoint live-range re-declaration does not, and the earlier sibling use
				// would otherwise stay out of scope. Kill-switch restores the existence-only skip.
				if os.Getenv("JDEC_NO_CROSS_SCOPE_DOMINATE") != "" || topLevelDeclDominatesAllUses(list, id) {
					continue
				}
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
