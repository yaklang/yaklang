package aid

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"

	"github.com/yaklang/yaklang/common/utils"
)

type RiskControlResult struct {
	Skipped bool
	Score   float64
	Reason  string
}

type riskControl struct {
	buildinForgeName  string
	buildinAICallback aicommon.AICallbackType
	callback          func(*Config, context.Context, io.Reader) *RiskControlResult
}

func (rc *riskControl) enabled() bool {
	if rc == nil {
		return false
	}

	if rc.buildinForgeName != "" {
		return true
	}

	if rc.callback == nil {
		return false
	}
	return true
}

func (rc *riskControl) setCallback(callback func(*Config, context.Context, io.Reader) *RiskControlResult) {
	if rc == nil {
		return
	}
	rc.callback = callback
}

func (rc *riskControl) doRiskControl(config *Config, ctx context.Context, reader io.Reader) (final *RiskControlResult) {
	defer func() {
		if err := recover(); err != nil {
			final = &RiskControlResult{
				Skipped: true,
				Score:   0,
				Reason:  "doRiskControl panic: " + utils.ErrorStack(err).Error(),
			}
		}
	}()
	if rc == nil {
		return &RiskControlResult{
			Skipped: true,
			Score:   0,
			Reason:  "not enabled",
		}
	}

	if rc.callback != nil {
		return rc.callback(config, ctx, reader)
	}

	if rc.buildinForgeName == "" {
		return &RiskControlResult{
			Skipped: true,
			Score:   0,
			Reason:  "not enabled (no aid forge set)",
		}
	}

	if !IsAIDBuildInForgeExisted(rc.buildinForgeName) {
		return &RiskControlResult{
			Skipped: true,
			Score:   0,
			Reason:  fmt.Sprintf("not enabled (aid forge [%v] not registered)", rc.buildinForgeName),
		}
	}

	raw, err := io.ReadAll(reader)
	if err != nil && len(raw) == 0 {
		return &RiskControlResult{
			Skipped: true,
			Score:   0,
			Reason:  fmt.Sprintf("read request body error: %v", err),
		}
	}

	var params []*ypb.ExecParamItem
	if _, ok := utils.IsJSON(string(raw)); ok {
		var i = make(map[string]any)
		err := json.Unmarshal(raw, &i)
		if err != nil {
			params = append(params, &ypb.ExecParamItem{
				Key:   "query",
				Value: string(raw),
			})
		} else {
			for k, v := range i {
				params = append(params, &ypb.ExecParamItem{
					Key:   k,
					Value: utils.InterfaceToString(v),
				})
			}
		}
	} else {
		params = append(params, &ypb.ExecParamItem{
			Key:   "query",
			Value: string(raw),
		})
	}

	action, err := ExecuteAIForge(ctx, rc.buildinForgeName, params, WithAICallback(rc.buildinAICallback))
	if err != nil {
		return &RiskControlResult{
			Skipped: true,
			Score:   0,
			Reason:  fmt.Sprintf("execute aid forge error: %v", err),
		}
	}
	obj := action.GetInvokeParams("params")
	prob := obj.GetFloat("probability")
	impact := obj.GetFloat("impact")
	reasonZh := obj.GetString("reason_zh")
	reasonEn := obj.GetString("reason_en")
	_ = reasonEn
	if prob > 0 && impact > 0 {
		return &RiskControlResult{
			Skipped: false,
			Score:   (prob + impact) / 2.0,
			Reason:  reasonZh,
		}
	}

	return &RiskControlResult{
		Skipped: true,
		Score:   0,
		Reason:  "aiforge execute failed, probability n impact all zero",
	}
}
