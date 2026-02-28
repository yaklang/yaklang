package loop_syntaxflow_rule

import (
	"bytes"
	_ "embed"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/thirdparty_bin"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/ziputil"
)

var defaultSyntaxFlowAIKBRagCollectionName = "yaklang-syntaxflow-aikb"

// createDocumentSearcher creates a document searcher from syntaxflow-aikb path
func createDocumentSearcher(aikbPath string) *ziputil.ZipGrepSearcher {
	var zipPath string
	if aikbPath != "" {
		zipPath = aikbPath
		log.Infof("using custom syntaxflow aikb path: %s", zipPath)
	} else {
		path, err := thirdparty_bin.GetBinaryPath("syntaxflow-aikb")
		if err != nil {
			log.Warnf("failed to get syntaxflow-aikb binary: %v", err)
			return nil
		}
		zipPath = path
	}
	searcher, err := ziputil.NewZipGrepSearcher(zipPath)
	if err != nil {
		log.Warnf("failed to create document searcher from %s: %v", zipPath, err)
		return nil
	}
	log.Infof("syntaxflow document searcher created successfully from: %s", zipPath)
	return searcher
}

func createTestDocumentSearcherByRag(collectionName string, aikbPath string) (*rag.RAGSystem, error) {
	if aikbPath == "" {
		path, err := thirdparty_bin.GetBinaryPath("syntaxflow-aikb-rag")
		if err != nil {
			log.Errorf("failed to get syntaxflow-aikb-rag binary path: %v", err)
			return nil, utils.Wrap(err, "failed to get syntaxflow-aikb-rag binary")
		}
		aikbPath = path
	}
	log.Infof("initializing SyntaxFlow RAG system with collection: %s, file: %s", collectionName, aikbPath)
	db, err := rag.NewTemporaryRAGDB()
	if err != nil {
		return nil, err
	}
	return rag.Get(collectionName, rag.WithDB(db), rag.WithImportFile(aikbPath), rag.WithEmbeddingClient(vectorstore.NewDefaultMockEmbedding()))
}

func createDocumentSearcherByRag(db *gorm.DB, collectionName string, aikbPath string) (*rag.RAGSystem, error) {
	if aikbPath == "" {
		path, err := thirdparty_bin.GetBinaryPath("syntaxflow-aikb-rag")
		if err != nil {
			log.Errorf("failed to get syntaxflow-aikb-rag binary path: %v", err)
			return nil, utils.Wrap(err, "failed to get syntaxflow-aikb-rag binary")
		}
		aikbPath = path
	}
	log.Infof("initializing SyntaxFlow RAG system with collection: %s, file: %s", collectionName, aikbPath)
	return rag.Get(collectionName, rag.WithDB(db), rag.WithImportFile(aikbPath))
}

//go:embed prompts/persistent_instruction.txt
var instruction string

//go:embed prompts/reflection_output_example.txt
var outputExample string

//go:embed prompts/reactive_data.txt
var reactiveData string

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_WRITE_SYNTAXFLOW,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			config := r.GetConfig()
			aikbPath := config.GetConfigString("syntaxflow_aikb_path")
			aikbRagPath := config.GetConfigString("syntaxflow_aikb_rag_path")
			enableTestSyntaxFlowAIKBRAG := config.GetConfigBool("test_syntaxflow_aikb_rag")

			docSearcher := createDocumentSearcher(aikbPath)
			var docSearcherByRag *rag.RAGSystem
			var ragErr error
			if enableTestSyntaxFlowAIKBRAG {
				docSearcherByRag, ragErr = createTestDocumentSearcherByRag(defaultSyntaxFlowAIKBRagCollectionName, aikbRagPath)
			} else if config.GetDB() != nil {
				docSearcherByRag, ragErr = createDocumentSearcherByRag(config.GetDB(), defaultSyntaxFlowAIKBRagCollectionName, aikbRagPath)
			}
			if ragErr != nil {
				log.Errorf("failed to create SyntaxFlow document searcher by rag: %v", ragErr)
				docSearcherByRag = nil
			}

			modSuite := loopinfra.NewSingleFileModificationSuiteFactory(
				r,
				loopinfra.WithLoopVarsPrefix("sf"),
				loopinfra.WithActionSuffix("rule"), // write_rule, modify_rule, insert_rule, delete_rule
				loopinfra.WithAITagConfig("GEN_RULE", "sf_rule", "syntaxflow-rule", "text/syntaxflow"),
				loopinfra.WithFileExtension(".sf"),
				loopinfra.WithFileChanged(func(content string, op *reactloops.LoopActionHandlerOperator) (string, bool) {
					return checkSyntaxFlowAndFormatErrors(content)
				}),
				loopinfra.WithEventType("syntaxflow_rule_editor"),
			)

			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(true),
				reactloops.WithAllowToolCall(true),
				reactloops.WithInitTask(buildInitTask(r, docSearcher, docSearcherByRag)),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				modSuite.GetAITagOption(),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					sfCode := loop.Get("full_sf_code")
					codeWithLine := utils.PrefixLinesWithLineNumbers(sfCode)
					feedbacks := feedbacker.String()
					feedbacks = strings.TrimSpace(feedbacks)
					sfFilename := loop.Get("sf_filename")
					renderMap := map[string]any{
						"Code":                      sfCode,
						"CurrentCodeWithLineNumber": codeWithLine,
						"Nonce":                     nonce,
						"FeedbackMessages":          feedbacks,
						"SfFilename":                sfFilename,
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
			}
			preset = append(preset, modSuite.GetActions()...)
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_WRITE_SYNTAXFLOW, r, preset...)
		},
		reactloops.WithLoopDescription("Enter focused mode for SyntaxFlow rule generation and modification with real-time syntax validation"),
		reactloops.WithLoopUsagePrompt("Use when user requests to write, modify, or debug SyntaxFlow vulnerability detection rules. Provides tools: write_rule, modify_rule, insert_rule, delete_rule, check-syntaxflow-syntax (for .sf syntax validation; do NOT use check-yaklang-syntax) with real-time SyntaxFlow compile validation"),
		reactloops.WithLoopOutputExample(`
* When user requests to write SyntaxFlow rule:
  {"@action": "write_syntaxflow_rule", "human_readable_thought": "I need to write a SyntaxFlow rule for vulnerability detection"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed", schema.AI_REACT_LOOP_NAME_WRITE_SYNTAXFLOW)
	}
}
