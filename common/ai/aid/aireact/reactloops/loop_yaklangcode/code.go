package loop_yaklangcode

import (
	"bytes"
	_ "embed"
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
						},
					)
					if err != nil {
						log.Errorf("failed to invoke liteforge: %v", err)
						return utils.Errorf("failed to invoke liteforge for identifying 'create_new_file': %v", err)
					}
					// loading filename
					createNewFile := result.GetBool("create_new_file")
					if !createNewFile {
						existed := result.GetString("existed_filepath")
						if existed == "" {
							return utils.Errorf("failed to identify 'existed_filepath': %v", err)
						}
						targetPath := result.GetString("existed_filepath")
						filename := utils.GetFirstExistedFile(targetPath)
						if filename == "" {
							var createFileErr error
							createFileErr = os.WriteFile(filename, nil, 0644)
							if createFileErr != nil {
								return utils.Errorf("not found existed file and cannot create file to disk, failed: %v", createFileErr)
							}
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
				queryDocumentAction(r, docSearcher),
				writeCode(r),
				modifyCode(r),
				insertCode(r),
				deleteCode(r),
			}

			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_WRITE_YAKLANG, r, preset...)
		},
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed", schema.AI_REACT_LOOP_NAME_WRITE_YAKLANG)
	}
}
