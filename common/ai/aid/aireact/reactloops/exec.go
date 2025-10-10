package reactloops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aitag"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func (r *ReActLoop) createAITagStreamMirrors(taskIndex string, nonce string, streamWg *sync.WaitGroup) []func(io.Reader) {
	var aiTagStreamMirror []func(io.Reader)
	var emitter = r.GetEmitter()
	var mirrorStart = time.Now()
	for _, _tagInstance := range r.aiTagFields.Values() {
		v := _tagInstance
		aiTagStreamMirror = append(aiTagStreamMirror, func(reader io.Reader) {
			log.Infof("[ai-tag] mirror[%s] started, time since mirror start: %v", v.TagName, time.Since(mirrorStart))

			pReader := utils.NewPeekableReader(reader)
			parseStart := time.Now()
			pReader.Peek(1)
			log.Infof("[ai-tag] mirror peeked first byte for tag[%s] cost: %v", v.TagName, time.Since(parseStart))
			log.Infof("starting aitag.Parse for tag[%s]", v.TagName)
			defer func() {
				cost := time.Since(parseStart)
				log.Infof("finished aitag.Parse for tag[%s], total cost: %v", v.TagName, cost)
				if cost.Milliseconds() <= 300 {
					log.Infof("AI Response Mirror[%s] stream too fast, cost %v, stream maybe not valid", v.TagName, cost)
				} else {
					log.Infof("AI Response Mirror[%s] stream cost %v, stream maybe valid", v.TagName, cost)
				}
				streamWg.Done()
			}()
			tagErr := aitag.Parse(utils.UTF8Reader(pReader), aitag.WithCallback(v.TagName, nonce, func(fieldReader io.Reader) {
				streamWg.Add(1)

				nodeId := v.AINodeId
				if nodeId == "" {
					nodeId = "re-act-loop-answer-payload"
				}

				callbackStart := time.Now()
				log.Infof("tag[%s] callback started, parse started %v ago", v.TagName, callbackStart.Sub(parseStart))
				var result bytes.Buffer
				fieldReader = io.TeeReader(utils.UTF8Reader(fieldReader), &result)
				emitter.EmitStreamEvent(
					nodeId,
					time.Now(),
					fieldReader,
					taskIndex,
					func() {
						// Use parseStart instead of callbackStart to measure the whole streaming process
						totalCost := time.Since(parseStart)
						contentLength := len(result.String())
						log.Infof("tag[%s] callback finished, content length: %d chars, total stream cost: %v",
							v.TagName, contentLength, totalCost)

						if totalCost.Milliseconds() <= 300 {
							log.Warnf("AITag[%s] stream too fast, cost %v (content: %d chars), stream maybe not valid",
								v.TagName, totalCost, contentLength)
						} else {
							log.Infof("AITag[%s] stream processing completed normally, cost %v for %d chars",
								v.TagName, totalCost, contentLength)
						}

						defer streamWg.Done()
						code := result.String()
						if code == "" {
							return
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

	if r.onTaskCreated != nil {
		r.onTaskCreated(task)
	}

	err := r.ExecuteWithExistedTask(task)
	if task.IsAsyncMode() {
		return err
	}
	task.Finish(err)
	return err
}

func (r *ReActLoop) callAITransaction(streamWg *sync.WaitGroup, prompt string, nonce string) (*aicommon.Action, *LoopAction, error) {
	var action *aicommon.Action
	var emitter = r.emitter
	var actionNames = r.GetAllActionNames()

	log.Infof("start to call aicommon.CallAITransaction in ReActLoop[%v]", r.loopName)
	transactionErr := aicommon.CallAITransaction(
		r.config,
		prompt,
		r.config.CallAI,
		func(resp *aicommon.AIResponse) error {
			rawStream := resp.GetOutputStreamReader(
				r.loopName,
				true,
				r.config.GetEmitter(),
			)

			var stream io.Reader
			aiTagStreamMirror := r.createAITagStreamMirrors(resp.GetTaskIndex(), nonce, streamWg)
			if len(aiTagStreamMirror) > 0 {
				streamWg.Add(len(aiTagStreamMirror))
				log.Debugf("creating %d aitag stream mirrors, will mirror the stream", len(aiTagStreamMirror))
				pr, pw := utils.NewPipe()
				go func() {
					defer func() {
						pw.Close()
					}()
					rawReader := utils.CreateUTF8StreamMirror(rawStream, aiTagStreamMirror...)
					io.Copy(pw, rawReader)
				}()
				stream = pr
			} else {
				stream = rawStream
			}

			actionNameFallback := ""

			streamFields := r.streamFields.Copy()

			for _, i := range r.GetAllActions() {
				for _, field := range i.StreamFields {
					streamFields.Set(field.FieldName, field)
				}
			}
			var actionErr error
			action, actionErr = aicommon.ExtractActionFromStreamWithJSONExtractOptions(
				stream, "object", actionNames,
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
						streamFields.Keys(),
						func(key string, reader io.Reader, parents []string) {
							streamWg.Add(1)
							doneOnce := utils.NewOnce()
							done := func() {
								doneOnce.Do(func() {
									streamWg.Done()
								})
							}

							reader = utils.JSONStringReader(reader)
							fieldIns, ok := streamFields.Get(key)
							if !ok {
								done()
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

							defaultNodeId := "re-act-loop-thought"
							if fieldIns.AINodeId != "" {
								defaultNodeId = fieldIns.AINodeId
							}

							emitter.EmitStreamEvent(
								defaultNodeId,
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
			actionName := action.Name()
			verifier, err := r.GetActionHandler(actionName)
			if err != nil {
				return utils.Wrapf(err, "action[%s] GetActionHandler failed", actionName)
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
	if transactionErr != nil {
		return nil, nil, transactionErr
	}
	if utils.IsNil(action) {
		return nil, nil, utils.Error("action is nil in ReActLoop")
	}

	handler, err := r.GetActionHandler(action.Name())
	if err != nil {
		return nil, nil, utils.Wrap(err, "GetActionHandler failed")
	}
	if utils.IsNil(handler) {
		return nil, nil, utils.Errorf("action[%s] 's handler is nil in ReActLoop.actions", action.Name())
	}

	return action, handler, nil
}

func (r *ReActLoop) ExecuteWithExistedTask(task aicommon.AIStatefulTask) error {
	if utils.IsNil(task) {
		return errors.New("re-act loop task is nil")
	}
	if r == nil {
		return errors.New("re-act loop is nil")
	}
	if r.taskMutex == nil {
		return errors.New("re-act loop taskMutex is nil")
	}
	r.SetCurrentTask(task)

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

	if r.NoActions() {
		abort(utils.Errorf("no action names in ReActLoop"))
		return utils.Error("no action names in ReActLoop")
	}

	var operator = newLoopActionHandlerOperator(task)
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
			task.GetUserInput(),
			operator,
		)
		if finalError != nil {
			log.Errorf("Failed to generate prompt: %v", finalError)
			return finalError
		}

		streamWg := new(sync.WaitGroup)
		/* Generate AI Action */
		actionParams, handler, transactionErr := r.callAITransaction(streamWg, prompt, nonce)
		streamWg.Wait()
		if transactionErr != nil {
			log.Errorf("Failed to execute loop: %v", transactionErr)
			break LOOP
		}

		if utils.IsNil(actionParams) {
			log.Errorf("action is nil in ReActLoop")
			break LOOP
		}
		actionName := actionParams.Name()

		// allow iteration info to be added to timeline
		r.GetInvoker().AddToTimeline("iteration", fmt.Sprintf(
			"======== ReAct iteration %d ========\n"+
				"%v", iterationCount, actionParams.GetString("human_readable_thought"),
		))

		if handler.AsyncMode {
			task.SetAsyncMode(true)
			// 异步模式不在主循环更新状态
			// 只能在异步回调中更新状态
			// 否则会出现状态被覆盖的问题
			if r.onAsyncTaskTrigger != nil {
				r.onAsyncTaskTrigger(handler, task)
			}
			done.Do(func() {
				log.Infof("async mode, not update task status in mainloop")
			})
		}

		// 重置上次操作状态对这次反应的影响
		operator = newLoopActionHandlerOperator(task)
		// 调用 ActionHandler
		if handler.ActionHandler == nil {
			// ActionHandler 必须存在
			finalError = utils.Errorf("action[%s] has no ActionHandler", actionName)
			return finalError
		}

		continueIter := func() {
			r.GetInvoker().AddToTimeline("iteration", fmt.Sprintf("ReAct loop finished END[%v]", iterationCount))
		}
		handler.ActionHandler(
			r,
			actionParams,
			operator,
		)

		// 检查 operator 状态
		if isTerminated, err := operator.IsTerminated(); isTerminated {
			if err != nil {
				finalError = err
				return finalError
			}
			if !operator.isSilence {
				// 正常退出
				continueIter()
			}
			break LOOP
		}

		if handler.AsyncMode {
			// 异步模式直接退出循环
			finalError = nil
			return nil
		}

		// 非异步模式，继续下一次循环
		if operator.IsContinued() {
			continueIter()
			continue
		}

		// 如果既没有调用 Exit/Fail 也没有调用 Continue，默认继续
		continueIter()
		continue
	}
	return nil
}
