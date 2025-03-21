package aid

import (
	"strings"
	"testing"
)

func TestEnhancedMetadata(t *testing.T) {
	// 测试使用多种元数据创建PlanRequest
	request, err := createPlanRequest(
		"创建一个电子商务网站",
		WithPlan_MetaInfo("这是一个综合元信息测试"),
		WithPlan_Framework("Spring Boot"),
		WithPlan_Language("Java"),
		WithPlan_Environment("AWS"),
		WithPlan_TargetPlatform("Web, iOS, Android"),
		WithPlan_APIVersion("v2.0"),
		WithPlan_DbType("MySQL"),
		WithPlan_SecurityLevel("高 - 需要支持HTTPS和数据加密"),
		WithPlan_Performance("支持1000并发用户"),
		WithPlan_Deadline("2023-12-31"),
		WithPlan_Budget("$50,000"),
		WithPlan_UserLevel("高级开发者"),
		WithPlan_MetaData("团队规模", "5人"),
		// 添加一个模拟的AICallback
		WithPlan_AICallback(func(req *AIRequest) (*AIResponse, error) {
			resp := NewAIResponse()
			defer resp.Close()
			resp.EmitOutputStream(strings.NewReader(`{"@action":"plan","query":"创建一个电子商务网站","tasks":[{"subtask_name":"测试子任务","subtask_goal":"测试目标"}]}`))
			return resp, nil
		}),
	)

	if err != nil {
		t.Fatalf("创建PlanRequest失败: %v", err)
	}

	// 检查所有元数据是否正确设置
	checkMetadata := func(key string, expected string) {
		if value, ok := request.MetaData[key]; !ok {
			t.Errorf("元数据 %s 未设置", key)
		} else if value != expected {
			t.Errorf("元数据 %s 不匹配，期望: %s, 实际: %v", key, expected, value)
		}
	}

	checkMetadata(MetaInfoKey, "这是一个综合元信息测试")
	checkMetadata(FrameworkKey, "Spring Boot")
	checkMetadata(LanguageKey, "Java")
	checkMetadata(EnvironmentKey, "AWS")
	checkMetadata(TargetPlatformKey, "Web, iOS, Android")
	checkMetadata(APIVersionKey, "v2.0")
	checkMetadata(DbTypeKey, "MySQL")
	checkMetadata(SecurityLevelKey, "高 - 需要支持HTTPS和数据加密")
	checkMetadata(PerformanceKey, "支持1000并发用户")
	checkMetadata(DeadlineKey, "2023-12-31")
	checkMetadata(BudgetKey, "$50,000")
	checkMetadata(UserLevelKey, "高级开发者")
	checkMetadata("团队规模", "5人")

	// 验证当前时间是否存在
	if _, ok := request.MetaData[CurrentTimeKey]; !ok {
		t.Error("默认的当前时间未设置")
	}

	// 生成prompt并验证内容
	prompt, err := request.GeneratePrompt()
	if err != nil {
		t.Fatalf("GeneratePrompt失败: %v", err)
	}

	// 检查所有元数据是否正确包含在prompt中
	expectedPhrases := []string{
		"这是一个综合元信息测试",
		"当前时间:",
		"使用框架: Spring Boot",
		"编程语言: Java",
		"运行环境: AWS",
		"目标平台: Web, iOS, Android",
		"API版本: v2.0",
		"数据库类型: MySQL",
		"安全级别要求: 高 - 需要支持HTTPS和数据加密",
		"性能要求: 支持1000并发用户",
		"截止日期: 2023-12-31",
		"预算: $50,000",
		"用户技术水平: 高级开发者",
		"团队规模: 5人",
	}

	for _, phrase := range expectedPhrases {
		if !strings.Contains(prompt, phrase) {
			t.Errorf("生成的prompt不包含预期短语: %s", phrase)
		}
	}

	t.Logf("生成的Prompt:\n%s", prompt)
}

