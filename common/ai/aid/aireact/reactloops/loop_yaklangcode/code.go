package loop_yaklangcode

import (
	"bytes"
	_ "embed"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/consts"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/thirdparty_bin"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/ziputil"
)

var defaultYaklangAIKBRagCollectionName = "yaklang_aikb_rag"

// createDocumentSearcher creates a document searcher from aikb path
func createDocumentSearcher(aikbPath string) *ziputil.ZipGrepSearcher {
	var zipPath string

	// Use custom aikb path if provided
	if aikbPath != "" {
		zipPath = aikbPath
		log.Infof("using custom aikb path: %s", zipPath)
	} else {
		// Get default yaklang-aikb binary path
		path, err := thirdparty_bin.GetBinaryPath("yaklang-aikb")
		if err != nil {
			log.Warnf("failed to get yaklang-aikb binary: %v", err)
			return nil
		}
		zipPath = path
	}

	// Create searcher
	searcher, err := ziputil.NewZipGrepSearcher(zipPath)
	if err != nil {
		log.Warnf("failed to create document searcher from %s: %v", zipPath, err)
		return nil
	}

	log.Infof("document searcher created successfully from: %s", zipPath)
	return searcher
}

func createDocumentSearcherByRag(db *gorm.DB, collectionName string, aikbPath string) (*rag.RAGSystem, error) {
	// 如果 aikbPath 为空，则使用默认的 yaklang-aikb-rag 二进制文件路径
	if aikbPath == "" {
		path, err := thirdparty_bin.GetBinaryPath("yaklang-aikb-rag")
		if err != nil {
			log.Errorf("failed to get yaklang-aikb-rag binary path: %v", err)
			return nil, utils.Wrap(err, "failed to get yaklang-aikb-rag binary")
		}
		aikbPath = path
	}

	log.Infof("initializing RAG system with collection: %s, file: %s", collectionName, aikbPath)
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
		schema.AI_REACT_LOOP_NAME_WRITE_YAKLANG,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			config := r.GetConfig()
			aikbPath := config.GetConfigString("aikb_path")
			aikbRagPath := config.GetConfigString("aikb_rag_path")
			docSearcher := createDocumentSearcher(aikbPath)
			docSearcherByRag, err := createDocumentSearcherByRag(consts.GetGormProfileDatabase(), defaultYaklangAIKBRagCollectionName, aikbRagPath)
			if err != nil {
				log.Errorf("failed to create document searcher by rag: %v", err)
				docSearcherByRag = nil // 明确设置为 nil，语义搜索将不可用
			}
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(true),
				reactloops.WithAllowToolCall(true),
				reactloops.WithInitTask(buildInitTask(r, docSearcher)),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithAITagFieldWithAINodeId("GEN_CODE", "yak_code", "yaklang-code", "code/yaklang"),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					yakCode := loop.Get("full_code")
					codeWithLine := utils.PrefixLinesWithLineNumbers(yakCode)

					feedbacks := feedbacker.String()
					feedbacks = strings.TrimSpace(feedbacks)
					renderMap := map[string]any{
						"Code":                      yakCode,
						"CurrentCodeWithLineNumber": codeWithLine,
						"Nonce":                     nonce,
						"FeedbackMessages":          feedbacks,
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
				// queryDocumentAction(r, docSearcher), // DEPRECATED: 已被 grepYaklangSamplesAction 替代
				grepYaklangSamplesAction(r, docSearcher),                // 快速 grep 代码样例（精确文本搜索）
				semanticSearchYaklangSamplesAction(r, docSearcherByRag), // 语义搜索 Yaklang 代码样例（基于向量相似度）
				writeCode(r),
				modifyCode(r),
				insertLines(r),
				deleteLines(r),
				// batchRegexReplace(r),
			}
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_WRITE_YAKLANG, r, preset...)
		},
		// Register metadata for better AI understanding
		reactloops.WithLoopDescription("Enter focused mode for Yaklang code generation and modification with access to code samples and syntax checking"),
		reactloops.WithLoopUsagePrompt("Use when user requests to write, modify, or debug Yaklang code. Provides specialized tools: grep_yaklang_samples, write_code, modify_code, insert_code, delete_code  and soon with real-time syntax validation"),
		reactloops.WithLoopOutputExample(`
* When user requests to write Yaklang code:
  {"@action": "write_yaklang_code", "human_readable_thought": "I need to write Yaklang code with proper syntax and access to code examples"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed", schema.AI_REACT_LOOP_NAME_WRITE_YAKLANG)
	}
}
