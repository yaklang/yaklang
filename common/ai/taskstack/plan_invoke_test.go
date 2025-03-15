package taskstack

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestPlanRequest_Invoke 测试PlanRequest的Invoke方法
func TestPlanRequest_Invoke(t *testing.T) {
	// 模拟的AI响应
	mockResponse := `{
    "name": "旅游规划",
    "goal": "规划一次完美的旅行",
    "subtasks": [
        {
            "name": "选择目的地",
            "goal": "根据用户需求选择合适的旅游城市"
        },
        {
            "name": "确定行程",
            "goal": "安排详细的日程计划"
        },
        {
            "name": "预算规划",
            "goal": "控制旅行总体花费"
        }
    ]
}`

	// 创建 PlanRequest 并设置mock回调
	request, err := CreatePlanRequest(
		"帮我计划一次旅游",
		WithLanguage("Go"),
		WithMetaInfo("这是一个旅游计划任务"),
		WithAICallback(func(prompt string) (io.Reader, error) {
			// 验证prompt包含期望的内容
			assert.Contains(t, prompt, "帮我计划一次旅游")
			assert.Contains(t, prompt, "编程语言: Go")
			assert.Contains(t, prompt, "这是一个旅游计划任务")

			return strings.NewReader(mockResponse), nil
		}),
	)
	assert.NoError(t, err)
	assert.NotNil(t, request)

	// 调用Invoke方法
	response, err := request.Invoke()
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.NotNil(t, response.RootTask)

	// 验证根任务
	assert.Equal(t, "旅游规划", response.RootTask.Name)
	assert.Equal(t, "规划一次完美的旅行", response.RootTask.Goal)
	assert.Len(t, response.RootTask.Subtasks, 3)

	// 验证子任务
	assert.Equal(t, "选择目的地", response.RootTask.Subtasks[0].Name)
	assert.Equal(t, "确定行程", response.RootTask.Subtasks[1].Name)
	assert.Equal(t, "预算规划", response.RootTask.Subtasks[2].Name)

	// 验证回调函数设置
	assert.NotNil(t, response.RootTask.AICallback)
}

// TestPlanRequest_InvokeWithCustomCallback 测试使用自定义回调函数的Invoke方法
func TestPlanRequest_InvokeWithCustomCallback(t *testing.T) {
	// 模拟的AI响应
	mockResponse := `{
    "name": "创建旅游计划",
    "goal": "为用户制定完整的旅游计划",
    "subtasks": [
        {
            "name": "确定旅游目的地",
            "goal": "根据用户偏好选择最佳旅游城市"
        },
        {
            "name": "安排交通方式",
            "goal": "规划往返目的地的最优交通方案"
        }
    ]
}`

	// 自定义的AICallback
	customCallback := func(prompt string) (io.Reader, error) {
		return strings.NewReader(mockResponse), nil
	}

	// 创建带有自定义回调的PlanRequest
	request, err := CreatePlanRequest(
		"帮我计划一次旅游",
		WithLanguage("Go"),
		WithAICallback(customCallback),
	)
	assert.NoError(t, err)
	assert.NotNil(t, request)

	// 调用Invoke方法
	response, err := request.Invoke()
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.NotNil(t, response.RootTask)

	// 验证任务内容
	assert.Equal(t, "创建旅游计划", response.RootTask.Name)
	assert.Equal(t, "为用户制定完整的旅游计划", response.RootTask.Goal)
	assert.Len(t, response.RootTask.Subtasks, 2)

	// 验证回调函数设置
	assert.NotNil(t, response.RootTask.AICallback)

	// 验证子任务的回调函数也被设置
	for _, subtask := range response.RootTask.Subtasks {
		assert.NotNil(t, subtask.AICallback)
	}
}

// TestPlanRequest_ExtractTask 测试从原始响应中提取Task
func TestPlanRequest_ExtractTask(t *testing.T) {
	// 原始 AI 响应示例
	mockAIResponse := `{
    "name": "创建旅游计划",
    "goal": "为用户制定完整的旅游计划",
    "subtasks": [
        {
            "name": "确定旅游目的地",
            "goal": "根据用户偏好选择最佳旅游城市"
        },
        {
            "name": "安排交通方式",
            "goal": "规划往返目的地的最优交通方案"
        },
        {
            "name": "选择住宿地点",
            "goal": "找到舒适且符合预算的住宿选项"
        },
        {
            "name": "制定景点行程",
            "goal": "安排每日游览景点和活动时间表"
        }
    ]
}`

	// 从响应中提取Task
	task, err := ExtractTaskFromRawResponse(mockAIResponse)
	assert.NoError(t, err)
	assert.NotNil(t, task)

	// 验证任务内容
	assert.Equal(t, "创建旅游计划", task.Name)
	assert.Equal(t, "为用户制定完整的旅游计划", task.Goal)
	assert.Len(t, task.Subtasks, 4)

	// 验证第一个子任务
	assert.Equal(t, "确定旅游目的地", task.Subtasks[0].Name)
	assert.Equal(t, "根据用户偏好选择最佳旅游城市", task.Subtasks[0].Goal)
}

