package taskstack

import (
	"strings"
	"testing"
)

func TestPlanRequest_GeneratePrompt(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		metaInfo string
		want     []string // 期望的结果中应包含的字符串
	}{
		{
			name:     "基本查询",
			query:    "创建一个网站",
			metaInfo: "这是一些前置信息",
			want: []string{
				"输出要求：",
				"你是一个负责任务规划的工具",
				"前置信息",
				"这是一些前置信息",
				"用户输入",
				"创建一个网站",
			},
		},
		{
			name:  "无元信息的查询",
			query: "分析日志文件",
			want: []string{
				"输出要求：",
				"你是一个负责任务规划的工具",
				"前置信息",
				"用户输入",
				"分析日志文件",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建PlanRequest
			request := &PlanRequest{
				MetaData: map[string]any{},
				Query:    tt.query,
			}

			// 如果有元信息，添加到请求中
			if tt.metaInfo != "" {
				request.MetaData["MetaInfo"] = tt.metaInfo
			}

			// 生成prompt
			prompt, err := request.GeneratePrompt()
			if err != nil {
				t.Fatalf("GeneratePrompt失败: %v", err)
			}

			// 检查结果是否包含所有期望的字符串
			for _, wantStr := range tt.want {
				if !strings.Contains(prompt, wantStr) {
					t.Errorf("prompt不包含预期字符串: %s", wantStr)
				}
			}

			// 检查模板标记是否被替换
			if strings.Contains(prompt, "{{ .TaskJsonSchema }}") {
				t.Error("TaskJsonSchema标记未被替换")
			}

			if strings.Contains(prompt, "{{ .Query }}") {
				t.Error("Query标记未被替换")
			}

			if strings.Contains(prompt, "{{ .MetaInfo }}") {
				t.Error("MetaInfo标记未被替换")
			}

			t.Logf("生成的Prompt:\n%s", prompt)
		})
	}
}
