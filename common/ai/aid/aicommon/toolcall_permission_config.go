package aicommon

import "github.com/yaklang/yaklang/common/log"

// WithToolCallPermissionJSON installs one or more Claude Code-style permissions
// documents as an in-memory tool-call gate. Multiple documents are spliced in
// declaration order; deny rules always outrank ask and allow rules.
//
// The compiled rules are temporary: they are not written to the syntax-flow
// database and do not change the built-in bash rule set. They are composed with
// the built-in gate in ReAct, and an explicit allow cannot erase a built-in
// deny/ask finding.
func WithToolCallPermissionJSON(configs ...string) ConfigOption {
	return func(c *Config) error {
		if c == nil {
			return nil
		}
		gate, err := CompileClaudeCodePermissions(configs...)
		if err != nil {
			log.Errorf("compile Claude Code permissions failed: %v", err)
			return nil
		}
		c.ToolCallPermissionGate = gate
		return nil
	}
}

// GetToolCallPermissionGate returns the optional user-permission gate.
func (c *Config) GetToolCallPermissionGate() ToolCallRuleGate {
	if c == nil {
		return nil
	}
	return c.ToolCallPermissionGate
}
