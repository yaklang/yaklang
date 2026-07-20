package loop_yaklangcode

import (
	"bytes"
	_ "embed"
	"strings"
	"sync"

	"github.com/yaklang/gorm"
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
	"github.com/yaklang/yaklang/common/yak/yakdoc/doc"
)

var defaultYaklangAIKBRagCollectionName = "yaklang_aikb_rag"

// searcherHolder 持有可变的 grep / rag 搜索器引用。
// 工厂在 init() 时创建并填入初始值, init task 在自动安装 AIKB 成功后回填新的搜索器。
// 由于 action 在工厂注册阶段就完成闭包捕获(早于自动安装), 必须通过 holder 在运行时
// 读取最新搜索器, 否则 action 会一直拿到安装前的 nil 快照。
// 关键词: searcherHolder, 运行时读取搜索器, 自动安装回填, 闭包捕获快照问题
type searcherHolder struct {
	mu   sync.RWMutex
	grep *ziputil.ZipGrepSearcher
	rag  *rag.RAGSystem
}

func (h *searcherHolder) getGrep() *ziputil.ZipGrepSearcher {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.grep
}

func (h *searcherHolder) getRAG() *rag.RAGSystem {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.rag
}

func (h *searcherHolder) setGrep(s *ziputil.ZipGrepSearcher) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.grep = s
}

func (h *searcherHolder) setRAG(s *rag.RAGSystem) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rag = s
}

