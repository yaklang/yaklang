package test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

func getQueryExcelDataTool(t *testing.T) *aitool.Tool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/doc/query_excel_data.yak")
	if err != nil {
		t.Fatalf("failed to read query_excel_data.yak: %v", err)
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools("query_excel_data", string(content))
	if aiTool == nil {
		t.Fatalf("failed to parse query_excel_data.yak metadata")
	}
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	if len(tools) == 0 {
		t.Fatalf("ConvertTools returned empty")
	}
	return tools[0]
}

func execQueryExcelDataTool(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) (stdout, stderr string) {
	t.Helper()
	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, err := tool.Callback(context.Background(), params, nil, w1, w2)
	if err != nil {
		t.Logf("tool execution error (may be expected): %v", err)
	}
	return w1.String(), w2.String()
}

func TestQueryExcelData_BasicQuery(t *testing.T) {
	testFile := createTestExcelForAITool(t)
	defer os.Remove(testFile)

	tool := getQueryExcelDataTool(t)
	stdout, _ := execQueryExcelDataTool(t, tool, aitool.InvokeParams{
		"input":         testFile,
		"output_format": "csv",
	})

	assert.Assert(t, strings.Contains(stdout, "Alice"), "should contain Alice")
	assert.Assert(t, strings.Contains(stdout, "Bob"), "should contain Bob")
	assert.Assert(t, strings.Contains(stdout, "Charlie"), "should contain Charlie")
	assert.Assert(t, strings.Contains(stdout, "50000"), "should contain amount 50000")
	t.Logf("stdout:\n%s", stdout)
}

func TestQueryExcelData_ColumnSelect(t *testing.T) {
	testFile := createTestExcelForAITool(t)
	defer os.Remove(testFile)

	tool := getQueryExcelDataTool(t)
	stdout, _ := execQueryExcelDataTool(t, tool, aitool.InvokeParams{
		"input":   testFile,
		"columns": "Name,Amount",
	})

	assert.Assert(t, strings.Contains(stdout, "Name"), "should contain Name column")
	assert.Assert(t, strings.Contains(stdout, "Amount"), "should contain Amount column")
	assert.Assert(t, strings.Contains(stdout, "Alice"), "should contain Alice data")
	t.Logf("stdout:\n%s", stdout)
}

func TestQueryExcelData_FilterGreaterThan(t *testing.T) {
	testFile := createTestExcelForAITool(t)
	defer os.Remove(testFile)

	tool := getQueryExcelDataTool(t)
	stdout, _ := execQueryExcelDataTool(t, tool, aitool.InvokeParams{
		"input":         testFile,
		"filter":        "Amount>50000",
		"output_format": "csv",
	})

	assert.Assert(t, strings.Contains(stdout, "80000"), "should contain Alice's 80000 row")
	assert.Assert(t, strings.Contains(stdout, "120000"), "should contain Charlie's 120000 row")
	t.Logf("stdout:\n%s", stdout)
}

func TestQueryExcelData_FilterEquals(t *testing.T) {
	testFile := createTestExcelForAITool(t)
	defer os.Remove(testFile)

	tool := getQueryExcelDataTool(t)
	stdout, _ := execQueryExcelDataTool(t, tool, aitool.InvokeParams{
		"input":         testFile,
		"filter":        "Type==income",
		"output_format": "csv",
	})

	assert.Assert(t, strings.Contains(stdout, "Alice"), "should contain Alice (income)")
	assert.Assert(t, strings.Contains(stdout, "Charlie"), "should contain Charlie (income)")
	assert.Assert(t, !strings.Contains(stdout, "expense"), "should not contain expense rows")
	t.Logf("stdout:\n%s", stdout)
}

func TestQueryExcelData_FilterAND(t *testing.T) {
	testFile := createTestExcelForAITool(t)
	defer os.Remove(testFile)

	tool := getQueryExcelDataTool(t)
	stdout, _ := execQueryExcelDataTool(t, tool, aitool.InvokeParams{
		"input":         testFile,
		"filter":        "Type==income AND Amount>60000",
		"output_format": "csv",
	})

	assert.Assert(t, strings.Contains(stdout, "80000"), "should contain Alice 80000")
	assert.Assert(t, strings.Contains(stdout, "120000"), "should contain Charlie 120000")
	assert.Assert(t, !strings.Contains(stdout, "50000"), "should not contain 50000 (not > 60000)")
	t.Logf("stdout:\n%s", stdout)
}

