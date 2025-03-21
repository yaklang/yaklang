package excelparser

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/fileparser/types"
)

// ParseExcel 解析 Excel 文件，将内部函数重定向到ParseExcelFile
func ParseExcel(filePath string) (map[string][]types.File, error) {
	// 使用ParseExcelFile解析Excel文档
	nodes, err := ParseExcelFile(filePath)
	if err != nil {
		log.Errorf("解析Excel文件失败: %v", err)
		return nil, err
	}

	// 使用classifier对节点进行分类
	classifier := ClassifyNodes(nodes)

	// 使用dumper转换为文件
	// 这样可以获得更详细的分类和更好的Markdown输出
	return classifier.DumpToFiles(), nil
}
