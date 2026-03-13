package excelparser

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	excelize "github.com/xuri/excelize/v2"
)

// 创建测试用的Excel文件
func createTestExcelFile(t *testing.T) string {
	f := excelize.NewFile()
	defer f.Close()

	// 创建第一个工作表
	sheet1 := "Sheet1"
	f.SetSheetName("Sheet1", sheet1)

	// 添加表头
	f.SetCellValue(sheet1, "A1", "姓名")
	f.SetCellValue(sheet1, "B1", "年龄")
	f.SetCellValue(sheet1, "C1", "职业")
	f.SetCellValue(sheet1, "D1", "链接")

	// 添加数据行
	f.SetCellValue(sheet1, "A2", "张三")
	f.SetCellValue(sheet1, "B2", 30)
	f.SetCellValue(sheet1, "C2", "工程师")
	f.SetCellValue(sheet1, "D2", "https://example.com/zhangsan")

	f.SetCellValue(sheet1, "A3", "李四")
	f.SetCellValue(sheet1, "B3", 25)
	f.SetCellValue(sheet1, "C3", "设计师")
	f.SetCellValue(sheet1, "D3", "https://example.com/lisi")

	// 添加单元格公式
	f.SetCellFormula(sheet1, "E1", "SUM(B2:B3)")
	// 同时设置公式的值，模拟Excel计算结果
	f.SetCellValue(sheet1, "E1", 55) // 30 + 25 = 55

	// 另外添加一个公式，确保测试充分
	f.SetCellFormula(sheet1, "E2", "AVERAGE(B2:B3)")
	f.SetCellValue(sheet1, "E2", 27.5) // (30 + 25) / 2 = 27.5

	// 创建第二个工作表
	sheet2 := "产品信息"
	f.NewSheet(sheet2)

	// 添加表头
	f.SetCellValue(sheet2, "A1", "产品名称")
	f.SetCellValue(sheet2, "B1", "价格")
	f.SetCellValue(sheet2, "C1", "库存")

	// 添加数据行
	f.SetCellValue(sheet2, "A2", "产品A")
	f.SetCellValue(sheet2, "B2", 99.9)
	f.SetCellValue(sheet2, "C2", 100)

	f.SetCellValue(sheet2, "A3", "产品B")
	f.SetCellValue(sheet2, "B3", 199.9)
	f.SetCellValue(sheet2, "C3", 50)

	// 创建一个隐藏工作表
	sheet3 := "隐藏数据"
	f.NewSheet(sheet3)
	f.SetCellValue(sheet3, "A1", "隐藏数据1")
	f.SetCellValue(sheet3, "B1", "隐藏数据2")
	f.SetSheetVisible(sheet3, false)

	// 添加自定义名称
	f.SetDefinedName(&excelize.DefinedName{
		Name:     "TestName",
		RefersTo: "=Sheet1!$A$1:$D$3",
		Scope:    "",
		Comment:  "测试定义名称",
	})

	// 保存到临时文件
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "test.xlsx")
	if err := f.SaveAs(tempFile); err != nil {
		t.Fatalf("无法保存测试Excel文件: %v", err)
	}

	return tempFile
}

