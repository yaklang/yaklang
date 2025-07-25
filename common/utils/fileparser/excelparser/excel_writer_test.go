package excelparser

import (
	"github.com/xuri/excelize/v2"
	"os"
	"path/filepath"
	"testing"
)

func TestExcelWriter(t *testing.T) {
	// 创建新文件
	file := CreateExcelFile()
	defer file.Close()

	// 写入数据
	err := WriteCell(file, "Sheet1", "A1", "测试数据")
	if err != nil {
		t.Fatalf("写入单元格失败: %v", err)
	}

	// 设置公式
	err = SetFormula(file, "Sheet1", "B1", "=A1&\"_公式\"")
	if err != nil {
		t.Fatalf("设置公式失败: %v", err)
	}

	// 创建新工作表
	_, err = NewSheet(file, "测试表")
	if err != nil {
		t.Fatalf("创建工作表失败: %v", err)
	}

	// 创建样式
	style := &excelize.Style{
		Font: &excelize.Font{
			Bold: true,
			Size: 12,
		},
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"#FFFF00"},
			Pattern: 1,
		},
	}
	styleID, err := CreateStyle(file, style)
	if err != nil {
		t.Fatalf("创建样式失败: %v", err)
	}

	// 应用样式
	err = SetCellStyle(file, "Sheet1", "A1", "A1", styleID)
	if err != nil {
		t.Fatalf("设置样式失败: %v", err)
	}

	// 保存文件
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "test_output.xlsx")
	err = SaveExcelFile(file, tempFile)
	if err != nil {
		t.Fatalf("保存文件失败: %v", err)
	}

	// 验证文件存在
	if _, err := os.Stat(tempFile); os.IsNotExist(err) {
		t.Fatalf("保存的文件不存在: %s", tempFile)
	}
}

func TestExcelIntegration(t *testing.T) {
	// 测试与现有解析功能的集成
	file := CreateExcelFile()
	defer file.Close()

	// 写入测试数据
	WriteCell(file, "Sheet1", "A1", "姓名")
	WriteCell(file, "Sheet1", "B1", "年龄")
	WriteCell(file, "Sheet1", "A2", "张三")
	WriteCell(file, "Sheet1", "B2", 30)

	// 保存临时文件
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "integration_test.xlsx")
	err := SaveExcelFile(file, tempFile)
	if err != nil {
		t.Fatalf("保存文件失败: %v", err)
	}

	// 使用现有解析功能读取
	nodes, err := ParseExcelFile(tempFile)
	if err != nil {
		t.Fatalf("解析文件失败: %v", err)
	}

	if len(nodes) == 0 {
		t.Fatalf("解析结果为空")
	}
}
