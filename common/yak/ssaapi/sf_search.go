package ssaapi

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// searchMembersWithOverlay 通过 overlay 跨 layer 查找对象的成员
// 返回一个 map，key 是成员名称，value 是成员值
// 上层 layer 的成员会覆盖下层 layer 的同名成员
//
// Member lookup uses per-layer symbol Ref (cheap). Upper layers use full Ref so
// extern/lib symbols (e.g. Runtime bound to an import file) are not dropped by
// include filters; base uses exclude of overridden files.
func searchMembersWithOverlay(value *Value, overlay *ProgramOverLay) map[string]*Value {
	memberMap := make(map[string]*Value)
	if overlay == nil || overlay.Base == nil {
		return memberMap
	}

	currentInst := value.getValue()
	if currentInst != nil {
		for _, pair := range ssa.GetLastWinsMemberPairs(currentInst) {
			keyName := pair.KeyString()
			if keyName == "" {
				continue
			}
			newValVal, err := value.ParentProgram.NewValue(pair.Member)
			if err == nil && newValVal != nil {
				memberMap[keyName] = newValVal
			}
		}
	}

	if len(memberMap) > 0 && value.ParentProgram != nil {
		filePath := ""
		if rng := value.GetRange(); rng != nil {
			if ed := rng.GetEditor(); ed != nil {
				filePath = ed.GetFilePath()
				if filePath == "" {
					filePath = ed.GetUrl()
				}
			}
		}
		if filePath != "" {
			normalized := normalizeOverlayFilePath(filePath, value.ParentProgram.GetProgramName())
			fromBase, di, ok := overlay.valueSource(value)
			if ok {
				if fromBase {
					if !overlay.IsExcludedPath(normalized) {
						return memberMap
					}
				} else if ownerDi, owned := overlay.ownerDiffIndex(normalized); owned && ownerDi == di {
					return memberMap
				}
			}
		}
	}

	valueName := value.GetName()
	if valueName == "" {
		valueName = value.String()
	}
	if valueName == "" {
		return memberMap
	}

	collectMembers := func(prog *Program, layerValues Values) {
		if prog == nil || len(layerValues) == 0 {
			return
		}
		var targetLayerValue *Value
		for _, layerValue := range layerValues {
			if layerValue != nil && layerValue.IsObject() {
				targetLayerValue = layerValue
				break
			}
		}
		if targetLayerValue == nil {
			return
		}
		layerInst := targetLayerValue.getValue()
		if layerInst == nil {
			return
		}
		for _, pair := range ssa.GetLastWinsMemberPairs(layerInst) {
			keyName := pair.KeyString()
			if keyName == "" {
				continue
			}
			if _, exists := memberMap[keyName]; exists {
				continue
			}
			newValVal, err := prog.NewValue(pair.Member)
			if err == nil && newValVal != nil {
				memberMap[keyName] = newValVal
			}
		}
	}

	for i := len(overlay.Diff) - 1; i >= 0; i-- {
		layer := overlay.Diff[i]
		if layer == nil || layer.Program == nil {
			continue
		}
		collectMembers(layer.Program, layer.Program.Ref(valueName))
	}
	if overlay.Base != nil {
		exclude := overlay.excludeFiles()
		var layerValues Values
		if len(exclude) > 0 {
			layerValues = overlay.Base.refWithExcludeFiles(valueName, exclude)
		} else {
			layerValues = overlay.Base.Ref(valueName)
		}
		collectMembers(overlay.Base, layerValues)
	}

	return memberMap
}

type userNodeItems struct {
	names  []string
	values ssa.Values
}

