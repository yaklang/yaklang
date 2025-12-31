package aiforge

import (
	_ "embed"

	"os"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed liteforge_prompt/search_index_build.txt
var searchIndexBuildPrompt string

// searchIndexSchema defines the schema for search index generation
// It generates 5-10 questions that users might ask to find this content
var searchIndexSchema = aitool.NewObjectSchemaWithAction(
	aitool.WithStructArrayParam(
		"question_list",
		[]aitool.PropertyOption{
			aitool.WithParam_Description("生成5-10个用户可能会用来搜索此工具/内容的问题。这些问题应该是自然语言查询，涵盖工具的功能、用途、使用场景等。"),
		}, nil,
		aitool.WithStringParam(
			"question",
			aitool.WithParam_Description(`生成一个用户可能会问的搜索问题。要求：
1. 问题应该是自然语言，如"如何进行端口扫描？"、"有什么工具可以检测SQL注入？"
2. 问题应该涵盖工具的核心功能和使用场景
3. 问题应该具体、实用，避免泛化
4. 每个问题长度在10-50个字符之间
5. 问题应该是用户真正可能会问的`),
			aitool.WithParam_Required(true),
		),
	),
)

// SearchIndexResult represents the result of building a search index
type SearchIndexResult struct {
	// Questions is the list of generated search questions
	Questions []string
	// OriginalContent is the original text content (knowledge details)
	OriginalContent string
	// EntryID is the unique identifier for the knowledge entry
	EntryID string
	// KnowledgeBaseEntry is the created knowledge base entry
	KnowledgeBaseEntry *schema.KnowledgeBaseEntry
}

// BuildSearchIndexKnowledge builds a search index for the given text content.
// It uses the same mechanism as BuildIndexKnowledgeFromFile:
// - Creates a KnowledgeBaseEntry to store the knowledge content
// - Uses AI to generate 5-10 search questions
// - Each question is indexed and linked to the KnowledgeBaseEntry
//
// When searching, the result will contain KnowledgeBaseEntry field,
// allowing direct access to the associated knowledge content.
//
// Parameters:
//   - kbName: the knowledge base name
//   - text: the content to index (e.g., tool description, usage, parameters)
//   - options: optional configuration (rag options, AI options, etc.)
//
// Returns the generated questions and knowledge entry.
func BuildSearchIndexKnowledge(kbName string, text string, options ...any) (*SearchIndexResult, error) {
	if text == "" {
		return nil, utils.Errorf("text content is empty")
	}

	refineConfig := NewRefineConfig(options...)

	// Build RAG system options
	ragOpts := []rag.RAGSystemConfigOption{
		rag.WithDB(refineConfig.Database),
		rag.WithDescription(refineConfig.KnowledgeBaseDesc),
		rag.WithKnowledgeBaseType(refineConfig.KnowledgeBaseType),
	}
	ragOpts = append(ragOpts, refineConfig.ragSystemOptions...)

	// Get or create RAG system
	ragSystem, err := rag.GetRagSystem(kbName, ragOpts...)
	if err != nil {
		return nil, utils.Errorf("failed to create rag system: %v", err)
	}

	// Step 1: Build the prompt for AI to generate search questions
	prompt := searchIndexBuildPrompt
	if refineConfig.ExtraPrompt != "" {
		prompt = prompt + "\n\n额外说明:\n" + refineConfig.ExtraPrompt
	}
	prompt = prompt + "\n\n待索引的内容:\n" + text

	// Get AI service
	ragConfig := rag.NewRAGSystemConfig(refineConfig.ragSystemOptions...)
	aiService := ragConfig.GetAIService()

	// Execute liteforge to generate questions
	forgeOpts := refineConfig.ForgeExecOption(searchIndexSchema)
	forgeOpts = append(forgeOpts, aicommon.WithAICallback(aiService))

	result, err := _executeLiteForgeTemp(prompt, forgeOpts...)
	if err != nil {
		return nil, utils.Errorf("failed to generate search questions: %v", err)
	}

	if result.Action == nil {
		return nil, utils.Errorf("AI did not return valid action")
	}

	// Extract questions from the result
	questionList := result.Action.GetInvokeParamsArray("question_list")
	if len(questionList) == 0 {
		return nil, utils.Errorf("no questions generated")
	}

	questions := make([]string, 0, len(questionList))
	for _, item := range questionList {
		q := item.GetString("question")
		if q != "" {
			questions = append(questions, q)
		}
	}

	if len(questions) == 0 {
		return nil, utils.Errorf("no valid questions extracted")
	}

	// Step 2: Create KnowledgeBaseEntry with the content and questions
	entryID := uuid.New().String()

	// Determine title: use search_target from docMetadata if available, otherwise use first question
	// Extract metadata from ragSystemOptions
	metadataConfig := rag.NewRAGSystemConfig(refineConfig.ragSystemOptions...)
	docMetadata := metadataConfig.GetDocumentMetadata()

	title := ""
	if docMetadata != nil {
		if searchTarget, ok := docMetadata["search_target"]; ok {
			title = utils.InterfaceToString(searchTarget)
		}
	}
	if title == "" {
		title = questions[0]
		if len(title) > 100 {
			title = title[:100] + "..."
		}
	}

	entry := &schema.KnowledgeBaseEntry{
		KnowledgeTitle:     title,
		KnowledgeType:      schema.KnowledgeEntryType_QuestionIndex,
		Summary:            utils.ShrinkString(text, 100),
		KnowledgeDetails:   text,
		PotentialQuestions: questions,
		HiddenIndex:        entryID,
	}

	// Step 3: Add the entry to the knowledge base
	// This will:
	// 1. Store the KnowledgeBaseEntry in the database
	// 2. Create vector indexes for each question
	// 3. Link questions to the entry via metadata
	//
	// Pass ragSystemOptions to AddKnowledgeEntryQuestion (includes docMetadata for search_target)
	err = ragSystem.AddKnowledgeEntryQuestion(entry, refineConfig.ragSystemOptions...)
	if err != nil {
		return nil, utils.Errorf("failed to add knowledge entry: %v", err)
	}

	log.Infof("added knowledge entry: %s with %d questions", entryID, len(questions))
	for i, q := range questions {
		log.Infof("  Q%d: %s", i+1, q)
	}

	return &SearchIndexResult{
		Questions:          questions,
		OriginalContent:    text,
		EntryID:            entryID,
		KnowledgeBaseEntry: entry,
	}, nil
}

// BuildSearchIndexKnowledgeFromFile builds a search index from a file.
// It reads the file content and calls BuildSearchIndexKnowledge.
//
// Parameters:
//   - kbName: the knowledge base name
//   - filename: the path to the file containing the content to index
//   - options: optional configuration (rag options, AI options, etc.)
//
// Returns the generated questions and any error.
func BuildSearchIndexKnowledgeFromFile(kbName string, filename string, options ...any) (*SearchIndexResult, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, utils.Errorf("failed to read file %s: %v", filename, err)
	}

	if len(content) == 0 {
		return nil, utils.Errorf("file %s is empty", filename)
	}

	return BuildSearchIndexKnowledge(kbName, string(content), options...)
}
