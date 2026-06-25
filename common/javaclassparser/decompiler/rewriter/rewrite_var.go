package rewriter

import (
	"fmt"
	"maps"
	"regexp"
	"sort"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
)

func RewriteVar(sts *[]statements.Statement, startVarId int, params []*values.JavaRef) {
	scope := NewScope(startVarId, sts)
	for _, v := range params {
		scope.assignedMap[v.VarUid] = v.Id
	}
	rewriteVar(scope)
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
}

type Scope struct {
	nowId       int
	deep        int
	sts         *[]statements.Statement
	varMap      []any
	assignedMap map[string]*utils.VariableId
}

func NewScope(startId int, sts *[]statements.Statement) *Scope {
	return &Scope{
		nowId:       startId,
		sts:         sts,
		assignedMap: map[string]*utils.VariableId{},
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
	}
	s.varMap = append(s.varMap, newScope)
	return newScope
}
// maxStructuredContainers bounds how many nested-block container statements a single method's
// structured tree may contain. It backstops AssertStatementsAcyclic's traversal against a degenerate
// (but technically acyclic) explosion of nested blocks.
const maxStructuredContainers = 1_000_000

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
	stack := append([]statements.Statement{}, roots...)
	count := 0
	for len(stack) > 0 {
		st := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
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
		if _, ok := visited[st]; ok {
			panic(fmt.Errorf("cyclic or shared container statement (%T) in structured tree; rejecting to avoid unbounded recursion", st))
		}
		visited[st] = struct{}{}
		count++
		if count > maxStructuredContainers {
			panic(fmt.Errorf("structured statement tree has more than %d container nodes; rejecting as pathological", maxStructuredContainers))
		}
		for _, body := range children {
			stack = append(stack, body...)
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

func rewriteVar(scope *Scope) int {
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
				}
			}
			if hasNamed {
				ref := statement.LeftValue.(*values.JavaRef)
				id, _ := scope.assignedMap[ref.VarUid]
				ref.Id = id
				scope.varMap = append(scope.varMap, statement)
			}
		case *statements.IfStatement:
			subScope := scope.SubScope(&statement.IfBody)
			rewriteVar(subScope)
			subScope = scope.SubScope(&statement.ElseBody)
			rewriteVar(subScope)
		case *statements.ForStatement:
			subScope := scope.SubScope(&statement.SubStatements)
			rewriteVar(subScope)
		case *statements.WhileStatement:
			subScope := scope.SubScope(&statement.Body)
			rewriteVar(subScope)
		case *statements.DoWhileStatement:
			subScope := scope.SubScope(&statement.Body)
			rewriteVar(subScope)
		case *statements.SwitchStatement:
			subScope := scope.SubScope(nil)
			for _, c := range statement.Cases {
				subScope.sts = &c.Body
				rewriteVar(subScope)
			}
		case *statements.TryCatchStatement:
			subScope := scope.SubScope(&statement.TryBody)
			rewriteVar(subScope)
			for _, c := range statement.CatchBodies {
				subScope = scope.SubScope(&c)
				rewriteVar(subScope)
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
	declaredInside := map[*utils.VariableId]bool{}
	assignsByUid := map[*utils.VariableId][]*statements.AssignStatement{}
	refByUid := map[*utils.VariableId]values.JavaValue{}
	for _, c := range sw.Cases {
		for _, st := range c.Body {
			as, ok := st.(*statements.AssignStatement)
			if !ok || as.ArrayMember != nil {
				continue
			}
			ref, ok := as.LeftValue.(*values.JavaRef)
			if !ok {
				continue
			}
			assignsByUid[ref.Id] = append(assignsByUid[ref.Id], as)
			refByUid[ref.Id] = as.LeftValue
			if as.IsFirst || as.IsDeclare {
				declaredInside[ref.Id] = true
			}
		}
	}
	uids := make([]*utils.VariableId, 0, len(assignsByUid))
	for uid := range assignsByUid {
		uids = append(uids, uid)
	}
	sort.SliceStable(uids, func(i, j int) bool {
		return uids[i].String() < uids[j].String()
	})
	var declares []statements.Statement
	for _, uid := range uids {
		if !declaredInside[uid] {
			continue
		}
		name := refByUid[uid].String(hoistProbeCtx)
		if name == "" || !statementsReferenceName(afterSts, name) {
			continue
		}
		for _, as := range assignsByUid[uid] {
			as.IsFirst = false
			as.IsDeclare = false
		}
		declares = append(declares, statements.NewDeclareStatement(refByUid[uid]))
	}
	return declares
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
