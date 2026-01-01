package loop_report_generating

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// buildInitTask creates the initialization task handler for report generating loop
func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
		emitter := r.GetConfig().GetEmitter()
		userInput := task.GetUserInput()

		log.Infof("report_generating init task: analyzing user requirements")

		// Step 1: 解析用户输入，确定输出文件路径和参考资料
		var outputFilename string
		var referenceFiles []string
		var knowledgeBases []string

		// 获取附加的数据（文件、知识库等）
		attachedDatas := task.GetAttachedDatas()
		for _, data := range attachedDatas {
			log.Infof("report_generating attached data: type=%s, value=%s", data.Type, data.Value)
			switch data.Type {
			case aicommon.CONTEXT_PROVIDER_TYPE_FILE:
				referenceFiles = append(referenceFiles, data.Value)
			case aicommon.CONTEXT_PROVIDER_TYPE_KNOWLEDGE_BASE:
				knowledgeBases = append(knowledgeBases, data.Value)
			}
		}

		// 从配置中获取输出文件路径（如果有指定）
		config := r.GetConfig()
		outputPath := config.GetConfigString("output_file")
		if outputPath != "" {
			outputFilename = outputPath
			log.Infof("report_generating: using configured output file: %s", outputFilename)
		}

		// 如果没有指定输出文件，创建一个默认的
		if outputFilename == "" {
			// 根据用户需求确定文件扩展名
			ext := ".md" // 默认使用 Markdown 格式
			if strings.Contains(strings.ToLower(userInput), "txt") ||
				strings.Contains(strings.ToLower(userInput), "纯文本") {
				ext = ".txt"
			}
			outputFilename = r.EmitFileArtifactWithExt("report", ext, "")
			log.Infof("report_generating: created new output file: %s", outputFilename)
		}

		// 确保输出目录存在
		dir := filepath.Dir(outputFilename)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				log.Errorf("report_generating: failed to create output directory: %v", err)
			}
		}

		// 检查输出文件是否已存在，如果存在则读取内容
		var existingContent string
		if utils.GetFirstExistedFile(outputFilename) != "" {
			content, err := os.ReadFile(outputFilename)
			if err == nil && len(content) > 0 {
				existingContent = string(content)
				log.Infof("report_generating: loaded existing report content, size=%d bytes", len(content))
			}
		} else {
			// 创建空文件
			if err := os.WriteFile(outputFilename, []byte(""), 0644); err != nil {
				log.Errorf("report_generating: failed to create output file: %v", err)
				return utils.Errorf("cannot create output file: %v", err)
			}
		}

		// 构建可用文件列表
		var availableFilesBuilder strings.Builder
		if len(referenceFiles) > 0 {
			availableFilesBuilder.WriteString("### 可用参考文件\n")
			for _, f := range referenceFiles {
				availableFilesBuilder.WriteString(fmt.Sprintf("- %s\n", f))
			}
		}

		// 构建可用知识库列表
		var availableKBBuilder strings.Builder
		if len(knowledgeBases) > 0 {
			availableKBBuilder.WriteString("### 可用知识库\n")
			for _, kb := range knowledgeBases {
				availableKBBuilder.WriteString(fmt.Sprintf("- %s\n", kb))
			}
		}

		// 初始化 loop 上下文
		loop.Set("filename", outputFilename)
		loop.Set("full_report", existingContent)
		loop.Set("user_requirements", userInput)
		loop.Set("reference_files", strings.Join(referenceFiles, ","))
		loop.Set("knowledge_bases", strings.Join(knowledgeBases, ","))
		loop.Set("available_files", availableFilesBuilder.String())
		loop.Set("available_knowledge_bases", availableKBBuilder.String())
		loop.Set("collected_references", "")

		// 发送文件名事件
		emitter.EmitPinFilename(outputFilename)

		r.AddToTimeline("task_initialized", fmt.Sprintf("Report generating task initialized: output=%s, ref_files=%d, knowledge_bases=%d",
			outputFilename, len(referenceFiles), len(knowledgeBases)))

		log.Infof("report_generating init completed: filename=%s, references=%d, kbs=%d",
			outputFilename, len(referenceFiles), len(knowledgeBases))

		return nil
	}
}
