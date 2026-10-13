package pcaputil

import "encoding/binary"

// WithDNP3BinaryEvents selects TCP indexed G2/G4V1..3 and G11V1/2 events,
// direct all/count READs and message-local G51V1/2 CTO. Selected counter events
// and typed static observations are retained. Reported synchronization does not
// certify a device clock. No point class/default/device identity is inferred.
func WithDNP3BinaryEvents(enabled bool) CaptureOption {
	return func(c *CaptureConfig) error {
		if c.binParserConfig == nil {
			c.binParserConfig = &BinParserConfig{}
		}
		c.binParserConfig.DecodeDNP3BinaryEvents = enabled
		return nil
	}
}

func WithSessionDNP3BinaryEvents(enabled bool) ProtocolSessionOption {
	return func(s *captureSession) error { s.f.a.config.DecodeDNP3BinaryEvents = enabled; return nil }
}

func dnp3BinaryEventWidth(g, v byte) int {
	if g != 2 && g != 4 && g != 11 {
		return 0
	}
	switch v {
	case 1:
		return 1
	case 2:
		return 7
	case 3:
		if g != 11 {
			return 3
		}
	}
	return 0
}

func dnp3ObservedEventGroups(binaryEvents bool) []byte {
	if binaryEvents {
		return []byte{2, 4, 11, 22, 23}
	}
	return []byte{22, 23}
}

func dnp3EventCursorIndex(g byte) int {
	switch g {
	case 2:
		return 0
	case 4:
		return 1
	case 11:
		return 2
	case 22:
		return 3
	case 23:
		return 4
	}
	return -1
}

type dnp3CTO struct {
	valid, synchronized bool
	ms                  uint64
}

func dnp3Time48(b []byte) uint64 {
	var v uint64
	for i := 5; i >= 0; i-- {
		v = v<<8 | uint64(b[i])
	}
	return v
}

func dnp3ReadCTO(v, q byte, b []byte, cto *dnp3CTO, remaining int) (map[string]any, []byte, error) {
	if v != 1 && v != 2 || q != 7 && q != 8 {
		return nil, nil, protocolError(ErrUnsupportedFeature, "DNP3 unsupported CTO variation/qualifier")
	}
	n := 1
	if q == 8 {
		n = 2
	}
	if len(b) < n {
		return nil, nil, protocolError(ErrMalformedMessage, "DNP3 truncated CTO count")
	}
	count := int(b[0])
	if n == 2 {
		count = int(binary.LittleEndian.Uint16(b))
	}
	b = b[n:]
	if count == 0 {
		return nil, nil, protocolError(ErrMalformedMessage, "DNP3 CTO requires exactly one value")
	}
	if count != 1 {
		return nil, nil, protocolError(ErrUnsupportedFeature, "DNP3 CTO requires exactly one value")
	}
	if remaining < 2 {
		return nil, nil, protocolError(ErrResourceExceeded, "DNP3 object/point collection count")
	}
	if len(b) < 6 {
		return nil, nil, protocolError(ErrMalformedMessage, "DNP3 truncated CTO time")
	}
	*cto = dnp3CTO{true, v == 1, dnp3Time48(b)}
	return map[string]any{"Group": 51, "Variation": int(v), "Qualifier": int(q), "Context": true, "Points": []any{}, "CTO Time [ms]": cto.ms, "CTO Synchronized": cto.synchronized}, b[6:], nil
}

func dnp3BinaryEventPoint(index int, g, v byte, b []byte, cto dnp3CTO) (map[string]any, error) {
	f := b[0]
	p := map[string]any{"Index": index, "Flags": int(f)}
	for bit, key := range []string{"Online", "Restart", "Communication Lost", "Remote Forced", "Local Forced"} {
		p[key] = f&(1<<uint(bit)) != 0
	}
	if g == 4 {
		dnp3DoubleBitValue(p, f>>6)
		p["Chatter Filter"] = f&32 != 0
	} else {
		p["Value"] = f&128 != 0
		if g == 2 {
			p["Chatter Filter"] = f&32 != 0
			p["Reserved Flags"] = int(f & 64)
		} else {
			p["Reserved Flags"] = int(f & 96)
		}
	}
	if v == 2 {
		p["Timestamp [ms]"] = dnp3Time48(b[1:])
		p["Timestamp Quality"] = "not-reported"
	}
	if v == 3 {
		if !cto.valid {
			return nil, protocolError(ErrContextRequired, "DNP3 relative event requires prior message-local CTO")
		}
		delta := binary.LittleEndian.Uint16(b[1:])
		p["Relative Time [ms]"] = delta
		p["CTO Time [ms]"] = cto.ms
		p["Timestamp [ms]"] = cto.ms + uint64(delta)
		p["Timestamp Quality"] = "reported-unsynchronized"
		if cto.synchronized {
			p["Timestamp Quality"] = "reported-synchronized"
		}
	}
	return p, nil
}
