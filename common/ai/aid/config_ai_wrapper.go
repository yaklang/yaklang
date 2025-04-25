package aid

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"time"
)

func (c *Config) wrapper(i AICallbackType) AICallbackType {
	return func(config *Config, request *AIRequest) (*AIResponse, error) {
		if c.debugPrompt {
			log.Infof(strings.Repeat("=", 20)+"AIRequest"+strings.Repeat("=", 20)+"\n%v\n", request.GetPrompt())
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

		for _idx := 0; _idx < int(c.aiAutoRetry); _idx++ {
			c.inputConsumptionCallback(tokenSize)
			resp, err := i(config, request)
			if err != nil {
				c.EmitWarning("ai request err: %v, retry auto time: [%v]", err, _idx+1)
				time.Sleep(500 * time.Millisecond)
				continue
			}
			if c.debugPrompt {
				resp.Debug(true)
			}
			return resp, err
		}
		return nil, utils.Errorf("")
	}
}
