package ssaconfig

import "github.com/yaklang/yaklang/common/utils"

// helper methods to centralize repeated checks in With... option functions.
// Behavior: if c == nil -> no-op (return nil). If mode bitmask doesn't include
// the required mode -> return an error. If nested struct is nil -> create default.

func (c *Config) ensureBase(field string) error {
	if c == nil {
		return nil
	}
	if c.Mode&ModeProjectBase == 0 {
		return utils.Errorf("Config: %s can only be set in Base mode", field)
	}
	if c.BaseInfo == nil {
		c.BaseInfo = defaultBaseInfo()
	}
	return nil
}

func (c *Config) ensureSSACompile(field string) error {
	if c == nil {
		return nil
	}
	if c.Mode&ModeSSACompile == 0 {
		return utils.Errorf("Config: %s can only be set in Compile mode", field)
	}
	if c.SSACompile == nil {
		c.SSACompile = defaultSSACompileConfig()
	}
	return nil
}

func (c *Config) ensureSyntaxFlow(field string) error {
	if c == nil {
		return nil
	}
	if c.Mode&ModeSyntaxFlow == 0 && c.Mode&ModeSyntaxFlowScanManager == 0 {
		return utils.Errorf("Config: %s can only be set in Scan mode", field)
	}
	if c.SyntaxFlow == nil {
		c.SyntaxFlow = defaultSyntaxFlowConfig()
	}
	return nil
}

func (c *Config) ensureSyntaxFlowScan(field string) error {
	if c == nil {
		return nil
	}
	if c.Mode&ModeSyntaxFlowScanManager == 0 {
		return utils.Errorf("Config: %s can only be set in Scan mode", field)
	}
	if c.SyntaxFlowScan == nil {
		c.SyntaxFlowScan = defaultSyntaxFlowScanConfig()
	}
	return nil
}

func (c *Config) ensureSyntaxFlowRule(field string) error {
	if c == nil {
		return nil
	}
	if c.Mode&ModeSyntaxFlowRule == 0 {
		return utils.Errorf("Config: %s can only be set in Rule mode", field)
	}
	if c.SyntaxFlowRule == nil {
		c.SyntaxFlowRule = defaultSyntaxFlowRuleConfig()
	}
	return nil
}

func (c *Config) ensureCodeSource(field string) error {
	if c == nil {
		return nil
	}
	if c.Mode&ModeCodeSource == 0 {
		return utils.Errorf("Config: %s can only be set in Code Source mode", field)
	}
	if c.CodeSource == nil {
		c.CodeSource = defaultCodeSourceConfig()
	}
	return nil
}
