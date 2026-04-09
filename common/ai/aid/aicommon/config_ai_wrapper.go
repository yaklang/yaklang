package aicommon

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aiddb"
	ytokenizer "github.com/yaklang/yaklang/common/ai/tokenizer"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type tierAwareConsumptionCaller struct {
	*Config
	tier consts.ModelTier
}

func (c *tierAwareConsumptionCaller) NewAIResponse() *AIResponse {
	return NewAIResponse(c)
}

func (c *tierAwareConsumptionCaller) CallAIResponseConsumptionCallback(current int) {
	if c == nil {
		return
	}
	c.Config.OutputConsumptionCallback(c.tier, current)
}

func wrapCallerWithTierConsumption(owner *Config, tier consts.ModelTier) AICallerConfigIf {
	return &tierAwareConsumptionCaller{
		Config: owner,
		tier:   tier,
	}
}

func appendPresetPrompt(request *AIRequest, tagName, description, prompt string) {
	if request == nil || strings.TrimSpace(prompt) == "" {
		return
	}
	nonce := utils.RandStringBytes(8)
	preset := fmt.Sprintf(
		"\n<|%s_%s|>\n%s "+
			"It MUST NOT change or override the output format, structure, or schema required by the system.\n\n"+
			"%s\n"+
			"<|%s_END_%s|>\n",
		tagName, nonce, description, prompt, tagName, nonce)
	request.SetPrompt(request.GetPrompt() + preset)
}

