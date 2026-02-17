package loop_report_generating

import (
	"context"
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

// analyzeUserIntent 使用 LiteForge 分析用户意图（修改现有文件还是创建新文件）
func analyzeUserIntent(ctx context.Context, r aicommon.AIInvokeRuntime, userInput string, attachedFiles []string) (isModify bool, targetFile string, err error) {
	analysisPrompt := `分析用户的报告生成需求，判断是要修改现有文件还是创建新文件。

## 用户输入
<|USER_INPUT_{{ .nonce }}|>
{{ .userInput }}
<|USER_INPUT_END_{{ .nonce }}|>

## 附加文件列表
{{ if .attachedFiles }}
{{ range .attachedFiles }}- {{ . }}
{{ end }}
{{ else }}
（无附加文件）
{{ end }}

## 判断规则
1. 如果用户明确提到要"修改"、"编辑"、"更新"某个现有文件 → is_modify=true
2. 如果用户指定了输出文件路径（如"保存到 xxx.md"、"写入 xxx"）→ is_modify=true, target_file=指定路径
3. 如果用户要求"生成"、"创建"、"撰写"新报告 → is_modify=false
4. 如果用户提到现有报告需要"补充"、"完善" → is_modify=true

请分析并返回结果。`

	renderedPrompt := utils.MustRenderTemplate(analysisPrompt, map[string]any{
		"nonce":         utils.RandStringBytes(4),
		"userInput":     userInput,
		"attachedFiles": attachedFiles,
	})

	result, err := r.InvokeLiteForge(
		ctx,
		"analyze-report-intent",
		renderedPrompt,
		[]aitool.ToolOption{
			aitool.WithBoolParam("is_modify", aitool.WithParam_Description("是否是修改现有文件（true）还是创建新文件（false）"), aitool.WithParam_Required(true)),
			aitool.WithStringParam("target_file", aitool.WithParam_Description("如果用户指定了目标文件路径，返回该路径；否则返回空字符串")),
			aitool.WithStringParam("analysis_reason", aitool.WithParam_Description("简要说明判断理由")),
		},
		aicommon.WithGeneralConfigStreamableFieldWithNodeId("intent", "analysis_reason"),
	)

	if err != nil {
		log.Warnf("failed to analyze user intent: %v, defaulting to create new file", err)
		return false, "", nil
	}

	isModify = result.GetBool("is_modify")
	targetFile = result.GetString("target_file")
	reason := result.GetString("analysis_reason")

	log.Infof("report_generating intent analysis: is_modify=%v, target_file=%s, reason=%s", isModify, targetFile, reason)

	return isModify, targetFile, nil
}

// buildInitTask creates the initialization task handler for report generating loop
func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
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

		// Step 2: 使用 LiteForge 分析用户意图
		isModifyExisting := false
		if outputFilename == "" {
			isModify, targetFile, err := analyzeUserIntent(task.GetContext(), r, userInput, referenceFiles)
			if err == nil {
				isModifyExisting = isModify
				if targetFile != "" {
					outputFilename = targetFile
					log.Infof("report_generating: user specified target file: %s", outputFilename)
				}
			}
		}

		// Step 3: 确定最终的输出文件
		var existingContent string
		var isNewFile bool

		if outputFilename != "" {
			// 用户指定了文件路径，检查是否存在
			if utils.GetFirstExistedFile(outputFilename) != "" {
				// 文件存在，读取内容
				content, err := os.ReadFile(outputFilename)
				if err == nil && len(content) > 0 {
					existingContent = string(content)
					log.Infof("report_generating: loaded existing file content, size=%d bytes", len(content))
				}
				isNewFile = false
			} else {
				// 文件不存在，将会创建新文件
				isNewFile = true
				log.Infof("report_generating: target file does not exist, will create: %s", outputFilename)
			}
		} else {
			// 没有指定文件，创建新文件
			ext := ".md" // 默认使用 Markdown 格式
			if strings.Contains(strings.ToLower(userInput), "txt") ||
				strings.Contains(strings.ToLower(userInput), "纯文本") {
				ext = ".txt"
			}
			outputFilename = r.EmitFileArtifactWithExt("report", ext, "")
			isNewFile = true
			log.Infof("report_generating: created new output file artifact: %s", outputFilename)
		}

		// Step 4: 确保输出目录存在
		dir := filepath.Dir(outputFilename)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				log.Errorf("report_generating: failed to create output directory: %v", err)
			}
		}

		// Step 5: 如果是新文件，创建空文件并 emit artifact
		if isNewFile {
			if err := os.WriteFile(outputFilename, []byte(""), 0644); err != nil {
				log.Errorf("report_generating: failed to create output file: %v", err)
				operator.Failed(utils.Errorf("cannot create output file: %v", err))
				return
			}
			// 新创建的文件也要 emit artifact
			emitter.EmitPinFilename(outputFilename)
		} else {
			// 修改现有文件也要 emit artifact
			emitter.EmitPinFilename(outputFilename)
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

		// 构建模式描述
		var modeDescription string
		if isModifyExisting && existingContent != "" {
			modeDescription = fmt.Sprintf("修改模式：正在编辑现有文件，当前内容 %d 字节", len(existingContent))
		} else if isNewFile {
			modeDescription = "创建模式：正在创建新报告文件"
		} else {
			modeDescription = "创建模式：目标文件为空，将创建新内容"
		}

		// 初始化 loop 上下文（变量名与 loopinfra.SingleFileModificationSuiteFactory 对齐）
		loop.Set("report_filename", outputFilename)
		loop.Set("full_report_code", existingContent)
		loop.Set("user_requirements", userInput)
		loop.Set("reference_files", strings.Join(referenceFiles, ","))
		loop.Set("knowledge_bases", strings.Join(knowledgeBases, ","))
		loop.Set("available_files", availableFilesBuilder.String())
		loop.Set("available_knowledge_bases", availableKBBuilder.String())
		loop.Set("collected_references", "")
		loop.Set("is_modify_mode", fmt.Sprintf("%v", isModifyExisting && existingContent != ""))

		r.AddToTimeline("task_initialized", fmt.Sprintf("Report task initialized: %s, output=%s, ref_files=%d, knowledge_bases=%d",
			modeDescription, outputFilename, len(referenceFiles), len(knowledgeBases)))

		log.Infof("report_generating init completed: filename=%s, is_modify=%v, existing_content=%d bytes, references=%d, kbs=%d",
			outputFilename, isModifyExisting, len(existingContent), len(referenceFiles), len(knowledgeBases))

		// Default: Continue with normal loop execution
		operator.Continue()
	}
}
