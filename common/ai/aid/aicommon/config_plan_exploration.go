package aicommon

// WithEnableSubagentsInPlan enables scoped investigation in the PLAN role.
// It does not enable generic dispatch in any derived task configuration.
func WithEnableSubagentsInPlan(enable bool) ConfigOption {
	return func(c *Config) error { c.EnableSubagentsInPlan = enable; return nil }
}
