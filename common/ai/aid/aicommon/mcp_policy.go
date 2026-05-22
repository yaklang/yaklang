package aicommon

// IsMCPServersAllowedConfig reports whether MCP tools may be discovered, listed,
// or recommended for this runtime config.
func IsMCPServersAllowedConfig(cfg AICallerConfigIf) bool {
	if cfg == nil {
		return true
	}
	if c, ok := cfg.(*Config); ok && c.DisallowMCPServers {
		return false
	}
	return true
}

// IsMCPServersAllowedRuntime reports whether MCP tools may be discovered, listed,
// or recommended for this invoke runtime.
func IsMCPServersAllowedRuntime(invoker AIInvokeRuntime) bool {
	if invoker == nil {
		return true
	}
	return IsMCPServersAllowedConfig(invoker.GetConfig())
}
