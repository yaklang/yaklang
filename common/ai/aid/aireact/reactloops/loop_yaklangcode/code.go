package loop_yaklangcode

import (
	"bytes"
	_ "embed"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/consts"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/thirdparty_bin"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/ziputil"
)

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
				reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
					result, err := r.InvokeLiteForge(
						task.GetContext(),
						"check-filepath",
						utils.MustRenderTemplate(
							`
你的目标是根据用户输入，判断这是创建一个新文件名还是使用用户提供的已有文件。

*. 一般来说，如果要修改某一个文件，用户会在输入中或者其他上下文中告诉你具体文件名。
*. 如果你可以在上下文中找到用户提到的文件名，根据描述信息决定是否使用已有文件名。
*. 如果用户仅仅只是描述想要实现的功能，而没有提及具体文件名，那么通常是创建一个新文件的任务。

<|DATA_{{ .nonce }}|>
{{ .data }}
<|DATA_END_{{ .nonce }}|>
`,
							map[string]any{
								"nonce": utils.RandStringBytes(4),
								"data":  task.GetUserInput(),
							}),
						[]aitool.ToolOption{
							aitool.WithBoolParam("create_new_file", aitool.WithParam_Description("Is this task to create a new file or modify an existing file? If modifying an existing file, return the file path to modify in 'existed_filepath' and set 'create_new_file' to false. If creating a new file, set 'create_new_file' to true and the system will create it automatically."), aitool.WithParam_Required(true)),
							aitool.WithStringParam("existed_filepath", aitool.WithParam_Description("Effective only when create_new_file is false. Set this field to the file path of the existing file to be modified.")),
							aitool.WithBoolParam("reason", aitool.WithParam_Description("给出这么做的理由，例如：'用户让我在/tmp/test.yak 创建文件，所以直接使用用户路径，无需创建新文件'"), aitool.WithParam_Required(true)),
						},
					)
					if err != nil {
						log.Errorf("failed to invoke liteforge: %v", err)
						return utils.Errorf("failed to invoke liteforge for identifying 'create_new_file': %v", err)
					}
					// loading filename
					createNewFile := result.GetBool("create_new_file")

					reason := result.GetString("reason")
					existed := result.GetString("existed_filepath")

					r.GetConfig().GetEmitter().EmitThoughtStream(task.GetIndex(), reason)

					log.Infof("identified create_new_file: %v", createNewFile)
					if !createNewFile || existed != "" {
						targetPath := existed
						log.Infof("identified target path: %s", targetPath)
						filename := utils.GetFirstExistedFile(targetPath)
						if filename == "" {
							var createFileErr error
							createFileErr = os.WriteFile(targetPath, []byte(""), 0644)
							if createFileErr != nil {
								return utils.Errorf("not found existed file and cannot create file to disk, failed: %v", createFileErr)
							}
							filename = targetPath
						}
						content, _ := os.ReadFile(targetPath)
						if len(content) > 0 {
							log.Infof("identified target file: %s, file size: %v", targetPath, len(content))
							loop.Set("full_code", string(content))
						}
						r.GetConfig().GetEmitter().EmitPinFilename(filename)
						loop.Set("filename", filename)
						return nil
					}
					filename := r.EmitFileArtifactWithExt("gen_code", ".yak", "")
					loop.Set("filename", filename)
					return nil
				}),
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

				writeCode(r),
				modifyCode(r),
				insertCode(r),
				deleteCode(r),
			}

			preset = append(preset, opts...)
			enhanceCollectionName := "yak"
			if config.GetConfigString("aikb_collection") != "" {
				enhanceCollectionName = config.GetConfigString("aikb_collection")
			}
			if !rag.CollectionIsExists(consts.GetGormProfileDatabase(), enhanceCollectionName) {
				preset = append(preset, queryDocumentAction(r, docSearcher))
			} else {
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
