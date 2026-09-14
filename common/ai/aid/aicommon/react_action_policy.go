package aicommon

// ReActActionPolicy is a runtime-owned permission check, independent of the
// application providing the runtime. It must be immutable and safe for parallel
// calls. A nil policy preserves ordinary action availability.
type ReActActionPolicy func(loopName, actionName string) bool

func WithReActActionPolicy(policy ReActActionPolicy) ConfigOption {
	return func(c *Config) error { c.reActActionPolicy = policy; return nil }
}

func (c *Config) IsReActActionAllowed(loopName, actionName string) bool {
	return c == nil || c.reActActionPolicy == nil || c.reActActionPolicy(loopName, actionName)
}

// Keep the optional policy separate from the broad caller interface so existing
// callers remain source compatible.
func IsReActActionAllowed(config any, loopName, actionName string) bool {
	p, ok := config.(interface{ IsReActActionAllowed(string, string) bool })
	return !ok || p.IsReActActionAllowed(loopName, actionName)
}
