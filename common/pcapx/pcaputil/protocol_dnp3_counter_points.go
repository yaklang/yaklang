package pcaputil

import "encoding/binary"

// WithDNP3CounterPoints selects TCP unsigned G20V1/2/5/6 and frozen
// G21V1/2/5/6/9/10 static points and their all/range READ context. It also retains
// the selected typed static observation. This does not implement counter events,
// freezing operations, inferred defaults, clock synchronization or device truth.
func WithDNP3CounterPoints(enabled bool) CaptureOption {
	return func(c *CaptureConfig) error {
		if c.binParserConfig == nil {
			c.binParserConfig = &BinParserConfig{}
		}
		c.binParserConfig.DecodeDNP3CounterPoints = enabled
		return nil
	}
}

// WithSessionDNP3CounterPoints selects the same bounded observation on TCP.
func WithSessionDNP3CounterPoints(enabled bool) ProtocolSessionOption {
	return func(s *captureSession) error { s.f.a.config.DecodeDNP3CounterPoints = enabled; return nil }
}

func dnp3StaticReadGroups(counters, statuses bool) []byte {
	if statuses {
		if counters {
			return []byte{1, 3, 10, 20, 21, 30, 40}
		}
		return []byte{1, 3, 10, 30, 40}
	}
	if counters {
		return []byte{1, 10, 20, 21, 30}
	}
	return []byte{1, 10, 30}
}

func dnp3CounterWidth(group, variation byte) int {
	if group == 20 {
		switch variation {
		case 1:
			return 5
		case 2:
			return 3
		case 5:
			return 4
		case 6:
			return 2
		}
	}
	if group == 21 {
		switch variation {
		case 1:
			return 5
		case 2:
			return 3
		case 5:
			return 11
		case 6:
			return 9
		case 9:
			return 4
		case 10:
			return 2
		}
	}
	return 0
}

// The caller has already bounded and validated each complete encoded point.
func dnp3CounterPoint(index int, group, variation byte, b []byte) map[string]any {
	out := map[string]any{"Index": index}
	noFlags := (group == 20 && (variation == 5 || variation == 6)) || (group == 21 && (variation == 9 || variation == 10))
	if noFlags {
		out["Quality"] = "not-reported"
	} else {
		flags := b[0]
		b = b[1:]
		out["Flags"], out["Reserved Flags"] = int(flags), int(flags&128)
		for bit, key := range []string{"Online", "Restart", "Communication Lost", "Remote Forced", "Local Forced", "Rollover", "Discontinuity"} {
			out[key] = flags&(1<<uint(bit)) != 0
		}
	}
	narrow := (group == 20 && (variation == 2 || variation == 6)) || (group == 21 && (variation == 2 || variation == 6 || variation == 10))
	if narrow {
		out["Value"] = uint32(binary.LittleEndian.Uint16(b))
		b = b[2:]
	} else {
		out["Value"] = binary.LittleEndian.Uint32(b)
		b = b[4:]
	}
	if group == 21 && (variation == 5 || variation == 6) {
		var ms uint64
		for i := 5; i >= 0; i-- {
			ms = (ms << 8) | uint64(b[i])
		}
		out["Timestamp [ms]"], out["Timestamp Quality"] = ms, "not-reported"
	}
	return out
}