func TestQueryExcelData_SortAscending(t *testing.T) {
	testFile := createTestExcelForAITool(t)
	defer os.Remove(testFile)

	tool := getQueryExcelDataTool(t)
	stdout, _ := execQueryExcelDataTool(t, tool, aitool.InvokeParams{
		"input":         testFile,
		"sort_by":       "Amount",
		"columns":       "Name,Amount",
		"output_format": "csv",
	})

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	var csvLines []string
	for _, line := range lines {
		if strings.Contains(line, ",") && !strings.HasPrefix(line, "[") {
			csvLines = append(csvLines, line)
		}
	}
	assert.Assert(t, len(csvLines) >= 2, "should have header + data rows in CSV")
	t.Logf("stdout:\n%s", stdout)
}

func TestQueryExcelData_SortDescending(t *testing.T) {
	testFile := createTestExcelForAITool(t)
	defer os.Remove(testFile)

	tool := getQueryExcelDataTool(t)
	stdout, _ := execQueryExcelDataTool(t, tool, aitool.InvokeParams{
		"input":         testFile,
		"sort_by":       "Amount",
		"sort_desc":     true,
		"columns":       "Name,Amount",
		"output_format": "csv",
	})

	assert.Assert(t, strings.Contains(stdout, "Name,Amount"), "should have CSV header")
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	var dataLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, ",") && !strings.HasPrefix(line, "[") && line != "Name,Amount" {
			dataLines = append(dataLines, line)
		}
	}
	if len(dataLines) >= 2 {
		assert.Assert(t, strings.Contains(dataLines[0], "120000"), "first row should be largest amount (120000)")
	}
	t.Logf("stdout:\n%s", stdout)
}

func TestQueryExcelData_Pagination(t *testing.T) {
	testFile := createTestExcelForAITool(t)
	defer os.Remove(testFile)

	tool := getQueryExcelDataTool(t)
	stdout, _ := execQueryExcelDataTool(t, tool, aitool.InvokeParams{
		"input":  testFile,
		"limit":  2,
		"offset": 0,
	})

	assert.Assert(t, strings.Contains(stdout, "Showing: 2"), "should show 2 rows")
	t.Logf("stdout:\n%s", stdout)
}

func TestQueryExcelData_GroupByCount(t *testing.T) {
	testFile := createTestExcelForAITool(t)
	defer os.Remove(testFile)

	tool := getQueryExcelDataTool(t)
	stdout, _ := execQueryExcelDataTool(t, tool, aitool.InvokeParams{
		"input":    testFile,
		"group_by": "Name",
		"agg":      "count",
	})

	assert.Assert(t, strings.Contains(stdout, "Alice"), "should contain Alice group")
	assert.Assert(t, strings.Contains(stdout, "Bob"), "should contain Bob group")
	assert.Assert(t, strings.Contains(stdout, "Charlie"), "should contain Charlie group")
	assert.Assert(t, strings.Contains(stdout, "grouped by"), "should show aggregation header")
	t.Logf("stdout:\n%s", stdout)
}

func TestQueryExcelData_GroupBySum(t *testing.T) {
	testFile := createTestExcelForAITool(t)
	defer os.Remove(testFile)

	tool := getQueryExcelDataTool(t)
	stdout, _ := execQueryExcelDataTool(t, tool, aitool.InvokeParams{
		"input":         testFile,
		"group_by":      "Name",
		"agg":           "sum",
		"agg_column":    "Amount",
		"sort_desc":     true,
		"output_format": "csv",
	})

	assert.Assert(t, strings.Contains(stdout, "sum(Amount)"), "should show sum(Amount) header")
	assert.Assert(t, strings.Contains(stdout, "Alice"), "should contain Alice group")
	assert.Assert(t, strings.Contains(stdout, "130000"), "should contain Alice sum (50000+80000)")
	t.Logf("stdout:\n%s", stdout)
}

func TestQueryExcelData_GroupByAvg(t *testing.T) {
	testFile := createTestExcelForAITool(t)
	defer os.Remove(testFile)

	tool := getQueryExcelDataTool(t)
	stdout, _ := execQueryExcelDataTool(t, tool, aitool.InvokeParams{
		"input":         testFile,
		"group_by":      "Type",
		"agg":           "avg",
		"agg_column":    "Amount",
		"output_format": "csv",
	})

	assert.Assert(t, strings.Contains(stdout, "avg(Amount)"), "should show avg(Amount) header")
	assert.Assert(t, strings.Contains(stdout, "income"), "should contain income group")
	assert.Assert(t, strings.Contains(stdout, "expense"), "should contain expense group")
	t.Logf("stdout:\n%s", stdout)
}

