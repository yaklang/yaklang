package buildinaitools

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"gotest.tools/v3/assert"
)

// TestToolManagerWithNoToolsCache 测试不缓存工具
func TestToolManagerWithNoToolsCache(t *testing.T) {
	toolManager := NewToolManager(WithNoToolsCache())
	assert.Equal(t, toolManager.noCacheTools, true)
	// 验证 toolsGetter 不为 nil
	assert.Check(t, toolManager.toolsGetter != nil, "toolsGetter should not be nil")
}

// 测试 GetAllToolsDynamically 是否能返回新增工具
func TestGetAllToolsDynamically(t *testing.T) {
	// 注册 YakScript 工具转换函数（模拟 yak 包的 init 函数）
	yakscripttools.RegisterYakScriptAiToolsCovertHandle(func(aitools []*schema.AIYakTool) []*aitool.Tool {
		tools := []*aitool.Tool{}
		for _, aiTool := range aitools {
			tool := mcp.NewTool(aiTool.Name)
			tool.Description = aiTool.Description
			dataMap := map[string]any{}
			err := json.Unmarshal([]byte(aiTool.Params), &dataMap)
			if err != nil {
				log.Errorf("unmarshal aiTool.Params failed: %v", err)
				continue
			}
			tool.InputSchema.FromMap(dataMap)
			at, err := aitool.NewFromMCPTool(
				tool,
				aitool.WithDescription(aiTool.Description),
				aitool.WithKeywords(strings.Split(aiTool.Keywords, ",")),
				aitool.WithCallback(func(ctx context.Context, params aitool.InvokeParams, runtimeConfig *aitool.ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error) {
					// 简单的测试回调
					return "test result", nil
				}),
			)
			if err != nil {
				log.Errorf("create aitool failed: %v", err)
				continue
			}
			tools = append(tools, at)
		}
		return tools
	})

	tempDB, err := utils.CreateTempTestDatabaseInMemory()
	if err != nil {
		t.Fatalf("create temp test database: %v", err)
	}

	// 迁移数据库表结构
	err = tempDB.AutoMigrate(&schema.AIYakTool{}).Error
	if err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	// 1. 先查询初始工具数量（应该包含基础工具和内置工具）
	initialTools := GetAllToolsDynamically(tempDB)
	initialCount := len(initialTools)
	t.Logf("Initial tools count: %d", initialCount)

	// 验证初始工具中不包含我们要添加的测试工具
	testToolName := "test_dynamic_tool"
	for _, tool := range initialTools {
		assert.Check(t, tool.Name != testToolName, "test tool should not exist initially")
	}

	// 2. Mock 一个新的 AI Yak Tool 并保存到数据库
	newTool := &schema.AIYakTool{
		Name:        testToolName,
		VerboseName: "Test Dynamic Tool",
		Description: "This is a test tool for dynamic loading",
		Keywords:    "test,dynamic,mock",
		Content: `# Test Tool
yakit.AutoInitYakit()

cli.String("param1", cli.setDefault("default_value"), cli.setHelp("test parameter"))

param1 = cli.String("param1")
println("Test tool executed with param:", param1)
`,
		Params: `{"type":"object","properties":{"param1":{"type":"string","description":"test parameter","default":"default_value"}}}`,
		Path:   "test/dynamic",
	}

	// 保存工具到数据库
	_, err = yakit.SaveAIYakTool(tempDB, newTool)
	if err != nil {
		t.Fatalf("save AI yak tool failed: %v", err)
	}

	// 3. 再次调用 GetAllToolsDynamically 查询工具
	updatedTools := GetAllToolsDynamically(tempDB)
	updatedCount := len(updatedTools)
	t.Logf("Updated tools count: %d", updatedCount)

	// 4. 验证工具数量增加了
	assert.Check(t, updatedCount > initialCount, "tools count should increase after adding new tool")

	// 5. 验证能够查询到新增的工具
	found := false
	for _, tool := range updatedTools {
		if tool.Name == testToolName {
			found = true
			assert.Equal(t, tool.Description, newTool.Description)
			t.Logf("Found new tool: %s", tool.Name)
			break
		}
	}
	assert.Check(t, found, "newly added tool should be found in updated tools")

	// 6. 再添加一个工具，验证动态加载
	anotherToolName := "test_dynamic_tool_2"
	anotherTool := &schema.AIYakTool{
		Name:        anotherToolName,
		VerboseName: "Test Dynamic Tool 2",
		Description: "This is another test tool for dynamic loading",
		Keywords:    "test,dynamic,mock,second",
		Content: `# Another Test Tool
yakit.AutoInitYakit()

cli.String("param2", cli.setDefault("value2"), cli.setHelp("another test parameter"))

param2 = cli.String("param2")
println("Another test tool executed with param:", param2)
`,
		Params: `{"type":"object","properties":{"param2":{"type":"string","description":"another test parameter","default":"value2"}}}`,
		Path:   "test/dynamic2",
	}

	_, err = yakit.SaveAIYakTool(tempDB, anotherTool)
	if err != nil {
		t.Fatalf("save second AI yak tool failed: %v", err)
	}

	// 7. 第三次查询，验证两个工具都能被找到
	finalTools := GetAllToolsDynamically(tempDB)
	finalCount := len(finalTools)
	t.Logf("Final tools count: %d", finalCount)

	assert.Check(t, finalCount == updatedCount+1, "tools count should increase by 1 after adding second tool")

	// 验证两个新增的工具都能被找到
	foundFirst := false
	foundSecond := false
	for _, tool := range finalTools {
		if tool.Name == testToolName {
			foundFirst = true
		}
		if tool.Name == anotherToolName {
			foundSecond = true
		}
	}
	assert.Check(t, foundFirst, "first tool should still be found")
	assert.Check(t, foundSecond, "second tool should be found")
}
