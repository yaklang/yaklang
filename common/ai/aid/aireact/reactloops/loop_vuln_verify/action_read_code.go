package loop_vuln_verify

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// readCodeAction 读取代码文件
func readCodeAction(r aicommon.AIInvokeRuntime, state *VerifyState) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"read_code",
		"读取指定文件的代码内容，可以指定起始行和结束行。用于查看 Sink 点上下文、追踪数据流来源。",
		[]aitool.ToolOption{
			aitool.WithStringParam("file_path",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("要读取的文件路径")),
			aitool.WithIntegerParam("start_line",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("起始行号（从1开始），不指定则从文件开头读取")),
			aitool.WithIntegerParam("end_line",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("结束行号，不指定则读取到文件末尾（最多500行）")),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			filePath := action.GetString("file_path")
			startLine := int(action.GetInt("start_line"))
			endLine := int(action.GetInt("end_line"))

			// 获取项目根目录 - 从 loop 状态中获取，或者使用当前工作目录
			projectRoot := loop.Get("project_root")
			if projectRoot == "" {
				projectRoot = "."
			}

			// 构建完整路径
			fullPath := filePath
			if !filepath.IsAbs(filePath) {
				fullPath = filepath.Join(projectRoot, filePath)
			}

			// 检查文件是否存在
			if !utils.IsFile(fullPath) {
				operator.Fail(fmt.Sprintf("文件不存在: %s", fullPath))
				return
			}

			// 读取文件内容
			content, err := os.ReadFile(fullPath)
			if err != nil {
				operator.Fail(fmt.Sprintf("读取文件失败: %v", err))
				return
			}

			lines := strings.Split(string(content), "\n")
			totalLines := len(lines)

			// 处理行号范围
			if startLine <= 0 {
				startLine = 1
			}
			if endLine <= 0 || endLine > totalLines {
				endLine = totalLines
			}
			if startLine > totalLines {
				startLine = totalLines
			}
			if endLine < startLine {
				endLine = startLine
			}

			// 限制最大行数
			maxLines := 500
			if endLine-startLine+1 > maxLines {
				endLine = startLine + maxLines - 1
			}

			// 提取指定范围的行
			selectedLines := lines[startLine-1 : endLine]

			// 添加行号
			var result strings.Builder
			result.WriteString(fmt.Sprintf("=== %s (lines %d-%d of %d) ===\n\n", filePath, startLine, endLine, totalLines))
			for i, line := range selectedLines {
				lineNum := startLine + i
				result.WriteString(fmt.Sprintf("%4d | %s\n", lineNum, line))
			}

			// 记录已读取的文件
			state.AddReadFile(fmt.Sprintf("%s:%d-%d", filePath, startLine, endLine))

			// 记录到时间线
			r.AddToTimeline("read_code", fmt.Sprintf("读取文件: %s (行 %d-%d)", filePath, startLine, endLine))

			log.Infof("[VulnVerify] Read code: %s lines %d-%d", filePath, startLine, endLine)

			operator.Feedback(result.String())
		},
	)
}
