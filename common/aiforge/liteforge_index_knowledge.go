package aiforge

import (
	_ "embed"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

//go:embed liteforge_prompt/knowledge_index_build.txt
var indexBuildPrompt string

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

func BuildIndexKnowledgeFromFile(kbName string, path string, option ...any) (<-chan *schema.KnowledgeBaseEntry, error) {
	analyzeResult, err := AnalyzeFile(path, option...)
	if err != nil {
		return nil, utils.Errorf("failed to start analyze file: %v", err)
	}
	option = append(option, RefineWithKnowledgeBaseName(kbName))
	return _buildIndex(analyzeResult, option...)
}

func _buildIndex(analyzeChannel <-chan AnalysisResult, options ...any) (<-chan *schema.KnowledgeBaseEntry, error) {
	refineConfig := NewRefineConfig(options...)
	knowledgeDatabaseName := refineConfig.KnowledgeBaseName
	opts := []rag.RAGSystemConfigOption{
		rag.WithDB(refineConfig.Database),
		rag.WithDescription(refineConfig.KnowledgeBaseDesc),
		rag.WithKnowledgeBaseType(refineConfig.KnowledgeBaseType),
	}

	opts = append(opts, refineConfig.ragSystemOptions...)
	ragSystem, err := rag.GetRagSystem(knowledgeDatabaseName, opts...)
	if err != nil {
		return nil, utils.Errorf("failed to create rag system: %v", err)
	}

	output := chanx.NewUnlimitedChan[*schema.KnowledgeBaseEntry](refineConfig.Ctx, 100)

	go func() {
		defer output.Close()

		buildWg := sync.WaitGroup{}
		defer buildWg.Wait()

		for res := range analyzeChannel {

			buildWg.Add(1)
			go func() {
				defer buildWg.Done()
				entries, err := BuildIndexFormAnalyzeResult(res, options...)
				if err != nil {
					log.Errorf("failed to build index from analyze result: %v", err)
					return
				}

				for _, entry := range entries {
					output.SafeFeed(entry)
					err := ragSystem.AddKnowledgeEntryQuestion(entry)
					if err != nil {
						log.Errorf("failed to create knowledge base entry: %v", err)
						return
					}
				}
			}()

		}
	}()

	return output.OutputChannel(), nil

}

func Index2KnowledgeEntity(
	action *aicommon.Action,
	Input string,
) ([]*schema.KnowledgeBaseEntry, error) {
	if action == nil {
		return nil, utils.Errorf("action is nil")
	}
	inputLineList := utils.ParseStringToRawLines(Input)

	questionList := action.GetInvokeParamsArray("question_list")
	if len(questionList) == 0 {
		return nil, utils.Errorf("no knowledge-collection found in action")
	}

	knowledgeMap := make(map[string]*schema.KnowledgeBaseEntry)

	safeGetSnippet := func(startLine, endLine int) string {
		if startLine < 1 || endLine > len(inputLineList) || startLine > endLine {
			return Input // fallback to full input
		}
		return strings.Join(inputLineList[startLine-1:endLine], "\n")
	}

	for _, item := range questionList {
		question := item.GetString("question")
		answerLocations := item.GetObject("answer_location")
		startLine := answerLocations.GetInt("start_line")
		endLine := answerLocations.GetInt("end_line")

		if knowledge, exists := knowledgeMap[utils.CalcSha1(startLine, endLine)]; exists {
			knowledge.PotentialQuestions = append(knowledge.PotentialQuestions)
		} else {
			answerSnippet := safeGetSnippet(int(startLine), int(endLine))
			entry := &schema.KnowledgeBaseEntry{
				KnowledgeTitle:     utils.ShrinkString(answerSnippet, 10),
				KnowledgeType:      "Standard",
				Summary:            utils.ShrinkString(answerSnippet, 20),
				KnowledgeDetails:   answerSnippet,
				PotentialQuestions: []string{question},
			}
			knowledgeMap[utils.CalcSha1(startLine, endLine)] = entry
		}
	}

	entries := make([]*schema.KnowledgeBaseEntry, 0, len(knowledgeMap))
	for _, entry := range knowledgeMap {
		entries = append(entries, entry)
	}

	return entries, nil
}

func BuildIndexFormAnalyzeResult(res AnalysisResult, option ...any) ([]*schema.KnowledgeBaseEntry, error) {
	refineConfig := NewRefineConfig(option...)
	rawInput := res.Dump()
	linedInput := utils.PrefixLinesWithLineNumbers(rawInput)
	query, err := LiteForgeQueryFromChunk(indexBuildPrompt, refineConfig.ExtraPrompt, chunkmaker.NewBufferChunk([]byte(linedInput)), 200)
	if err != nil {
		return nil, err
	}

	ragConfig := rag.NewRAGSystemConfig(refineConfig.ragSystemOptions...)
	aiService := ragConfig.GetAIService()

	refineOpts := refineConfig.ForgeExecOption(indexBuildSchema)
	refineOpts = append(refineOpts, aicommon.WithAICallback(aiService))
	indexResult, err := _executeLiteForgeTemp(query, refineOpts...)
	if err != nil {
		return nil, err
	}

	entries, err := Index2KnowledgeEntity(indexResult.Action, rawInput)
	if err != nil {
		log.Errorf("failed to convert action to knowledge base entries: %v", err)
		return nil, err
	}
	return entries, nil
}