// aikbInstallConfig 携带自动安装 / 重建搜索器所需的配置, 供 init task 在 AIKB 缺失时
// 触发 thirdparty_bin.Install 并回填 searcherHolder。
// 关键词: AIKB 自动安装配置, 重建搜索器参数
type aikbInstallConfig struct {
	aikbPath      string
	aikbRagPath   string
	enableTestRAG bool
	ragCollection string
	db            *gorm.DB
	autoInstall   bool
}

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
func createTestDocumentSearcherByRag(collectionName string, aikbPath string) (*rag.RAGSystem, error) {
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
	db, err := rag.NewTemporaryRAGDB()
	if err != nil {
		return nil, err
	}
	return rag.Get(collectionName, rag.WithDB(db), rag.WithImportFile(aikbPath), rag.WithEmbeddingClient(vectorstore.NewDefaultMockEmbedding()))
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

func yaklangPromptRenderMap(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) map[string]any {
	yakCode := loop.Get("full_code")
	lineBase := loop.GetInt(loopinfra.LoopVarCodeLineBase)
	codeWithLine := utils.PrefixLinesWithLineNumbersFrom(lineBase+1, yakCode)
	editorFilePath := strings.TrimSpace(loop.Get("editor_file_path"))
	hasCode := strings.TrimSpace(yakCode) != ""

	feedbacks := ""
	if feedbacker != nil {
		feedbacks = strings.TrimSpace(feedbacker.String())
	}
	policy := ClassifyYakScriptRunPolicy(yakCode)
	runFeedback := strings.TrimSpace(loop.Get(loopVarYakRunLastFeedback))
	runOk := loop.Get(loopVarYakRunOK)
	runOutput := loop.Get(loopVarYakRunOutput)
	scriptKind := string(policy.Kind)
	needsSelfTest := policy.BlockExitNoSelfTest

	initialSamples := loop.Get("initial_code_samples")
	hasInitialSamples := loop.Get("init_samples_ready") == "true" || strings.TrimSpace(initialSamples) != ""
	aikbAvailable := loop.Get("aikb_available") != "false"

	return map[string]any{
		"Code":                      yakCode,
		"CurrentCodeWithLineNumber": codeWithLine,
		"WorkspacePath":             loop.Get("workspace_path"),
		"EditorFilePath":            editorFilePath,
		"EditorFilePathWithoutCode": editorFilePath != "" && !hasCode,
		"IsCreateMode":              editorFilePath == "",
		"Nonce":                     nonce,
		"FeedbackMessages":          feedbacks,
		"RunOk":                     runOk,
		"RunOutput":                 runOutput,
		"RunFeedback":               runFeedback,
		"ScriptKind":                scriptKind,
		"NeedsSelfTestBlock":        needsSelfTest,
		"SelfTestHint":              policy.HintForAI,
		"InitialCodeSamples":        initialSamples,
		"HasInitialSamples":         hasInitialSamples,
		"AIKBAvailable":             aikbAvailable,
		"InitSearchManifest":        FormatManifestForPrompt(loop.Get("init_search_manifest")),
		// ModuleOverviewReady: 模块速览(库选择索引)是否已烤进 doc.gob.zst, 用常用库 http 做廉价探针。
		"ModuleOverviewReady": doc.GetLibOverviewShort("http") != "",
		// PinnedAPIs: init 阶段为选定核心库 PIN 的权威函数签名(接口速查卡), 注入反应数据降低类型/猜名错误。
		"PinnedAPIs":      loop.Get("pinned_apis"),
		"PinnedLibraries": loop.Get("pinned_libraries"),
	}
}

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_WRITE_YAKLANG,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			config := r.GetConfig()
			// 获取配置参数
			aikbPath := config.GetConfigString("aikb_path")
			aikbRagPath := config.GetConfigString("aikb_rag_path")
			enableTestYaklangAIKBRAG := config.GetConfigBool("test_yaklang_aikb_rag")

			// 创建 grep 搜索器
			docSearcher := createDocumentSearcher(aikbPath)

			// 创建语义搜索搜索器
			var docSearcherByRag *rag.RAGSystem
			var err error
			if enableTestYaklangAIKBRAG {
				docSearcherByRag, err = createTestDocumentSearcherByRag(defaultYaklangAIKBRagCollectionName, aikbRagPath)
			} else {
				docSearcherByRag, err = createDocumentSearcherByRag(config.GetDB(), defaultYaklangAIKBRagCollectionName, aikbRagPath)
			}
			if err != nil {
				log.Errorf("failed to create document searcher by rag: %v", err)
				docSearcherByRag = nil // 明确设置为 nil，语义搜索将不可用
			}

			// 用可变 holder 持有搜索器, 让 init task 在自动安装后能回填, action 运行时读取最新值。
			holder := &searcherHolder{grep: docSearcher, rag: docSearcherByRag}
			// aikb_auto_install 默认开启; 仅当用户显式传 false 才关闭(测试/离线场景)。
			installCfg := &aikbInstallConfig{
				aikbPath:      aikbPath,
				aikbRagPath:   aikbRagPath,
				enableTestRAG: enableTestYaklangAIKBRAG,
				ragCollection: defaultYaklangAIKBRagCollectionName,
				db:            config.GetDB(),
				autoInstall:   !config.GetConfigBool("aikb_auto_install_disabled", false),
			}

			// 创建单文件修改工厂
			modSuite := loopinfra.NewSingleFileModificationSuiteFactory(
				r,
				loopinfra.WithLoopVarsPrefix("yak"),
				loopinfra.WithActionSuffix("code"),
				loopinfra.WithAITagConfig("GEN_CODE", "yak_code", "yaklang-code", "code/yaklang"),
				loopinfra.WithFileExtension(".yak"),
				loopinfra.WithExitWhenSyntaxClean(true),
				loopinfra.WithFileChanged(func(loop *reactloops.ReActLoop, content string, op *reactloops.LoopActionHandlerOperator) (string, bool) {
					// Code just changed: invalidate prior YAK_MAIN self-test result.
					// yak_run_ok is only re-set by postSyntaxCleanHook after lint passes.
					if loop != nil {
						resetYakRunStatusAfterCodeChange(loop)
					}
					lineBase := 0
					if loop != nil {
						lineBase = loop.GetInt(loopinfra.LoopVarCodeLineBase)
					}
					errMsg, blocking := checkCodeAndFormatErrors(content, lineBase)
					// On blocking lint: auto-inject AIKB sample snippets + keep mid-loop
					// grep/yakdoc available (GrepAlreadyCovered unlocks when yak_lint_ok=false).
					if blocking {
						if searcher := holder.getGrep(); searcher != nil {
							if extra := autoGrepSamplesForLintErrors(searcher, errMsg, content); extra != "" {
								errMsg += extra
							}
						} else if loop != nil && loop.Get("aikb_available") == "false" {
							errMsg += "\n【提示】AIKB 不可用：请用 yakdoc_* 查 API；无法 grep 样例时只能按【下一步·强制】中的 yakdoc 动作补全。\n"
						}
					}
					return errMsg, blocking
				}),
				loopinfra.WithPostSyntaxCleanHook(buildYaklangPostSyntaxCleanRunHook(r, holder)),
			)

			// 创建预设选项
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(true),
				reactloops.WithAllowToolCall(true),
				reactloops.WithInitTask(buildInitTask(r, holder, installCfg)),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				// write_yaklang_code 是"直接写代码"的 focus 模式, 标准工作流为
				// 选库→搜样例→写代码→修语法→收尾, 本就没有"向用户提问"环节。
				// 这里显式关闭 ask_for_clarification: 用户已给出编码任务, 即使业务需求
				// 存在多种合理实现(如"标记敏感数据"可只打 tag / 可落盘 / 可高亮), 也应基于
				// 最通用合理的默认直接产出可运行代码, 把可调项写进 __DESC__/cli 参数供用户事后切换,
				// 而不是反复反问用户造成空转。API/签名不确定时用 grep/yakdoc 查, 同样不需要问用户。
				reactloops.WithAllowUserInteract(false),
				modSuite.GetAITagOption(),
				reactloops.WithPersistentContextProvider(func(loop *reactloops.ReActLoop, nonce string) (string, error) {
					return utils.RenderTemplate(instruction, yaklangPromptRenderMap(loop, nil, nonce))
				}),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					return utils.RenderTemplate(reactiveData, yaklangPromptRenderMap(loop, feedbacker, nonce))
				}),
				grepYaklangSamplesAction(r, holder),
				semanticSearchYaklangSamplesAction(r, holder),
			}
			preset = append(preset, yakdocActions(r)...)
			// 添加工厂生成的 actions (write_code, modify_code, insert_code, delete_code)
			preset = append(preset, modSuite.GetActions()...)
			preset = append(preset, withYaklangDeferredEditorSync())
			// 结束总结(工作流第 9 步): loop 退出时由 AI 生成阶段总结, 失败 lite 兜底
			preset = append(preset, BuildOnPostIterationHook(r))
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_WRITE_YAKLANG, r, preset...)
		},
		// Register metadata for better AI understanding
		reactloops.WithLoopDescription("Enter focused mode for Yaklang code generation and modification with access to code samples and syntax checking"),
		reactloops.WithLoopDescriptionZh("Yaklang 代码生成模式：用于编写或修改 Yaklang 代码，支持样例检索与语法检查。"),
		reactloops.WithLoopUsagePrompt("Use when user requests to write, modify, or debug Yaklang code. Provides specialized tools: grep_yaklang_samples, semantic_search_yaklang_samples, yakdoc_search, yakdoc_get_all_library_names, yakdoc_module_overview, yakdoc_library_details, yakdoc_function_details, yakdoc_variable_details, write_code, modify_code (line range or old_snippet), insert_code, delete_code with real-time syntax validation"),
		reactloops.WithLoopOutputExample(`
* When user requests to write Yaklang code:
  {"@action": "write_yaklang_code", "human_readable_thought": "I need to write Yaklang code with proper syntax and access to code examples"}
`),

		reactloops.WithVerboseName("Yaklang Code Builder"),
		reactloops.WithVerboseNameZh("Yaklang 代码生成"),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed", schema.AI_REACT_LOOP_NAME_WRITE_YAKLANG)
	}
}
