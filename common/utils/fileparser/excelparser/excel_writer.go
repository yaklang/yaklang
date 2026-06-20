package excelparser

import (
	"github.com/xuri/excelize/v2"
)

// NewFile 创建一个新的 Excel 文件对象（导出名为 excel.NewFile）
// 新文件默认包含一个名为 Sheet1 的工作表，可继续写入单元格、添加工作表，最后用 excel.Save 保存
//
// 返回值:
//   - Excel 文件对象
//
// Example:
// ```
// f = excel.NewFile()
// excel.WriteCell(f, "Sheet1", "A1", "hello")~
// path = file.Join(os.TempDir(), "excel_newfile_demo.xlsx")
// excel.Save(f, path)~
// println(file.IsExisted(path))   // OUT: true
// assert file.IsExisted(path), "NewFile + Save should create a workbook on disk"
// file.Remove(path)
// ```
func CreateExcelFile() *excelize.File {
	return excelize.NewFile()
}

// WriteCell 向指定工作表的单元格写入数据（导出名为 excel.WriteCell）
//
// 参数:
//   - file: Excel 文件对象
//   - sheet: 工作表名称（如 "Sheet1"）
//   - cell: 单元格坐标（如 "A1"）
//   - value: 写入的值，支持字符串、数字等
//
// 返回值:
//   - 错误信息（参数非法或写入失败时返回）
//
// Example:
// ```
// f = excel.NewFile()
// excel.WriteCell(f, "Sheet1", "A1", "yak")~
// path = file.Join(os.TempDir(), "excel_writecell_demo.xlsx")
// excel.Save(f, path)~
// nodes = excel.Parse(path)~
// cls = excel.ClassifyNodes(nodes)
// println(cls.Tables[0].Headers[0])   // OUT: yak
// assert cls.Tables[0].Headers[0] == "yak", "WriteCell should persist the value into the cell"
// file.Remove(path)
// ```
func WriteCell(file *excelize.File, sheet, cell string, value interface{}) error {
	if file == nil || sheet == "" || cell == "" {
		return excelize.ErrParameterInvalid
	}
	return file.SetCellValue(sheet, cell, value)
}

// SetFormula 设置单元格的公式（导出名为 excel.SetFormula）
//
// 参数:
//   - file: Excel 文件对象
//   - sheet: 工作表名称
//   - cell: 单元格坐标（如 "C2"）
//   - formula: 公式字符串（如 "=B2*2"）
//
// 返回值:
//   - 错误信息（参数非法或设置失败时返回）
//
// Example:
// ```
// f = excel.NewFile()
// excel.WriteCell(f, "Sheet1", "B2", 90)~
// excel.SetFormula(f, "Sheet1", "C2", "=B2*2")~
// path = file.Join(os.TempDir(), "excel_setformula_demo.xlsx")
// excel.Save(f, path)~
// println(file.IsExisted(path))   // OUT: true
// assert file.IsExisted(path), "SetFormula should not break saving the workbook"
// file.Remove(path)
// ```
func SetFormula(file *excelize.File, sheet, cell, formula string) error {
	if file == nil || sheet == "" || cell == "" || formula == "" {
		return excelize.ErrParameterInvalid
	}
	return file.SetCellFormula(sheet, cell, formula)
}

// Save 将 Excel 文件对象保存到指定路径（导出名为 excel.Save）
//
// 参数:
//   - file: Excel 文件对象
//   - path: 保存路径（.xlsx）
//
// 返回值:
//   - 错误信息（参数非法或保存失败时返回）
//
// Example:
// ```
// f = excel.NewFile()
// excel.WriteCell(f, "Sheet1", "A1", "data")~
// path = file.Join(os.TempDir(), "excel_save_demo.xlsx")
// excel.Save(f, path)~
// println(file.IsExisted(path))   // OUT: true
// assert file.IsExisted(path), "Save should write the workbook to disk"
// file.Remove(path)
// ```
func SaveExcelFile(file *excelize.File, path string) error {
	if file == nil || path == "" {
		return excelize.ErrParameterInvalid
	}
	return file.SaveAs(path)
}

// NewSheet 在 Excel 文件中创建一个新工作表（导出名为 excel.NewSheet）
//
// 参数:
//   - file: Excel 文件对象
//   - name: 新工作表名称
//
// 返回值:
//   - 新工作表的索引
//   - 错误信息（参数非法或创建失败时返回）
//
// Example:
// ```
// f = excel.NewFile()
// idx = excel.NewSheet(f, "Extra")~
// println(idx >= 0)   // OUT: true
// assert idx >= 0, "NewSheet should return a non-negative sheet index"
// ```
func NewSheet(file *excelize.File, name string) (int, error) {
	if file == nil || name == "" {
		return -1, excelize.ErrParameterInvalid
	}
	return file.NewSheet(name)
}

