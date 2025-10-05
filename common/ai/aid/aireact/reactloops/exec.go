package reactloops

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aitag"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func (r *ReActLoop) Execute(ctx context.Context, startup string) error {
	nonce := utils.RandStringBytes(4)
	_ = nonce

	var iterationCount int
	var maxIterations int
	if r.maxIterations > 0 {
		maxIterations = r.maxIterations
	} else {
		maxIterations = 100
	}
	var emitter = r.emitter
	if utils.IsNil(emitter) {
		return utils.Error("emitter is nil in ReActLoop")
	}

	allActionNames := r.actions.Keys()
	if len(allActionNames) == 0 {
		return utils.Error("no action names in ReActLoop")
	}

	var operator = newLoopActionHandlerOperator()

LOOP:
	for {
		iterationCount++
		if iterationCount > maxIterations {
			log.Warnf("Reached max iterations (%d), stopping code generation loop", maxIterations)
			break LOOP
		}

		prompt, err := r.generateLoopPrompt(
			nonce,
			startup,
			operator,
		)
		if err != nil {
			log.Errorf("Failed to generate loop prompt: %v", err)
			break LOOP
		}

		// 重置上次操作状态对这次反应的影响
		operator = newLoopActionHandlerOperator()

		var actionName string
		var action *aicommon.Action
		var actionErr error
		streamWg := new(sync.WaitGroup)
		transactionErr := aicommon.CallAITransaction(
			r.config,
			prompt,
			r.config.CallAI,
			func(resp *aicommon.AIResponse) error {
				stream := resp.GetOutputStreamReader(
					r.loopName,
					true,
					r.config.GetEmitter(),
				)

				stream = io.TeeReader(stream, os.Stdout)

				var aiTagStreamMirror []func(io.Reader)
				for _, tagIns := range r.aiTagFields.Values() {
					v := tagIns
					aiTagStreamMirror = append(aiTagStreamMirror, func(reader io.Reader) {
						tagErr := aitag.Parse(reader, aitag.WithCallback(v.TagName, nonce, func(fieldReader io.Reader) {
							var result bytes.Buffer
							fieldReader = io.TeeReader(fieldReader, &result)
							emitter.EmitStreamEvent(
								"yaklang-code",
								time.Now(),
								fieldReader,
								resp.GetTaskIndex(),
								func() {
									defer streamWg.Done()
									code := result.String()
									if code == "" {
										return
									}
									if strings.HasPrefix(code, "\n") {
										code = code[1:]
									}
									if strings.HasSuffix(code, "\n") {
										code = code[:len(code)-1]
									}
									r.Set(v.VariableName, code)
								},
							)
						}))
						if tagErr != nil {
							log.Errorf("Failed to emit tag event for[%v]: %v", v.TagName, tagErr)
						}
					})
				}

				if len(aiTagStreamMirror) > 0 {
					streamWg.Add(len(aiTagStreamMirror))
					log.Infof("there aitag detected, will mirror the stream")
					stream = utils.CreateUTF8StreamMirror(stream, aiTagStreamMirror...)
				}

				action, actionErr = aicommon.ExtractActionFromStreamWithJSONExtractOptions(
					stream,
					allActionNames[0],
					allActionNames[1:],
					[]jsonextractor.CallbackOption{
						jsonextractor.WithRegisterMultiFieldStreamHandler(
							r.streamFields.Keys(),
							func(key string, reader io.Reader, parents []string) {
								streamWg.Add(1)
								doneOnce := utils.NewOnce()
								done := func() {
									doneOnce.Do(func() {
										streamWg.Done()
									})
								}
								defer func() {
									done()
								}()

								reader = utils.JSONStringReader(reader)
								fieldIns, ok := r.streamFields.Get(key)
								if !ok {
									return
								}

								pr, pw := utils.NewPipe()
								go func(field *LoopStreamField) {
									defer pw.Close()
									if field.Prefix != "" {
										pw.WriteString(field.Prefix + ": ")
									}
									io.Copy(pw, reader)
								}(fieldIns)
								emitter.EmitStreamEvent(
									"re-act-loop-thought",
									time.Now(),
									pr,
									resp.GetTaskIndex(),
									func() { done() },
								)
							},
						),
					},
				)
				if actionErr != nil {
					return utils.Wrap(actionErr, "failed to parse action")
				}
				actionName = action.Name()

				verifier, ok := r.actions.Get(actionName)
				if !ok {
					return utils.Errorf("action[%s] not found", actionName)
				}
				if utils.IsNil(verifier) {
					return utils.Errorf("action[%s] verifier is nil", actionName)
				}
				if verifier.ActionVerifier == nil {
					return nil
				}
				return verifier.ActionVerifier(r, action)
			},
		)
		streamWg.Wait()

		if transactionErr != nil {
			log.Errorf("Failed to execute loop: %v", transactionErr)
			break LOOP
		}

		if utils.IsNil(action) {
			log.Errorf("action is nil in ReActLoop")
			break LOOP
		}

		instance, ok := r.actions.Get(actionName)
		if !ok {
			log.Errorf("action[%s] instance is nil in ReActLoop", actionName)
			break LOOP
		}

		execOnce := utils.NewOnce()
		var continueTriggered = utils.NewAtomicBool()
		var failedReason any
		var failedTriggered = utils.NewAtomicBool()
		instance.ActionHandler(
			r,
			action,
			operator,
		)
		execOnce.Do(func() {
			continueTriggered.SetTo(true)
		})

		// handle result value
		if failedTriggered.IsSet() {
			if utils.IsNil(failedReason) {
				break LOOP
			} else {
				return utils.Errorf("ReActLoop failed: %v", failedReason)
			}
		}

		if continueTriggered.IsSet() {
			continue
		}
	}
	return nil
}