func TestParseExcelFile(t *testing.T) {
	// 创建测试文件
	testFile := createTestExcelFile(t)
	defer os.Remove(testFile)

	// 解析Excel文件
	nodes, err := ParseExcelFile(testFile)
	if err != nil {
		t.Fatalf("解析Excel文件失败: %v", err)
	}

	// 验证解析结果
	tableCount := 0
	textCount := 0
	urlCount := 0
	formulaCount := 0
	hiddenSheetCount := 0
	nameDefCount := 0

	for _, node := range nodes {
		switch node.Type {
		case TableNode:
			tableCount++
			if content, ok := node.Content.(TableContent); ok {
				// 验证表格内容
				if content.SheetName == "Sheet1" {
					// 注意：由于添加了E列的公式，表头列数现在是5，而不是4
					if len(content.Headers) != 5 {
						t.Errorf("Sheet1表头数量错误: 期望5, 实际%d", len(content.Headers))
					}
					if len(content.Rows) != 2 {
						t.Errorf("Sheet1数据行数量错误: 期望2, 实际%d", len(content.Rows))
					}
				} else if content.SheetName == "产品信息" {
					if len(content.Headers) != 3 {
						t.Errorf("产品信息表头数量错误: 期望3, 实际%d", len(content.Headers))
					}
					if len(content.Rows) != 2 {
						t.Errorf("产品信息数据行数量错误: 期望2, 实际%d", len(content.Rows))
					}
				}
			}
		case TextNode:
			textCount++
		case URLNode:
			urlCount++
			if content, ok := node.Content.(URLContent); ok {
				if !strings.HasPrefix(content.URL, "https://example.com/") {
					t.Errorf("URL格式错误: %s", content.URL)
				}
			}
		case FormulaNode:
			formulaCount++
			t.Logf("发现公式节点: %+v", node.Content)
			if content, ok := node.Content.(FormulaContent); ok {
				if content.Formula != "SUM(B2:B3)" && content.Formula != "AVERAGE(B2:B3)" {
					t.Errorf("公式内容错误: %s", content.Formula)
				}
			}
		case HiddenSheetNode:
			hiddenSheetCount++
			if content, ok := node.Content.(HiddenSheetContent); ok {
				if content.SheetName != "隐藏数据" {
					t.Errorf("隐藏工作表名称错误: %s", content.SheetName)
				}
			}
		case NameDefNode:
			nameDefCount++
			if content, ok := node.Content.(NameDefContent); ok {
				if content.Name != "TestName" {
					t.Errorf("自定义名称错误: %s", content.Name)
				}
				if content.Comment != "测试定义名称" {
					t.Errorf("自定义名称注释错误: %s", content.Comment)
				}
			}
		}
	}

	// 验证节点数量
	if tableCount != 3 { // 包括隐藏工作表
		t.Errorf("表格节点数量错误: 期望3, 实际%d", tableCount)
	}
	if urlCount != 2 {
		t.Errorf("URL节点数量错误: 期望2, 实际%d", urlCount)
	}
	if formulaCount < 1 {
		t.Errorf("公式节点数量错误: 期望至少1, 实际%d", formulaCount)
	}
	if hiddenSheetCount != 1 {
		t.Errorf("隐藏工作表节点数量错误: 期望1, 实际%d", hiddenSheetCount)
	}
	if nameDefCount != 1 {
		t.Errorf("自定义名称节点数量错误: 期望1, 实际%d", nameDefCount)
	}
}

func TestClassifyNodes(t *testing.T) {
	// 创建测试文件
	testFile := createTestExcelFile(t)
	defer os.Remove(testFile)

	// 解析Excel文件
	nodes, err := ParseExcelFile(testFile)
	if err != nil {
		t.Fatalf("解析Excel文件失败: %v", err)
	}

	// 使用分类器分类节点
	classifier := ClassifyNodes(nodes)

	// 验证分类结果
	if len(classifier.Tables) < 3 {
		t.Errorf("表格数量错误: 期望至少3, 实际%d", len(classifier.Tables))
	}

	if len(classifier.URLs) != 2 {
		t.Errorf("URL数量错误: 期望2, 实际%d", len(classifier.URLs))
	}

	if len(classifier.Formulas) < 1 {
		t.Errorf("公式数量错误: 期望至少1, 实际%d", len(classifier.Formulas))
	}

	if len(classifier.HiddenSheets) != 1 {
		t.Errorf("隐藏工作表数量错误: 期望1, 实际%d", len(classifier.HiddenSheets))
	}

	if len(classifier.NameDefs) != 1 {
		t.Errorf("自定义名称数量错误: 期望1, 实际%d", len(classifier.NameDefs))
	}

	// 测试摘要
	summary := classifier.PrintSummary()
	if !strings.Contains(summary, "Excel文档统计信息") {
		t.Errorf("摘要内容错误，未包含预期的统计信息头")
	}
}

