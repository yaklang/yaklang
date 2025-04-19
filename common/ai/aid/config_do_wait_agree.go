package aid

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"sync"
	"time"
)

func (c *Config) doWaitAgree(ctx any, ep *Endpoint) {
	switch c.agreePolicy {
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
		if c.agreeAssistant != nil {
			go func() {
				res, err := c.agreeAssistant.Callback(manualCtx, c)
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
			c.EmitInfo("ai agree risk control is not enabled, use manual agree")
			ep.Wait()
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

			if c.agreeAIScore > 0 && result.Score >= c.agreeAIScore {
				c.EmitInfo("ai agree risk control is not enabled, use manual agree")
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

			if c.agreeAIScore > 0 && result.Score >= c.agreeAIScore {
				c.EmitInfo("ai agree risk control is not enabled, use manual agree")
				time.Sleep(c.agreeInterval)
				for i := 0; i < 3; i++ {
					ep.Release()
					time.Sleep(time.Second)
				}
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
