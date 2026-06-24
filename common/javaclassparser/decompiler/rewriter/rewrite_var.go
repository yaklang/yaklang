package rewriter

import (
	"fmt"
	"maps"
	"sort"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
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
func rewriteVar(scope *Scope) int {
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
