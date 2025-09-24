package aireact

import (
	"bytes"
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"io"
	"os"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/utils"
)

func (r *ReAct) invokeWriteYaklangCode(ctx context.Context, approach string) error {
	// start to write code:
	// * (optional)query code snippets
	// * write to file
	// * check syntax and lint
	// * (optional) run test cases
	// * (query document && modify to file)
	// * return

	satisfied := false
	iterationCount := 0
	currentCode := ""
	errorMessages := ""
	userQuery := ""
	if r.config.memory != nil {
		userQuery = r.config.memory.Query
	}

	//tempFile, err := consts.TempAIFile("codegen-%v.yak")
	//if err != nil {
	//	return utils.Errorf("Failed to create temp file for code generation: %v", err)
	//}
	//var filename string
	//_ = tempFile.Close()
	//filename = tempFile.Name()
	filename := "/tmp/a.yak"

	// Get available tools
	tools := buildinaitools.GetAllTools()

	for !satisfied {
		iterationCount++
		prompt, err := r.promptManager.GenerateYaklangCodeActionLoop(
			userQuery+"\n\n"+approach, // userQuery
			currentCode,               // currentCode
			errorMessages,             // errorMessages
			iterationCount,            // iterationCount
			tools,                     // tools
		)
		if err != nil {
			log.Errorf("Failed to generate prompt for yaklang code action loop: %v", err)
			return err
		}

		var actionName string
		var payload string

		transactionErr := aicommon.CallAITransaction(
			r.config, prompt, r.config.CallAI,
			func(resp *aicommon.AIResponse) error {
				stream := resp.GetOutputStreamReader("yaklang-code-loop", true, r.config.Emitter)
				subCtx, cancel := context.WithCancel(ctx)
				defer cancel()
				action, actionErr := aicommon.ExtractWaitableActionFromStream(
					subCtx,
					stream,
					"write_code",
					[]string{
						"query_document",
						"require_tool",
					},
					[]jsonextractor.CallbackOption{
						jsonextractor.WithRegisterFieldStreamHandler("query_document", func(key string, reader io.Reader, parents []string) {
							r.Emitter.EmitStreamEvent(
								"query-yaklang-document",
								time.Now(),
								reader,
								resp.GetTaskIndex(),
							)
						}),
					})
				if actionErr != nil {
					return utils.Errorf("Failed to parse action: %v", actionErr)
				}

				actionName = action.Name()
				switch actionName {
				case "write_code":
					payload = action.WaitString("code")
					if payload == "" {
						return utils.Error("code action must have 'code' field")
					}
				case "query_document":
					payload = action.WaitString("query_document")
					if payload == "" {
						return utils.Error("query_document action must have 'query_document' field")
					}
				case "require_tool":
					payload = action.WaitString("tool_require_payload")
					if payload == "" {
						return utils.Error("require_tool action must have 'tool_require_payload' field")
					}
				default:
					// For other actions, we don't have specific payload requirements
					return utils.Errorf("unknown action: %s", actionName)
				}
				return nil
			})
		if transactionErr != nil {
			return utils.Wrap(transactionErr, "AI transaction failed in code generation loop")
		}

		// Handle different action types
		switch actionName {
		case "write_code":
			// Update current code
			code := payload
			err := os.WriteFile(filename, []byte(code), 0644)
			if err != nil {
				return utils.Errorf("Failed to write code to file: %v", err)
			}
			result := static_analyzer.YaklangScriptChecking(code, "yak")
			var buf bytes.Buffer
			for _, msg := range result {
				buf.WriteString(msg.String())
				buf.WriteString("\n")
			}
			continue
		}

		// If satisfied, break the loop
		if satisfied {
			r.addToTimeline("code_generation", "Code generation completed successfully")
			break
		}
	}

	return nil
}
