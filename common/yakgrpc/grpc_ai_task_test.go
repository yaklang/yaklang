package yakgrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/schema"
	"math/rand"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestAITask(t *testing.T) {
	if utils.InGithubActions() {
		return
	}

	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx := utils.TimeoutContextSeconds(60)
	stream, err := client.StartAITask(ctx)
	if err != nil {
		t.Fatal(err)
	}

	tempDir := t.TempDir()
	tempFile, err := os.CreateTemp(tempDir, "*.txt")
	require.NoError(t, err)
	tempFile.WriteString("1+1")
	tempFile.Close()

	stream.Send(&ypb.AIInputEvent{
		IsStart: true,
		Params: &ypb.AIStartParams{
			UserQuery:                      "打开" + tempFile.Name() + "计算里面的表达式",
			EnableSystemFileSystemOperator: true,
			UseDefaultAIConfig:             true,
		},
	})

	for {
		event, err := stream.Recv()
		if err != nil {
			break
		}
		if event.IsStream {
			continue
		}
		fmt.Println(event.String())
	}
}

func TestAITaskWithBreadth(t *testing.T) {
	t.SkipNow()
	if t.Skipped() {
		return
	}

	client, err := NewLocalClientForceNew()
	require.NoError(t, err)

	n, m := rand.Intn(100)+10, rand.Intn(100)+10
	tempDir := t.TempDir()
	nFile, err := os.CreateTemp(tempDir, "*.txt")
	require.NoError(t, err)
	nFile.WriteString(fmt.Sprintf("%d", n))
	nFile.Close()
	mFile, err := os.CreateTemp(tempDir, "*.txt")
	require.NoError(t, err)
	mFile.WriteString(fmt.Sprintf("%d", m))
	mFile.Close()
	nFileName := strings.ReplaceAll(nFile.Name(), "\\", "/")
	mFileName := strings.ReplaceAll(mFile.Name(), "\\", "/")
	RegisterMockAIChat(func(prompt string, opts ...aispec.AIConfigOption) (string, error) {
		fmt.Println(strings.Repeat("=", 100))
		fmt.Println(prompt)
		fmt.Println(strings.Repeat("=", 100))
		// plan
		if strings.Contains(prompt, "你是一个输出JSON的任务规划的工具") {
			return fmt.Sprintf(`{
    "@action": "plan",
    "query": "打开%[1]s和%[2]s, 计算它们的和",
    "main_task": "计算两个文本文件中的数值和",
    "main_task_goal": "成功读取两个文件中的数值，并计算它们的和，输出结果",
    "tasks": [
        {
            "subtask_name": "读取文件1",
            "subtask_goal": "成功读取%[1]s文件内容"
        },
        {
            "subtask_name": "读取文件2",
            "subtask_goal": "成功读取%[2]s文件内容"
        },
        {
            "subtask_name": "计算和",
            "subtask_goal": "成功计算两个文件中的数值和，并输出结果"
        }
    ]
}`, nFileName, mFileName), nil
		} else if strings.Contains(prompt, "你是一个任务执行助手") && !strings.Contains(prompt, `你是一个按Schema输出JSON的上下文总结者`) {
			if strings.Contains(prompt, `"读取文件1"  (执行中)`) || strings.Contains(prompt, `"读取文件2"  (执行中)`) {
				return `{
					"@action": "require-tool",
					"tool": "read_file"
				}`, nil
			} else if strings.Contains(prompt, `"计算和"  (执行中)`) {
				return fmt.Sprintf(`根据当前任务状态，已经成功读取了两个文件的内容，分别为"%[1]d"和"%[2]d"。接下来，我们需要计算这两个数值的和。
由于已经获取了文件中的数值，我们可以直接进行计算，无需再调用其他工具。
计算结果如下：
%[1]d + %[2]d = %[3]d
因此，两个文件中的数值和为%[3]d。`, n, m, n+m), nil
			}
		} else if strings.Contains(prompt, "你决定调用下面的一个工具") && strings.Contains(prompt, `当前任务："读取文件`) {
			fName := ""
			if strings.Contains(prompt, `当前任务："读取文件1"`) {
				fName = nFileName
			} else if strings.Contains(prompt, `当前任务："读取文件2"`) {
				fName = mFileName
			}
			if fName != "" {
				return fmt.Sprintf(`{
					"tool": "read_file",
					"@action": "call-tool",
					"params": {
					  "path": "%s"
					}
				  }`, fName), nil
			}
		} else if utils.MatchAllOfSubString(prompt, `"continue-current-task"`, `"finished"`, `"status_summary"`) {
			return `{"@action": "finished"}`, nil
		} else if strings.Contains(prompt, "你是一个按Schema输出JSON的上下文总结者") {
			fName := ""
			num := 0
			if strings.Contains(prompt, `当前任务: "读取文件1"`) {
				fName = nFileName
				num = n
			} else if strings.Contains(prompt, `当前任务: "读取文件2"`) {
				fName = mFileName
				num = m
			}
			if fName != "" {
				return fmt.Sprintf(`{
  "@action": "summary",
  "short_summary": "任务执行助手正在执行'计算两个文本文件中的数值和'任务，当前进度为'读取文件1'，已成功读取文件内容。",
  "long_summary": "任务执行助手当前正在处理的任务是'计算两个文本文件中的数值和'。目前，任务进度停留在'读取文件1'阶段，目标是读取位于'%s'的文件内容。通过调用'read_file'工具，并设置参数'chunk_size'为2048，'offset'为0，成功读取了文件内容，结果为'%d'。接下来的任务步骤包括'读取文件2'和'计算和'。"
}`, fName, num), nil
			}

			if strings.Contains(prompt, `当前任务: "计算和"`) {
				return fmt.Sprintf(`{
  "@action": "summary",
  "short_summary": "任务执行助手已成功读取两个文本文件的内容，分别为%[1]d和%[2]d，并计算了它们的和为%[3]d。",
  "long_summary": "任务执行助手正在执行'计算两个文本文件中的数值和'任务。当前进度为'计算和'。已经成功读取了文件1和文件2的内容，分别为%[1]d和%[2]d。通过直接计算，得出了两个文件中的数值和为%[3]d。在此过程中，使用了'read_file'工具来读取文件内容，并成功获取了所需数据。"
}`, n, m, n+m), nil
			}
		} else if strings.Contains(prompt, `你是一个输出 Markdown 计划书和报告的工具`) {
			return `任务执行报告...`, nil
		}

		return "", fmt.Errorf("not implemented")
	})
	defer RegisterMockAIChat(nil)

	ctx := utils.TimeoutContextSeconds(60)
	stream, err := client.StartAITask(ctx)
	require.NoError(t, err)
	stream.Send(&ypb.AIInputEvent{
		IsStart: true,
		Params: &ypb.AIStartParams{
			UserQuery:                      "打开" + nFileName + "和" + mFileName + ", 计算它们的和",
			EnableSystemFileSystemOperator: true,
			UseDefaultAIConfig:             true,
		},
	})

	//
	//mock := mockey.Mock(ai.Chat).To().Build()
	//defer mock.Release()

	existMarkdownReport := false
	for {
		event, err := stream.Recv()
		if err != nil {
			break
		}
		if event.IsStream {
			continue
		}
		fmt.Println(event.String())
		if event.Type == "task_review_require" || event.Type == "plan_review_require" || event.Type == "tool_use_review_require" {
			interactiveId := gjson.GetBytes(event.Content, "id").String()
			stream.Send(&ypb.AIInputEvent{
				IsInteractiveMessage: true,
				InteractiveId:        interactiveId,
				InteractiveJSONInput: `{"suggestion": "continue"}`,
			})
		}
		if event.Type == "structured" && event.NodeId == "result" {
			existMarkdownReport = true
			break
		}
	}
	require.True(t, existMarkdownReport)
}

