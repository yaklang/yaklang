package yakgrpc

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/ai"
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
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx := utils.TimeoutContextSeconds(60)
	stream, err := client.StartAITask(ctx)
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

	stream.Send(&ypb.AIInputEvent{
		IsStart: true,
		Params: &ypb.AIStartParams{
			UserQuery:                      "打开" + nFileName + "和" + mFileName + ", 计算它们的和",
			EnableSystemFileSystemOperator: true,
			UseDefaultAIConfig:             true,
		},
	})

	mock := mockey.Mock(ai.Chat).To(func(prompt string, opts ...aispec.AIConfigOption) (string, error) {
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
		} else if strings.Contains(prompt, "你是一个任务执行引擎，在完成用户任务的时候，并且成功执行了外部工具") && strings.Contains(prompt, `当前任务: "读取文件`) {
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
			return fmt.Sprintf(`任务执行报告
任务概述
本次任务的主要目标是计算两个文本文件中的数值和。任务分为三个步骤：读取文件1、读取文件2、以及计算两个文件中的数值和。
任务执行详情
1. 读取文件1

工具调用: read_file
调用参数: {"chunk_size":2048,"offset":0,"path":"%[1]s"}


执行结果: {"stdout":"","stderr":"","result":"%[3]d"}


状态: 成功读取文件1内容，数值为 %[3]d。

2. 读取文件2

工具调用: read_file
调用参数: {"chunk_size":2048,"offset":0,"path":"%[2]s"}


执行结果: {"stdout":"","stderr":"","result":"%[4]d"}


状态: 成功读取文件2内容，数值为 %[4]d。

3. 计算和

执行结果: 成功计算两个文件中的数值和。
计算过程: %[3]d + %[4]d = %[5]d
状态: 计算成功，结果为 %[5]d。

任务总结
本次任务顺利完成，成功读取了两个文本文件的内容，并计算了它们的数值和。最终结果为 151。
图表展示
pie
    title 数值分布
    "文件1" : %[3]d
    "文件2" : %[4]d

结论
通过本次任务，我们验证了文件读取和数值计算的流程，确保了数据的准确性和任务的可靠性。`, nFileName, mFileName, n, m, n+m), nil
		}

		return "", fmt.Errorf("not implemented")
	}).Build()
	defer mock.Release()

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
		if event.Type == "review_require" {
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
