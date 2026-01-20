package aiforge

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"

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
	// chunk_list: 聚合后的“知识分片”(稍大)列表；每个分片包含标题、答案范围、以及若干个可检索问题
	aitool.WithStructArrayParam(
		"chunk_list",
		[]aitool.PropertyOption{
			aitool.WithParam_Description("对输入进行合理分片（稍大、按同类主题聚合），每个分片给出标题、答案范围（行号），并生成若干个上下文无关的问题索引。不要一个问题一个分片。"),
		},
		nil,
		aitool.WithStringParam(
			"title",
			aitool.WithParam_Description("该知识分片的标题（应概括主题/模块/功能点，避免把某个问题原样当标题）。"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStructParam(
			"answer_location",
			[]aitool.PropertyOption{
				aitool.WithParam_Description("该分片在输入中的精确范围（使用行号，1-based）。建议覆盖完整函数/配置块/逻辑单元，避免过小切片。"),
			},
			aitool.WithIntegerParam(
				"start_line",
				aitool.WithParam_Description("起始行（包含，1-based）。"),
				aitool.WithParam_Required(true),
			),
			aitool.WithIntegerParam(
				"end_line",
				aitool.WithParam_Description("结束行（包含，1-based）。"),
				aitool.WithParam_Required(true),
			),
		),
		aitool.WithStringArrayParam(
			"question_list",
			aitool.WithParam_Description("该分片对应的若干个上下文无关检索问题（5-12个左右，去重，避免模糊指代，包含语言/技术栈/领域术语）。\n"+questionDescription),
			aitool.WithParam_Required(true),
		),
	))

var questionDescription = `上下文无关的检索问题。核心要求：1)禁用'该/这/上述'等指代词，使用具体技术术语(如'Go语言regexp包'而非'这个包')；2)必含语言名+技术栈+功能描述；3)问题独立完整可脱离代码理解。类型分布：实现方法40%('如何在[语言]中实现[功能]')、技术概念20%('[技术]的工作原理')、问题解决25%('如何优化[场景]性能')、代码模式15%('[功能]的通用架构')。优质例：'Go语言中正则表达式预编译的性能优势是什么？'；劣质例：'这段代码做什么？'；劣质例2：'testStr变量值是多少？'；`

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

	// Apply any document options (metadata/runtime_id/related_entities, etc.) to each inserted question-index document.
	output := chanx.NewUnlimitedChan[*schema.KnowledgeBaseEntry](refineConfig.Ctx, 100)

	go func() {
		defer output.Close()

		swg := utils.NewSizedWaitGroup(refineConfig.AnalyzeConcurrency)
		defer swg.Wait()

		for res := range analyzeChannel {
			swg.Add()
			go func(r AnalysisResult) {
				defer swg.Done()
				entries, err := BuildIndexFormAnalyzeResult(r, options...)
				if err != nil {
					log.Errorf("failed to build index from analyze result: %v", err)
					return
				}

				for _, entry := range entries {
					output.SafeFeed(entry)
					if err := addQuestionIndexVectorOnly(ragSystem, entry, refineConfig.ragSystemOptions...); err != nil {
						log.Errorf("failed to add question-index vectors: %v", err)
						return
					}
				}
			}(res)

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
	inputHash := utils.CalcSha1(Input)

	safeGetSnippet := func(startLine, endLine int) string {
		if startLine < 1 || endLine > len(inputLineList) || startLine > endLine {
			return Input // fallback to full input
		}
		return strings.Join(inputLineList[startLine-1:endLine], "\n")
	}

	type chunk struct {
		Title     string
		StartLine int
		EndLine   int
		Questions []string
	}

	normalizeRange := func(startLine, endLine int) (int, int, bool) {
		if startLine <= 0 || endLine <= 0 {
			return 0, 0, false
		}
		if startLine > endLine {
			startLine, endLine = endLine, startLine
		}
		if startLine < 1 {
			startLine = 1
		}
		if endLine > len(inputLineList) {
			endLine = len(inputLineList)
		}
		if startLine > endLine || len(inputLineList) == 0 {
			return 0, 0, false
		}
		return startLine, endLine, true
	}

	expandSmallRange := func(startLine, endLine, minLines int) (int, int) {
		if minLines <= 0 {
			return startLine, endLine
		}
		cur := endLine - startLine + 1
		if cur >= minLines {
			return startLine, endLine
		}
		need := minLines - cur
		expandBefore := need / 2
		expandAfter := need - expandBefore
		startLine -= expandBefore
		endLine += expandAfter
		if startLine < 1 {
			endLine += 1 - startLine
			startLine = 1
		}
		if endLine > len(inputLineList) {
			shift := endLine - len(inputLineList)
			endLine = len(inputLineList)
			startLine -= shift
			if startLine < 1 {
				startLine = 1
			}
		}
		return startLine, endLine
	}

	pickFallbackTitle := func(title string, questions []string, snippet string) string {
		title = strings.TrimSpace(title)
		if title != "" {
			return title
		}
		if len(questions) > 0 && strings.TrimSpace(questions[0]) != "" {
			return strings.TrimSpace(questions[0])
		}
		for _, line := range utils.ParseStringToLines(snippet) {
			line = strings.TrimSpace(line)
			if line != "" {
				return utils.ShrinkString(line, 80)
			}
		}
		return utils.ShrinkString(snippet, 80)
	}

	var chunks []chunk

	// New schema: chunk_list
	chunkList := action.GetInvokeParamsArray("chunk_list")
	if len(chunkList) > 0 {
		for _, item := range chunkList {
			title := item.GetString("title")
			loc := item.GetObject("answer_location")
			startLine, endLine, ok := normalizeRange(int(loc.GetInt("start_line")), int(loc.GetInt("end_line")))
			if !ok {
				continue
			}
			questions := utils.RemoveRepeatStringSlice(utils.StringArrayFilterEmpty(item.GetStringSlice("question_list")))
			if len(questions) == 0 {
				continue
			}
			startLine, endLine = expandSmallRange(startLine, endLine, 12)
			snippet := safeGetSnippet(startLine, endLine)
			title = pickFallbackTitle(title, questions, snippet)
			chunks = append(chunks, chunk{
				Title:     title,
				StartLine: startLine,
				EndLine:   endLine,
				Questions: questions,
			})
		}
	} else {
		// Backward compatibility: question_list with per-question answer_location
		questionList := action.GetInvokeParamsArray("question_list")
		if len(questionList) == 0 {
			return nil, utils.Errorf("no knowledge-collection found in action")
		}

		byRange := make(map[string]*chunk)
		for _, item := range questionList {
			question := strings.TrimSpace(item.GetString("question"))
			if question == "" {
				continue
			}
			loc := item.GetObject("answer_location")
			startLine, endLine, ok := normalizeRange(int(loc.GetInt("start_line")), int(loc.GetInt("end_line")))
			if !ok {
				continue
			}
			startLine, endLine = expandSmallRange(startLine, endLine, 12)
			key := fmt.Sprintf("%d:%d", startLine, endLine)
			exist := byRange[key]
			if exist == nil {
				snippet := safeGetSnippet(startLine, endLine)
				byRange[key] = &chunk{
					Title:     pickFallbackTitle("", []string{question}, snippet),
					StartLine: startLine,
					EndLine:   endLine,
					Questions: []string{question},
				}
				continue
			}
			if !utils.StringArrayContains(exist.Questions, question) {
				exist.Questions = append(exist.Questions, question)
			}
		}
		for _, c := range byRange {
			c.Questions = utils.RemoveRepeatStringSlice(utils.StringArrayFilterEmpty(c.Questions))
			if len(c.Questions) == 0 {
				continue
			}
			chunks = append(chunks, *c)
		}
	}

	if len(chunks) == 0 {
		return nil, utils.Errorf("no valid chunks found in action")
	}

	sort.SliceStable(chunks, func(i, j int) bool { return chunks[i].StartLine < chunks[j].StartLine })

	// Merge overlapping/adjacent chunks as a safety net against over-splitting.
	const mergeGap = 3
	const mergeMaxLines = 220
	merged := make([]chunk, 0, len(chunks))
	for _, c := range chunks {
		if len(merged) == 0 {
			merged = append(merged, c)
			continue
		}
		last := &merged[len(merged)-1]
		combinedLines := max(last.EndLine, c.EndLine) - min(last.StartLine, c.StartLine) + 1
		if c.StartLine <= last.EndLine+mergeGap && combinedLines <= mergeMaxLines {
			last.StartLine = min(last.StartLine, c.StartLine)
			last.EndLine = max(last.EndLine, c.EndLine)
			last.Questions = utils.RemoveRepeatStringSlice(append(last.Questions, c.Questions...))
			if last.Title == "" {
				last.Title = c.Title
			}
			continue
		}
		merged = append(merged, c)
	}

	entries := make([]*schema.KnowledgeBaseEntry, 0, len(merged))
	for _, c := range merged {
		snippet := safeGetSnippet(c.StartLine, c.EndLine)
		title := pickFallbackTitle(c.Title, c.Questions, snippet)
		locationKey := utils.CalcSha1(inputHash, c.StartLine, c.EndLine)
		entry := &schema.KnowledgeBaseEntry{
			KnowledgeTitle:     title,
			KnowledgeType:      schema.KnowledgeEntryType_QuestionIndex,
			Summary:            utils.ShrinkString(snippet, 120),
			KnowledgeDetails:   snippet,
			PotentialQuestions: utils.RemoveRepeatStringSlice(utils.StringArrayFilterEmpty(c.Questions)),
			HiddenIndex:        locationKey, // stable ID for vector-only question index documents
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

func BuildIndexFormAnalyzeResult(res AnalysisResult, option ...any) ([]*schema.KnowledgeBaseEntry, error) {
	return BuildIndexFromRaw(res.Dump(), option...)
}

func BuildIndexFromRaw(rawInput string, option ...any) ([]*schema.KnowledgeBaseEntry, error) {
	refineConfig := NewRefineConfig(option...)
	lineCount := len(utils.ParseStringToRawLines(rawInput))
	chunkHints := fmt.Sprintf("输入行数约为 %d 行。请优先输出较少数量的 chunk_list（通常 3-8 个），每个 chunk 覆盖完整逻辑单元（如完整函数/配置块），并把同类/近似问题聚合到同一个 chunk。避免碎片化。", lineCount)
	extraPrompt := refineConfig.ExtraPrompt
	if strings.TrimSpace(extraPrompt) != "" {
		extraPrompt += "\n\n"
	}
	extraPrompt += chunkHints

	linedInput := utils.PrefixLinesWithLineNumbers(rawInput)
	query, err := LiteForgeQueryFromChunk(indexBuildPrompt, extraPrompt, chunkmaker.NewBufferChunk([]byte(linedInput)), 200)
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

func addQuestionIndexVectorOnly(ragSys *rag.RAGSystem, entry *schema.KnowledgeBaseEntry, options ...rag.RAGSystemConfigOption) error {
	if ragSys == nil {
		return utils.Errorf("rag system is nil")
	}

	if entry == nil {
		return utils.Errorf("entry is nil")
	}

	if entry.HiddenIndex == "" {
		// Best-effort stable ID for linking question docs to the same chunk.
		entry.HiddenIndex = utils.CalcSha1(entry.KnowledgeTitle, entry.KnowledgeDetails, entry.Summary)
	}

	questions := utils.RemoveRepeatStringSlice(utils.StringArrayFilterEmpty(entry.PotentialQuestions))
	if len(questions) == 0 {
		return nil
	}

	entry.PotentialQuestions = questions

	return ragSys.AddKnowledgeEntryQuestion(entry, options...)
}
