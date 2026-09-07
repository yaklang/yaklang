package stream_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// Different byte-fold implementation from production's bit-wise CRC.
func c37118FixtureCRC(data []byte) uint16 {
	crc := uint16(0xffff)
	for _, b := range data {
		crc = crc>>8 | crc<<8
		crc ^= uint16(b)
		crc ^= (crc & 0xff) >> 4
		crc ^= crc << 12
		crc ^= (crc & 0xff) << 5
	}
	return crc
}

func c37118Fixture(kind byte, body []byte) []byte {
	wire := make([]byte, len(body)+16)
	wire[0], wire[1] = 0xaa, kind<<4|1
	binary.BigEndian.PutUint16(wire[2:], uint16(len(wire)))
	binary.BigEndian.PutUint16(wire[4:], 42)
	binary.BigEndian.PutUint32(wire[6:], 0x12345678)
	binary.BigEndian.PutUint32(wire[10:], 0x71abcdef)
	copy(wire[14:], body)
	binary.BigEndian.PutUint16(wire[len(wire)-2:], c37118FixtureCRC(wire[:len(wire)-2]))
	return wire
}

func c37118Reseal(wire []byte) []byte {
	binary.BigEndian.PutUint16(wire[2:], uint16(len(wire)))
	binary.BigEndian.PutUint16(wire[len(wire)-2:], c37118FixtureCRC(wire[:len(wire)-2]))
	return wire
}

func c37118FixtureConfig(formats []uint16, channels bool) []byte {
	var body bytes.Buffer
	put16 := func(v uint16) {
		requireWrite := binary.Write(&body, binary.BigEndian, v)
		if requireWrite != nil {
			panic(requireWrite)
		}
	}
	put32 := func(v uint32) {
		requireWrite := binary.Write(&body, binary.BigEndian, v)
		if requireWrite != nil {
			panic(requireWrite)
		}
	}
	put32(1000000)
	put16(uint16(len(formats)))
	for i, format := range formats {
		fmt.Fprintf(&body, "%-16s", fmt.Sprintf("PMU %d", i+1))
		put16(uint16(i + 1))
		put16(format)
		if channels {
			put16(1)
			put16(1)
			put16(1)
			fmt.Fprintf(&body, "%-16s%-16s", "PHASOR", "ANALOG")
			for d := 0; d < 16; d++ {
				fmt.Fprintf(&body, "%-16s", fmt.Sprintf("DIGITAL %d", d))
			}
			put32(0x01012345)
			put32(0x02fedcba)
			put32(0xa55a5aa5)
		} else {
			put16(0)
			put16(0)
			put16(0)
		}
		put16(0x8001)
		put16(uint16(i + 7))
	}
	put16(0xfffe) // A negative rate describes seconds per frame.
	return c37118Fixture(3, body.Bytes())
}

func TestC37118CRCReferenceAndWireLimit(t *testing.T) {
	require.Equal(t, uint16(0x29b1), c37118CRC([]byte("123456789")))
	wire, err := hex.DecodeString("aa41001200f100000000000000000005d7d0")
	require.NoError(t, err)
	require.Equal(t, uint16(0xd7d0), c37118CRC(wire[:16]))
	_, err = decodeC37118Frame(wire, nil)
	require.NoError(t, err)
	max := c37118Fixture(1, bytes.Repeat([]byte{'x'}, 65535-16))
	m, err := decodeC37118Frame(max, nil)
	require.NoError(t, err)
	require.Len(t, m.HeaderText, 65519)
	_, err = decodeC37118Frame(append(max, 0), nil)
	require.ErrorContains(t, err, "size")
	for cut := 0; cut < len(wire); cut++ {
		_, err = decodeC37118Frame(wire[:cut], nil)
		require.Error(t, err)
	}
	for pos := range max {
		if pos%257 != 0 && pos != len(max)-1 {
			continue
		}
		copyWire := append([]byte(nil), max...)
		copyWire[pos] ^= 1
		_, err = decodeC37118Frame(copyWire, nil)
		require.Error(t, err)
	}
}

