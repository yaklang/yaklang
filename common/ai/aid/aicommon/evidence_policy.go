package aicommon

// WithReadOnlyEvidence permanently forbids tool execution for this config and
// child agents. No user-supplied runtime option or hotpatch can clear this flag.
func WithReadOnlyEvidence() ConfigOption {
	return func(c *Config) error {
		c.readOnlyEvidence = true
		c.DisableToolUse = true
		c.DisableWebSearch = true
		return nil
	}
}

func (c *Config) IsReadOnlyEvidence() bool { return c != nil && c.readOnlyEvidence }
