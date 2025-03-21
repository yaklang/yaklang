package excelparser

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils/fileparser/types"
)

// ExcelNodeDumper 用于将 Excel 节点转换为文件
// 注意：这是一个简单实现，推荐使用classifier.go中的方法
// 此代码保留是为了向后兼容
type ExcelNodeDumper struct {
	nodes []ExcelNode
}

// NewDumper 创建一个新的 dumper
// 注意：推荐使用classifier.go中的方法
// 此代码保留是为了向后兼容
func NewDumper(nodes []ExcelNode) *ExcelNodeDumper {
	return &ExcelNodeDumper{nodes: nodes}
}

// DumpToFiles 将节点转换为文件
// 注意：推荐使用classifier.go中的方法
// 此代码保留是为了向后兼容
func (d *ExcelNodeDumper) DumpToFiles() map[string][]types.File {
	// 这里直接使用新的ClassifyNodes方法，避免代码重复
	classifier := ClassifyNodes(d.nodes)
	return classifier.DumpToFiles()
}

// convertTableToMarkdown 将表格内容转换为 Markdown 格式
func (d *ExcelNodeDumper) convertTableToMarkdown(table TableContent) string {
	var buf bytes.Buffer

	// 添加表格标题
	buf.WriteString(fmt.Sprintf("### %s\n\n", table.SheetName))

	// 如果没有数据，返回空字符串
	if len(table.Headers) == 0 {
		return buf.String()
	}

	// 处理表头中的空值
	for i, header := range table.Headers {
		if header == "" {
			table.Headers[i] = fmt.Sprintf("列%d", i+1)
		}
	}

	// 写入表头
	buf.WriteString("|")
	for _, header := range table.Headers {
		buf.WriteString(fmt.Sprintf(" %s |", header))
	}
	buf.WriteString("\n")

	// 写入分隔行
	buf.WriteString("|")
	for range table.Headers {
		buf.WriteString(" --- |")
	}
	buf.WriteString("\n")

	// 写入数据行
	for _, row := range table.Rows {
		buf.WriteString("|")
		// 确保行的长度与表头一致
		for i := 0; i < len(table.Headers); i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			// 处理单元格中的换行符和竖线符号
			cell = strings.ReplaceAll(cell, "\n", "<br>")
			cell = strings.ReplaceAll(cell, "|", "\\|")
			buf.WriteString(fmt.Sprintf(" %s |", cell))
		}
		buf.WriteString("\n")
	}

	buf.WriteString("\n")
	return buf.String()
}
