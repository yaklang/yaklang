package pcaputil

import (
	"math"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// WithProtocolC37118Engineering adds explicitly configured numeric conversions
// to native C37.118 observations. It does not verify source or physical quality.
// Raw fields remain unchanged. CFG-2 integer analog scaling requires an external
// channel convention and is reported as ContextRequired, never guessed.
func WithProtocolC37118Engineering() CaptureOption {
	return func(c *CaptureConfig) error { c.c37118Engineering = true; return nil }
}

// WithSessionC37118Engineering selects the same conversions for TCP or UDP Feed.
func WithSessionC37118Engineering() ProtocolSessionOption {
	return func(s *captureSession) error { s.f.a.c37118Engineering = true; return nil }
}

func (f *binFlow) c37118EngineeringEnabled() bool {
	return f.a.c37118Engineering || f.c37118 != nil && f.c37118.engineering
}

func c37118Quantity(v float64) map[string]any {
	var value any = v
	state := "finite"
	switch {
	case math.IsNaN(v):
		value, state = "NaN", "undefined"
	case math.IsInf(v, 1):
		value, state = "+Inf", "non-finite"
	case math.IsInf(v, -1):
		value, state = "-Inf", "non-finite"
	}
	return map[string]any{"Value": value, "Numeric State": state}
}

func c37118EngineeringNumber(n stream_parser.C37118Number, integerUndefined bool) float64 {
	// Fixed GPA/GSF IEEE implementations designate the signed minimum as NaN
	// for rectangular phasors and frequency, but not for integer analog values.
	if integerUndefined && !n.FloatingPoint && n.Signed && n.RawBits == 0x8000 {
		return math.NaN()
	}
	return n.Value
}

// Fixed GPA/GSF 22a9aa... value/definition layouts, with CFG-3 AngleAdder in
// documented radians. Its display Attributes mix degrees with this radian field;
// that display behavior is deliberately not used as a numeric oracle.
func c37118EngineeringFields(m *stream_parser.C37118Message, cfg *stream_parser.C37118Configuration) (map[string]any, error) {
	out := map[string]any{"Physical Validity Verified": false, "Complete Conversion": true, "Angle Unit": "radian"}
	pmus := make([]map[string]any, 0, len(m.Data))
	missingAnalog := false
	for i, d := range m.Data {
		p := cfg.PMUs[i]
		nominal := 60.0
		if p.NominalFrequencyFlags&1 != 0 {
			nominal = 50
		}
		freq, df := c37118EngineeringNumber(d.Frequency, true), c37118EngineeringNumber(d.FrequencyDerivative, true)
		if !d.Frequency.FloatingPoint {
			freq = nominal + freq/1000
			df /= 100
		}
		phasors := make([]map[string]any, 0, len(d.Phasors))
		for j, ph := range d.Phasors {
			first, second := c37118EngineeringNumber(ph.First, !d.Polar), c37118EngineeringNumber(ph.Second, !d.Polar)
			scale, angleAdd, indication := 1.0, 0.0, byte(0)
			unit := "unknown"
			if x := p.Extended; x != nil {
				conv := x.PhasorConversions[j]
				scale, angleAdd, indication = conv.MagnitudeMultiplier.Value, conv.AngleAdder.Value, conv.TypeIndication
				unit = "V"
				if indication&8 != 0 {
					unit = "A"
				}
			} else {
				u := p.PhasorUnits[j]
				indication = u.Type
				if !ph.First.FloatingPoint {
					scale = float64(u.Scale) / 100000
				}
				if u.Type == 0 {
					unit = "V"
				} else if u.Type == 1 {
					unit = "A"
				}
			}
			var real, imag, magnitude, angle float64
			if d.Polar {
				if !ph.Second.FloatingPoint {
					second /= 10000
				}
				magnitude, angle = first*scale, second+angleAdd
				real, imag = magnitude*math.Cos(angle), magnitude*math.Sin(angle)
			} else {
				magnitude, angle = math.Hypot(first, second)*scale, math.Atan2(second, first)+angleAdd
				if angleAdd == 0 {
					real, imag = first*scale, second*scale
				} else {
					c, s := math.Cos(angleAdd), math.Sin(angleAdd)
					real, imag = scale*(first*c-second*s), scale*(first*s+second*c)
				}
			}
			phasors = append(phasors, map[string]any{"Label": p.PhasorNames[j], "Unit": unit, "Type Indication": indication, "Magnitude": c37118Quantity(magnitude), "Angle Radians": c37118Quantity(angle), "Real": c37118Quantity(real), "Imaginary": c37118Quantity(imag)})
		}
		analogs := make([]map[string]any, 0, len(d.Analogs))
		for j, a := range d.Analogs {
			item := map[string]any{"Label": p.AnalogNames[j], "Unit": "unspecified"}
			if x := p.Extended; x != nil {
				v := x.AnalogConversions[j]
				item["Conversion"] = "CFG-3 linear"
				item["Quantity"] = c37118Quantity(a.Value*v.Multiplier.Value + v.Adder.Value)
			} else if a.FloatingPoint {
				item["Conversion"] = "CFG-2 direct floating point"
				item["Quantity"] = c37118Quantity(a.Value)
			} else {
				item["Conversion"] = "context-required"
				item["Declared Type"], item["Declared Scale"] = p.AnalogUnits[j].Type, p.AnalogUnits[j].Scale
				missingAnalog = true
			}
			analogs = append(analogs, item)
		}
		digitals := make([]map[string]any, 0, len(d.Digitals))
		for j, word := range d.Digitals {
			bits := make([]map[string]any, 0, 16)
			masks := p.DigitalUnits[j]
			for k := 0; k < 16; k++ {
				bit := uint16(1) << k
				bits = append(bits, map[string]any{"Bit": k, "Wire Set": word&bit != 0, "Declared Normal Set": masks.NormalMask&bit != 0, "Declared Valid Mask Set": masks.ValidMask&bit != 0, "Differs From Declared Normal": (word^masks.NormalMask)&bit != 0})
			}
			digitals = append(digitals, map[string]any{"Raw Word": word, "Labels In Wire Order": cloneScalarValues(p.DigitalNames[j]), "Bits": bits})
		}
		status := d.Status
		pmus = append(pmus, map[string]any{"ID Code": d.IDCode, "Station Name": p.StationName, "Nominal Frequency Hz": nominal, "Frequency Hz": c37118Quantity(freq), "Frequency Derivative Hz/s": c37118Quantity(df), "Phasors": phasors, "Analogs": analogs, "Digitals": digitals, "Declared Status": map[string]any{"Interpretation": "GPA/GSF StatusFlags", "Raw": status, "Invalid Or Test Flag": status&0x8000 != 0, "Device Error Flag": status&0x4000 != 0, "Synchronization Error Flag": status&0x2000 != 0, "Arrival Sorted Flag": status&0x1000 != 0, "Trigger Detected Flag": status&0x0800 != 0, "Configuration Changed Flag": status&0x0400 != 0, "Data Modified Flag": status&0x0200 != 0, "Time Quality Code": (status >> 6) & 7, "Unlocked Time Code": (status >> 4) & 3, "Trigger Reason Code": status & 15}})
	}
	out["PMUs"] = pmus
	if missingAnalog {
		out["Complete Conversion"] = false
		return out, protocolError(ErrContextRequired, "CFG-2 integer analog conversion requires independent channel scaling convention")
	}
	return out, nil
}
