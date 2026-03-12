package test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	excelize "github.com/xuri/excelize/v2"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

func createTestExcelForAITool(t *testing.T) string {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()

	sheet1 := "Transactions"
	f.SetSheetName("Sheet1", sheet1)

	f.SetCellValue(sheet1, "A1", "Name")
	f.SetCellValue(sheet1, "B1", "Amount")
	f.SetCellValue(sheet1, "C1", "Type")
	f.SetCellValue(sheet1, "D1", "Date")

	f.SetCellValue(sheet1, "A2", "Alice")
	f.SetCellValue(sheet1, "B2", 50000)
	f.SetCellValue(sheet1, "C2", "income")
	f.SetCellValue(sheet1, "D2", "2024-01-15")

	f.SetCellValue(sheet1, "A3", "Bob")
	f.SetCellValue(sheet1, "B3", 30000)
	f.SetCellValue(sheet1, "C3", "expense")
	f.SetCellValue(sheet1, "D3", "2024-01-16")

	f.SetCellValue(sheet1, "A4", "Alice")
	f.SetCellValue(sheet1, "B4", 80000)
	f.SetCellValue(sheet1, "C4", "income")
	f.SetCellValue(sheet1, "D4", "2024-02-01")

	f.SetCellValue(sheet1, "A5", "Charlie")
	f.SetCellValue(sheet1, "B5", 120000)
	f.SetCellValue(sheet1, "C5", "income")
	f.SetCellValue(sheet1, "D5", "2024-02-10")

	f.SetCellValue(sheet1, "A6", "Bob")
	f.SetCellValue(sheet1, "B6", 5000)
	f.SetCellValue(sheet1, "C6", "expense")
	f.SetCellValue(sheet1, "D6", "2024-03-01")

	sheet2 := "Summary"
	f.NewSheet(sheet2)
	f.SetCellValue(sheet2, "A1", "Category")
	f.SetCellValue(sheet2, "B1", "Total")
	f.SetCellValue(sheet2, "A2", "income")
	f.SetCellValue(sheet2, "B2", 250000)
	f.SetCellValue(sheet2, "A3", "expense")
	f.SetCellValue(sheet2, "B3", 35000)

	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "test_transactions.xlsx")
	if err := f.SaveAs(tempFile); err != nil {
		t.Fatalf("failed to create test Excel: %v", err)
	}
	return tempFile
}

func getReadExcelInfoTool(t *testing.T) *aitool.Tool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/doc/read_excel_info.yak")
	if err != nil {
		t.Fatalf("failed to read read_excel_info.yak: %v", err)
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools("read_excel_info", string(content))
	if aiTool == nil {
		t.Fatalf("failed to parse read_excel_info.yak metadata")
	}
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	if len(tools) == 0 {
		t.Fatalf("ConvertTools returned empty")
	}
	return tools[0]
}

func execReadExcelInfoTool(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) (stdout, stderr string) {
	t.Helper()
	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, err := tool.Callback(context.Background(), params, nil, w1, w2)
	if err != nil {
		t.Logf("tool execution error (may be expected): %v", err)
	}
	return w1.String(), w2.String()
}

func TestReadExcelInfo_BasicStructure(t *testing.T) {
	testFile := createTestExcelForAITool(t)
	defer os.Remove(testFile)

	tool := getReadExcelInfoTool(t)
	stdout, _ := execReadExcelInfoTool(t, tool, aitool.InvokeParams{
		"input": testFile,
	})

	assert.Assert(t, strings.Contains(stdout, "Transactions"), "should contain sheet name Transactions")
	assert.Assert(t, strings.Contains(stdout, "Summary"), "should contain sheet name Summary")
	assert.Assert(t, strings.Contains(stdout, "Name"), "should contain column Name")
	assert.Assert(t, strings.Contains(stdout, "Amount"), "should contain column Amount")
	assert.Assert(t, strings.Contains(stdout, "Type"), "should contain column Type")
	assert.Assert(t, strings.Contains(stdout, "Date"), "should contain column Date")
	t.Logf("stdout:\n%s", stdout)
}

func TestReadExcelInfo_SpecificSheet(t *testing.T) {
	testFile := createTestExcelForAITool(t)
	defer os.Remove(testFile)

	tool := getReadExcelInfoTool(t)
	stdout, _ := execReadExcelInfoTool(t, tool, aitool.InvokeParams{
		"input": testFile,
		"sheet": "Summary",
	})

	assert.Assert(t, strings.Contains(stdout, "Summary"), "should contain Summary sheet")
	assert.Assert(t, strings.Contains(stdout, "Category"), "should contain Category column")
	assert.Assert(t, strings.Contains(stdout, "Total"), "should contain Total column")
	t.Logf("stdout:\n%s", stdout)
}

func TestReadExcelInfo_PreviewRows(t *testing.T) {
	testFile := createTestExcelForAITool(t)
	defer os.Remove(testFile)

	tool := getReadExcelInfoTool(t)
	stdout, _ := execReadExcelInfoTool(t, tool, aitool.InvokeParams{
		"input":        testFile,
		"preview_rows": 2,
	})

	assert.Assert(t, strings.Contains(stdout, "Preview (first 2 rows)"), "should show 2 preview rows")
	assert.Assert(t, strings.Contains(stdout, "Alice"), "should contain first row data")
	t.Logf("stdout:\n%s", stdout)
}

func TestReadExcelInfo_OutputToFile(t *testing.T) {
	testFile := createTestExcelForAITool(t)
	defer os.Remove(testFile)

	outputFile := filepath.Join(t.TempDir(), "output_info.txt")

	tool := getReadExcelInfoTool(t)
	stdout, _ := execReadExcelInfoTool(t, tool, aitool.InvokeParams{
		"input":  testFile,
		"output": outputFile,
	})

	assert.Assert(t, strings.Contains(stdout, "saved to"), "should confirm file saved")

	content, err := os.ReadFile(outputFile)
	assert.NilError(t, err, "output file should exist")
	assert.Assert(t, strings.Contains(string(content), "Transactions"), "output file should contain sheet info")
	t.Logf("output file content:\n%s", string(content))
}

func TestReadExcelInfo_NonExistentFile(t *testing.T) {
	tool := getReadExcelInfoTool(t)
	stdout, _ := execReadExcelInfoTool(t, tool, aitool.InvokeParams{
		"input": "/nonexistent/path/file.xlsx",
	})

	assert.Assert(t, strings.Contains(stdout, "does not exist"), "should report file not found")
}

func TestReadExcelInfo_NonExistentSheet(t *testing.T) {
	testFile := createTestExcelForAITool(t)
	defer os.Remove(testFile)

	tool := getReadExcelInfoTool(t)
	stdout, _ := execReadExcelInfoTool(t, tool, aitool.InvokeParams{
		"input": testFile,
		"sheet": "NonExistent",
	})

	assert.Assert(t, strings.Contains(stdout, "not found"), "should report sheet not found")
	assert.Assert(t, strings.Contains(stdout, "Transactions"), "should list available sheets")
}
