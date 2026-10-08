package stream_parser

import (
	"encoding/binary"
	"fmt"
	"math"
)

// C37118Message describes one IEEE C37.118-2005/2011 frame. Data decoding
// requires an explicit, current CFG-2 from the same transport association.
// No network activity, implicit configuration cache, scaling, or measurement
// accuracy/validity claim is made. Status and raw numeric encodings are kept.
// Layout and CRC cross-check: publisher-maintained GPA/GSF implementation:
// https://github.com/GridProtectionAlliance/gsf/tree/master/Source/Libraries/GSF.PhasorProtocols/IEEEC37_118
// https://github.com/GridProtectionAlliance/gsf/blob/master/Source/Libraries/GSF.Core.Shared/IO/Checksums/CrcCCITT.cs
type C37118Message struct {
	FrameType, Version                 uint8
	FrameSize, IDCode                  uint16
	SecondOfCentury, FractionOfSecond  uint32
	TimeQuality                        uint8
	Checksum                           uint16
	BodyDecoded, ConfigurationRequired bool
	HeaderText                         string
	Command                            uint16
	CommandData, OpaqueBody            []byte
	Configuration                      *C37118Configuration
	Data                               []C37118PMUData
}

type C37118Configuration struct {
	TimeBaseReserved uint8
	TimeBase         uint32
	PMUs             []C37118PMUConfiguration
	DataRate         int16 // Positive: frames/second; negative: seconds/frame.
}

type C37118PMUConfiguration struct {
	StationName                        string // Exact 16-byte label, including padding.
	IDCode, Format                     uint16
	PhasorNames, AnalogNames           []string
	DigitalNames                       [][]string // Sixteen labels per digital word.
	PhasorUnits, AnalogUnits           []C37118Unit
	DigitalUnits                       []C37118DigitalUnit
	NominalFrequencyFlags, ConfigCount uint16
}

type C37118Unit struct {
	Type  uint8
	Scale uint32 // Raw low 24 bits; interpretation depends on channel type.
}

type C37118DigitalUnit struct{ NormalMask, ValidMask uint16 }

// C37118Number retains the on-wire representation, not engineering units.
// Floating-point NaN/Inf are preserved, not silently converted to valid data.
type C37118Number struct {
	RawBits       uint32
	Width         uint8
	FloatingPoint bool
	Signed        bool
	Value         float64
}

type C37118Phasor struct{ First, Second C37118Number }

type C37118PMUData struct {
	IDCode, Status                 uint16
	Polar                          bool
	Phasors                        []C37118Phasor
	Frequency, FrequencyDerivative C37118Number
	Analogs                        []C37118Number
	Digitals                       []uint16
}

// IEEE C37.118 CHK: CRC-16/CCITT-FALSE, poly 0x1021, init 0xffff,
// no reflection or final XOR, covering SYNC through the last body byte.
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