// SetCellStyle 为指定区域的单元格应用样式（导出名为 excel.SetCellStyle）
// 样式 ID 由 excel.CreateStyle 创建
//
// 参数:
//   - file: Excel 文件对象
//   - sheet: 工作表名称
//   - hCell: 区域左上角单元格坐标
//   - vCell: 区域右下角单元格坐标
//   - styleID: 由 excel.CreateStyle 返回的样式 ID
//
// 返回值:
//   - 错误信息（参数非法或设置失败时返回）
//
// Example:
// ```
// // 示意性示例: CreateStyle 需要一个 *excelize.Style 样式对象
// f = excel.NewFile()
// excel.WriteCell(f, "Sheet1", "A1", "title")~
// styleID = excel.CreateStyle(f, style)~
// excel.SetCellStyle(f, "Sheet1", "A1", "A1", styleID)~
// ```
func SetCellStyle(file *excelize.File, sheet, hCell, vCell string, styleID int) error {
	if file == nil || sheet == "" || hCell == "" || vCell == "" {
		return excelize.ErrParameterInvalid
	}
	return file.SetCellStyle(sheet, hCell, vCell, styleID)
}

// CreateStyle 创建一个单元格样式并返回样式 ID（导出名为 excel.CreateStyle）
// 返回的样式 ID 可传给 excel.SetCellStyle 应用到单元格
//
// 参数:
//   - file: Excel 文件对象
//   - style: 样式对象（*excelize.Style，描述字体、填充、边框等）
//
// 返回值:
//   - 样式 ID
//   - 错误信息（参数非法或创建失败时返回）
//
// Example:
// ```
// // 示意性示例: style 为 *excelize.Style 样式对象，描述字体/填充/边框等
// f = excel.NewFile()
// styleID = excel.CreateStyle(f, style)~
// excel.SetCellStyle(f, "Sheet1", "A1", "A1", styleID)~
// ```
func CreateStyle(file *excelize.File, style *excelize.Style) (int, error) {
	if file == nil || style == nil {
		return -1, excelize.ErrParameterInvalid
	}
	return file.NewStyle(style)
}

// DeleteSheet 删除指定名称的工作表（导出名为 excel.DeleteSheet）
//
// 参数:
//   - file: Excel 文件对象
//   - name: 要删除的工作表名称
//
// 返回值:
//   - 错误信息（参数非法或删除失败时返回）
//
// Example:
// ```
// f = excel.NewFile()
// excel.NewSheet(f, "Extra")~
// excel.DeleteSheet(f, "Extra")~
// path = file.Join(os.TempDir(), "excel_deletesheet_demo.xlsx")
// excel.Save(f, path)~
// println(file.IsExisted(path))   // OUT: true
// assert file.IsExisted(path), "DeleteSheet should not break saving the workbook"
// file.Remove(path)
// ```
func DeleteSheet(file *excelize.File, name string) error {
	if file == nil || name == "" {
		return excelize.ErrParameterInvalid
	}
	return file.DeleteSheet(name)
}

// SetSheetVisible 设置工作表的可见性（导出名为 excel.SetSheetVisible）
//
// 参数:
//   - file: Excel 文件对象
//   - name: 工作表名称
//   - visible: 是否可见，false 表示隐藏
//
// 返回值:
//   - 错误信息（参数非法或设置失败时返回）
//
// Example:
// ```
// f = excel.NewFile()
// excel.NewSheet(f, "Extra")~
// excel.SetSheetVisible(f, "Extra", false)~
// path = file.Join(os.TempDir(), "excel_visible_demo.xlsx")
// excel.Save(f, path)~
// println(file.IsExisted(path))   // OUT: true
// assert file.IsExisted(path), "SetSheetVisible should not break saving the workbook"
// file.Remove(path)
// ```
func SetSheetVisible(file *excelize.File, name string, visible bool) error {
	if file == nil || name == "" {
		return excelize.ErrParameterInvalid
	}
	return file.SetSheetVisible(name, visible)
}

// InsertImage 在指定单元格位置插入一张图片（导出名为 excel.InsertImage）
//
// 参数:
//   - file: Excel 文件对象
//   - sheet: 工作表名称
//   - cell: 图片锚定的单元格坐标（如 "A1"）
//   - picture: 图片文件路径
//
// 返回值:
//   - 错误信息（参数非法或插入失败时返回）
//
// Example:
// ```
// // 示意性示例: picture 需为本地存在的图片文件路径
// f = excel.NewFile()
// excel.InsertImage(f, "Sheet1", "A1", "/path/to/logo.png")~
// excel.Save(f, file.Join(os.TempDir(), "excel_image_demo.xlsx"))~
// ```
func InsertImage(file *excelize.File, sheet, cell, picture string) error {
	if file == nil || sheet == "" || cell == "" || picture == "" {
		return excelize.ErrParameterInvalid
	}
	return file.AddPicture(sheet, cell, picture, &excelize.GraphicOptions{})
}
