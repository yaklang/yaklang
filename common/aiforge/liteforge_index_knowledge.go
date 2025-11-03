package aiforge

import (
	_ "embed"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
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
			aitool.WithParam_Description("A schema for an array of question-answer pairs derived from a source document, designed for knowledge base indexing and retrieval"),
		}, nil,
		// 定义数组中每个对象的字段
		questionSchema...,
	))

var questionSchema = []aitool.ToolOption{
	aitool.WithStringParam(
		"question",
		aitool.WithParam_Description(`A high-quality, user-centric question that the answer_snippet directly addresses. This will be embedded for semantic search.`),
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

func _buildIndex(analyzeChannel <-chan AnalysisResult, options ...rag.RAGSystemConfigOption) (<-chan *schema.KnowledgeBaseEntry, error) {
	refineConfig := NewRefineConfig(options...)
	knowledgeDatabaseName := refineConfig.KnowledgeBaseName
	kb, err := knowledgebase.NewKnowledgeBase(refineConfig.Database, knowledgeDatabaseName, refineConfig.KnowledgeBaseDesc, refineConfig.KnowledgeBaseType, options...)
	if err != nil {
		return nil, utils.Errorf("fial to create knowledgDatabase: %v", err)
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
					err := kb.AddKnowledgeEntryQuestion(entry)
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

	indexResult, err := _executeLiteForgeTemp(query, refineConfig.ForgeExecOption(indexBuildSchema)...)
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
