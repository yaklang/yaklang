package stream_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

func etherSBusFixture(attribute byte, body ...byte) []byte {
	wire := make([]byte, 11+len(body))
	binary.BigEndian.PutUint32(wire, uint32(len(wire)))
	wire[7], wire[8] = 7, attribute
	copy(wire[9:], body)
	binary.BigEndian.PutUint16(wire[len(wire)-2:], etherSBusCRC(wire[:len(wire)-2]))
	return wire
}

func TestDecodeEtherSBusFieldsAndContext(t *testing.T) {
	require.Equal(t, uint16(0x31c3), etherSBusCRC([]byte("123456789")))
	original, err := hex.DecodeString("0000000d01000001000a205318")
	require.NoError(t, err)
	m, err := decodeEtherSBusDatagram(original, nil)
	require.NoError(t, err)
	require.Equal(t, EtherSBusMessage{Length: 13, Version: 1, Sequence: 1, Destination: 10, Command: 0x20, CommandSource: "wire", BodyKind: "version-request", BodyDecoded: true, Checksum: 0x5318}, *m)
	request := etherSBusFixture(0, 10, 0x1e, 1, 0x12, 0x34, 0x56)
	req, err := decodeEtherSBusDatagram(request, nil)
	require.NoError(t, err)
	require.Equal(t, "word-read-request", req.BodyKind)
	require.Equal(t, uint16(2), req.Count)
	require.Equal(t, uint32(0x123456), req.Address)
	wire := etherSBusFixture(1, 0xff, 0xff, 0xff, 0xff, 0x80, 0, 0, 0)
	m, err = decodeEtherSBusDatagram(wire, request)
	require.NoError(t, err)
	require.Equal(t, []uint32{0xffffffff, 0x80000000}, m.Words)
	require.Equal(t, req.Address, m.Address)
	require.Equal(t, "request", m.CommandSource)
	require.Equal(t, "word-read-response", m.BodyKind)
	withoutContext, err := decodeEtherSBusDatagram(wire, nil)
	require.NoError(t, err)
	require.False(t, withoutContext.BodyDecoded)
	require.True(t, withoutContext.RequestContextRequired)
	require.Empty(t, withoutContext.CommandSource)
	require.Equal(t, wire[9:len(wire)-2], withoutContext.Bytes)
	for _, command := range []byte{0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b} {
		m, err = decodeEtherSBusDatagram(etherSBusFixture(1, 0x52), etherSBusFixture(0, 10, command))
		require.NoError(t, err)
		require.Equal(t, uint8(0x52), m.CPUStatus)
		require.Equal(t, "status-response", m.BodyKind)
	}
	m, err = decodeEtherSBusDatagram(etherSBusFixture(1, []byte("CPU01420\x00")...), etherSBusFixture(0, 10, 0x20))
	require.NoError(t, err)
	require.Equal(t, "CPU01", m.CPUType)
	require.Equal(t, "420", m.FirmwareVersion)
	require.Zero(t, m.FirmwareSuffix)
	for _, command := range []byte{0x1e, 0x1f, 0x47} {
		unit := 4
		if command == 0x47 {
			unit = 1
		}
		m, err = decodeEtherSBusDatagram(etherSBusFixture(1, bytes.Repeat([]byte{0xff}, 256*unit)...), etherSBusFixture(0, 10, command, 255, 255, 255, 255))
		require.NoError(t, err)
		require.Equal(t, uint16(256), m.Count, "count+1 must not wrap to zero")
		require.Equal(t, uint32(0xffffff), m.Address)
	}
	m, err = decodeEtherSBusDatagram(etherSBusFixture(0, 10, 0x51, 4, 0, 0, 1, 0x80, 0xff), nil)
	require.NoError(t, err)
	require.Equal(t, uint16(2), m.Count)
	require.Equal(t, []byte{0x80, 0xff}, m.Bytes)
	for _, code := range []uint16{0, 1, 2, 3, 4, 0xffff} {
		m, err = decodeEtherSBusDatagram(etherSBusFixture(2, byte(code>>8), byte(code)), nil)
		require.NoError(t, err)
		require.Equal(t, code, m.ACKCode)
		require.True(t, m.BodyDecoded)
		require.Equal(t, "ack-nak", m.BodyKind)
	}
	unknown := etherSBusFixture(0, 10, 0xfe, 0x80, 0xff)
	m, err = decodeEtherSBusDatagram(unknown, nil)
	require.NoError(t, err)
	require.False(t, m.BodyDecoded)
	require.Equal(t, "opaque-request", m.BodyKind)
	m, err = decodeEtherSBusDatagram(etherSBusFixture(1, 0x80), unknown)
	require.NoError(t, err)
	require.False(t, m.BodyDecoded)
	require.False(t, m.RequestContextRequired)
	require.Equal(t, []byte{0x80}, m.Bytes)
}

