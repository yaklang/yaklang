package aid

import (
	"bytes"
	"io"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/yaklang/yaklang/common/ai/aid/aiddb"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func (c *Config) wrapper(i aicommon.AICallbackType) aicommon.AICallbackType {
	aidConfig := c
	return func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (rsp *aicommon.AIResponse, err error) {
		defer func() {
			// set rsp start time
			if rsp != nil && !utils.IsNil(rsp) {
				rsp.SetResponseStartTime(time.Now())
				rsp.SetRequestStartTime(request.GetStartTime())
			}
		}()
		if c.promptHook != nil {
			request.SetPrompt(c.promptHook(request.GetPrompt()))
		}
		if c.debugPrompt {
			log.Infof(strings.Repeat("=", 20)+"AIRequest"+strings.Repeat("=", 20)+"\n%v\n", request.GetPrompt())
		}

		// 不需要 checkpoint 的请求直接执行就好
		if request.IsDetachedCheckpoint() {
			if c.aiAutoRetry <= 0 {
				c.aiAutoRetry = 1
			}
			for _idx := 0; _idx < int(c.aiAutoRetry); _idx++ {
				rsp, err = i(config, request)
				if err != nil || rsp == nil {
					c.EmitWarning("ai request err: %v, retry auto time: [%v]", err, _idx+1)
					time.Sleep(500 * time.Millisecond)
					continue
				}
				rsp.SetTaskIndex(request.GetTaskIndex())
				return rsp, err
			}
			return nil, utils.Errorf("ai request err with max retry: %v", err)
		}

		var seq = request.GetSeqId()
		if seq <= 0 {
			seq = aidConfig.AcquireId()
			request.CallOnAcquireSeq(seq)
			aidConfig.EmitInfo("prepare to call ai, create new seq is %v", seq)
		} else {
			aidConfig.EmitInfo("prepare to retry call ai, with an existed seq: %v", seq)
		}
		//log.Infof("start to check uuid:%v seq:%v", c.id, seq)
		if ret, ok := yakit.GetAIInteractiveCheckpoint(c.GetDB(), c.id, seq); ok && ret.Finished {
			// checkpoint is finished, return the result
			var rsp *aicommon.AIResponse
			if config != nil {
				rsp = aicommon.NewAIResponse(config)
			} else {
				rsp = aicommon.NewUnboundAIResponse()
			}
			rsp.SetTaskIndex(request.GetTaskIndex())
			rspParams := aiddb.AiCheckPointGetResponseParams(ret)
			rsp.EmitReasonStream(bytes.NewBufferString(rspParams.GetString("reason")))
			rsp.EmitOutputStream(bytes.NewBufferString(rspParams.GetString("output")))
			rsp.Close()
			return rsp, nil
		}

		// create or update a new checkpoint
		cp := c.CreateAIInteractiveCheckpoint(seq)
		if err := c.submitAIRequestCheckpoint(cp, request); err != nil {
			log.Errorf("submit ai request checkpoint failed: %v", err)
		}

		if c.aiAutoRetry <= 0 {
			c.aiAutoRetry = 1
		}
		tokenSize := estimateTokens([]byte(request.GetPrompt()))
		c.EmitJSON(schema.EVENT_TYPE_PRESSURE, "system", map[string]any{
			"current_cost_token_size": tokenSize,
			"pressure_token_size":     c.aiCallTokenLimit,
		})
		//if int64(tokenSize) > c.aiCallTokenLimit {
		//	go func() {
		//		c.emitJson(EVENT_TYPE_PRESSURE, "system", map[string]any{
		//			"message":          fmt.Sprintf("token size is too large, now[%v > limit: %v]", tokenSize, c.aiCallTokenLimit),
		//			"tokenSize":        tokenSize,
		//			"aiCallTokenLimit": c.aiCallTokenLimit,
		//		})
		//	}()
		//}

		start := time.Now()
		for _idx := 0; _idx < int(c.aiAutoRetry); _idx++ {
			c.inputConsumptionCallback(tokenSize)
			rsp, err = i(config, request)
			if err != nil || rsp == nil {
				c.EmitWarning("ai request err: %v, retry auto time: [%v]", err, _idx+1)
				time.Sleep(500 * time.Millisecond)
				continue
			}
			rsp.SetTaskIndex(request.GetTaskIndex())

			var haveFirstByte = utils.NewBool(false)
			saveHandler := func(tee *aicommon.AIResponse) {
				reasonReader, outputReader := tee.GetUnboundStreamReaderEx(nil, nil, nil)
				reason, _ := io.ReadAll(reasonReader)
				output, _ := io.ReadAll(outputReader)
				if !request.HaveSaveCheckpointCallback() {
					err := c.submitAIResponseCheckpoint(cp, &AIResponseSimple{
						Reason: string(reason),
						Output: string(output),
					})
					if err != nil {
						aidConfig.EmitError("ai request save response checkpoint failed err: %v", err)
					}
				} else {
					request.CallSaveCheckpointCallback(func() (*schema.AiCheckpoint, error) {
						return cp, c.submitAIResponseCheckpoint(cp, &AIResponseSimple{
							Reason: string(reason),
							Output: string(output),
						})
					})
				}

			}

			rsp = aicommon.TeeAIResponse(config, rsp, func(teeResp *aicommon.AIResponse) {
				du := time.Since(start)
				c.EmitInfo("ai response first byte cost: %v", du.String())

				// save response to checkpoint
				aidConfig.Add(1)
				go func() {
					defer aidConfig.Done()
					saveHandler(teeResp)
				}()

				haveFirstByte.SetTo(true)
				c.EmitJSON(schema.EVENT_TYPE_AI_FIRST_BYTE_COST_MS, "system", map[string]any{
					"ms":     du.Milliseconds(),
					"second": du.Seconds(),
				})
			}, func() {
				du := time.Since(start)
				c.EmitInfo("ai response close cost: %v", du)
				c.EmitJSON(schema.EVENT_TYPE_AI_TOTAL_COST_MS, "system", map[string]any{
					"ms":     du.Milliseconds(),
					"second": du.Seconds(),
				})
			})
			if c.debugPrompt {
				rsp.Debug(true)
			}
			return rsp, err
		}
		return nil, utils.Errorf("")
	}
}