func TestC37118MixedPMUFormatsAndUnits(t *testing.T) {
	for _, formats := range [][]uint16{{0, 15}, {1, 14}} {
		cfg := c37118FixtureConfig(formats, true)
		cm, err := decodeC37118Frame(cfg, nil)
		require.NoError(t, err)
		require.Equal(t, int16(-2), cm.Configuration.DataRate)
		require.Len(t, cm.Configuration.PMUs, 2)
		for i, p := range cm.Configuration.PMUs {
			require.Equal(t, fmt.Sprintf("%-16s", fmt.Sprintf("PMU %d", i+1)), p.StationName)
			require.Equal(t, uint16(i+1), p.IDCode)
			require.Equal(t, formats[i], p.Format)
			require.Equal(t, []string{fmt.Sprintf("%-16s", "PHASOR")}, p.PhasorNames)
			require.Equal(t, []string{fmt.Sprintf("%-16s", "ANALOG")}, p.AnalogNames)
			for d := 0; d < 16; d++ {
				require.Equal(t, fmt.Sprintf("%-16s", fmt.Sprintf("DIGITAL %d", d)), p.DigitalNames[0][d])
			}
			require.Equal(t, []C37118Unit{{Type: 1, Scale: 0x012345}}, p.PhasorUnits)
			require.Equal(t, []C37118Unit{{Type: 2, Scale: 0xfedcba}}, p.AnalogUnits)
			require.Equal(t, []C37118DigitalUnit{{NormalMask: 0xa55a, ValidMask: 0x5aa5}}, p.DigitalUnits)
			require.Equal(t, uint16(0x8001), p.NominalFrequencyFlags)
			require.Equal(t, uint16(i+7), p.ConfigCount)
		}
		// First PMU: fixed phasor, frequency, analog; second: all floats.
		// Explicit bytes avoid a production serializer determining expected widths.
		body := []byte{0xab, 0xc1, 0xff, 0xfe, 0xff, 0xfd, 0xff, 0xfc, 0xff, 0xfb, 0xff, 0xfa, 0xa5, 0x5a, 0x12, 0x34, 0x3f, 0xa0, 0, 0, 0xc0, 0x20, 0, 0, 0x42, 0x6f, 0, 0, 0xbe, 0x80, 0, 0, 0xc2, 0, 0, 0, 0x5a, 0xa5}
		wire := c37118Fixture(0, body)
		m, err := decodeC37118Frame(wire, cfg)
		require.NoError(t, err)
		require.True(t, m.BodyDecoded)
		require.Len(t, m.Data, 2)
		first, second := m.Data[0], m.Data[1]
		require.Equal(t, uint16(0xabc1), first.Status)
		require.Equal(t, uint16(0x1234), second.Status)
		require.Equal(t, formats[0]&1 != 0, first.Polar)
		require.Equal(t, formats[1]&1 != 0, second.Polar)
		firstMagnitude := float64(-2)
		if first.Polar {
			firstMagnitude = 65534
		}
		require.Equal(t, C37118Number{RawBits: 65534, Width: 16, Signed: !first.Polar, Value: firstMagnitude}, first.Phasors[0].First)
		require.Equal(t, C37118Number{RawBits: 65533, Width: 16, Signed: true, Value: -3}, first.Phasors[0].Second)
		require.Equal(t, float64(-4), first.Frequency.Value)
		require.Equal(t, float64(-5), first.FrequencyDerivative.Value)
		require.Equal(t, float64(-6), first.Analogs[0].Value)
		require.Equal(t, []uint16{0xa55a}, first.Digitals)
		require.Equal(t, C37118Number{RawBits: 0x3fa00000, Width: 32, FloatingPoint: true, Signed: true, Value: 1.25}, second.Phasors[0].First)
		require.Equal(t, float64(-2.5), second.Phasors[0].Second.Value)
		require.Equal(t, float64(59.75), second.Frequency.Value)
		require.Equal(t, float64(-0.25), second.FrequencyDerivative.Value)
		require.Equal(t, float64(-32), second.Analogs[0].Value)
		require.Equal(t, []uint16{0x5aa5}, second.Digitals)
		for _, delta := range []int{-1, 1} {
			bad := c37118Fixture(0, append(append([]byte(nil), body...), 0)[:len(body)+delta])
			_, err = decodeC37118Frame(bad, cfg)
			require.ErrorContains(t, err, "CFG-2")
		}
		// Input and returned opaque buffers are not aliases.
		u, err := decodeC37118Frame(wire, nil)
		require.NoError(t, err)
		u.OpaqueBody[0] ^= 0xff
		require.Equal(t, byte(0xab), wire[14])
		// IEEE special float bit patterns remain observable, not measurement claims.
		binary.BigEndian.PutUint32(wire[30:], 0x7fc01234)
		c37118Reseal(wire)
		n, err := decodeC37118Frame(wire, cfg)
		require.NoError(t, err)
		require.Equal(t, uint32(0x7fc01234), n.Data[1].Phasors[0].First.RawBits)
		require.True(t, math.IsNaN(n.Data[1].Phasors[0].First.Value))
	}
}