func TestAITaskWithAdjustPlan(t *testing.T) {
	t.SkipNow()
	if t.Skipped() {
		return
	}

	client, err := NewLocalClientForceNew()
	require.NoError(t, err)

	n, m := rand.Intn(100)+10, rand.Intn(100)+10
	tempDir := t.TempDir()
	nnFile, err := os.CreateTemp(tempDir, "*.txt")
	require.NoError(t, err)
	nnFile.WriteString(fmt.Sprintf("%d", n))
	nnFile.Close()
	mmFile, err := os.CreateTemp(tempDir, "*.txt")
	require.NoError(t, err)
	mmFile.WriteString(fmt.Sprintf("%d", m))
	mmFile.Close()

	nnFileName := strings.ReplaceAll(nnFile.Name(), "\\", "/")
	mmFileName := strings.ReplaceAll(mmFile.Name(), "\\", "/")

	nFile, err := os.CreateTemp(tempDir, "*.txt")
	require.NoError(t, err)
	nFile.WriteString(fmt.Sprintf("去读取%s", nnFileName))
	nFile.Close()
	mFile, err := os.CreateTemp(tempDir, "*.txt")
	require.NoError(t, err)
	mFile.WriteString(fmt.Sprintf("去读取%s", mmFileName))
	mFile.Close()
	nFileName := strings.ReplaceAll(nFile.Name(), "\\", "/")
	mFileName := strings.ReplaceAll(mFile.Name(), "\\", "/")

	RegisterMockAIChat(func(prompt string, opts ...aispec.AIConfigOption) (string, error) {
		fmt.Println(strings.Repeat("=", 100))
		fmt.Println(prompt)
		fmt.Println(strings.Repeat("=", 100))
		// plan
		if strings.Contains(prompt, "你是一个输出JSON的任务规划的工具") {
			return fmt.Sprintf(`{
    "@action": "plan",
    "query": "打开%[1]s和%[2]s, 计算它们的和",
    "main_task": "计算两个文本文件中的数值和",
    "main_task_goal": "成功读取两个文件中的数值，并计算它们的和，输出结果",
    "tasks": [
        {
            "subtask_name": "读取文件1",
            "subtask_goal": "成功读取%[1]s文件内容"
        },
        {
            "subtask_name": "读取文件2",
            "subtask_goal": "成功读取%[2]s文件内容"
        },
        {
            "subtask_name": "计算和",
            "subtask_goal": "成功计算两个文件中的数值和，并输出结果"
        }
    ]
}`, nFileName, mFileName), nil
		} else if strings.Contains(prompt, "你是一个任务执行助手") && !strings.Contains(prompt, `你是一个按Schema输出JSON的上下文总结者`) {
			if strings.Contains(prompt, `"读取文件1"  (执行中)`) || strings.Contains(prompt, `"读取文件2"  (执行中)`) || strings.Contains(prompt, `"读取引用文件1内容"  (执行中)`) || strings.Contains(prompt, `"读取引用文件2内容"  (执行中)`) {
				return `{
					"@action": "require-tool",
					"tool": "read_file"
				}`, nil
			}
		} else if strings.Contains(prompt, "你决定调用下面的一个工具") && strings.Contains(prompt, `当前任务："读取`) {
			fName := ""
			if strings.Contains(prompt, `当前任务："读取文件1"`) {
				fName = nFileName
			} else if strings.Contains(prompt, `当前任务："读取文件2"`) {
				fName = mFileName
			} else if strings.Contains(prompt, `当前任务："读取引用文件1内容"`) {
				fName = nnFileName
			} else if strings.Contains(prompt, `当前任务："读取引用文件2内容"`) {
				fName = mmFileName
			}
			if fName != "" {
				return fmt.Sprintf(`{
"tool": "read_file",
"@action": "call-tool",
"params": {
"path": "%s"
}
}`, fName), nil
			}
		} else if strings.Contains(prompt, `"计算最终数值和"  (执行中)`) {
			return fmt.Sprintf(`{
  "@action": "summary",
  "short_summary": "已完成文件1和文件2的引用内容读取，分别获取数值%[1]d和%[2]d。当前正在执行两个数值的求和计算，预计总和为%[3]d。",
  "long_summary": "任务已执行至最终计算阶段：\n1. 文件1路径(%[4]s)首次读取后，发现实际引用文件为%[5]s，其内容为%[1]d\n2. 文件2路径(%[6]s)首次读取后，发现实际引用文件为%[7]s，其内容为%[2]d\n3. 四次read_file工具调用中，前两次分别解析主文件路径获取真实路径，后两次成功提取数值内容\n4. 当前处于'计算最终数值和'阶段，无需工具调用，可直接对已获得的%[1]d和%[2]d执行加法运算，预期结果为%[3]d"
}`, n, m, n+m, nFileName, nnFileName, mFileName, mmFileName), nil
		} else if strings.Contains(prompt, "你是一个任务执行引擎，在完成用户任务的时候，并且成功执行了外部工具") && strings.Contains(prompt, `当前任务: "读取文件`) {
			return `{"@action": "finished"}`, nil
		} else if strings.Contains(prompt, `你是一个按Schema输出JSON的上下文总结者`) {
			if strings.Contains(prompt, `当前任务: "读取文件1"`) {
				return fmt.Sprintf(`{
                    "@action": "summary",
                    "short_summary": "当前任务为读取文件1，目标文件路径为%[1]s。已调用read_file工具，但结果中显示读取的是另一个文件路径。",
                    "long_summary": "当前任务为'读取文件1'，目标文件路径为%[1]s。在执行过程中，已调用read_file工具，工具参数设置为chunk_size为2048，offset为0，路径为目标文件路径。然而，工具调用结果显示实际读取的是另一个文件路径%[2]s，这可能导致任务执行出现偏差。需要进一步确认文件读取的正确性以确保任务目标达成。"
                  }`, nFileName, nnFileName), nil
			} else if strings.Contains(prompt, `当前任务: "读取文件2"`) {
				return fmt.Sprintf(`{
                    "@action": "summary",
                    "short_summary": "当前任务为读取文件2，目标文件路径为%[1]s。已调用read_file工具，但结果中显示读取的是另一个文件路径。",
                    "long_summary": "当前任务为'读取文件2'，目标文件路径为%[1]s。在执行过程中，已调用read_file工具，工具参数设置为chunk_size为2048，offset为0，路径为目标文件路径。然而，工具调用结果显示实际读取的是另一个文件路径%[2]s，这可能导致任务执行出现偏差。需要进一步确认文件读取的正确性以确保任务目标达成。"
                  }`, mFileName, mmFileName), nil
			} else if strings.Contains(prompt, `当前任务: "读取引用文件1内容"`) {
				return fmt.Sprintf(`{
  "@action": "summary",
  "short_summary": "任务执行至'读取引用文件1内容'阶段，已成功读取主文件路径并解析出实际引用文件路径。通过两次read_file工具调用，最终获取到目标文件数值内容'%[1]d'，为后续计算数值和奠定基础。",
  "long_summary": "任务执行按计划分阶段推进：\n1. 已完成阶段：\n   - 完成'读取文件1'子任务，调用read_file工具时原计划读取路径%[2]s，但工具返回结果提示实际需读取%[3]s\n   - 二次调用read_file工具成功读取实际引用文件，获得数值内容'%[1]d'\n\n2. 当前阶段：\n   - 正在执行'读取引用文件1内容'核心任务，已确认引用文件数值内容获取成功\n\n3. 待执行阶段：\n   - 需继续完成'读取文件2'和'计算最终数值和'任务\n\n关键转折点在于首次读取文件时系统返回的路径自动调整，通过工具反馈机制实现了任务路径的动态修正。"
}`, n, nFileName, nnFileName), nil
			} else if strings.Contains(prompt, `当前任务: "读取引用文件2内容"`) {
				return fmt.Sprintf(`{
  "@action": "summary",
  "short_summary": "任务执行至'读取引用文件2内容'阶段，已成功读取主文件路径并解析出实际引用文件路径。通过两次read_file工具调用，最终获取到目标文件数值内容'%[1]d'，为后续计算数值和奠定基础。",
  "long_summary": "任务执行按计划分阶段推进：\n1. 已完成阶段：\n   - 完成'读取文件2'子任务，调用read_file工具时原计划读取路径%[2]s，但工具返回结果提示实际需读取%[3]s\n   - 二次调用read_file工具成功读取实际引用文件，获得数值内容'%[1]d'\n\n2. 当前阶段：\n   - 正在执行'读取引用文件2内容'核心任务，已确认引用文件数值内容获取成功\n\n3. 待执行阶段：\n   - 需继续完成'读取文件1'和'计算最终数值和'任务\n\n关键转折点在于首次读取文件时系统返回的路径自动调整，通过工具反馈机制实现了任务路径的动态修正。"
}`, m, mFileName, mmFileName), nil
			}
		} else if strings.Contains(prompt, `当前虽然已经有了一个任务规划，但是我们在最近一步正在执行的任务过程中，发现从当前任务起重新规划需要重新规划任务`) {
			if strings.Contains(prompt, `当前任务: "读取文件1"`) {
				return fmt.Sprintf(`{
    "@action": "re-plan",
    "next_plans": [
        {
            "name": "读取引用文件1内容",
            "goal": "读取文件1中引用的新文件的实际数值内容"
        },
        {
            "name": "读取文件2",
            "goal": "成功读取%[2]s文件内容"
        },
        {
            "name": "计算最终数值和",
            "goal": "基于两个引用文件的实际数值内容执行求和计算"
        }
    ]
}`, nFileName, mFileName), nil
			} else if strings.Contains(prompt, `当前任务: "读取文件2"`) {
				return `{
    "@action": "re-plan",
    "next_plans": [
        {
            "name": "读取引用文件2内容",
            "goal": "读取文件2中引用的新文件的实际数值内容"
        },
        {
            "name": "计算最终数值和",
            "goal": "基于两个引用文件的实际数值内容执行求和计算"
        }
    ]
}`, nil
			}
		} else if strings.Contains(prompt, `你是一个输出 Markdown 计划书和报告的工具`) {
			return `任务执行报告...`, nil
		}
		return "", fmt.Errorf("not implemented")
	})

	ctx := utils.TimeoutContextSeconds(60)
	stream, err := client.StartAITask(ctx)
	require.NoError(t, err)
	stream.Send(&ypb.AIInputEvent{
		IsStart: true,
		Params: &ypb.AIStartParams{
			UserQuery:                      "打开" + nFileName + "和" + mFileName + ", 计算它们的和",
			EnableSystemFileSystemOperator: true,
			UseDefaultAIConfig:             true,
		},
	})

	existMarkdownReport := false
	reviewCount := 0
	for {
		event, err := stream.Recv()
		if err != nil {
			break
		}
		if event.IsStream {
			continue
		}
		fmt.Println(event.String())
		if event.Type == "task_review_require" {
			interactiveId := gjson.GetBytes(event.Content, "id").String()
			reviewCount++
			if reviewCount == 1 || reviewCount == 3 {
				m := make(map[string]any, 2)
				m["suggestion"] = "adjust_plan"
				m["plan"] = `需要修改任务规划,去读取文件里引用的文件`
				jsonBytes, _ := json.Marshal(m)
				stream.Send(&ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        interactiveId,
					InteractiveJSONInput: string(jsonBytes),
				})
			} else {
				stream.Send(&ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        interactiveId,
					InteractiveJSONInput: `{"suggestion": "continue"}`,
				})
			}
		} else if event.Type == "plan_review_require" {
			interactiveId := gjson.GetBytes(event.Content, "id").String()
			stream.Send(&ypb.AIInputEvent{
				IsInteractiveMessage: true,
				InteractiveId:        interactiveId,
				InteractiveJSONInput: `{"suggestion": "continue"}`,
			})
		} else if event.Type == "tool_use_review_require" {
			interactiveId := gjson.GetBytes(event.Content, "id").String()
			stream.Send(&ypb.AIInputEvent{
				IsInteractiveMessage: true,
				InteractiveId:        interactiveId,
				InteractiveJSONInput: `{"suggestion": "continue"}`,
			})
		}
		if event.Type == "structured" && event.NodeId == "result" {
			existMarkdownReport = true
			break
		}
	}
	require.True(t, existMarkdownReport)
}

