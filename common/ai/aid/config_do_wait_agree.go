package aid

import (
	"bytes"
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"sync"
	"time"
)

func (c *Config) doWaitAgreeWithPolicy(ctx context.Context, doWaitAgreeWithPolicy AgreePolicyType, ep *Endpoint) {
	if ep.checkpoint != nil && ep.checkpoint.Finished { // check ep finished, is recover task or not
		return
	}
	defer func() {
		if ep.checkpoint != nil {
			if err := c.submitCheckpointResponse(ep.checkpoint, ep.GetParams()); err != nil {
				log.Errorf("submit review checkpoint to db response err: %v", err)
			}
		}
	}()

	switch doWaitAgreeWithPolicy {
	case AgreePolicyYOLO:
		c.EmitInfo("yolo policy auto agree all")
	case AgreePolicyAuto:
		if c.agreeInterval <= 0 {
			c.EmitError("auto agree interval is not set")
			c.agreeInterval = 10 * time.Second
		}
		if ep.WaitTimeout(c.agreeInterval) {
			c.EmitInfo("auto agree timeout, use default action: pass")
		}
	case AgreePolicyManual:
		manualCtx, cancel := context.WithCancel(c.epm.ctx)
		defer cancel()
		if c.agreeManualCallback != nil { // if agreeManualCallback is not nil, use it help manual agree
			go func() {
				res, err := c.agreeManualCallback(manualCtx, c)
				if err != nil {
					log.Errorf("agree assistant callback error: %v", err)
				} else {
					ep.SetParams(res.Param)
					for i := 0; i < 3; i++ {
						ep.Release()
						time.Sleep(time.Second)
					}
				}
			}()
		}
		ep.Wait()
	case AgreePolicyAI:
		if !c.agreeRiskCtrl.enabled() {
			c.EmitInfo("policy[ai]: ai agree risk control is not enabled, use manual agree (risk control is disabled)")
			ep.Wait()
			return
		}

		riskCtrlCtx, cancel := context.WithCancel(c.epm.ctx)
		defer cancel()

		wg := new(sync.WaitGroup)
		wg.Add(1)
		go func() {
			defer wg.Done()

			params := ep.GetReviewMaterials()

			result := c.agreeRiskCtrl.doRiskControl(c, riskCtrlCtx, bytes.NewBufferString(string(utils.Jsonify(params))))
			if result == nil {
				c.EmitInfo("ai agree risk control is not configured or impl bug, wait manual agree")
				return
			}

			if result != nil {
				c.EmitRiskControlPrompt(ep.id, result)
			}
			if c.agreeAIScore > 0 && result.Score >= c.agreeAIScore {
				c.EmitInfo("ai got risk score: %v >= %v, use manual agree", result.Score, c.agreeAIScore)
				return
			}
			c.EmitInfo("ai agree risk control ")
			ep.Release()
		}()
		ep.Wait()
		cancel()
		wg.Wait()
	case AgreePolicyAIAuto:
		if c.agreeInterval <= 0 {
			c.EmitError("auto agree interval is not set")
			c.agreeInterval = 10 * time.Second
		}

		if !c.agreeRiskCtrl.enabled() {
			c.EmitInfo("ai agree risk control is not enabled, use manual agree")
			ep.WaitTimeout(c.agreeInterval)
			return
		}

		riskCtrlCtx, cancel := context.WithCancel(c.epm.ctx)
		defer cancel()

		wg := new(sync.WaitGroup)
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := c.agreeRiskCtrl.doRiskControl(c, riskCtrlCtx, nil)
			if result == nil {
				c.EmitInfo("ai agree risk control is not enabled, use manual agree")
				return
			}
			if result != nil && !result.Skipped {
				c.EmitRiskControlPrompt(ep.id, result)
			}

			if c.agreeAIScore > 0 && result.Score >= c.agreeAIScore {
				c.EmitInfo("ai agree risk control is not enabled, use manual agree")
				time.Sleep(c.agreeInterval)
				ep.ReleaseContext(ctx)
				return
			}
			//
			for i := 0; i < 3; i++ {
				time.Sleep(time.Second)
				ep.Release()
			}
		}()
		ep.Wait()
		cancel()
		wg.Wait()
	}
}

func (c *Config) doWaitAgree(ctx context.Context, ep *Endpoint) {
	c.doWaitAgreeWithPolicy(ctx, c.agreePolicy, ep)
}
