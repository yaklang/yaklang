package aid

import (
	"context"
	"io"

	"github.com/yaklang/yaklang/common/utils"
)

type RiskControlResult struct {
	Skipped bool
	Score   float64
	Reason  string
}

type riskControl struct {
	callback func(*Config, context.Context, io.Reader) *RiskControlResult
}

func (rc *riskControl) enabled() bool {
	if rc == nil {
		return false
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
	if rc.callback == nil {
		return &RiskControlResult{
			Skipped: true,
			Score:   0,
			Reason:  "callback is nil",
		}
	}
	return rc.callback(config, ctx, reader)
}