func TestAITaskForge(t *testing.T) {
	if utils.InGithubActions() {
		return
	}

	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	stream, err := client.StartAITask(ctx)
	if err != nil {
		t.Fatal(err)
	}

	tempDir := t.TempDir()
	tempFile, err := os.CreateTemp(tempDir, "*.txt")
	require.NoError(t, err)
	tempFile.WriteString("1+1")
	tempFile.Close()

	stream.Send(&ypb.AIInputEvent{
		IsStart: true,
		Params: &ypb.AIStartParams{
			ForgeName: "long_text_summarizer",
			ForgeParams: []*ypb.ExecParamItem{
				{Key: "filePath", Value: "C:\\Users\\Rookie\\home\\code\\yaklang\\common\\aiforge\\aisecretary\\long_text_summarizer_data\\我的叔叔于勒.txt"},
			},
			UseDefaultAIConfig: true,
		},
	})

	for {
		event, err := stream.Recv()
		if err != nil {
			break
		}
		if event.IsStream {
			continue
		}
		fmt.Println(event.String())
	}
}

func TestAITaskForgeTriage(t *testing.T) {
	if utils.InGithubActions() {
		return
	}

	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	stream, err := client.StartAITask(ctx)
	if err != nil {
		t.Fatal(err)
	}

	stream.Send(&ypb.AIInputEvent{
		IsStart: true,
		Params: &ypb.AIStartParams{
			ForgeName:          "",
			UserQuery:          "我想做渗透测试",
			UseDefaultAIConfig: true,
		},
	})

	for {
		event, err := stream.Recv()
		if err != nil {
			break
		}
		if event.Type == schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE {
			eventId := ""
			jsonextractor.ExtractStructuredJSONFromStream(bytes.NewReader(event.Content), jsonextractor.WithObjectCallback(func(data map[string]any) {
				if id, ok := data["id"]; ok {
					eventId = id.(string)
				}
			}))
			stream.Send(&ypb.AIInputEvent{
				IsInteractiveMessage: true,
				InteractiveId:        eventId,
				InteractiveJSONInput: `{"suggestion": "xss"}`,
			})
		}
	}
}
