package excelparser

// ExcelExports 导出给yak脚本使用的Excel操作函数
var ExcelExports = map[string]interface{}{
	// 文件操作
	"NewFile": CreateExcelFile,
	"Save":    SaveExcelFile,

	// 工作表操作
	"NewSheet":        NewSheet,
	"DeleteSheet":     DeleteSheet,
	"SetSheetVisible": SetSheetVisible,

	// 单元格操作
	"WriteCell":  WriteCell,
	"SetFormula": SetFormula,

	// 样式操作
	"SetCellStyle": SetCellStyle,
	"CreateStyle":  CreateStyle,

	// 图片操作
	"InsertImage": InsertImage,

	// 现有的解析功能
	"Parse":         ParseExcelFile,
	"ClassifyNodes": ClassifyNodes,
}
