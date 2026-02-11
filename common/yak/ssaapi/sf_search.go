package ssaapi

import (
	"fmt"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// searchMembersWithOverlay 通过 overlay 跨 layer 查找对象的成员
// 返回一个 map，key 是成员名称，value 是成员值
// 上层 layer 的成员会覆盖下层 layer 的同名成员
func searchMembersWithOverlay(value *Value, overlay *ProgramOverLay) map[string]*Value {
	memberMap := make(map[string]*Value)
	if overlay == nil || len(overlay.Layers) == 0 {
		return memberMap
	}

	// 首先尝试直接使用当前 value 的 instruction 来获取成员
	// 如果当前 value 的 instruction 有成员，直接使用（这是最快的路径）
	currentInst := value.getValue()
	if currentInst != nil {
		currentAllMember := currentInst.GetAllMember()
		for k, v := range currentAllMember {
			keyName := k.String()
			if keyName == "" {
				continue
			}
			// 创建新的 Value，使用当前 value 的 ParentProgram
			newValVal, err := value.ParentProgram.NewValue(v)
			if err == nil && newValVal != nil {
				memberMap[keyName] = newValVal
			}
		}
	}

	// 如果当前 instruction 没有成员，或者需要跨 layer 查找，则通过名称查找
	// 获取当前 value 的名称，用于在所有 layer 中查找相同类型的值
	valueName := value.GetName()
	if valueName == "" {
		valueName = value.String()
	}

	// 从所有 layer 中查找成员，上层覆盖下层
	// 从最上层开始遍历，这样上层的成员会自动覆盖下层的同名成员
	for i := len(overlay.Layers) - 1; i >= 0; i-- {
		layer := overlay.Layers[i]
		if layer == nil || layer.Program == nil {
			continue
		}

		// 在当前 layer 的 Program 中查找相同名称的值
		layerValues := layer.Program.Ref(valueName)
		if len(layerValues) == 0 {
			continue
		}

		// 对同一 layer 中的多个 Ref 结果去重：只处理第一个匹配的对象值
		// 因为同一 layer 中可能有多个同名但不同类型的值，我们只需要对象类型的值
		var targetLayerValue *Value
		for _, layerValue := range layerValues {
			if layerValue.IsObject() {
				targetLayerValue = layerValue
				break // 只取第一个匹配的对象值
			}
		}

		if targetLayerValue == nil {
			continue
		}

		layerInst := targetLayerValue.getValue()
		if layerInst == nil {
			continue
		}

		// 获取该值的所有成员
		layerAllMember := layerInst.GetAllMember()
		for k, v := range layerAllMember {
			keyName := k.String()
			if keyName == "" {
				continue
			}

			// 上层覆盖下层：如果成员已存在，跳过（保持上层优先）
			// 由于我们从上层向下遍历，已存在的成员一定是上层的，所以直接跳过
			if _, exists := memberMap[keyName]; exists {
				continue
			}

			// 创建新的 Value，使用当前 layer 的 Program
			newValVal, err := layer.Program.NewValue(v)
			if err == nil && newValVal != nil {
				memberMap[keyName] = newValVal
			}
		}
	}

	return memberMap
}

type userNodeItems struct {
	names  []string
	values ssa.Values
}

func SearchWithCFG(value *Value, mod int, compare func(string) bool, opt ...sfvm.AnalysisContextOption) Values {
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
			// log.Errorf("instruction type: %T", inst)

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
	allMember := inst.GetAllMember()
	for k, v := range allMember {
		if check(value.NewValue(k)) {
			add(value.NewValue(v))
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
		// 只有当 overlay 存在且至少有 2 个 layer 时，才考虑使用 overlay 逻辑
		if overlay != nil && len(overlay.Layers) >= 2 {
			isFromOverlay := false
			for _, layer := range overlay.Layers {
				if layer != nil && layer.Program != nil && layer.Program == value.ParentProgram {
					isFromOverlay = true
					break
				}
			}

			// 只有当 value 来自 overlay 的查询时，才使用 overlay 逻辑
			if isFromOverlay {
				// 通过 overlay 跨 layer 查找成员
				memberMap := searchMembersWithOverlay(value, overlay)
				// 检查所有聚合的成员
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

func SearchWithValue(value *Value, mod int, compare func(string) bool, opt ...sfvm.AnalysisContextOption) Values {
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

		if key := value.GetKey(); key != nil {
			keyName := fmt.Sprint(key.GetConstValue())
			if keyName != "" && compare(keyName) {
				return true
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
