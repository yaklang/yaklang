package aid

func (c *Config) doWaitAgree(ctx any, ep *Endpoint) {
	switch c.agreePolicy {
	case AgreePolicyYOLO:
		c.EmitInfo("yolo policy auto agree all")
	case AgreePolicyAuto:
		if c.agreeInterval <= 0 {
			c.EmitError("auto agree interval is not set")
			c.agreeInterval = 10
		}
		if ep.WaitTimeout(c.agreeInterval) {
			c.EmitInfo("auto agree timeout, use default action: pass")
		}
	case AgreePolicyManual:
		ep.Wait()
	case AgreePolicyAI:
		if !c.agreeRiskCtrl.enabled() {
			c.EmitInfo("ai agree risk control is not enabled, use manual agree")
			ep.Wait()
			return
		}
		result := c.agreeRiskCtrl.doRiskControl(c, nil)
		if result == nil {
			c.EmitInfo("ai agree risk control is not enabled, use manual agree")
			ep.Wait()
			return
		}
		if c.agreeAIScore > 0 && result.Score >= c.agreeAIScore {
			c.EmitInfo("ai agree risk control is not enabled, use manual agree")
			ep.Wait()
			return
		}
	case AgreePolicyAIAuto:
		if c.agreeInterval <= 0 {
			c.EmitError("auto agree interval is not set")
			c.agreeInterval = 10
		}
		if !c.agreeRiskCtrl.enabled() {
			c.EmitInfo("ai agree risk control is not enabled, use manual agree")
			ep.WaitTimeout(c.agreeInterval)
			return
		}
		result := c.agreeRiskCtrl.doRiskControl(c, nil)
		if result == nil {
			c.EmitInfo("ai agree risk control is not enabled, use manual agree")
			ep.WaitTimeout(c.agreeInterval)
			return
		}
		if c.agreeAIScore > 0 && result.Score >= c.agreeAIScore {
			c.EmitInfo("ai agree risk control is not enabled, use manual agree")
			ep.WaitTimeout(c.agreeInterval)
			return
		}
	}
}
