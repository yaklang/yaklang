package excelparser

import (
	"github.com/xuri/excelize/v2"
)

// CreateExcelFile 创建新的Excel文件
func CreateExcelFile() *excelize.File {
	return excelize.NewFile()
}

// WriteCell 向指定单元格写入数据
func WriteCell(file *excelize.File, sheet, cell string, value interface{}) error {
	if file == nil || sheet == "" || cell == "" {
		return excelize.ErrParameterInvalid
	}
	return file.SetCellValue(sheet, cell, value)
}

// SetFormula 设置单元格公式
func SetFormula(file *excelize.File, sheet, cell, formula string) error {
	if file == nil || sheet == "" || cell == "" || formula == "" {
		return excelize.ErrParameterInvalid
	}
	return file.SetCellFormula(sheet, cell, formula)
}

// SaveExcelFile 保存Excel文件
func SaveExcelFile(file *excelize.File, path string) error {
	if file == nil || path == "" {
		return excelize.ErrParameterInvalid
	}
	return file.SaveAs(path)
}

// NewSheet 创建新工作表
func NewSheet(file *excelize.File, name string) (int, error) {
	if file == nil || name == "" {
		return -1, excelize.ErrParameterInvalid
	}
	return file.NewSheet(name)
}

// SetCellStyle 设置单元格样式
func SetCellStyle(file *excelize.File, sheet, hCell, vCell string, styleID int) error {
	if file == nil || sheet == "" || hCell == "" || vCell == "" {
		return excelize.ErrParameterInvalid
	}
	return file.SetCellStyle(sheet, hCell, vCell, styleID)
}

// CreateStyle 创建样式
func CreateStyle(file *excelize.File, style *excelize.Style) (int, error) {
	if file == nil || style == nil {
		return -1, excelize.ErrParameterInvalid
	}
	return file.NewStyle(style)
}

// DeleteSheet 删除工作表
func DeleteSheet(file *excelize.File, name string) error {
	if file == nil || name == "" {
		return excelize.ErrParameterInvalid
	}
	return file.DeleteSheet(name)
}

// SetSheetVisible 设置工作表可见性
func SetSheetVisible(file *excelize.File, name string, visible bool) error {
	if file == nil || name == "" {
		return excelize.ErrParameterInvalid
	}
	return file.SetSheetVisible(name, visible)
}

// InsertImage 插入图片
func InsertImage(file *excelize.File, sheet, cell, picture string) error {
	if file == nil || sheet == "" || cell == "" || picture == "" {
		return excelize.ErrParameterInvalid
	}
	return file.AddPicture(sheet, cell, picture, &excelize.GraphicOptions{})
}
