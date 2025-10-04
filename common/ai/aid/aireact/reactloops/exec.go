package reactloops

import (
	"bytes"
	"context"
	"io"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
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
		)
		if err != nil {
			log.Errorf("Failed to generate loop prompt: %v", err)
			break LOOP
		}

		var actionName string
		var action *aicommon.Action
		var actionErr error

		transactionErr := aicommon.CallAITransaction(
			r.config,
			prompt,
			r.caller.CallAI,
			func(resp *aicommon.AIResponse) error {
				stream := resp.GetOutputStreamReader(
					r.loopName,
					true,
					r.config.GetEmitter(),
				)

				stream = utils.CreateUTF8StreamMirror(
					stream, func(reader io.Reader) {
						return
					},
				)

				action, actionErr = aicommon.ExtractActionFromStreamWithJSONExtractOptions(
					stream,
					allActionNames[0],
					allActionNames[1:],
					[]jsonextractor.CallbackOption{
						jsonextractor.WithRegisterMultiFieldStreamHandler(
							r.streamFields.Keys(),
							func(key string, reader io.Reader, parents []string) {
								// TODO: handle stream field
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
				return verifier.ActionVerifier(action)
			},
		)
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
		var failedTriggered = utils.NewAtomicBool()
		var feedback bytes.Buffer
		instance.ActionHandler(
			action,
			func() {
				execOnce.DoOr(func() {
					continueTriggered.SetTo(true)
				}, func() {
					log.Warn("continue triggered multiple times in ReActLoop")
				})
			}, func(i any) {
				feedback.WriteString(utils.InterfaceToString(i))
				feedback.WriteString("\n")
			}, func(err error) {
				execOnce.DoOr(func() {
					feedback.WriteString(err.Error())
					feedback.WriteString("\n")
					failedTriggered.SetTo(true)
				}, func() {
					log.Warn("failed/error triggered multiple times in ReActLoop")
				})
			},
		)

		if continueTriggered.IsSet() {
			continue
		}
		if failedTriggered.IsSet() {
			break LOOP
		}
	}
	return nil
}