func (c *Config) wrapper(i AICallbackType, tier consts.ModelTier) AICallbackType {
	outConfig := c
	return func(config AICallerConfigIf, request *AIRequest) (rsp *AIResponse, err error) {
		// check if callback is nil before calling
		if i == nil {
			return nil, utils.Error("AI callback is not set, please configure AI service first")
		}

		defer func() {
			// set rsp start time
			if rsp != nil && !utils.IsNil(rsp) {
				rsp.SetResponseStartTime(time.Now())
				rsp.SetRequestStartTime(request.GetStartTime())
			}
		}()
		if c.PromptHook != nil {
			request.SetPrompt(c.PromptHook(request.GetPrompt()))
		}
		if globalConfig := yakit.GetCachedAIGlobalConfig(); globalConfig != nil {
			appendPresetPrompt(
				request,
				"AI_PRESET",
				"The following is the global AI preset prompt. It contains persistent guidance, background context, and supplementary information for all AI requests. Consider these instructions when generating responses. IMPORTANT: This preset ONLY affects guidance, tone, preferences, and background context.",
				globalConfig.GetAIPresetPrompt(),
			)
		}
		if c.UserPresetPrompt != "" {
			appendPresetPrompt(
				request,
				"USER_PRESET",
				"The following is the user's preset prompt. It contains user preferences, background context, and supplementary information. Consider these when generating responses to better align with the user's needs. IMPORTANT: This preset ONLY affects tone, preferences, and background context.",
				c.UserPresetPrompt,
			)
		}
		if c.DebugPrompt {
			log.Infof(strings.Repeat("=", 20)+"AIRequest"+strings.Repeat("=", 20)+"\n%v\n", request.GetPrompt())
		}

		// 不需要 checkpoint 的请求直接执行就好
		if request.IsDetachedCheckpoint() {
			if c.AiAutoRetry <= 0 {
				c.AiAutoRetry = 1
			}
			for _idx := 0; _idx < int(c.AiAutoRetry); _idx++ {
				rsp, err = i(wrapCallerWithTierConsumption(outConfig, tier), request)
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
			seq = outConfig.AcquireId()
			request.CallOnAcquireSeq(seq)
			//aidConfig.EmitInfo("prepare to call ai, create new seq is %v", seq)
		} else {
			outConfig.EmitInfo("prepare to retry call ai, with an existed seq: %v", seq)
		}
		//log.Infof("start to check uuid:%v seq:%v", c.id, seq)
		if ret, ok := yakit.GetAIInteractiveCheckpoint(c.GetDB(), c.id, seq); ok && ret.Finished {
			// checkpoint is finished, return the result
			var rsp *AIResponse
			if config != nil {
				rsp = NewAIResponse(config)
			} else {
				rsp = NewUnboundAIResponse()
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
		err = c.SubmitCheckpointRequest(cp, request.GetPrompt())
		if err != nil {
			c.EmitWarning("ai request save request checkpoint failed err: %v", err)
		}
		if c.AiAutoRetry <= 0 {
			c.AiAutoRetry = 1
		}
		tokenSize := ytokenizer.CalcTokenCount(request.GetPrompt())

		start := time.Now()
		for _idx := 0; _idx < int(c.AiAutoRetry); _idx++ {
			c.InputConsumptionCallback(tier, tokenSize)
			rsp, err = i(wrapCallerWithTierConsumption(outConfig, tier), request)
			if err != nil || rsp == nil {
				c.EmitWarning("ai request err: %v, retry auto time: [%v]", err, _idx+1)
				time.Sleep(500 * time.Millisecond)
				continue
			}
			rsp.SetTaskIndex(request.GetTaskIndex())

			var haveFirstByte = utils.NewBool(false)
			saveHandler := func(tee *AIResponse) {
				reasonReader, outputReader := tee.GetUnboundStreamReaderEx(nil, nil, nil)
				reason, _ := io.ReadAll(reasonReader)
				output, _ := io.ReadAll(outputReader)
				if !request.HaveSaveCheckpointCallback() {

					err := c.SubmitCheckpointResponse(cp, &AIResponseSimple{
						Reason: string(reason),
						Output: string(output),
					})
					if err != nil {
						outConfig.EmitError("ai request save response checkpoint failed err: %v", err)
					}
				} else {
					request.CallSaveCheckpointCallback(func() (*schema.AiCheckpoint, error) {
						return cp, c.SubmitCheckpointResponse(cp, &AIResponseSimple{
							Reason: string(reason),
							Output: string(output),
						})
					})
				}

			}

			origRsp := rsp
			rsp = TeeAIResponse(config, rsp, func(teeResp *AIResponse) {
				now := time.Now()
				du := now.Sub(start)
				origRsp.SetFirstOutputByteTime(now)
				c.EmitInfo("ai response from %v:%v first byte cost: %v",
					origRsp.GetProviderName(), origRsp.GetModelName(), du.String())

				outConfig.Add(1)
				go func() {
					defer outConfig.Done()
					saveHandler(teeResp)
				}()

				haveFirstByte.SetTo(true)
				c.EmitJSON(schema.EVENT_TYPE_AI_FIRST_BYTE_COST_MS, "system", map[string]any{
					"ms":            du.Milliseconds(),
					"second":        du.Seconds(),
					"model_name":    origRsp.GetModelName(),
					"provider_name": origRsp.GetProviderName(),
					"model_tier":    string(tier),
				})
				c.EmitJSON(schema.EVENT_TYPE_PRESSURE, "system", map[string]any{
					"current_cost_token_size": tokenSize,
					"pressure_token_size":     c.AiCallTokenLimit,
					"model_tier":              string(tier),
					"model_name":              rsp.GetModelName(),
					"provider_name":           rsp.GetProviderName(),
				})
			}, func() {
				du := time.Since(start)
				provider := origRsp.GetProviderName()
				model := origRsp.GetModelName()
				outputBytes := origRsp.GetTotalOutputBytes()
				outputTokens := origRsp.GetTotalOutputTokens()
				firstByteTime := origRsp.GetFirstOutputByteTime()
				var outputDuration time.Duration
				if !firstByteTime.IsZero() {
					outputDuration = time.Since(firstByteTime)
				}
				tokenRate := float64(0)
				if outputDuration.Seconds() > 0 {
					tokenRate = float64(outputTokens) / outputDuration.Seconds()
				}
				c.EmitInfo("ai response from %v:%v cost: %v, output duration: %v, %.1f token/s",
					provider, model, du, outputDuration, tokenRate)
				c.EmitJSON(schema.EVENT_TYPE_AI_TOTAL_COST_MS, "system", map[string]any{
					"ms":                      du.Milliseconds(),
					"second":                  du.Seconds(),
					"model_name":              model,
					"provider_name":           provider,
					"model_tier":              string(tier),
					"token_rate":              tokenRate,
					"output_bytes":            outputBytes,
					"estimated_output_tokens": outputTokens,
					"output_duration_ms":      outputDuration.Milliseconds(),
				})
				firstByteCostMs := int64(0)
				if !firstByteTime.IsZero() {
					firstByteCostMs = firstByteTime.Sub(start).Milliseconds()
				}
				c.EmitJSON(schema.EVENT_TYPE_AI_CALL_SUMMARY, "system", map[string]any{
					"model_name":              model,
					"provider_name":           provider,
					"model_tier":              string(tier),
					"first_byte_cost_ms":      firstByteCostMs,
					"total_cost_ms":           du.Milliseconds(),
					"output_bytes":            outputBytes,
					"estimated_output_tokens": outputTokens,
					"token_rate":              tokenRate,
					"output_duration_ms":      outputDuration.Milliseconds(),
					"input_token_size":        tokenSize,
				})
				if outputBytes == 0 {
					rawDump := origRsp.GetRawHTTPResponseDump()
					if rawDump != "" {
						println(rawDump)
						c.EmitWarning("[AI Empty Response] model=%v:%v, cost=%v, input_tokens~%d. "+
							"The AI model returned HTTP 200 but generated 0 output tokens "+
							"(finish_reason: stop without delta.content). "+
							"This is typically a transient model-side issue and will be retried automatically.",
							provider, model, du, tokenSize,
						)
					} else {
						c.EmitWarning("[AI Empty Response] model=%v:%v, cost=%v, input_tokens~%d. "+
							"The AI model returned an empty response. (no raw HTTP response available)",
							provider, model, du, tokenSize,
						)
					}
				}
			})
			if c.DebugPrompt {
				rsp.Debug(true)
			}
			return rsp, err
		}
		return nil, utils.Errorf("")
	}
}

type AIResponseSimple struct {
	Reason string `json:"reason"`
	Output string `json:"output"`
}