func TestQueryExcelData_OutputCSV(t *testing.T) {
	testFile := createTestExcelForAITool(t)
	defer os.Remove(testFile)

	tool := getQueryExcelDataTool(t)
	stdout, _ := execQueryExcelDataTool(t, tool, aitool.InvokeParams{
		"input":         testFile,
		"columns":       "Name,Amount",
		"output_format": "csv",
	})

	assert.Assert(t, strings.Contains(stdout, "Name,Amount"), "should contain CSV header")
	assert.Assert(t, strings.Contains(stdout, "Alice,"), "should contain CSV data")
	t.Logf("stdout:\n%s", stdout)
}

func TestQueryExcelData_OutputJSON(t *testing.T) {
	testFile := createTestExcelForAITool(t)
	defer os.Remove(testFile)

	tool := getQueryExcelDataTool(t)
	stdout, _ := execQueryExcelDataTool(t, tool, aitool.InvokeParams{
		"input":         testFile,
		"columns":       "Name,Amount",
		"limit":         2,
		"output_format": "json",
	})

	assert.Assert(t, strings.Contains(stdout, "\"Name\""), "should contain JSON key Name")
	assert.Assert(t, strings.Contains(stdout, "["), "should start with JSON array")
	t.Logf("stdout:\n%s", stdout)
}

func TestQueryExcelData_OutputToFile(t *testing.T) {
	testFile := createTestExcelForAITool(t)
	defer os.Remove(testFile)

	outputFile := filepath.Join(t.TempDir(), "query_result.csv")

	tool := getQueryExcelDataTool(t)
	stdout, _ := execQueryExcelDataTool(t, tool, aitool.InvokeParams{
		"input":         testFile,
		"output_format": "csv",
		"output":        outputFile,
	})

	assert.Assert(t, strings.Contains(stdout, "saved to"), "should confirm file saved")

	content, err := os.ReadFile(outputFile)
	assert.NilError(t, err, "output file should exist")
	assert.Assert(t, len(content) > 0, "output file should not be empty")
	t.Logf("output file content:\n%s", string(content))
}

func TestQueryExcelData_SpecificSheet(t *testing.T) {
	testFile := createTestExcelForAITool(t)
	defer os.Remove(testFile)

	tool := getQueryExcelDataTool(t)
	stdout, _ := execQueryExcelDataTool(t, tool, aitool.InvokeParams{
		"input": testFile,
		"sheet": "Summary",
	})

	assert.Assert(t, strings.Contains(stdout, "income"), "should contain income category")
	assert.Assert(t, strings.Contains(stdout, "expense"), "should contain expense category")
	assert.Assert(t, strings.Contains(stdout, "250000"), "should contain income total")
	t.Logf("stdout:\n%s", stdout)
}

func TestQueryExcelData_NonExistentFile(t *testing.T) {
	tool := getQueryExcelDataTool(t)
	stdout, _ := execQueryExcelDataTool(t, tool, aitool.InvokeParams{
		"input": "/nonexistent/path/file.xlsx",
	})

	assert.Assert(t, strings.Contains(stdout, "does not exist"), "should report file not found")
}

func TestQueryExcelData_InvalidColumn(t *testing.T) {
	testFile := createTestExcelForAITool(t)
	defer os.Remove(testFile)

	tool := getQueryExcelDataTool(t)
	stdout, _ := execQueryExcelDataTool(t, tool, aitool.InvokeParams{
		"input":   testFile,
		"columns": "NonExistent",
	})

	assert.Assert(t, strings.Contains(stdout, "not found"), "should report column not found")
}

func TestQueryExcelData_InvalidFilter(t *testing.T) {
	testFile := createTestExcelForAITool(t)
	defer os.Remove(testFile)

	tool := getQueryExcelDataTool(t)
	stdout, _ := execQueryExcelDataTool(t, tool, aitool.InvokeParams{
		"input":  testFile,
		"filter": "InvalidColumn>100",
	})

	assert.Assert(t, strings.Contains(stdout, "not found"), "should report invalid filter column")
}
