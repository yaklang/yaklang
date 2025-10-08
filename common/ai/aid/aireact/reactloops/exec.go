package reactloops

import (
	"bytes"
	"context"
	"errors"
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

func (r *ReActLoop) createMirrors(taskIndex string, nonce string, streamWg *sync.WaitGroup) []func(io.Reader) {
	var aiTagStreamMirror []func(io.Reader)
	var emitter = r.GetEmitter()
	for _, tagIns := range r.aiTagFields.Values() {
		v := tagIns
		aiTagStreamMirror = append(aiTagStreamMirror, func(reader io.Reader) {
			defer func() {
				streamWg.Done()
			}()
			tagErr := aitag.Parse(reader, aitag.WithCallback(v.TagName, nonce, func(fieldReader io.Reader) {
				streamWg.Add(1)
				var result bytes.Buffer
				fieldReader = io.TeeReader(fieldReader, &result)
				emitter.EmitStreamEvent(
					"yaklang-code",
					time.Now(),
					fieldReader,
					taskIndex,
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
	return aiTagStreamMirror
}

func (r *ReActLoop) Execute(taskId string, ctx context.Context, userInput string) error {
	task := aicommon.NewStatefulTaskBase(
		taskId,
		userInput,
		ctx,
		r.GetEmitter(),
	)
	return r.ExecuteWithExistedTask(task)
}

func (r *ReActLoop) ExecuteWithExistedTask(task aicommon.AIStatefulTask) error {
	if utils.IsNil(task) {
		return errors.New("re-act loop task is nil")
	}

	done := utils.NewOnce()
	abort := func(err error) {
		result := task.GetResult()
		result += "\n\n[Error]: " + err.Error()
		task.SetResult(result)
		done.Do(func() {
			task.SetStatus(aicommon.AITaskState_Aborted)
		})
	}
	complete := func(err any) {
		if !utils.IsNil(err) {
			result := task.GetResult()
			result += "\n\n[Reason]: " + utils.InterfaceToString(err)
			task.SetResult(result)
		}
		done.Do(func() {
			task.SetStatus(aicommon.AITaskState_Completed)
		})
	}

	taskStartProcessing := func() {
		task.SetStatus(aicommon.AITaskState_Processing)
	}

	defer func() {
		if err := recover(); err != nil {
			abort(utils.Errorf("ReActLoop panicked: %v", err))
		} else {
			complete(nil)
		}
	}()

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
		abort(utils.Errorf("Emitter is nil"))
		return utils.Error("emitter is nil in ReActLoop")
	}

	allActionNames := r.actions.Keys()
	if len(allActionNames) == 0 {
		abort(utils.Errorf("no action names in ReActLoop"))
		return utils.Error("no action names in ReActLoop")
	}

	var operator = newLoopActionHandlerOperator()
	var finalError error
	defer func() {
		if finalError != nil {
			abort(finalError)
		} else {
			complete(nil)
		}
	}()

	taskStartProcessing()
LOOP:
	for {
		iterationCount++
		if iterationCount > maxIterations {
			log.Warnf("Reached max iterations (%d), stopping code generation loop", maxIterations)
			break LOOP
		}

		var prompt string
		prompt, finalError = r.generateLoopPrompt(
			nonce,
			task.GetId(),
			operator,
		)
		if finalError != nil {
			log.Errorf("Failed to generate prompt: %v", finalError)
			return finalError
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

				aiTagStreamMirror := r.createMirrors(resp.GetTaskIndex(), nonce, streamWg)
				if len(aiTagStreamMirror) > 0 {
					streamWg.Add(len(aiTagStreamMirror))
					log.Infof("there aitag detected, will mirror the stream")
					stream = utils.CreateUTF8StreamMirror(stream, aiTagStreamMirror...)
				}

				actionNameFallback := ""
				action, actionErr = aicommon.ExtractActionFromStreamWithJSONExtractOptions(
					stream, "object", allActionNames,
					[]jsonextractor.CallbackOption{
						jsonextractor.WithRegisterFieldStreamHandler(
							"type",
							func(key string, reader io.Reader, parents []string) {
								if len(parents) <= 0 {
									return
								}
								if parents[len(parents)-1] != "next_action" {
									return
								}
								raw, err := io.ReadAll(utils.JSONStringReader(reader))
								if err != nil {
									return
								}
								actionNameFallback = string(raw)
							},
						),
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
				if actionNameFallback != "" && action.ActionType() == "object" {
					action.SetActionType(actionNameFallback)
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
				finalError = utils.Errorf("action[%s] failed: %v", actionName, failedReason)
				return finalError
			}
		}

		if continueTriggered.IsSet() {
			continue
		}
	}
	return nil
}