func TestDumpToFiles(t *testing.T) {
	// 创建测试文件
	testFile := createTestExcelFile(t)
	defer os.Remove(testFile)

	// 解析Excel文件
	nodes, err := ParseExcelFile(testFile)
	if err != nil {
		t.Fatalf("解析Excel文件失败: %v", err)
	}

	// 转换为文件
	classifier := ClassifyNodes(nodes)
	files := classifier.DumpToFiles()

	// 验证文件类型
	if len(files[string(FileTypeTable)]) < 3 {
		t.Errorf("表格文件数量错误: 期望至少3, 实际%d", len(files[string(FileTypeTable)]))
	}

	// 验证Markdown格式
	for _, file := range files[string(FileTypeTable)] {
		content := string(file.BinaryData)
		t.Logf("表格文件内容: %s", content)

		// 验证表格标记
		if !strings.Contains(content, "| --- |") {
			t.Errorf("表格内容不包含Markdown分隔符")
		}

		// 验证表头 - 注意现在可能有新的表头
		foundExpectedHeader := false
		if strings.Contains(content, "| 姓名 |") ||
			strings.Contains(content, "| 产品名称 |") ||
			strings.Contains(content, "| 隐藏数据1 |") {
			foundExpectedHeader = true
		}

		if !foundExpectedHeader {
			t.Errorf("表格内容不包含预期的表头: %s", content)
		}
	}

	// 验证URL文件
	if len(files[string(FileTypeURL)]) != 1 {
		t.Errorf("URL文件数量错误: 期望1, 实际%d", len(files[string(FileTypeURL)]))
	} else {
		urlContent := string(files[string(FileTypeURL)][0].BinaryData)
		if !strings.Contains(urlContent, "https://example.com/") {
			t.Errorf("URL内容不包含预期的链接")
		}
	}

	// 验证公式文件
	if len(files[string(FileTypeFormula)]) < 1 {
		t.Errorf("公式文件数量错误: 期望至少1, 实际%d", len(files[string(FileTypeFormula)]))
	} else {
		formulaContent := string(files[string(FileTypeFormula)][0].BinaryData)
		if !strings.Contains(formulaContent, "SUM(B2:B3)") {
			t.Errorf("公式内容不包含预期的公式: %s", formulaContent)
		}
	}

	// 验证隐藏工作表文件
	if len(files[string(FileTypeHiddenSheet)]) != 1 {
		t.Errorf("隐藏工作表文件数量错误: 期望1, 实际%d", len(files[string(FileTypeHiddenSheet)]))
	} else {
		hiddenSheetContent := string(files[string(FileTypeHiddenSheet)][0].BinaryData)
		if !strings.Contains(hiddenSheetContent, "隐藏工作表") {
			t.Errorf("隐藏工作表内容不包含预期的标题: %s", hiddenSheetContent)
		}
	}

	// 验证自定义名称文件
	if len(files[string(FileTypeNameDef)]) != 1 {
		t.Errorf("自定义名称文件数量错误: 期望1, 实际%d", len(files[string(FileTypeNameDef)]))
	} else {
		nameDefContent := string(files[string(FileTypeNameDef)][0].BinaryData)
		if !strings.Contains(nameDefContent, "TestName") {
			t.Errorf("自定义名称内容不包含预期的名称: %s", nameDefContent)
		}
	}
}

func TestParseExcelTableOnly_DataConsistency(t *testing.T) {
	testFile := createTestExcelFile(t)
	defer os.Remove(testFile)

	fullNodes, err := ParseExcelFile(testFile)
	if err != nil {
		t.Fatalf("ParseExcelFile failed: %v", err)
	}

	tableOnlyNodes, err := ParseExcelTableOnly(testFile)
	if err != nil {
		t.Fatalf("ParseExcelTableOnly failed: %v", err)
	}

	fullClassifier := ClassifyNodes(fullNodes)
	tableOnlyClassifier := ClassifyNodes(tableOnlyNodes)

	if len(fullClassifier.Tables) != len(tableOnlyClassifier.Tables) {
		t.Fatalf("table count mismatch: ParseExcelFile=%d, ParseExcelTableOnly=%d",
			len(fullClassifier.Tables), len(tableOnlyClassifier.Tables))
	}

	for i, fullTable := range fullClassifier.Tables {
		lightTable := tableOnlyClassifier.Tables[i]

		if fullTable.SheetName != lightTable.SheetName {
			t.Errorf("table[%d] sheet name mismatch: %q vs %q", i, fullTable.SheetName, lightTable.SheetName)
		}

		if len(fullTable.Headers) != len(lightTable.Headers) {
			t.Errorf("table[%d] %q header count mismatch: %d vs %d",
				i, fullTable.SheetName, len(fullTable.Headers), len(lightTable.Headers))
		} else {
			for j := range fullTable.Headers {
				if fullTable.Headers[j] != lightTable.Headers[j] {
					t.Errorf("table[%d] %q header[%d] mismatch: %q vs %q",
						i, fullTable.SheetName, j, fullTable.Headers[j], lightTable.Headers[j])
				}
			}
		}

		if len(fullTable.Rows) != len(lightTable.Rows) {
			t.Errorf("table[%d] %q row count mismatch: %d vs %d",
				i, fullTable.SheetName, len(fullTable.Rows), len(lightTable.Rows))
		} else {
			for r := range fullTable.Rows {
				fullRow := fullTable.Rows[r]
				lightRow := lightTable.Rows[r]
				if len(fullRow) != len(lightRow) {
					t.Errorf("table[%d] %q row[%d] column count mismatch: %d vs %d",
						i, fullTable.SheetName, r, len(fullRow), len(lightRow))
					continue
				}
				for c := range fullRow {
					if fullRow[c] != lightRow[c] {
						t.Errorf("table[%d] %q row[%d] col[%d] mismatch: %q vs %q",
							i, fullTable.SheetName, r, c, fullRow[c], lightRow[c])
					}
				}
			}
		}
	}

	if len(fullClassifier.HiddenSheets) != len(tableOnlyClassifier.HiddenSheets) {
		t.Errorf("hidden sheet count mismatch: %d vs %d",
			len(fullClassifier.HiddenSheets), len(tableOnlyClassifier.HiddenSheets))
	}

	t.Logf("data consistency verified: %d tables, %d hidden sheets", len(fullClassifier.Tables), len(fullClassifier.HiddenSheets))
}