func TestDefaultCurrentTime(t *testing.T) {
	// 测试默认添加当前时间
	request, err := createPlanRequest(
		"简单查询",
		// 添加一个模拟的AICallback
		WithPlan_AICallback(func(req *AIRequest) (*AIResponse, error) {
			resp := NewAIResponse()
			defer resp.Close()
			resp.EmitOutputStream(strings.NewReader(`{"@action":"plan","query":"简单查询","tasks":[{"subtask_name":"测试子任务","subtask_goal":"测试目标"}]}`))
			return resp, nil
		}),
	)
	if err != nil {
		t.Fatalf("创建PlanRequest失败: %v", err)
	}

	// 检查是否自动添加了当前时间
	if _, ok := request.MetaData[CurrentTimeKey]; !ok {
		t.Error("默认的当前时间未设置")
	}

	// 生成prompt
	prompt, err := request.GeneratePrompt()
	if err != nil {
		t.Fatalf("GeneratePrompt失败: %v", err)
	}

	// 验证prompt中包含当前时间
	if !strings.Contains(prompt, "当前时间:") {
		t.Error("生成的prompt不包含当前时间")
	}

	t.Logf("生成的Prompt (带默认当前时间):\n%s", prompt)
}

func TestOverrideCurrentTime(t *testing.T) {
	// 测试通过选项覆盖默认当前时间
	customTime := "2023-01-01 00:00:00"
	request, err := createPlanRequest(
		"覆盖当前时间的查询",
		WithPlan_MetaData(CurrentTimeKey, customTime),
		// 添加一个模拟的AICallback
		WithPlan_AICallback(func(req *AIRequest) (*AIResponse, error) {
			resp := NewAIResponse()
			defer resp.Close()
			resp.EmitOutputStream(strings.NewReader(`{"@action":"plan","query":"覆盖当前时间的查询","tasks":[{"subtask_name":"测试子任务","subtask_goal":"测试目标"}]}`))
			return resp, nil
		}),
	)
	if err != nil {
		t.Fatalf("创建PlanRequest失败: %v", err)
	}

	// 检查时间是否被正确覆盖
	if value, ok := request.MetaData[CurrentTimeKey]; !ok {
		t.Error("当前时间未设置")
	} else if value != customTime {
		t.Errorf("当前时间不匹配，期望: %s, 实际: %v", customTime, value)
	}

	// 生成prompt
	prompt, err := request.GeneratePrompt()
	if err != nil {
		t.Fatalf("GeneratePrompt失败: %v", err)
	}

	// 验证prompt中包含指定的时间
	expectedPhrase := "当前时间: " + customTime
	if !strings.Contains(prompt, expectedPhrase) {
		t.Errorf("生成的prompt不包含预期时间: %s", expectedPhrase)
	}

	t.Logf("生成的Prompt (带自定义时间):\n%s", prompt)
}

func TestWithCurrentTimeFunction(t *testing.T) {
	// 测试使用WithCurrentTime函数更新当前时间
	request, err := createPlanRequest(
		"使用WithCurrentTime的查询",
		WithPlan_CurrentTime(), // 显式调用WithCurrentTime会更新时间戳
		// 添加一个模拟的AICallback
		WithPlan_AICallback(func(req *AIRequest) (*AIResponse, error) {
			resp := NewAIResponse()
			defer resp.Close()
			resp.EmitOutputStream(strings.NewReader(`{"@action":"plan","query":"使用WithCurrentTime的查询","tasks":[{"subtask_name":"测试子任务","subtask_goal":"测试目标"}]}`))
			return resp, nil
		}),
	)
	if err != nil {
		t.Fatalf("创建PlanRequest失败: %v", err)
	}

	// 检查时间是否设置
	if _, ok := request.MetaData[CurrentTimeKey]; !ok {
		t.Error("当前时间未设置")
	}

	// 生成prompt
	prompt, err := request.GeneratePrompt()
	if err != nil {
		t.Fatalf("GeneratePrompt失败: %v", err)
	}

	// 验证prompt中包含当前时间
	if !strings.Contains(prompt, "当前时间:") {
		t.Error("生成的prompt不包含当前时间")
	}

	t.Logf("生成的Prompt (使用WithCurrentTime):\n%s", prompt)
}