func decodeC37118Frame(wire, configuration []byte) (*C37118Message, error) {
	if len(wire) < 16 || len(wire) > 65535 {
		return nil, fmt.Errorf("c37118: invalid frame size")
	}
	if wire[0] != 0xaa || wire[1]&0x80 != 0 {
		return nil, fmt.Errorf("c37118: invalid sync word")
	}
	m := &C37118Message{FrameType: wire[1] >> 4, Version: wire[1] & 15,
		FrameSize: binary.BigEndian.Uint16(wire[2:]), IDCode: binary.BigEndian.Uint16(wire[4:]),
		SecondOfCentury: binary.BigEndian.Uint32(wire[6:]), TimeQuality: wire[10],
		FractionOfSecond: binary.BigEndian.Uint32(wire[10:]) & 0xffffff,
		Checksum:         binary.BigEndian.Uint16(wire[len(wire)-2:])}
	if int(m.FrameSize) != len(wire) {
		return nil, fmt.Errorf("c37118: frame size differs from message boundary")
	}
	if m.Version != 1 && m.Version != 2 {
		return nil, fmt.Errorf("c37118: unsupported version")
	}
	if m.FrameType > 5 || (m.FrameType == 5 && m.Version != 2) {
		return nil, fmt.Errorf("c37118: unsupported frame type")
	}
	if c37118CRC(wire[:len(wire)-2]) != m.Checksum {
		return nil, fmt.Errorf("c37118: CRC mismatch")
	}
	body := wire[14 : len(wire)-2]
	switch m.FrameType {
	case 1:
		m.HeaderText, m.BodyDecoded = string(body), true
	case 2, 3:
		var err error
		m.Configuration, err = c37118ReadConfiguration(body)
		if err != nil {
			return nil, err
		}
		m.BodyDecoded = true
	case 4:
		if len(body) < 2 {
			return nil, fmt.Errorf("c37118: truncated command")
		}
		m.Command = binary.BigEndian.Uint16(body)
		if m.Command >= 1 && m.Command <= 6 {
			if len(body) != 2 {
				return nil, fmt.Errorf("c37118: unexpected standard command data")
			}
			m.BodyDecoded = true
		} else {
			// Extended/vendor commands are described, never performed.
			m.CommandData = append([]byte(nil), body[2:]...)
		}
	case 0:
		if len(body) < 6 {
			return nil, fmt.Errorf("c37118: truncated data frame")
		}
		if len(configuration) == 0 {
			m.ConfigurationRequired = true
			m.OpaqueBody = append([]byte(nil), body...)
			break
		}
		cfg, err := decodeC37118Frame(configuration, nil)
		if err != nil {
			return nil, fmt.Errorf("c37118: invalid configuration context: %w", err)
		}
		if cfg.FrameType != 3 || cfg.IDCode != m.IDCode || cfg.Version != m.Version {
			return nil, fmt.Errorf("c37118: CFG-2 context type, ID code or version mismatch")
		}
		m.Data, err = c37118ReadData(body, cfg.Configuration)
		if err != nil {
			return nil, err
		}
		m.BodyDecoded = true
	case 5:
		// CFG-3 continuation and expanded definitions need a separate decoder.
		// Keeping this explicit avoids pretending CFG-1/2 grammar applies.
		m.OpaqueBody = append([]byte(nil), body...)
	}
	return m, nil
}