func createLargeTestExcelFile(t testing.TB, rowCount int) string {
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Sheet1"
	f.SetCellValue(sheet, "A1", "ID")
	f.SetCellValue(sheet, "B1", "Name")
	f.SetCellValue(sheet, "C1", "Amount")
	f.SetCellValue(sheet, "D1", "Date")
	f.SetCellValue(sheet, "E1", "Type")

	types := []string{"income", "expense", "transfer"}
	for i := 0; i < rowCount; i++ {
		rowStr := strconv.Itoa(i + 2)
		f.SetCellValue(sheet, "A"+rowStr, i+1)
		f.SetCellValue(sheet, "B"+rowStr, fmt.Sprintf("User_%d", i%1000))
		f.SetCellValue(sheet, "C"+rowStr, float64(i)*1.5+100.0)
		f.SetCellValue(sheet, "D"+rowStr, fmt.Sprintf("2024-01-%02d", i%28+1))
		f.SetCellValue(sheet, "E"+rowStr, types[i%3])
	}

	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "large_test.xlsx")
	if err := f.SaveAs(tempFile); err != nil {
		t.Fatalf("failed to create large test Excel: %v", err)
	}
	return tempFile
}

func TestParseExcelTableOnly_LargeFile_DataConsistency(t *testing.T) {
	rowCount := 10000
	testFile := createLargeTestExcelFile(t, rowCount)
	defer os.Remove(testFile)

	fullNodes, err := ParseExcelFile(testFile)
	if err != nil {
		t.Fatalf("ParseExcelFile failed: %v", err)
	}
	tableOnlyNodes, err := ParseExcelTableOnly(testFile)
	if err != nil {
		t.Fatalf("ParseExcelTableOnly failed: %v", err)
	}

	fullClassifier := ClassifyNodes(fullNodes)
	tableOnlyClassifier := ClassifyNodes(tableOnlyNodes)

	if len(fullClassifier.Tables) != len(tableOnlyClassifier.Tables) {
		t.Fatalf("table count mismatch: full=%d, tableOnly=%d",
			len(fullClassifier.Tables), len(tableOnlyClassifier.Tables))
	}

	for i, fullTable := range fullClassifier.Tables {
		lightTable := tableOnlyClassifier.Tables[i]
		if fullTable.SheetName != lightTable.SheetName {
			t.Errorf("sheet name mismatch at %d", i)
		}
		if len(fullTable.Rows) != len(lightTable.Rows) {
			t.Fatalf("row count mismatch for sheet %q: full=%d, tableOnly=%d",
				fullTable.SheetName, len(fullTable.Rows), len(lightTable.Rows))
		}
		if len(fullTable.Headers) != len(lightTable.Headers) {
			t.Fatalf("header count mismatch for sheet %q: full=%d, tableOnly=%d",
				fullTable.SheetName, len(fullTable.Headers), len(lightTable.Headers))
		}

		sampleRows := []int{0, 1, 100, 1000, 5000, len(fullTable.Rows) - 1}
		for _, rowIdx := range sampleRows {
			if rowIdx >= len(fullTable.Rows) {
				continue
			}
			fullRow := fullTable.Rows[rowIdx]
			lightRow := lightTable.Rows[rowIdx]
			for colIdx := range fullTable.Headers {
				fullVal, lightVal := "", ""
				if colIdx < len(fullRow) {
					fullVal = fullRow[colIdx]
				}
				if colIdx < len(lightRow) {
					lightVal = lightRow[colIdx]
				}
				if fullVal != lightVal {
					t.Errorf("cell mismatch at row %d col %d: full=%q, tableOnly=%q",
						rowIdx, colIdx, fullVal, lightVal)
				}
			}
		}
	}

	t.Logf("large file data consistency verified: %d rows", rowCount)
}

