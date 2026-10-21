package pcaputil

// WithDNP3EventCountReads selects TCP G22/G23V0/1/2/5/6 READs with
// 8/16-bit count qualifiers plus the existing counter-event profile. Counts
// bound ordered event records, not unique indices. A response may contain fewer
// events; device completeness, default variation and class membership remain
// unreported. Count selectors mixed with event-class selectors need independent
// class context and are refused. Application fragments and event-range READs
// remain separate capabilities. No endpoint confirmation is sent.
func WithDNP3EventCountReads(enabled bool) CaptureOption {
	return func(c *CaptureConfig) error {
		if c.binParserConfig == nil {
			c.binParserConfig = &BinParserConfig{}
		}
		c.binParserConfig.DecodeDNP3EventCountReads = enabled
		return nil
	}
}

// WithSessionDNP3EventCountReads selects the same bounded TCP observation.
func WithSessionDNP3EventCountReads(enabled bool) ProtocolSessionOption {
	return func(s *captureSession) error {
		s.f.a.config.DecodeDNP3EventCountReads = enabled
		return nil
	}
}
