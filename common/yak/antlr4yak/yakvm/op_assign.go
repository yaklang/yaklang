package yakvm

import (
	"fmt"
)

func (v *Frame) assign(left, right *Value) {
	if !(left.IsValueList() && right.IsValueList()) {
		panic("BUG: assign: left and right must be value list")
	}

	leftValueList := left.ValueList()
	rightValueList := right.ValueList()

	if len(leftValueList) <= 0 || len(rightValueList) <= 0 {
		return
	}

	// 最基础的情况，左边是一个值，右边是 n 个值，右边的可以直接全部给左边
	if len(leftValueList) == 1 {
		left := leftValueList[0]
		if len(rightValueList) == 1 {
			// 左一右一最简单赋值
			left.Assign(v, rightValueList[0])
		} else {
			// 左一右N
			left.Assign(v, NewValues(rightValueList))
			//tbl, err := v.RootSymbolTable.FindSymbolTableBySymbolId(leftValueList[0].SymbolId)
			//if err != nil {
			//	panic(err)
			//}
			//leftValueList[0].AssignBySymbol(tbl, right.ValueListToInterface())
		}
		return
	}

	// 左边是 n 个值，右边是 1 个值，右边要拆开，类型需要时 Array 和 Slice
	if len(rightValueList) == 1 {
		right := rightValueList[0]
		if !right.IsIterable() {
			panic("multi-assign failed: right value is not iterable")
		}
		rightValueLen := right.Len()
		if rightValueLen != len(leftValueList) {
			panic(fmt.Sprintf("multi-assign failed: left value length[%d] != right value length[%d]", len(leftValueList), rightValueLen))
		}
		for index, val := range leftValueList {
			val.Assign(v, NewValue("__assign_middle__", right.CallSliceIndex(index), ""))
			//data := right.CallSliceIndex(index)
			//tbl, err := v.RootSymbolTable.FindSymbolTableBySymbolId(leftValueList[index].SymbolId)
			//if err != nil {
			//	panic(err)
			//}
			//val.AssignBySymbol(tbl, data)
		}
		return
	}

	// 左边是 n 个值，右边是 m 个值，都大于一，那么，必须相等才可以，否则挂掉
	if len(rightValueList) != len(leftValueList) {
		panic("multi-assign failed: left value length[" + fmt.Sprint(len(leftValueList)) + "] != right value length[" + fmt.Sprint(len(rightValueList)) + "]")
	}

	for i := 0; i < len(rightValueList); i++ {
		leftValueList[i].Assign(v, rightValueList[i])
		//leftValue := leftValueList[i]
		//tbl, err := v.RootSymbolTable.FindSymbolTableBySymbolId(leftValue.SymbolId)
		//if err != nil {
		//	panic(err)
		//}
		//leftValue.AssignBySymbol(tbl, rightValueList[i].Value)
	}
}

func (v *Frame) luaLocalAssign(leftPart, rightPart *Value) {
	if !(leftPart.IsValueList() && rightPart.IsValueList()) {
		panic("BUG: assign: left and right must be value list")
	}

	leftValueList := leftPart.ValueList()
	rightValueList := rightPart.ValueList()

	if len(leftValueList) <= 0 || len(rightValueList) <= 0 {
		return
	}

	// 右边只有一个值 这种情况就不存在覆盖的情况
	if len(rightValueList) == 1 {
		right := rightValueList[0]
		if right.Value == nil {
			for _, left := range leftValueList {
				left.Assign(v, right)
			}
			return
		}
		// 在lua中table在赋值时直接会被赋值 属于不可迭代 对应table实现用的是map 也是不可迭代 正好处理了这种情况
		if !right.IsIterable() {
			for index, left := range leftValueList {
				if index == 0 {
					left.Assign(v, right)
					continue
				}
				left.Assign(v, undefined)
			}
			return
		}
		// 右侧可迭代 考虑其值个数
		rightValueLen := right.Len()
		leftValueLen := len(leftValueList)
		if rightValueLen != leftValueLen { // 左右不等了 这时候就得看情况忽略值或者补nil
			if rightValueLen > leftValueLen {
				if _, ok := right.Value.([]interface{}); ok {
					right.Value = right.Value.([]interface{})[:len(leftValueList)]
				} else {
					right.Value = right.Value.([]*Value)[:len(leftValueList)]
				}
			} else {
				for i := 0; i < leftValueLen-rightValueLen; i++ {
					if _, ok := right.Value.([]interface{}); ok {
						right.Value = append(right.Value.([]interface{}), nil)
					} else {
						right.Value = append(right.Value.([]*Value), undefined)
					}
				}
			}
		}
		for index, left := range leftValueList {
			if _, ok := right.CallSliceIndex(0).(*Value); ok {
				left.Assign(v, NewValue("__assign_middle__", right.CallSliceIndex(index).(*Value).Value, ""))
			} else {
				left.Assign(v, NewValue("__assign_middle__", right.CallSliceIndex(index), ""))
			}
		}
		return
	}

	// 左右均存在多个值 这时会有类似绑定赋值的操作
	// 此时左右一一对应 左右拆开后 类似于 len(leftValueList) == 1的情况

	leftLen, rightLen := len(leftValueList), len(rightValueList)

	if leftLen != rightLen {
		if rightLen > leftLen {
			rightValueList = rightValueList[:len(leftValueList)]
		} else {
			for i := 0; i < leftLen-rightLen; i++ {
				rightValueList = append(rightValueList, undefined)
			}
		}
	}

	for index := 0; index < len(leftValueList); index++ {
		left := leftValueList[index]
		right := rightValueList[index]
		if right.Value == nil {
			left.Assign(v, right)
			continue
		}
		// 这里有个小坑 reflect.TypeOf(nil).Kind() 会异常 所以先处理右边存在nil的情况
		if right.IsIterable() {
			if _, ok := right.CallSliceIndex(0).(*Value); ok {
				left.Assign(v, NewValue("__assign_middle__", right.CallSliceIndex(0).(*Value).Value, ""))
			} else {
				left.Assign(v, NewValue("__assign_middle__", right.CallSliceIndex(0), ""))
			}
			continue
		} else { // 右侧不可迭代
			// 左一右一最简单赋值
			left.Assign(v, right)
			continue
		}
	}
}

