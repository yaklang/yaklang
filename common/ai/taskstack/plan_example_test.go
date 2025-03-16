package taskstack

import (
	"io"
	"strings"
	"testing"
)

func TestPlanRequestUsage(t *testing.T) {
	// 演示使用WithPlan_Query和WithPlan_MetaData辅助函数创建PlanRequest
	request, err := CreatePlanRequest(
		"编写一个REST API服务",
		WithPlan_MetaData("MetaInfo", "服务需要支持用户认证和数据加密"),
		WithPlan_MetaData("Framework", "Gin框架"),
		// 添加一个模拟的AICallback
		WithPlan_AICallback(func(prompt string) (io.Reader, error) {
			return strings.NewReader(`{"@action":"plan","query":"编写一个REST API服务","main_task":"创建REST API服务","main_task_goal":"完成一个基于Gin框架的REST API服务","tasks":[{"subtask_name":"设计API接口","subtask_goal":"定义所有API端点和数据结构"}]}`), nil
		}),
	)

	if err != nil {
		t.Fatalf("创建PlanRequest失败: %v", err)
	}

	// 检查request的内容
	if request.Query != "编写一个REST API服务" {
		t.Errorf("Query不匹配，期望: %s, 实际: %s", "编写一个REST API服务", request.Query)
	}

	// 检查元数据
	metaInfo, ok := request.MetaData["MetaInfo"]
	if !ok || metaInfo != "服务需要支持用户认证和数据加密" {
		t.Errorf("MetaInfo不匹配，期望: %s, 实际: %v", "服务需要支持用户认证和数据加密", metaInfo)
	}

	framework, ok := request.MetaData["Framework"]
	if !ok || framework != "Gin框架" {
		t.Errorf("Framework不匹配，期望: %s, 实际: %v", "Gin框架", framework)
	}

	// 生成prompt
	prompt, err := request.GeneratePrompt()
	if err != nil {
		t.Fatalf("生成prompt失败: %v", err)
	}

	// 验证prompt包含正确的内容
	if len(prompt) == 0 {
		t.Error("生成的prompt不应为空")
	}

	t.Logf("生成的Prompt预览: %s...", prompt[:100])
}
