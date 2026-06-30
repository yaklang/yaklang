package doc

import (
	"sort"
	"strings"
)

// GetLibOverviewShort 返回某个库的一句话定位(OverviewShort)。
// 该内容随 doc.gob.zst 一并加载(由文档生成期从 overviews/<lib>.md 首段派生),
// 不存在或未生成时返回空字符串。
// 关键词: 库选择, OverviewShort, 运行时零成本
func GetLibOverviewShort(libName string) string {
	helper := GetDefaultDocumentHelper()
	if helper == nil {
		return ""
	}
	lib, ok := helper.Libs[libName]
	if !ok || lib == nil {
		return ""
	}
	return lib.OverviewShort
}

// BuildLibrarySelectionIndex 遍历所有库, 把带 OverviewShort 的库拼成"库选择索引":
// 每行 `lib — 一句话定位`, 按库名排序。供 AI 在拆解需求时先据此选 2-4 个库,
// 再决定关键字/语义问题。无任何带 OverviewShort 的库时返回空字符串(优雅降级)。
// 关键词: 库选择索引, 目标到模块, 拆解 prompt 注入
func BuildLibrarySelectionIndex() string {
	helper := GetDefaultDocumentHelper()
	if helper == nil || len(helper.Libs) == 0 {
		return ""
	}

	names := make([]string, 0, len(helper.Libs))
	for name, lib := range helper.Libs {
		if lib == nil || strings.TrimSpace(lib.OverviewShort) == "" {
			continue
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		b.WriteString(name)
		b.WriteString(" — ")
		b.WriteString(strings.TrimSpace(helper.Libs[name].OverviewShort))
		b.WriteString("\n")
	}
	return b.String()
}
