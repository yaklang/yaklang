package rag

import (
	"bytes"
	"strings"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/localmodel"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// ChunkText 将长文本分割成多个小块，以便于处理和嵌入
// 使用rune来分割文本，更好地支持Unicode字符（如中文）
func ChunkText(text string, maxChunkSize int, overlap int) []string {
	if maxChunkSize <= 0 {
		maxChunkSize = 1000 // 默认块大小
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= maxChunkSize {
		overlap = maxChunkSize / 2
	}

	// 如果文本为空，返回空切片
	if text == "" {
		return []string{}
	}

	// 将文本转换为rune切片，以正确处理Unicode字符
	runes := []rune(text)
	textLen := len(runes)

	// 如果文本长度小于等于最大块大小，直接返回原文本
	if textLen <= maxChunkSize {
		return []string{text}
	}

	var chunks []string
	for i := 0; i < textLen; i += maxChunkSize - overlap {
		end := i + maxChunkSize
		if end > textLen {
			end = textLen
		}

		// 尝试在合适的位置分割，避免在单词中间分割
		actualEnd := end
		if end < textLen {
			// 向后查找合适的分割点（空格、标点符号等）
			for j := end; j > i && j < textLen && (end-j) < 50; j-- {
				char := runes[j]
				if char == ' ' || char == '\n' || char == '\t' ||
					char == '。' || char == '！' || char == '？' || char == '；' ||
					char == '.' || char == '!' || char == '?' || char == ';' ||
					char == ',' || char == '，' {
					actualEnd = j + 1
					break
				}
			}
		}

		chunk := string(runes[i:actualEnd])
		// 移除首尾空白字符
		chunk = strings.TrimSpace(chunk)
		if chunk != "" {
			chunks = append(chunks, chunk)
		}

		if actualEnd >= textLen {
			break
		}

		// 调整下一次的起始位置
		if actualEnd != end {
			i = actualEnd - (maxChunkSize - overlap)
			if i < 0 {
				i = 0
			}
		}
	}

	return chunks
}

// TextToDocuments 将文本转换为文档对象
func TextToDocuments(text string, maxChunkSize int, overlap int, metadata map[string]any) []vectorstore.Document {
	chunks := ChunkText(text, maxChunkSize, overlap)
	docs := make([]vectorstore.Document, len(chunks))

	for i, chunk := range chunks {
		// 生成唯一ID
		id := uuid.New().String()

		// 创建文档
		doc := vectorstore.Document{
			ID:       id,
			Content:  chunk,
			Metadata: make(map[string]any),
		}

		// 复制元数据
		for k, v := range metadata {
			doc.Metadata[k] = v
		}

		// 添加额外元数据
		doc.Metadata["chunk_index"] = i
		doc.Metadata["total_chunks"] = len(chunks)
		doc.Metadata["created_at"] = time.Now().Unix()

		docs[i] = doc
	}

	return docs
}

func CheckConfigEmbeddingAvailable(opts ...RAGSystemConfigOption) bool {
	config := NewRAGSystemConfig(opts...)

	if config.embeddingClient != nil {
		return true
	}
	modelName := "Qwen3-Embedding-0.6B-Q4_K_M"
	if config.modelName != "" {
		modelName = config.modelName
	}
	_, err := localmodel.GetModelPath(modelName)
	return err == nil
}
func NewVectorStoreDatabase(path string) (*gorm.DB, error) {
	db, err := gorm.Open("sqlite3", path)
	if err != nil {
		return db, err
	}
	err = autoMigrateRAGSystem(db)
	if err != nil {
		return db, err
	}

	return db, nil
}

func NewTemporaryRAGDB() (*gorm.DB, error) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	if err != nil {
		return nil, err
	}
	err = autoMigrateRAGSystem(db)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func autoMigrateRAGSystem(db *gorm.DB) error {
	return db.AutoMigrate(
		&schema.KnowledgeBaseEntry{},
		&schema.KnowledgeBaseInfo{},
		&schema.VectorStoreCollection{},
		&schema.VectorStoreDocument{},

		&schema.ERModelEntity{},
		&schema.ERModelRelationship{},
		&schema.EntityRepository{},

		&schema.VectorStoreDocument{},
		&schema.VectorStoreCollection{},
	).Error
}

func MockAIService(handle func(message string) string) aicommon.AICallbackType {
	return func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		rsp := config.NewAIResponse()
		rspMsg := handle(req.GetPrompt())
		rsp.EmitOutputStream(strings.NewReader(rspMsg))
		rsp.Close()
		return rsp, nil
	}
}

// type ragSystemCoreTables struct {
// 	VectorStore      *schema.VectorStoreCollection
// 	KnowledgeBase    *schema.KnowledgeBaseInfo
// 	EntityRepository *schema.EntityRepository
// }

// func loadRagSystemCoreTables(opts ...RAGSystemConfigOption) (*ragSystemCoreTables, error) {
// 	config := NewRAGSystemConfig(opts...)
// 	coreTables := &ragSystemCoreTables{}

// 	// 加载集合信息
// 	collection, _ := loadCollectionInfoByConfig(config)
// 	if collection == nil {
// 		vectorstore.CreateCollection(config.db, config.Name, config.Description, opts...)

// 	}
// 	coreTables.VectorStore = collection

// 	// 加载知识库信息
// 	knowledgeBase, err := loadKnowledgeBaseInfoByConfig(config)
// 	if err != nil {
// 		return nil, err
// 	}
// 	coreTables.KnowledgeBase = knowledgeBase

// 	// 加载实体仓库信息
// 	entityRepository, err := loadEntityRepositoryInfoByConfig(config)
// 	if err != nil {
// 		return nil, err
// 	}
// 	coreTables.EntityRepository = entityRepository
// 	return coreTables, nil
// }

func loadCollectionInfoByConfig(config *RAGSystemConfig) (*schema.VectorStoreCollection, error) {
	if config.vectorStore != nil {
		return config.vectorStore.GetCollectionInfo(), nil
	} else {
		if config.ragID != "" {
			var collection schema.VectorStoreCollection
			err := config.db.Model(&schema.VectorStoreCollection{}).Where("rag_id = ?", config.ragID).First(&collection).Error
			if err == nil {
				return &collection, nil
			}
		}
		if config.Name != "" {
			collection, _ := yakit.GetRAGCollectionInfoByName(config.db, config.Name)
			if collection != nil {
				return collection, nil
			}
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func loadKnowledgeBaseInfoByConfig(config *RAGSystemConfig) (*schema.KnowledgeBaseInfo, error) {
	if config.knowledgeBase != nil {
		return config.knowledgeBase.GetKnowledgeBaseInfo(), nil
	} else {
		if config.ragID != "" {
			knowledgeBaseInfo, _ := yakit.GetKnowledgeBaseByRAGID(config.db, config.ragID)
			if knowledgeBaseInfo != nil {
				return knowledgeBaseInfo, nil
			}
		}
		if config.Name != "" {
			knowledgeBaseInfo, _ := yakit.GetKnowledgeBaseByName(config.db, config.Name)
			if knowledgeBaseInfo != nil {
				return knowledgeBaseInfo, nil
			}
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func loadEntityRepositoryInfoByConfig(config *RAGSystemConfig) (*schema.EntityRepository, error) {
	if config.entityRepository != nil {
		info, err := config.entityRepository.GetInfo()
		if err != nil {
			return nil, utils.Wrap(err, "get entity repository info failed")
		}
		return info, nil
	} else {
		if config.ragID != "" {
			entityRepositoryInfo, _ := yakit.GetEntityRepositoryByRAGID(config.db, config.ragID)
			if entityRepositoryInfo != nil {
				return entityRepositoryInfo, nil
			}
		}
		if config.Name != "" {
			entityRepositoryInfo, _ := yakit.GetEntityRepositoryByName(config.db, config.Name)
			if entityRepositoryInfo != nil {
				return entityRepositoryInfo, nil
			}
		}
	}
	return nil, gorm.ErrRecordNotFound
}

// -----------------------------------------------------------------------------
// Index Building (Moved from aiforge to resolve circular dependency)
// -----------------------------------------------------------------------------

var indexBuildPrompt = `### **通用版核心工作指令 (Universal Core Directives)**

**身份 (Persona):** 一位经验丰富的技术学习者和知识管理专家。我的核心能力是深入理解任意领域的知识材料，并为其建立一个精准、以实际应用问题为导向的查询索引。

**核心任务 (Core Mission):** 阅读您提供的任何主题的知识材料（可能包含理论、代码、配置、步骤等），并为其生成一个高质量的“问题-答案”索引。这个索引将由一系列**“一个正在学习或实践的用户可能会问什么问题？”**以及**“原文中哪部分能精准回答这个问题？”**的映射对组成。最终目标是为知识库的 Embedding 和向量检索创建一个理想的数据源。

**关键指令 (Key Instructions):**

1.  **上下文理解与主题识别 (Contextual Understanding & Topic Identification):**
    *   我将首先通读整个材料，并结合您提供的 ` + "`source_filename`" + `（如果存在），来准确理解其核心主题、目标读者和整体结构。
    *   随后，我将逐段、逐个代码块或逻辑单元进行精读，识别出其中包含的、可独立理解和查询的有价值的知识点。

2.  **生成高质量、用户视角的“问题” (Generate High-Quality, User-Centric Questions):**
    *   对于每一个识别出的知识点，我将站在一个**正在学习该主题或试图解决相关问题的用户**的角度，提出一个或多个高度相关的问题。
    *   **问题多样性:** 我会从不同角度提问，以覆盖多样的查询意图：
        *   **操作方法 (How-to):** "如何配置一个[指定功能]？" / "在[某软件]中执行[某项操作]的步骤是什么？"
        *   **概念定义 (What-is):** "[某个专业术语]是什么意思？" / "[某个函数/组件]的核心作用是什么？"
        *   **原理/原因 (Why):** "为什么推荐使用[某种方法]而不是[另一种方法]？" / "出现[某个错误]的根本原因是什么？"
        *   **最佳实践 (Best-Practice):** "在[特定场景]下，处理[某类问题]的最佳实践是什么？"
        *   **故障排查 (Troubleshooting):** "如果我遇到[某个具体问题]，应该如何排查和解决？"
        *   **对比分析 (Comparison):** "[概念A]和[概念B]有什么关键区别？"
    *   **问题具体化:** 问题应尽可能具体、清晰，直接指向原文中可以回答它的那一小块内容。

3.  **精确定位答案范围 (Precise Answer Scoping):**
    *   对于我提出的每一个问题，我必须在原文中精确地定位能够**直接、完整地回答**该问题的**连续文本/代码范围**。
    *   我将使用 **起始行号 (` + "`start_line`" + `)** 和 **结束行号 (` + "`end_line`" + `)** 来标记这个范围。
    *   这个范围的选择原则是“完整”，即它应包含所有必要的信息，而不遗漏任何关键细节，可以有一定程度的上下文，但不应过于宽泛。
    *   如果是一个代码做用或配置相关的问题，答案范围应尽可能涵盖完整的代码块或配置段落。这非常重要

4.  **尊重原文性质  **
    *   我不会对原文内容进行任何修改、总结或重述。所有的问题和答案范围都必须严格基于原文。
`

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

var queryPrompt = `{{.PROMPT}}

{{ if .EXTRA }}
<|EXTRA_{{ .Nonce }}|>
{{.EXTRA}}
<|EXTRA_END_{{ .Nonce }}|>
{{ end }}

{{ if .OVERLAP }}
<|OVERLAP_{{ .Nonce }}|>
{{.OVERLAP}}
<|OVERLAP_END_{{ .Nonce }}|>
{{ end }}


<|INPUT_{{ .Nonce }}|>
{{.INPUT}}
<|INPUT_END_{{ .Nonce }}|>
`

func LiteForgeQueryFromChunk(prompt string, extraPrompt string, chunk chunkmaker.Chunk, overlapSize int) (string, error) {
	param := map[string]interface{}{
		"PROMPT": prompt,
		"INPUT":  string(chunk.Data()),
		"EXTRA":  extraPrompt,
		"Nonce":  utils.RandStringBytes(4),
	}

	if overlapSize > 0 || chunk.HaveLastChunk() {
		param["OVERLAP"] = string(chunk.PrevNBytes(overlapSize))
	}
	queryTemplate, err := template.New("query").Parse(queryPrompt)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = queryTemplate.ExecuteTemplate(&buf, "query", param)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func BuildIndexQuestions(rawInput string, options ...RAGSystemConfigOption) ([]string, error) {
	ragConfig := NewRAGSystemConfig(options...)
	aiService := ragConfig.GetAIService()

	linedInput := utils.PrefixLinesWithLineNumbers(rawInput)
	query, err := LiteForgeQueryFromChunk(indexBuildPrompt, "", chunkmaker.NewBufferChunk([]byte(linedInput)), 200)
	if err != nil {
		return nil, err
	}

	forgeOpts := []any{
		aicommon.WithAICallback(aiService),
		aicommon.WithLiteForgeOutputSchema(indexBuildSchema),
	}

	result, err := aicommon.InvokeLiteForge(query, forgeOpts...)
	if err != nil {
		return nil, err
	}

	entries, err := Index2KnowledgeEntity(result.Action, rawInput)
	if err != nil {
		log.Errorf("failed to convert action to knowledge base entries: %v", err)
		return nil, err
	}
	return entries, nil
}

func Index2KnowledgeEntity(
	action *aicommon.Action,
	Input string,
) ([]string, error) {
	if action == nil {
		return nil, utils.Errorf("action is nil")
	}
	inputLineList := utils.ParseStringToRawLines(Input)

	questionList := action.GetInvokeParamsArray("question_list")
	if len(questionList) == 0 {
		return nil, utils.Errorf("no knowledge-collection found in action")
	}

	knowledgeMap := make(map[string][]string)

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

		posHash := safeGetSnippet(int(startLine), int(endLine))
		if knowledge, exists := knowledgeMap[posHash]; exists {
			knowledge = append(knowledge, question)
			knowledgeMap[posHash] = knowledge
		} else {
			knowledgeMap[posHash] = []string{question}
		}
	}

	var questionListResult []string
	for _, questions := range knowledgeMap {
		questionListResult = append(questionListResult, questions...)
	}
	return questionListResult, nil
}