func TestDecodeEtherSBusRejectsInvalidFramingAndResponses(t *testing.T) {
	valid := etherSBusFixture(0, 10, 0x20)
	for cut := 0; cut < len(valid); cut++ {
		_, err := decodeEtherSBusDatagram(valid[:cut], nil)
		require.Error(t, err)
	}
	for i := range valid {
		changed := append([]byte(nil), valid...)
		changed[i] ^= 1
		_, err := decodeEtherSBusDatagram(changed, nil)
		require.Error(t, err)
	}
	for _, mutation := range []struct{ index, value int }{{4, 2}, {5, 1}, {8, 3}} {
		changed := append([]byte(nil), valid...)
		changed[mutation.index] = byte(mutation.value)
		binary.BigEndian.PutUint16(changed[len(changed)-2:], etherSBusCRC(changed[:len(changed)-2]))
		_, err := decodeEtherSBusDatagram(changed, nil)
		require.Error(t, err, "correct CRC cannot validate an unsupported header")
	}
	for _, pair := range []struct{ wire, request []byte }{
		{etherSBusFixture(0, 10), nil},
		{etherSBusFixture(0, 10, 0x20, 0), nil},
		{etherSBusFixture(0, 10, 0x1e, 0, 0, 0), nil},
		{etherSBusFixture(0, 10, 0x51, 2, 0, 0, 0), nil},
		{etherSBusFixture(0, 10, 0x51, 4, 0, 0, 0, 1), nil},
		{etherSBusFixture(2, 0), nil},
		{etherSBusFixture(2, 0, 0, 0), nil},
		{etherSBusFixture(1, 0), etherSBusFixture(0, 10, 0x20)},
		{etherSBusFixture(1, 0, 0), etherSBusFixture(0, 10, 0x1b)},
		{etherSBusFixture(1, 0, 0, 0), etherSBusFixture(0, 10, 0x1e, 0, 0, 0, 0)},
		{etherSBusFixture(1, 0, 0), etherSBusFixture(0, 10, 0x47, 0, 0, 0, 0)},
		{etherSBusFixture(1, 0), etherSBusFixture(0, 10, 0x51, 3, 0, 0, 0, 0)},
		{etherSBusFixture(1, 0), etherSBusFixture(2, 0, 0)},
		{valid, valid},
		{append(append([]byte(nil), valid...), 0), nil},
		{make([]byte, 65508), nil},
	} {
		_, err := decodeEtherSBusDatagram(pair.wire, pair.request)
		require.Error(t, err)
	}
	response := etherSBusFixture(1, 0x52)
	wrongSequence := etherSBusFixture(0, 10, 0x1b)
	wrongSequence[7] = 8
	binary.BigEndian.PutUint16(wrongSequence[len(wrongSequence)-2:], etherSBusCRC(wrongSequence[:len(wrongSequence)-2]))
	_, err := decodeEtherSBusDatagram(response, wrongSequence)
	require.ErrorContains(t, err, "sequence mismatch")
	badCRC := etherSBusFixture(0, 10, 0x1b)
	badCRC[len(badCRC)-1] ^= 1
	_, err = decodeEtherSBusDatagram(response, badCRC)
	require.ErrorContains(t, err, "invalid request context")
}

func FuzzDecodeEtherSBusDatagram(f *testing.F) {
	f.Add(etherSBusFixture(0, 10, 0x20), []byte{})
	f.Add(etherSBusFixture(1, 0x52), etherSBusFixture(0, 10, 0x1b))
	f.Add(etherSBusFixture(1, 0, 0, 0, 1), etherSBusFixture(0, 10, 0x1e, 0, 0, 0, 0))
	f.Add(etherSBusFixture(2, 0, 0), []byte{})
	f.Fuzz(func(t *testing.T, wire, request []byte) {
		if len(wire) > 65508 || len(request) > 65508 {
			return
		}
		for attempt := 0; attempt < 2; attempt++ {
			m, err := decodeEtherSBusDatagram(wire, request)
			if err == nil {
				require.Equal(t, uint32(len(wire)), m.Length)
				require.Equal(t, etherSBusCRC(wire[:len(wire)-2]), m.Checksum)
				require.NotEmpty(t, m.BodyKind)
			}
			// Reach body interpretation as well as the length/CRC rejection path.
			if len(wire) < 12 {
				return
			}
			wire = append([]byte(nil), wire...)
			binary.BigEndian.PutUint32(wire, uint32(len(wire)))
			binary.BigEndian.PutUint16(wire[len(wire)-2:], etherSBusCRC(wire[:len(wire)-2]))
		}
	})
}