// TestPlanRequest_InvokeWithNoiseResponse 测试在AI回复包含大量干扰信息时，Invoke方法的鲁棒性
func TestPlanRequest_InvokeWithNoiseResponse(t *testing.T) {
	// 模拟的AI响应，包含大量干扰信息
	mockNoisyResponse := `我将帮您计划一次旅游。基于您的需求，我制定了以下任务列表：

首先，让我思考一下旅游规划的基本步骤。一般来说，旅游规划需要考虑目的地选择、交通安排、住宿预订、行程安排等多个方面。
还需要考虑预算控制、时间分配等因素。

根据您的需求，我生成了以下任务计划：

{
    "name": "度假旅行规划",
    "goal": "制定一个全面且可行的旅游计划",
    "subtasks": [
        {
            "name": "目的地调研与选择",
            "goal": "基于季节、预算和个人偏好选择最佳旅游目的地"
        },
        {
            "name": "交通方案规划",
            "goal": "安排往返和当地交通，平衡便捷性和成本"
        },
        {
            "name": "住宿预订策略",
            "goal": "寻找合适的住宿选项并优化预订时机"
        }
    ]
}

希望这个规划对您有所帮助！如果您需要更详细的计划，我可以进一步细化上述任务。每个子任务还可以分解为更具体的步骤，例如目的地调研可以包括查询天气情况、了解当地文化和确认旅游景点开放时间等。

您还可以考虑添加以下内容到计划中：
1. 行李打包清单
2. 旅游保险购买
3. 紧急联系人安排
4. 当地货币兑换计划

祝您旅途愉快！`

	// 创建带有自定义回调的PlanRequest
	request, err := CreatePlanRequest(
		"帮我计划一次旅游",
		WithLanguage("Go"),
		WithAICallback(func(prompt string) (io.Reader, error) {
			return strings.NewReader(mockNoisyResponse), nil
		}),
	)
	assert.NoError(t, err)
	assert.NotNil(t, request)

	// 调用Invoke方法
	response, err := request.Invoke()
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.NotNil(t, response.RootTask)

	// 验证从嘈杂数据中正确提取了任务内容
	assert.Equal(t, "度假旅行规划", response.RootTask.Name)
	assert.Equal(t, "制定一个全面且可行的旅游计划", response.RootTask.Goal)
	assert.Len(t, response.RootTask.Subtasks, 3)

	// 验证子任务
	assert.Equal(t, "目的地调研与选择", response.RootTask.Subtasks[0].Name)
	assert.Equal(t, "基于季节、预算和个人偏好选择最佳旅游目的地", response.RootTask.Subtasks[0].Goal)
	assert.Equal(t, "交通方案规划", response.RootTask.Subtasks[1].Name)
}

