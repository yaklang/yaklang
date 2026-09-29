package enhancesearch

import (
	"bytes"
	"strings"
	"text/template"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var indexBuildPrompt = promptloader.MustLoad("inline/ai/rag/enhancesearch/build_questions/indexBuildPrompt.txt")

var questionSchema = []aitool.ToolOption{
	aitool.WithStringParam(
		"question",
		aitool.WithParam_Description(`上下文无关的检索问题。核心要求：1)禁用'该/这/上述'等指代词，使用具体技术术语(如'Go语言regexp包'而非'这个包')；2)必含语言名+技术栈+功能描述；3)问题独立完整可脱离代码理解。类型分布：实现方法40%('如何在[语言]中实现[功能]')、技术概念20%('[技术]的工作原理')、问题解决25%('如何优化[场景]性能')、代码模式15%('[功能]的通用架构')。优质例：'Go语言中正则表达式预编译的性能优势是什么？'；劣质例：'这段代码做什么？'；劣质例2：'testStr变量值是多少？'；`),
		aitool.WithParam_Required(true),
	),
	aitool.WithStructParam(
		"answer_location",
		[]aitool.PropertyOption{
			aitool.WithParam_Description("Specifies the exact location of the answer snippet within the input using line numbers."),
		},
		aitool.WithIntegerParam(
			"start_line",
			aitool.WithParam_Description("The starting line number of the snippet (inclusive, 1-based)."),
		),
		aitool.WithIntegerParam(
			"end_line",
			aitool.WithParam_Description("The ending line number of the snippet (inclusive, 1-based)."),
		),
	),
}

var indexBuildSchema = aitool.NewObjectSchemaWithAction(
	// 定义 question_list 字段，类型为字符串数组
	aitool.WithStructArrayParam(
		"question_list",
		[]aitool.PropertyOption{
			aitool.WithParam_Description("你是代码知识索引生成器，为代码片段生成10个以内的上下文无关、功能导向的检索问题。核心原则：1)上下文无关-禁用'该/这个/这段'等指代词，使用具体技术术语；2)技术明确-必须包含语言名称、核心技术栈、领域术语；3)检索友好-使用开发者常见搜索表达。问题类型分布：实现方法类40%(如'如何在Go中使用正则表达式提取邮箱？')、技术概念类20%(如'正则表达式预编译的性能优势是什么？')、问题解决类25%(如'如何优化大文本正则匹配性能？')、代码模式类15%(如'文本提取工具的通用架构是什么？')。优质示例：'Go语言regexp包的FindAllString方法如何使用？'；劣质示例：'这段代码的功能是什么？'。质量要求：问题独立完整、包含2-3个技术关键词、避免模糊指代、长度15-50字符、对其他开发者有参考价值。"),
		}, nil,
		// 定义数组中每个对象的字段
		questionSchema...,
	))

// queryPrompt 是本包的"动态内容"模板（包级私有副本）
// B 档改造：去掉 INPUT/EXTRA/OVERLAP 的内层 nonce
// 安全性：返回内容塞给 LiteForge.Params/Prompt 进 dynamic 段，外层 PROMPT_SECTION_dynamic_NONCE 已防 prompt-injection
// 关键词: aicache, PROMPT_SECTION, queryPrompt, B 档, 去 nonce
var queryPrompt = promptloader.MustLoad("inline/ai/rag/enhancesearch/build_questions/queryPrompt.txt")

// LiteForgeQueryFromChunk 包级兼容函数：B 档改造把模板拆为"调用方稳定指令头部"+"动态内容"
// 此函数仅返回拼接版（兼容老调用方）；新调用方应使用 BuildLiteForgeStaticAndDynamic 拆开传给 LiteForge
// 关键词: aicache, PROMPT_SECTION, LiteForgeQueryFromChunk, B 档兼容
func LiteForgeQueryFromChunk(prompt string, extraPrompt string, chunk chunkmaker.Chunk, overlapSize int) (string, error) {
	_, dynamic, err := BuildLiteForgeStaticAndDynamic(extraPrompt, chunk, overlapSize)
	if err != nil {
		return "", err
	}
	return prompt + "\n" + dynamic, nil
}

// BuildLiteForgeStaticAndDynamic 拆分 chunk 内容为静态/动态两段（B 档新增）
// 静态部分（调用方稳定指令头部）由调用方自己保管，传给 LiteForge.StaticInstruction 进 high-static 段
// 动态部分（INPUT/EXTRA/OVERLAP）传给 LiteForge.Prompt 进 dynamic 段
// 关键词: aicache, PROMPT_SECTION, BuildLiteForgeStaticAndDynamic, B 档拆分
func BuildLiteForgeStaticAndDynamic(extraPrompt string, chunk chunkmaker.Chunk, overlapSize int) (static string, dynamic string, err error) {
	param := map[string]interface{}{
		"INPUT": string(chunk.Data()),
		"EXTRA": extraPrompt,
	}
	if overlapSize > 0 || chunk.HaveLastChunk() {
		param["OVERLAP"] = string(chunk.PrevNBytes(overlapSize))
	}

	queryTemplate, parseErr := template.New("query").Parse(queryPrompt)
	if parseErr != nil {
		return "", "", parseErr
	}
	var buf bytes.Buffer
	if execErr := queryTemplate.ExecuteTemplate(&buf, "query", param); execErr != nil {
		return "", "", execErr
	}
	return "", buf.String(), nil
}

func BuildIndexQuestions(rawInput []string, aiService aicommon.AICallbackType) (map[string][]string, error) {
	linedInput := utils.PrefixLinesWithLineNumbers(rawInput)

	// 关键词: aicache, PROMPT_SECTION, StaticInstruction, BuildIndexQuestions, B 档
	// indexBuildPrompt 是稳定指令，通过 StaticInstruction 进入 high-static 段，跨调用稳定哈希
	// 动态内容（INPUT 等）通过 query 走 Params/dynamic 段
	_, dynamic, err := BuildLiteForgeStaticAndDynamic("", chunkmaker.NewBufferChunk([]byte(linedInput)), 200)
	if err != nil {
		return nil, err
	}

	forgeOpts := []any{
		aicommon.WithLiteForgeOutputSchema(indexBuildSchema),
		aicommon.LiteForgeStaticInstruction(indexBuildPrompt),
	}
	if aiService != nil {
		forgeOpts = append(forgeOpts, aicommon.WithAICallback(aiService))
	} else {
		forgeOpts = append(forgeOpts, aicommon.WithAICallback(aicommon.MustGetSpeedPriorityAIModelCallback()))
	}

	result, err := aicommon.InvokeLiteForge(dynamic, forgeOpts...)
	if err != nil {
		return nil, err
	}

	entries, err := index2KnowledgeEntity(result.Action, rawInput)
	if err != nil {
		log.Errorf("failed to convert action to knowledge base entries: %v", err)
		return nil, err
	}
	return entries, nil
}

func index2KnowledgeEntity(
	action *aicommon.Action,
	inputs []string,
) (map[string][]string, error) {
	if action == nil {
		return nil, utils.Errorf("action is nil")
	}
	input := strings.Join(inputs, "\n")
	inputLineList := utils.ParseStringToRawLines(input)

	questionList := action.GetInvokeParamsArray("question_list")
	if len(questionList) == 0 {
		return nil, utils.Errorf("no knowledge-collection found in action")
	}

	knowledgeMap := make(map[string][]string)

	safeGetSnippet := func(startLine, endLine int) string {
		if startLine < 1 || endLine > len(inputLineList) || startLine > endLine {
			return input // fallback to full input
		}
		return strings.Join(inputLineList[startLine-1:endLine], "\n")
	}

	for _, item := range questionList {
		question := item.GetString("question")
		answerLocations := item.GetObject("answer_location")
		startLine := answerLocations.GetInt("start_line")
		endLine := answerLocations.GetInt("end_line")

		posHash := safeGetSnippet(int(startLine), int(endLine))
		if knowledge, exists := knowledgeMap[posHash]; exists {
			knowledge = append(knowledge, question)
			knowledgeMap[posHash] = knowledge
		} else {
			knowledgeMap[posHash] = []string{question}
		}
	}

	return knowledgeMap, nil
}