func TestParseExcelTableFast_MaxRows(t *testing.T) {
	rowCount := 5000
	maxRows := 100
	testFile := createLargeTestExcelFile(t, rowCount)
	defer os.Remove(testFile)

	nodes, err := ParseExcelTableFast(testFile, maxRows)
	if err != nil {
		t.Fatalf("ParseExcelTableFast failed: %v", err)
	}

	classifier := ClassifyNodes(nodes)
	if len(classifier.Tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(classifier.Tables))
	}

	table := classifier.Tables[0]
	if len(table.Rows) != maxRows {
		t.Errorf("expected %d sampled rows, got %d", maxRows, len(table.Rows))
	}
	if len(table.Headers) != 5 {
		t.Errorf("expected 5 headers, got %d", len(table.Headers))
	}

	totalDataRows := table.Metadata["total_data_rows"]
	if totalDataRows != strconv.Itoa(rowCount) {
		t.Errorf("expected total_data_rows=%d, got %s", rowCount, totalDataRows)
	}

	totalRows := table.Metadata["total_rows"]
	if totalRows != strconv.Itoa(rowCount+1) {
		t.Errorf("expected total_rows=%d, got %s", rowCount+1, totalRows)
	}
}

func TestParseExcelTableFast_AllRows(t *testing.T) {
	rowCount := 2000
	testFile := createLargeTestExcelFile(t, rowCount)
	defer os.Remove(testFile)

	nodes, err := ParseExcelTableFast(testFile, 0)
	if err != nil {
		t.Fatalf("ParseExcelTableFast failed: %v", err)
	}

	classifier := ClassifyNodes(nodes)
	if len(classifier.Tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(classifier.Tables))
	}

	table := classifier.Tables[0]
	if len(table.Rows) != rowCount {
		t.Errorf("expected %d rows, got %d", rowCount, len(table.Rows))
	}

	fullNodes, err := ParseExcelFile(testFile)
	if err != nil {
		t.Fatalf("ParseExcelFile failed: %v", err)
	}
	fullClassifier := ClassifyNodes(fullNodes)
	fullTable := fullClassifier.Tables[0]

	sampleRows := []int{0, 1, 500, 999, rowCount - 1}
	for _, idx := range sampleRows {
		if idx >= len(table.Rows) || idx >= len(fullTable.Rows) {
			continue
		}
		for c := range table.Headers {
			got, want := "", ""
			if c < len(table.Rows[idx]) {
				got = table.Rows[idx][c]
			}
			if c < len(fullTable.Rows[idx]) {
				want = fullTable.Rows[idx][c]
			}
			if got != want {
				t.Errorf("row[%d] col[%d] mismatch: fast=%q, full=%q", idx, c, got, want)
			}
		}
	}
}

func BenchmarkParseExcelFile(b *testing.B) {
	testFile := createLargeTestExcelFile(b, 5000)
	defer os.Remove(testFile)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseExcelFile(testFile)
	}
}

func BenchmarkParseExcelTableOnly(b *testing.B) {
	testFile := createLargeTestExcelFile(b, 5000)
	defer os.Remove(testFile)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseExcelTableOnly(testFile)
	}
}

func BenchmarkParseExcelTableFast_LimitedRows(b *testing.B) {
	testFile := createLargeTestExcelFile(b, 5000)
	defer os.Remove(testFile)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseExcelTableFast(testFile, 200)
	}
}

func TestParseExcel(t *testing.T) {
	// 创建测试文件
	testFile := createTestExcelFile(t)
	defer os.Remove(testFile)

	// 调用ParseExcel函数
	files, err := ParseExcel(testFile)
	if err != nil {
		t.Fatalf("ParseExcel失败: %v", err)
	}

	// 验证结果
	if len(files) == 0 {
		t.Error("ParseExcel返回空结果")
	}

	// 验证表格文件
	if len(files[string(FileTypeTable)]) == 0 {
		t.Error("没有表格文件")
	}

	// 验证公式文件
	if len(files[string(FileTypeFormula)]) == 0 {
		t.Error("没有公式文件")
	}

	// 验证自定义名称文件
	if len(files[string(FileTypeNameDef)]) == 0 {
		t.Error("没有自定义名称文件")
	}
}