// TestPlanRequest_InvokeWithMultipleJSON 测试当AI输出多个JSON对象或更混乱的格式时的健壮性
func TestPlanRequest_InvokeWithMultipleJSON(t *testing.T) {
	// 模拟的AI响应，包含多个JSON对象和混乱的格式
	mockChaosResponse := `
首先，我认为这个旅游计划需要考虑多个方面。

这里是一个初步的想法：
{"idea": "先规划大致行程，再确定细节"}

但更完整的计划应该是这样的：

{ 
  "临时想法": "这只是草稿，最终计划在下面",
  "不要采用这个": {
    "name": "错误的计划",
    "goal": "这不是正确的计划",
    "subtasks": []
  }
}

好的，以下是我建议的完整任务列表：

{
    "name": "综合旅游规划方案",
    "goal": "创建一个详尽的旅游计划，涵盖所有必要环节",
    "subtasks": [
        {
            "name": "旅行前准备",
            "goal": "完成所有出行前的必要准备工作"
        },
        {
            "name": "旅途安排",
            "goal": "规划旅途中的各项活动和时间分配"
        },
        {
            "name": "应急预案",
            "goal": "准备应对可能出现的突发情况"
        },
        {
            "name": "返程计划",
            "goal": "安排返程相关事宜，确保旅行圆满结束"
        }
    ]
}

此外，我还想补充一些额外建议：
{
    "额外建议": [
        "携带必要的药品",
        "准备多种支付方式",
        "提前下载离线地图"
    ]
}

希望以上计划对您有帮助！`

	// 创建带有自定义回调的PlanRequest
	request, err := CreatePlanRequest(
		"帮我规划一次复杂的旅行",
		WithAICallback(func(prompt string) (io.Reader, error) {
			return strings.NewReader(mockChaosResponse), nil
		}),
	)
	assert.NoError(t, err)
	assert.NotNil(t, request)

	// 调用Invoke方法
	response, err := request.Invoke()
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.NotNil(t, response.RootTask)

	// 验证正确提取了完整任务
	assert.Equal(t, "综合旅游规划方案", response.RootTask.Name)
	assert.Equal(t, "创建一个详尽的旅游计划，涵盖所有必要环节", response.RootTask.Goal)
	assert.Len(t, response.RootTask.Subtasks, 4)

	// 验证子任务
	expectedSubtasks := []string{"旅行前准备", "旅途安排", "应急预案", "返程计划"}
	for i, expectedName := range expectedSubtasks {
		assert.Equal(t, expectedName, response.RootTask.Subtasks[i].Name)
	}
}

// TestPlanRequest_InvokeWithMalformedResponse 测试当AI回复格式严重错误时的错误处理
func TestPlanRequest_InvokeWithMalformedResponse(t *testing.T) {
	// 模拟的AI响应，完全没有有效的JSON结构
	mockMalformedResponse := `
我理解您需要一个旅游计划。以下是我的建议：

1. 确定目的地
2. 预订机票和酒店
3. 规划每日行程
4. 准备必要物品
5. 办理相关证件

希望这个计划对您有所帮助！如有其他问题，请随时提问。
`

	// 创建带有自定义回调的PlanRequest
	request, err := CreatePlanRequest(
		"帮我计划一次旅游",
		WithAICallback(func(prompt string) (io.Reader, error) {
			return strings.NewReader(mockMalformedResponse), nil
		}),
	)
	assert.NoError(t, err)
	assert.NotNil(t, request)

	// 调用Invoke方法，预期会失败
	response, err := request.Invoke()
	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "从 AI 响应中提取任务失败")
}

// TestPlanRequest_InvokeWithMarkdownCodeBlock 测试当AI回复包含Markdown代码块时的提取
func TestPlanRequest_InvokeWithMarkdownCodeBlock(t *testing.T) {
	// 模拟的AI响应，使用Markdown代码块包装JSON
	mockMarkdownResponse := `
# 旅游规划方案

为您制定了以下旅游计划：

## 核心任务

` + "```json" + `
{
    "name": "豪华欧洲游",
    "goal": "体验欧洲文化和美食",
    "subtasks": [
        {
            "name": "选择欧洲国家",
            "goal": "根据兴趣和季节选择2-3个国家"
        },
        {
            "name": "签证办理",
            "goal": "准备材料并申请申根签证"
        },
        {
            "name": "预订机票和酒店",
            "goal": "寻找性价比高的交通和住宿"
        }
    ]
}
` + "```" + `

## 补充建议

在旅行前请确保：
1. 了解当地文化和礼仪
2. 准备必要的应急物品
3. 购买旅游保险

希望您旅途愉快！
`

	// 创建带有自定义回调的PlanRequest
	request, err := CreatePlanRequest(
		"帮我规划欧洲旅游",
		WithAICallback(func(prompt string) (io.Reader, error) {
			return strings.NewReader(mockMarkdownResponse), nil
		}),
	)
	assert.NoError(t, err)
	assert.NotNil(t, request)

	// 调用Invoke方法
	response, err := request.Invoke()
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.NotNil(t, response.RootTask)

	// 验证正确提取了任务
	assert.Equal(t, "豪华欧洲游", response.RootTask.Name)
	assert.Equal(t, "体验欧洲文化和美食", response.RootTask.Goal)
	assert.Len(t, response.RootTask.Subtasks, 3)

	// 验证子任务
	assert.Equal(t, "选择欧洲国家", response.RootTask.Subtasks[0].Name)
	assert.Equal(t, "签证办理", response.RootTask.Subtasks[1].Name)
	assert.Equal(t, "预订机票和酒店", response.RootTask.Subtasks[2].Name)
}
