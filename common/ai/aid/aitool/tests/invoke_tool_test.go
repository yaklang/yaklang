package tests

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"

	_ "github.com/yaklang/yaklang/common/ai/rag/plugins_rag"
)

type AiToolTestCase struct {
	Name  string
	Input aitool.InvokeParams
	Data  string
}

var ToolsMap = map[string]*aitool.Tool{}

func init() {
	tools := buildinaitools.GetAllTools()
	for _, tool := range tools {
		ToolsMap[tool.Name] = tool
	}
}
func GetTool(t *testing.T, name string) *aitool.Tool {
	tool, ok := ToolsMap[name]
	if !ok {
		t.Fatalf("tool %s not found", name)
	}
	return tool
}
func CheckAiTool(t *testing.T, testCase *AiToolTestCase) {
	tool, ok := ToolsMap[testCase.Name]
	if !ok {
		t.Fatalf("tool %s not found", testCase.Name)
	}
	result, err := tool.InvokeWithParams(testCase.Input)
	if err != nil {
		t.Fatalf("invoke tool %s failed: %v", testCase.Name, err)
	}
	res, ok := result.Data.(*aitool.ToolExecutionResult)
	if !ok {
		t.Fatalf("tool %s data type mismatch: %T != %T", testCase.Name, result.Data, &aitool.ToolExecutionResult{})
	}
	resJson := res.Result.(string)
	resMap := map[string]interface{}{}
	err = json.Unmarshal([]byte(resJson), &resMap)
	if err != nil {
		t.Fatalf("tool %s data unmarshal failed: %v", testCase.Name, err)
	}
	ok = compare(resMap["results"], testCase.Data)
	if !ok {
		t.Fatalf("tool %s data mismatch: %s != %s", testCase.Name, resJson, testCase.Data)
	}
}

func compare(a, b interface{}) bool {
	aStr := fmt.Sprintf("%v", a)
	bStr := fmt.Sprintf("%v", b)
	return aStr == bStr
}