func SearchWithCFG(value *Value, mod ssadb.MatchMode, compare func(string) bool, opt ...sfvm.AnalysisContextOption) Values {
	var newValue Values
	inst := value.getUser()
	if utils.IsNil(inst) {
		return newValue
	}

	items := []*userNodeItems{}
	addItems := func(names []string, value ...int64) {
		items = append(items, &userNodeItems{
			names:  names,
			values: inst.GetValuesByIDs(value),
		})
	}

	var searchInstructionCFG func(ssa.Instruction)
	searchInstructionCFG = func(inst ssa.Instruction) {
		switch inst := inst.(type) {
		case *ssa.Function:
			addItems([]string{"throws"}, inst.Throws...)
		case *ssa.ErrorHandler:
			addItems([]string{"catch"}, inst.Catch...)
			addItems([]string{"finally"}, inst.Final)
			addItems([]string{"try"}, inst.Try)
			addItems([]string{"final"}, inst.Final)
		case *ssa.ErrorCatch:
			addItems([]string{"body"}, inst.CatchBody)
			addItems([]string{"exception"}, inst.Exception)
		case *ssa.LazyInstruction:
			searchInstructionCFG(inst.Self())
		default:
		}
	}
	searchInstructionCFG(inst)

	add := func(vvs ...ssa.Value) {
		for _, vv := range vvs {
			if utils.IsNil(vv) {
				continue
			}
			v := value.NewValue(vv)
			v.AppendPredecessor(value, opt...)
			newValue = append(newValue, v)
		}
	}
	for _, item := range items {
		for _, name := range item.names {
			if compare(name) {
				add(item.values...)
			}
		}
	}
	return newValue

}

// searchMembersFromInst 从 SSA instruction 中查找成员
func searchMembersFromInst(value *Value, inst ssa.Value, check func(*Value) bool, add func(*Value)) {
	for _, pair := range ssa.GetMemberPairs(inst) {
		if check(value.NewValue(pair.Key)) {
			add(value.NewValue(pair.Member))
		}
	}
}

// searchMembersInKeyMatchMode 在 KeyMatch 模式下查找对象的成员
func searchMembersInKeyMatchMode(value *Value, inst ssa.Value, check func(*Value) bool, add func(*Value)) {
	if !value.IsObject() {
		return
	}

	searchMembersFromInst(value, inst, check, add)

	if value.ParentProgram != nil && value.ParentProgram.overlay != nil {
		overlay := value.ParentProgram.GetOverlay()
		if overlay != nil && overlay.Base != nil && len(overlay.Diff) > 0 {
			isFromOverlay := overlay.Base == value.ParentProgram
			if !isFromOverlay {
				for _, layer := range overlay.Diff {
					if layer != nil && layer.Program == value.ParentProgram {
						isFromOverlay = true
						break
					}
				}
			}
			if isFromOverlay {
				memberMap := searchMembersWithOverlay(value, overlay)
				for keyName, memberVal := range memberMap {
					keyVal := value.NewValue(ssa.NewConst(keyName))
					if check(keyVal) {
						add(memberVal)
					}
				}
			}
		}
	}
}

func SearchWithValue(value *Value, mod ssadb.MatchMode, compare func(string) bool, opt ...sfvm.AnalysisContextOption) Values {
	var newValue Values

	inst := value.getValue()
	if utils.IsNil(inst) {
		return newValue
	}

	add := func(v *Value) {
		v.AppendPredecessor(value, opt...)
		newValue = append(newValue, v)
	}

	check := func(value *Value) bool {
		if compare(value.GetName()) || compare(value.String()) {
			return true
		}

		if value.IsConstInst() && compare(codec.AnyToString(value.GetConstValue())) {
			return true
		}

		for name := range value.GetAllVariables() {
			if compare(name) {
				return true
			}
		}

		if raw := value.getValue(); raw != nil {
			for _, pair := range ssa.GetObjectKeyPairs(raw) {
				keyName := pair.KeyString()
				if keyName != "" && compare(keyName) {
					return true
				}
			}
		}

		return false
	}

	if mod&ssadb.ConstType != 0 {
		if check(value) {
			add(value)
		}
	}

	if mod&ssadb.NameMatch != 0 {
		if check(value) {
			add(value)
		}
	}

	if mod&ssadb.KeyMatch != 0 {
		// 查找对象的成员
		searchMembersInKeyMatchMode(value, inst, check, add)

		// 处理 FlatOccultation
		for _, ov := range inst.FlatOccultation() {
			searchMembersFromInst(value, ov, check, add)
		}
	}

	return newValue
}
