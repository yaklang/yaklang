package loop_yaklangcode

import (
	"bytes"
	_ "embed"
	"os"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/thirdparty_bin"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/ziputil"
)

var defaultYaklangRagCollection = "yak"

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
	path, err := thirdparty_bin.GetBinaryPath("yaklang-aikb-rag")
	if err != nil {
		return nil, utils.Wrap(err, "failed to get yaklang-aikb-rag binary")
	}
	// 集合不存在则导入
	if !rag.CollectionIsExists(db, collectionName) {
		err = rag.ImportRAGFromFile(path, rag.WithCollectionName(collectionName))
		if err != nil {
			return nil, utils.Wrap(err, "failed to import rag collection")
		}
	}
	// 集合存在则检查版本
	if !utils.FileExists(aikbPath) {
		return rag.Get(collectionName, rag.WithDB(db))
	}
	file, err := os.OpenFile(aikbPath, os.O_RDONLY, 0644)
	if err != nil {
		return nil, utils.Wrap(err, "failed to open aikb file")
	}
	defer file.Close()
	ragData, err := rag.LoadRAGFileHeader(file)
	if err != nil {
		return nil, utils.Wrap(err, "failed to load rag file header")
	}

	ragData

	collectionInfo, err := yakit.GetRAGCollectionInfoByName(db, collectionName)
	if err != nil {
		return nil, utils.Wrap(err, "failed to get rag collection info")
	}
	collectionInfo.
		err = rag.ImportRAGFromFile(path, rag.WithCollectionName(collectionName))
	if err != nil {
		return nil, utils.Wrap(err, "failed to import rag collection")
	}
	if !rag.CollectionIsExists(db, collectionName) {
		return nil, utils.Errorf("rag collection %s not found", collectionName)
	}
	ragSystem, err := rag.LoadCollection(db, collectionName)
	if err != nil {
		log.Warnf("failed to load rag collection: %v", err)
		return nil, utils.Wrap(err, "failed to load rag collection")
	}
	return ragSystem
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
			docSearcher := createDocumentSearcher(aikbPath)
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(true),
				reactloops.WithAllowToolCall(true),
				reactloops.WithInitTask(buildInitTask(r, docSearcher)),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithAITagFieldWithAINodeId("GEN_CODE", "yak_code", "re-act-loop-answer-payload"),
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
				grepYaklangSamplesAction(r, docSearcher), // 快速 grep 代码样例
				writeCode(r),
				modifyCode(r),
				insertLines(r),
				deleteLines(r),
				batchRegexReplace(r),
			}

			preset = append(preset, opts...)
			enhanceCollectionName := defaultYaklangRagCollection
			if config.GetConfigString("aikb_collection") != "" {
				enhanceCollectionName = config.GetConfigString("aikb_collection")
			}
			if rag.CollectionIsExists(consts.GetGormProfileDatabase(), enhanceCollectionName) {
				log.Infof("RAG collection '%s' loaded successfully for WriteYakLangCode loop", enhanceCollectionName)
				preset = append(preset, ragQueryDocumentAction(r, consts.GetGormProfileDatabase(), enhanceCollectionName))
			}

			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_WRITE_YAKLANG, r, preset...)
		},
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed", schema.AI_REACT_LOOP_NAME_WRITE_YAKLANG)
	}
}