func (v *Frame) luaGlobalAssign(leftPart, rightPart *Value) {
	if !(leftPart.IsValueList() && rightPart.IsValueList()) {
		panic("BUG: assign: left and right must be value list")
	}

	leftValueList := leftPart.ValueList()
	rightValueList := rightPart.ValueList()

	if len(leftValueList) <= 0 || len(rightValueList) <= 0 {
		return
	}

	//多赋值有点复杂 这里分几种情况
	//左边只有一个值 算是多赋值的一种特例 特殊处理
	//if len(leftValueList) == 1 {
	//	left := leftValueList[0]
	//	right := rightValueList[0]
	//	if right.Value == nil {
	//		left.GlobalAssign(v, right)
	//		return
	//	}
	//	// 这里有个小坑 reflect.TypeOf(nil).Kind() 会异常 所以先处理右边存在nil的情况
	//	if right.IsIterable() {
	//		if _, ok := right.CallSliceIndex(0).(*Value); ok { // 这里是检测函数返回值是一个函数的情况
	//			left.GlobalAssign(v, NewValue("__assign_middle__", right.CallSliceIndex(0).(*Value).Value, ""))
	//		} else {
	//			left.GlobalAssign(v, NewValue("__assign_middle__", right.CallSliceIndex(0), ""))
	//		}
	//		return
	//	} else { // 右侧不可迭代
	//		// 左一右一最简单赋值
	//		left.GlobalAssign(v, right)
	//		return
	//	}
	//}

	// 右边只有一个值 这种情况就不存在覆盖的情况
	if len(rightValueList) == 1 {
		right := rightValueList[0]
		if right.Value == nil {
			for _, left := range leftValueList {
				left.GlobalAssign(v, right)
			}
			return
		}
		// 在lua中table在赋值时直接会被赋值 属于不可迭代 对应table实现用的是map 也是不可迭代 正好处理了这种情况
		if !right.IsIterable() {
			for index, left := range leftValueList {
				if index == 0 {
					left.GlobalAssign(v, right)
					continue
				}
				left.GlobalAssign(v, undefined)
			}
			return
		}
		// 右侧可迭代 考虑其值个数
		rightValueLen := right.Len()
		leftValueLen := len(leftValueList)
		if rightValueLen != leftValueLen { // 左右不等了 这时候就得看情况忽略值或者补nil
			if rightValueLen > leftValueLen {
				if _, ok := right.Value.([]interface{}); ok {
					right.Value = right.Value.([]interface{})[:len(leftValueList)]
				} else {
					right.Value = right.Value.([]*Value)[:len(leftValueList)]
				}
			} else {
				for i := 0; i < leftValueLen-rightValueLen; i++ {
					if _, ok := right.Value.([]interface{}); ok {
						right.Value = append(right.Value.([]interface{}), nil)
					} else {
						right.Value = append(right.Value.([]*Value), undefined)
					}
				}
			}
		}
		for index, left := range leftValueList {
			if _, ok := right.CallSliceIndex(0).(*Value); ok {
				left.GlobalAssign(v, NewValue("__assign_middle__", right.CallSliceIndex(index).(*Value).Value, ""))
			} else {
				left.GlobalAssign(v, NewValue("__assign_middle__", right.CallSliceIndex(index), ""))
			}
		}
		return
	}

	// 左右均存在多个值 这时会有类似绑定赋值的操作
	// 此时左右一一对应 左右拆开后 类似于 len(leftValueList) == 1的情况

	leftLen, rightLen := len(leftValueList), len(rightValueList)

	if leftLen != rightLen {
		if rightLen > leftLen {
			rightValueList = rightValueList[:len(leftValueList)]
		} else {
			for i := 0; i < leftLen-rightLen; i++ {
				rightValueList = append(rightValueList, undefined)
			}
		}
	}

	for index := 0; index < len(leftValueList); index++ {
		left := leftValueList[index]
		right := rightValueList[index]
		if right.Value == nil {
			left.GlobalAssign(v, right)
			continue
		}
		// 这里有个小坑 reflect.TypeOf(nil).Kind() 会异常 所以先处理右边存在nil的情况
		if right.IsIterable() {
			if _, ok := right.CallSliceIndex(0).(*Value); ok {
				left.GlobalAssign(v, NewValue("__assign_middle__", right.CallSliceIndex(0).(*Value).Value, ""))
			} else {
				left.GlobalAssign(v, NewValue("__assign_middle__", right.CallSliceIndex(0), ""))
			}
			continue
		} else { // 右侧不可迭代
			// 左一右一最简单赋值
			left.GlobalAssign(v, right)
			continue
		}
	}
}