func TestC37118ConfigurationValidationAndNoImplicitContext(t *testing.T) {
	cfg := c37118FixtureConfig([]uint16{0}, true)
	for _, pos := range []int{18, 40, 42, 44} {
		bad := append([]byte(nil), cfg...)
		binary.BigEndian.PutUint16(bad[pos:], 65535)
		c37118Reseal(bad)
		_, err := decodeC37118Frame(bad, nil)
		require.Error(t, err)
	}
	for _, pos := range []int{14, len(cfg) - 4} {
		bad := append([]byte(nil), cfg...)
		if pos == 14 {
			clear(bad[14:18])
		} else {
			clear(bad[pos : pos+2])
		}
		c37118Reseal(bad)
		_, err := decodeC37118Frame(bad, nil)
		require.Error(t, err)
	}
	for cut := 16; cut < len(cfg); cut++ {
		bad := c37118Reseal(append([]byte(nil), cfg[:cut]...))
		_, err := decodeC37118Frame(bad, nil)
		require.Error(t, err, "nested config prefix %d", cut)
	}
	zero := c37118FixtureConfig([]uint16{0}, false)
	data := c37118Fixture(0, []byte{0, 0, 0, 1, 0, 2})
	_, err := decodeC37118Frame(data, zero)
	require.NoError(t, err)
	for _, mutate := range []func([]byte){func(w []byte) { w[1] = 0x21 }, func(w []byte) { w[4]++ }, func(w []byte) { w[1] = 0x32 }, func(w []byte) { w[len(w)-1]++ }} {
		bad := append([]byte(nil), zero...)
		mutate(bad)
		// Leave a bad checksum in the last case; all other contexts remain CRC-valid.
		if !bytes.Equal(bad[:len(bad)-2], zero[:len(zero)-2]) {
			c37118Reseal(bad)
		}
		_, err = decodeC37118Frame(data, bad)
		require.ErrorContains(t, err, "context")
	}
	_, err = decodeC37118Frame(data, data)
	require.ErrorContains(t, err, "context")
	u, err := decodeC37118Frame(data, nil)
	require.NoError(t, err)
	require.True(t, u.ConfigurationRequired)
	require.False(t, u.BodyDecoded)
	// A maximal no-channel PMU count still fits the 16-bit frame budget.
	large := c37118FixtureConfig(make([]uint16, 2183), false)
	m, err := decodeC37118Frame(large, nil)
	require.NoError(t, err)
	require.Len(t, m.Configuration.PMUs, 2183)
	oversize := c37118FixtureConfig(make([]uint16, 2184), false)
	_, err = decodeC37118Frame(oversize, nil)
	require.ErrorContains(t, err, "size")
}

func TestC37118HeaderCommandsVersionsAndOpaqueExtensions(t *testing.T) {
	header := c37118Fixture(1, []byte("Station description\r\n"))
	m, err := decodeC37118Frame(header, nil)
	require.NoError(t, err)
	require.Equal(t, "Station description\r\n", m.HeaderText)
	require.True(t, m.BodyDecoded)
	for command := uint16(1); command <= 6; command++ {
		m, err = decodeC37118Frame(c37118Fixture(4, []byte{0, byte(command)}), nil)
		require.NoError(t, err)
		require.Equal(t, command, m.Command)
		require.True(t, m.BodyDecoded)
		_, err = decodeC37118Frame(c37118Fixture(4, []byte{0, byte(command), 0}), nil)
		require.ErrorContains(t, err, "command data")
	}
	_, err = decodeC37118Frame(c37118Fixture(4, []byte{0}), nil)
	require.ErrorContains(t, err, "command")
	ext := c37118Fixture(4, []byte{0, 8, 0xde, 0xad})
	m, err = decodeC37118Frame(ext, nil)
	require.NoError(t, err)
	require.False(t, m.BodyDecoded)
	require.Equal(t, []byte{0xde, 0xad}, m.CommandData)
	cfg1 := c37118FixtureConfig([]uint16{0}, false)
	cfg1[1] = 0x21
	c37118Reseal(cfg1)
	m, err = decodeC37118Frame(cfg1, nil)
	require.NoError(t, err)
	require.NotNil(t, m.Configuration)
	for _, value := range []byte{0x10, 0x13, 0x61, 0x71, 0x91, 0x51} {
		bad := append([]byte(nil), header...)
		bad[1] = value
		c37118Reseal(bad)
		_, err = decodeC37118Frame(bad, nil)
		require.Error(t, err)
	}
	version2 := append([]byte(nil), header...)
	version2[1] = 0x12
	c37118Reseal(version2)
	m, err = decodeC37118Frame(version2, nil)
	require.NoError(t, err)
	require.Equal(t, uint8(2), m.Version)
	cfg3 := c37118Fixture(5, []byte{0, 1, 0xde, 0xad})
	cfg3[1] = 0x52
	c37118Reseal(cfg3)
	m, err = decodeC37118Frame(cfg3, nil)
	require.NoError(t, err)
	require.False(t, m.BodyDecoded)
	require.Equal(t, []byte{0, 1, 0xde, 0xad}, m.OpaqueBody)
}
