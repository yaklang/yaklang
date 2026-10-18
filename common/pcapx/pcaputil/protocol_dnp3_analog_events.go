package pcaputil

// WithDNP3AnalogEvents selects TCP indexed G32/G42V1..8 events and direct
// all/count READs. Existing binary/counter events and typed static observations
// are retained. Times/classes do not certify clock, device or configuration.
// G42 disputed quality bits remain uninterpreted literal masks.
func WithDNP3AnalogEvents(enabled bool) CaptureOption {
	return func(c *CaptureConfig) error {
		if c.binParserConfig == nil {
			c.binParserConfig = &BinParserConfig{}
		}
		c.binParserConfig.DecodeDNP3AnalogEvents = enabled
		return nil
	}
}
func WithSessionDNP3AnalogEvents(enabled bool) ProtocolSessionOption {
	return func(s *captureSession) error { s.f.a.config.DecodeDNP3AnalogEvents = enabled; return nil }
}
func dnp3AnalogEventWidth(group, variation byte) int {
	if group != 32 && group != 42 || variation < 1 || variation > 8 {
		return 0
	}
	return [9]int{0, 5, 3, 11, 9, 5, 9, 11, 15}[variation]
}
func dnp3AnalogEventPoint(index int, group, variation byte, b []byte) map[string]any {
	timed := variation == 3 || variation == 4 || variation == 7 || variation == 8
	width := len(b)
	if timed {
		width -= 6
	}
	scalar := [9]byte{0, 1, 2, 1, 2, 3, 4, 3, 4}[variation]
	point := dnp3StatusPoint(index, 40, scalar, b[:width])
	if group == 32 {
		delete(point, "Uninterpreted Quality Bits")
		point["Over Range"] = b[0]&32 != 0
		point["Reference Error"] = b[0]&64 != 0
	}
	if timed {
		point["Timestamp [ms]"] = dnp3Time48(b[width:])
		point["Timestamp Quality"] = "not-reported"
	}
	return point
}
