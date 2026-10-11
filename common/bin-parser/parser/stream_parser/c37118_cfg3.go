package stream_parser

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"unicode/utf8"
)

// CFG-3 extension layout: GPA/GSF 22a9aa1028c6e53d115c4ea81026055a57b803c2,
// ConfigurationCell3 and Phasor/Analog/DigitalDefinition3. Conversion factors
// and coordinates are observations with raw IEEE bits, not applied scaling or
// assertions of measurement quality. GlobalID uses RFC UUID byte order.
type C37118PMUExtendedConfiguration struct {
	GlobalID                       [16]byte
	PhasorConversions              []C37118PhasorConversion
	AnalogConversions              []C37118AnalogConversion
	Latitude, Longitude, Elevation C37118Number
	ServiceClass                   uint8
	Window, GroupDelay             int32
}
type C37118PhasorConversion struct {
	DataModifications               uint16
	TypeIndication, UserFlags       uint8
	MagnitudeMultiplier, AngleAdder C37118Number
}
type C37118AnalogConversion struct{ Multiplier, Adder C37118Number }

// Fragmented CFG-3 needs cumulative CRC and an explicitly bounded continuation
// collector. A standalone frame must not masquerade as a complete context.
var ErrC37118ContinuationRequired = errors.New("c37118: CFG-3 continuation collection required")

func c37118ReadConfiguration3(body []byte, maxElements int) (*C37118Configuration, error) {
	return c37118ReadConfiguration3Body(body, maxElements, false)
}

func c37118ReadConfiguration3Body(body []byte, maxElements int, collected bool) (*C37118Configuration, error) {
	if len(body) < 10 {
		return nil, fmt.Errorf("c37118: truncated CFG-3 header")
	}
	if binary.BigEndian.Uint16(body) != 0 && !collected {
		return nil, ErrC37118ContinuationRequired
	}
	c := &C37118Configuration{TimeBaseReserved: body[2], TimeBase: binary.BigEndian.Uint32(body[2:]) & 0xffffff}
	count := int(binary.BigEndian.Uint16(body[6:]))
	// A zero-channel cell still needs a string length, 26 header bytes and
	// 25 footer bytes. Reject advertised sizes before allocating collections.
	if c.TimeBase == 0 || count == 0 || count > (len(body)-10)/52 {
		return nil, fmt.Errorf("c37118: invalid CFG-3 time base or PMU count")
	}
	if maxElements < 1 || count > maxElements {
		return nil, ErrC37118CollectionLimit
	}
	pos, elements := 8, count
	take := func(n int) ([]byte, error) {
		if n < 0 || n > len(body)-pos-2 {
			return nil, fmt.Errorf("c37118: CFG-3 field exceeds boundary")
		}
		b := body[pos : pos+n]
		pos += n
		return b, nil
	}
	text := func() (string, error) {
		b, err := take(1)
		if err != nil {
			return "", err
		}
		b, err = take(int(b[0]))
		if err != nil {
			return "", err
		}
		if !utf8.Valid(b) {
			return "", fmt.Errorf("c37118: invalid CFG-3 UTF-8 label")
		}
		return string(b), nil
	}
	float := func(b []byte) C37118Number {
		raw := binary.BigEndian.Uint32(b)
		return C37118Number{RawBits: raw, Width: 32, FloatingPoint: true, Signed: true, Value: float64(math.Float32frombits(raw))}
	}
	for i := 0; i < count; i++ {
		name, err := text()
		if err != nil {
			return nil, err
		}
		h, err := take(26)
		if err != nil {
			return nil, err
		}
		p := C37118PMUConfiguration{StationName: name, IDCode: binary.BigEndian.Uint16(h), Format: binary.BigEndian.Uint16(h[18:])}
		x := &C37118PMUExtendedConfiguration{}
		copy(x.GlobalID[:], h[2:18])
		p.Extended = x
		ph, an, dg := int(binary.BigEndian.Uint16(h[20:])), int(binary.BigEndian.Uint16(h[22:])), int(binary.BigEndian.Uint16(h[24:]))
		if ph*13+an*9+dg*20+25 > len(body)-pos-2 {
			return nil, fmt.Errorf("c37118: CFG-3 channel counts exceed boundary")
		}
		elements += ph + an + dg*16
		if elements > maxElements {
			return nil, ErrC37118CollectionLimit
		}
		for j := 0; j < ph; j++ {
			name, err = text()
			if err != nil {
				return nil, err
			}
			p.PhasorNames = append(p.PhasorNames, name)
		}
		for j := 0; j < an; j++ {
			name, err = text()
			if err != nil {
				return nil, err
			}
			p.AnalogNames = append(p.AnalogNames, name)
		}
		for j := 0; j < dg; j++ {
			names := make([]string, 16)
			for k := range names {
				names[k], err = text()
				if err != nil {
					return nil, err
				}
			}
			p.DigitalNames = append(p.DigitalNames, names)
		}
		for j := 0; j < ph; j++ {
			b, err := take(12)
			if err != nil {
				return nil, err
			}
			x.PhasorConversions = append(x.PhasorConversions, C37118PhasorConversion{binary.BigEndian.Uint16(b), b[2], b[3], float(b[4:]), float(b[8:])})
		}
		for j := 0; j < an; j++ {
			b, err := take(8)
			if err != nil {
				return nil, err
			}
			x.AnalogConversions = append(x.AnalogConversions, C37118AnalogConversion{float(b), float(b[4:])})
		}
		for j := 0; j < dg; j++ {
			b, err := take(4)
			if err != nil {
				return nil, err
			}
			p.DigitalUnits = append(p.DigitalUnits, C37118DigitalUnit{binary.BigEndian.Uint16(b), binary.BigEndian.Uint16(b[2:])})
		}
		b, err := take(25)
		if err != nil {
			return nil, err
		}
		x.Latitude, x.Longitude, x.Elevation = float(b), float(b[4:]), float(b[8:])
		x.ServiceClass = b[12]
		x.Window = int32(binary.BigEndian.Uint32(b[13:]))
		x.GroupDelay = int32(binary.BigEndian.Uint32(b[17:]))
		p.NominalFrequencyFlags, p.ConfigCount = binary.BigEndian.Uint16(b[21:]), binary.BigEndian.Uint16(b[23:])
		c.PMUs = append(c.PMUs, p)
	}
	if pos+2 != len(body) {
		return nil, fmt.Errorf("c37118: CFG-3 length differs from field counts")
	}
	c.DataRate = int16(binary.BigEndian.Uint16(body[pos:]))
	if c.DataRate == 0 {
		return nil, fmt.Errorf("c37118: zero CFG-3 data rate")
	}
	return c, nil
}
