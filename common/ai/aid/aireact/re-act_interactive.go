package aireact

import (
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (r *ReAct) handleInteractiveEvent(event *ypb.AIInputEvent) error {
	// Handle interactive messages (tool review responses)
	if r.config.DebugEvent {
		log.Infof("Processing interactive message: ID=%s", event.InteractiveId)
	}

	err := jsonextractor.ExtractStructuredJSON(
		event.InteractiveJSONInput,
		jsonextractor.WithObjectCallback(func(data map[string]any) {
			sug, ok := data["suggestion"]
			if !ok || sug == "" {
				sug = "continue" // Default fallback if no suggestion provided
			}

			params := aitool.InvokeParams(data)
			r.config.Epm.Feed(event.InteractiveId, params)
		}),
	)
	if err != nil {
		err = utils.Wrap(err, "Error processing interactive message")
		return err
	}
	return nil
}
