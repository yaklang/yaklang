package aid

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aiddb"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func (c *Config) wrapper(i AICallbackType) AICallbackType {
	return func(config *Config, request *AIRequest) (*AIResponse, error) {
		if c.debugPrompt {
			log.Infof(strings.Repeat("=", 20)+"AIRequest"+strings.Repeat("=", 20)+"\n%v\n", request.GetPrompt())
		}

		seq := config.AcquireId()
		log.Infof("start to check uuid:%v seq:%v", c.id, seq)
		if ret, ok := aiddb.GetAIInteractiveCheckpoint(c.GetDB(), c.id, seq); ok && ret.Finished {
			// checkpoint is finished, return the result
			rsp := config.NewAIResponse()
			rspParams := aiddb.AiCheckPointGetResponseParams(ret)
			rsp.EmitReasonStream(bytes.NewBufferString(rspParams.GetString("reason")))
			rsp.EmitOutputStream(bytes.NewBufferString(rspParams.GetString("output")))
			rsp.Close()
			return rsp, nil
		}

		// create or update a new checkpoint
		cp := c.createAIInteractiveCheckpoint(seq)
		if err := c.submitAIRequestCheckpoint(cp, request); err != nil {
			log.Errorf("submit ai request checkpoint failed: %v", err)
		}

		if c.aiAutoRetry <= 0 {
			c.aiAutoRetry = 1
		}
		tokenSize := estimateTokens([]byte(request.GetPrompt()))
		if int64(tokenSize) > c.aiCallTokenLimit {
			go func() {
				c.emitJson(EVENT_TYPE_PRESSURE, "system", map[string]any{
					"message":          fmt.Sprintf("token size is too large, now[%v > limit: %v]", tokenSize, c.aiCallTokenLimit),
					"tokenSize":        tokenSize,
					"aiCallTokenLimit": c.aiCallTokenLimit,
				})
			}()
		}

		start := time.Now()
		for _idx := 0; _idx < int(c.aiAutoRetry); _idx++ {
			c.inputConsumptionCallback(tokenSize)
			resp, err := i(config, request)
			if err != nil {
				c.EmitWarning("ai request err: %v, retry auto time: [%v]", err, _idx+1)
				time.Sleep(500 * time.Millisecond)
				continue
			}

			var haveFirstByte = utils.NewBool(false)
			var first, tee *AIResponse
			teeMux := new(sync.Mutex)
			onClose := func() {
				teeMux.Lock()
				defer teeMux.Unlock()
				reasonReader, outputReader := tee.GetUnboundStreamReaderEx(nil, nil, nil)
				reason, _ := io.ReadAll(reasonReader)
				output, _ := io.ReadAll(outputReader)
				c.submitAIResponseCheckpoint(cp, &AIResponseSimple{
					Reason: string(reason),
					Output: string(output),
				})
			}

			teeMux.Lock()
			defer teeMux.Unlock()
			first, tee = c.teeAIResponse(resp, func() {
				c.EmitInfo("ai response first byte cost: %v", time.Since(start))
				haveFirstByte.SetTo(true)
			}, func() {
				c.EmitInfo("ai response close cost: %v", time.Since(start))
				if onClose != nil {
					onClose()
				}
			})
			if c.debugPrompt {
				first.Debug(true)
			}
			return first, err
		}
		return nil, utils.Errorf("")
	}
}
