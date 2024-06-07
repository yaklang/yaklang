package ssaapi

import "github.com/yaklang/yaklang/common/utils/omap"

// FindFlexibleDependsIntersection searches for intersections between flexible dependencies
// of the root collection and elements from the target collection, returning matched elements.
//
// FindFlexibleDependsIntersection 搜索根集合中的灵活依赖与目标集合中元素的交集，并返回匹配的元素。
//
// 这个函数是 ExtractTopDefsIntersection 的一个封装，专门用于处理灵活依赖关系。
// This function is a wrapper around ExtractTopDefsIntersection, specifically tailored for
// handling flexible dependencies.
//
// 它接收三个参数：root（根集合），element（目标集合），以及可选的 opts（操作选项）。
// It takes three parameters: root (the root collection), element (the target collection),
// and optionally opts (operation options).
//
// 通过将灵活依赖的特定处理逻辑传递给 ExtractTopDefsIntersection，该函数利用已有的逻辑
// 来检查和返回交集元素。
// By passing specific handling logic for flexible dependencies to ExtractTopDefsIntersection,
// this function leverages existing logic to check and return intersecting elements.
//
// 使用此函数可以灵活地处理不同类型的依赖关系，如在计算或数据分析场景中常见的依赖查找。
// Using this function allows flexible handling of different types of dependencies,
// commonly seen in computing or data analysis scenarios.
func FindFlexibleDependsIntersection(root Values, element Values, opts ...OperationOption) Values {
	return root.ExtractTopDefsIntersection(element, opts...)
}

// ExtractTopDefsIntersection explores the possibility of top-level definitions in the caller's elements
// including elements from the target collection and returns them if found.
//
// ExtractTopDefsIntersection 寻找调用者中的顶级定义过程包含目标元素的可能性，如果找到则直接返回。
//
// 该函数通过遍历调用者集合中的每一个元素，检查其顶级定义是否与目标集合中的某个元素匹配。
// This function iterates through each element in the caller's collection to check if its top-level
// definitions match any of the elements in the target collection.
func (value Values) ExtractTopDefsIntersection(targets Values, opts ...OperationOption) Values {
	targetMap := omap.NewOrderedMap(map[int64]*Value{})
	for _, t := range targets {
		targetMap.Set(t.GetId(), t)
	}
	ret := omap.NewOrderedMap(map[int64]*Value{})
	value.GetTopDefs(append(opts, WithHookEveryNode(func(everItem *Value) error {
		result, ok := targetMap.Get(everItem.GetId())
		if ok {
			ret.Set(result.GetId(), result)
		}
		return nil
	}))...)
	return ret.Values()
}
