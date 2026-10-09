package pcaputil

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"math"
)

// C37.118 CFG-2 context belongs to one observed publisher direction.
// Ports never supply configuration, scaling or measurement-validity claims.
type binC37118 struct {
	configuration   [2][]byte
	changeIndicated [2]bool
}

func c37118RecoverableMessageError(err *ProtocolError) bool {
	// These errors concern an already delimited message and retained context.
	// They do not justify resynchronizing after malformed framing or CRC.
	return err != nil && (err.Kind == ErrContextRequired || err.Kind == ErrResourceExceeded)
}

func probeC37118(w []byte, limit int) ProbeResult {
	if len(w) < 4 || w[0] != 0xaa || w[1]&0x80 != 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	ver := w[1] & 0x0f
	kind := w[1] >> 4
	if ver != 1 && ver != 2 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if kind > 5 {
		return ProbeResult{Verdict: ProbeReject}
	}
	n := int(binary.BigEndian.Uint16(w[2:4]))
	if n < 16 || n > 65535 {
		return ProbeResult{Verdict: ProbeReject}
	}
	_ = limit
	return probeAccept("c37118", "ieee", 90)
}

func c37118CRC(wire []byte) uint16 {
	crc := uint16(0xffff)
	for _, b := range wire {
		crc ^= uint16(b) << 8
		for bit := 0; bit < 8; bit++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

func (f *binFlow) frameC37118(w []byte) (int, *binSpec, error) {
	s := f.c37118
	if s == nil {
		return 0, nil, sessionContext("C37.118 session was not observed")
	}
	if err := f.reserveSession(256); err != nil {
		return 0, nil, err
	}
	if len(w) < 4 {
		return 0, nil, nil
	}
	if w[0] != 0xaa {
		return 0, nil, fmt.Errorf("c37118: invalid sync word")
	}
	n := int(binary.BigEndian.Uint16(w[2:4]))
	if n < 16 {
		return 0, nil, fmt.Errorf("c37118: invalid frame size")
	}
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if n > len(w) {
		return n, nil, nil
	}
	return n, f.spec("c37118", "C37118"), nil
}

func c37118TypeName(kind byte) string {
	switch kind {
	case 0:
		return "DATA"
	case 1:
		return "HEADER"
	case 2:
		return "CFG-1"
	case 3:
		return "CFG-2"
	case 4:
		return "CMD"
	case 5:
		return "CFG-3"
	default:
		return fmt.Sprintf("Type %d", kind)
	}
}

// consumeC37118 reserves both retained configurations and projected fields before
// decoding. A denied replacement invalidates that publisher's old context: a
// later DATA frame cannot silently use the config whose replacement we missed.
func (f *binFlow) consumeC37118(dir int, raw []byte) (map[string]any, error) {
	s := f.c37118
	if s == nil {
		return nil, sessionContext("C37.118 session was not observed")
	}
	retained := len(s.configuration[0]) + len(s.configuration[1])
	if err := f.reserveSession(512 + int64(retained)*8 + int64(len(raw))*32); err != nil {
		if len(raw) > 1 && raw[1]>>4 == 3 {
			s.configuration[dir] = nil
		}
		return nil, err
	}
	if len(raw) < 4 {
		return nil, protocolError(ErrNeedMore, "C37.118 header is truncated")
	}
	if raw[0] != 0xaa || raw[1]&0x80 != 0 {
		return nil, protocolError(ErrMalformedMessage, "C37.118 invalid sync word")
	}
	if raw[1]&15 != 1 && raw[1]&15 != 2 {
		return nil, protocolError(ErrUnsupportedVersion, "C37.118 selected v1/v2 profile")
	}
	if raw[1]>>4 >= 5 {
		return nil, protocolError(ErrUnsupportedFeature, "C37.118 unsupported frame type")
	}
	if int(binary.BigEndian.Uint16(raw[2:4])) > len(raw) {
		return nil, protocolError(ErrNeedMore, "C37.118 declared frame is truncated")
	}
	// A first observed change indication may precede an uncaptured new CFG.
	// Quarantine that direction once. A subsequently observed valid CFG recovers
	// it even while the publisher keeps the flag set for the specified minute.
	changed := false
	if raw[1]>>4 == 0 && len(raw) >= 18 && c37118CRC(raw[:len(raw)-2]) == binary.BigEndian.Uint16(raw[len(raw)-2:]) && int(binary.BigEndian.Uint16(raw[2:4])) == len(raw) {
		changed = binary.BigEndian.Uint16(raw[14:16])&0x0400 != 0
		if changed && !s.changeIndicated[dir] {
			s.configuration[dir] = nil
		}
	}
	cfg := s.configuration[dir]
	// A matching stream ID alone does not establish the publisher direction
	// or version of the observed configuration.
	contextMismatch := false
	if len(raw) >= 6 && raw[1]>>4 == 0 && len(cfg) > 0 && (raw[1]&15 != cfg[1]&15 || !bytes.Equal(raw[4:6], cfg[4:6])) {
		cfg = nil
		contextMismatch = true
	}
	m, err := stream_parser.DecodeC37118Message(raw, cfg, f.a.budget.MaxCollectionElements)
	if err != nil {
		if errors.Is(err, stream_parser.ErrC37118CollectionLimit) {
			return nil, protocolError(ErrResourceExceeded, err.Error())
		}
		return nil, protocolError(ErrMalformedMessage, err.Error())
	}
	if m.FrameType == 0 {
		// PMU blocks have different widths selected by CFG-2. Inspect every
		// decoded STAT, rather than treating the first block as the whole frame.
		for _, p := range m.Data {
			changed = changed || p.Status&0x0400 != 0
		}
		if changed && !s.changeIndicated[dir] && !m.ConfigurationRequired {
			s.configuration[dir] = nil
			m, err = stream_parser.DecodeC37118Message(raw, nil, f.a.budget.MaxCollectionElements)
			if err != nil {
				return nil, protocolError(ErrMalformedMessage, err.Error())
			}
		}
		s.changeIndicated[dir] = changed
	}
	out := c37118Fields(m)
	if changed {
		out["Configuration Change Indicated"] = true
	}
	switch m.FrameType {
	case 3:
		s.configuration[dir] = bytes.Clone(raw)
	case 0:
		if m.ConfigurationRequired || contextMismatch {
			return out, protocolError(ErrContextRequired, "C37.118 DATA without this publisher's observed CFG-2")
		}
	}
	return out, nil
}
func c37118Fields(m *stream_parser.C37118Message) map[string]any {
	out := map[string]any{"Sync": 170, "Sync Reserved": 0, "Packet Name": c37118TypeName(m.FrameType), "Frame Type": int(m.FrameType), "Version": int(m.Version), "ID Code": int(m.IDCode), "Frame Size": int(m.FrameSize), "Second Of Century": m.SecondOfCentury, "Time Quality": m.TimeQuality, "Fraction Of Second": m.FractionOfSecond, "Checksum": m.Checksum, "Body Decoded": m.BodyDecoded}
	switch m.FrameType {
	case 0:
		out["Role"] = "data"
	case 2, 3:
		out["Role"] = "configuration"
	case 4:
		out["Role"] = "command"
		out["Command"] = int(m.Command)
		if len(m.CommandData) > 0 {
			out["Command Data"] = bytes.Clone(m.CommandData)
		}
	case 1:
		out["Header Text"] = m.HeaderText
	}
	if m.ConfigurationRequired {
		out["Configuration Required"] = true
		out["Opaque Body"] = bytes.Clone(m.OpaqueBody)
	}
	if m.FrameType == 5 {
		out["Opaque Body"] = bytes.Clone(m.OpaqueBody)
	}
	if c := m.Configuration; c != nil {
		ps := make([]map[string]any, 0, len(c.PMUs))
		for _, p := range c.PMUs {
			units := func(us []stream_parser.C37118Unit) []map[string]any {
				vs := make([]map[string]any, 0, len(us))
				for _, u := range us {
					vs = append(vs, map[string]any{"Type": u.Type, "Scale": u.Scale})
				}
				return vs
			}
			names := make([]any, 0, len(p.DigitalNames))
			for _, ns := range p.DigitalNames {
				names = append(names, cloneScalarValues(ns))
			}
			ds := make([]map[string]any, 0, len(p.DigitalUnits))
			for _, u := range p.DigitalUnits {
				ds = append(ds, map[string]any{"Normal Mask": u.NormalMask, "Valid Mask": u.ValidMask})
			}
			ps = append(ps, map[string]any{"Station Name": p.StationName, "ID Code": p.IDCode, "Format": p.Format, "Phasor Names": cloneScalarValues(p.PhasorNames), "Analog Names": cloneScalarValues(p.AnalogNames), "Digital Names": names, "Phasor Units": units(p.PhasorUnits), "Analog Units": units(p.AnalogUnits), "Digital Units": ds, "Nominal Frequency Flags": p.NominalFrequencyFlags, "Config Count": p.ConfigCount})
		}
		out["Configuration"] = map[string]any{"Time Base Reserved": c.TimeBaseReserved, "Time Base": c.TimeBase, "Data Rate": c.DataRate, "PMUs": ps}
	}
	number := func(n stream_parser.C37118Number) map[string]any {
		var value any = n.Value
		// Preserve exact raw bits; a non-finite encoding is never replaced by zero
		// or exported as a JSON-invalid floating point number.
		if math.IsNaN(n.Value) {
			value = "NaN"
		} else if math.IsInf(n.Value, 1) {
			value = "+Inf"
		} else if math.IsInf(n.Value, -1) {
			value = "-Inf"
		}
		return map[string]any{"Raw Bits": n.RawBits, "Width": n.Width, "Floating Point": n.FloatingPoint, "Signed": n.Signed, "Value": value}
	}
	if m.BodyDecoded && m.FrameType == 0 {
		data := make([]map[string]any, 0, len(m.Data))
		for _, p := range m.Data {
			ph := make([]map[string]any, 0, len(p.Phasors))
			for _, v := range p.Phasors {
				ph = append(ph, map[string]any{"First": number(v.First), "Second": number(v.Second)})
			}
			an := make([]map[string]any, 0, len(p.Analogs))
			for _, v := range p.Analogs {
				an = append(an, number(v))
			}
			data = append(data, map[string]any{"ID Code": p.IDCode, "Status": p.Status, "Polar": p.Polar, "Phasors": ph, "Frequency": number(p.Frequency), "Frequency Derivative": number(p.FrequencyDerivative), "Analogs": an, "Digitals": cloneScalarValues(p.Digitals)})
		}
		out["Data"] = data
	}
	return out
}
