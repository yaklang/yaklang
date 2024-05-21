package ssaapi

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