func c37118ReadConfiguration(body []byte) (*C37118Configuration, error) {
	if len(body) < 8 {
		return nil, fmt.Errorf("c37118: truncated configuration header")
	}
	c := &C37118Configuration{TimeBaseReserved: body[0], TimeBase: binary.BigEndian.Uint32(body) & 0xffffff}
	count := int(binary.BigEndian.Uint16(body[4:]))
	// A PMU with no channels still needs 26 fixed and four footer bytes.
	// Check all advertised counts against remaining wire before allocating.
	if c.TimeBase == 0 || count == 0 || count > (len(body)-8)/30 {
		return nil, fmt.Errorf("c37118: invalid time base or PMU count")
	}
	pos := 6
	for i := 0; i < count; i++ {
		if len(body)-pos < 32 {
			return nil, fmt.Errorf("c37118: truncated PMU configuration")
		}
		p := C37118PMUConfiguration{StationName: string(body[pos : pos+16]), IDCode: binary.BigEndian.Uint16(body[pos+16:]), Format: binary.BigEndian.Uint16(body[pos+18:])}
		ph, an, dg := int(binary.BigEndian.Uint16(body[pos+20:])), int(binary.BigEndian.Uint16(body[pos+22:])), int(binary.BigEndian.Uint16(body[pos+24:]))
		pos += 26
		need := (ph+an)*20 + dg*260 + 4
		if need > len(body)-pos-2 {
			return nil, fmt.Errorf("c37118: channel counts exceed configuration boundary")
		}
		for j := 0; j < ph; j++ {
			p.PhasorNames = append(p.PhasorNames, string(body[pos:pos+16]))
			pos += 16
		}
		for j := 0; j < an; j++ {
			p.AnalogNames = append(p.AnalogNames, string(body[pos:pos+16]))
			pos += 16
		}
		for j := 0; j < dg; j++ {
			names := make([]string, 16)
			for k := range names {
				names[k] = string(body[pos : pos+16])
				pos += 16
			}
			p.DigitalNames = append(p.DigitalNames, names)
		}
		unit := func() C37118Unit {
			v := C37118Unit{Type: body[pos], Scale: binary.BigEndian.Uint32(body[pos:]) & 0xffffff}
			pos += 4
			return v
		}
		for j := 0; j < ph; j++ {
			p.PhasorUnits = append(p.PhasorUnits, unit())
		}
		for j := 0; j < an; j++ {
			p.AnalogUnits = append(p.AnalogUnits, unit())
		}
		for j := 0; j < dg; j++ {
			p.DigitalUnits = append(p.DigitalUnits, C37118DigitalUnit{binary.BigEndian.Uint16(body[pos:]), binary.BigEndian.Uint16(body[pos+2:])})
			pos += 4
		}
		p.NominalFrequencyFlags, p.ConfigCount = binary.BigEndian.Uint16(body[pos:]), binary.BigEndian.Uint16(body[pos+2:])
		pos += 4
		c.PMUs = append(c.PMUs, p)
	}
	if pos+2 != len(body) {
		return nil, fmt.Errorf("c37118: configuration length differs from field counts")
	}
	c.DataRate = int16(binary.BigEndian.Uint16(body[pos:]))
	if c.DataRate == 0 {
		return nil, fmt.Errorf("c37118: zero data rate")
	}
	return c, nil
}

func c37118ReadData(body []byte, cfg *C37118Configuration) ([]C37118PMUData, error) {
	want := 0
	for _, p := range cfg.PMUs {
		phWidth, anWidth, fqWidth := 4, 2, 4
		if p.Format&2 != 0 {
			phWidth = 8
		}
		if p.Format&4 != 0 {
			anWidth = 4
		}
		if p.Format&8 != 0 {
			fqWidth = 8
		}
		want += 2 + phWidth*len(p.PhasorNames) + anWidth*len(p.AnalogNames) + fqWidth + 2*len(p.DigitalNames)
	}
	if want != len(body) {
		return nil, fmt.Errorf("c37118: data length differs from CFG-2")
	}
	pos := 0
	number := func(floating, signed bool) C37118Number {
		n := C37118Number{FloatingPoint: floating, Signed: signed || floating, Width: 16}
		if floating {
			n.Width, n.RawBits = 32, binary.BigEndian.Uint32(body[pos:])
			pos += 4
			n.Value = float64(math.Float32frombits(n.RawBits))
		} else {
			n.RawBits = uint32(binary.BigEndian.Uint16(body[pos:]))
			pos += 2
			if signed {
				n.Value = float64(int16(n.RawBits))
			} else {
				n.Value = float64(n.RawBits)
			}
		}
		return n
	}
	var result []C37118PMUData
	for _, p := range cfg.PMUs {
		d := C37118PMUData{IDCode: p.IDCode, Status: binary.BigEndian.Uint16(body[pos:]), Polar: p.Format&1 != 0}
		pos += 2
		for range p.PhasorNames {
			d.Phasors = append(d.Phasors, C37118Phasor{number(p.Format&2 != 0, p.Format&1 == 0), number(p.Format&2 != 0, true)})
		}
		d.Frequency, d.FrequencyDerivative = number(p.Format&8 != 0, true), number(p.Format&8 != 0, true)
		for range p.AnalogNames {
			d.Analogs = append(d.Analogs, number(p.Format&4 != 0, true))
		}
		for range p.DigitalNames {
			d.Digitals = append(d.Digitals, binary.BigEndian.Uint16(body[pos:]))
			pos += 2
		}
		result = append(result, d)
	}
	return result, nil
}
