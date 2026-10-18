package pcaputil

// WithDNP3CounterEvents selects TCP indexed G22/G23V1/2/5/6 counter events,
// direct all-object and class READ observations, unsolicited delivery and the
// selected static counter profile. It does not infer event classes or device
// configuration, execute confirmation, assemble application fragments or select
// event count/range requests. Repeated event indices retain their wire order.
func WithDNP3CounterEvents(enabled bool) CaptureOption {
	return func(c *CaptureConfig) error {
		if c.binParserConfig == nil {
			c.binParserConfig = &BinParserConfig{}
		}
		c.binParserConfig.DecodeDNP3CounterEvents = enabled
		return nil
	}
}

// WithSessionDNP3CounterEvents selects the same bounded TCP observation.
func WithSessionDNP3CounterEvents(enabled bool) ProtocolSessionOption {
	return func(s *captureSession) error { s.f.a.config.DecodeDNP3CounterEvents = enabled; return nil }
}
func dnp3CounterEventWidth(group, variation byte) int {
	if group != 22 && group != 23 {
		return 0
	}
	switch variation {
	case 1:
		return 5
	case 2:
		return 3
	case 5:
		return 11
	case 6:
		return 9
	}
	return 0
}
