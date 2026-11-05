package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func helloObservationRecord(body []byte) []byte {
	handshake := reliabilityHS(2, body)
	record := []byte{22, 3, 3}
	record = binary.BigEndian.AppendUint16(record, uint16(len(handshake)))
	return append(record, handshake...)
}

func TestTLSServerHelloRecordObservationLiterals(t *testing.T) {
	for _, c := range []struct {
		name        string
		version     uint16
		compression byte
		selected    bool
	}{
		{"TLS13", 0x303, 0, true}, {"bad-legacy-version", 0x301, 0, true},
		{"bad-compression", 0x303, 1, true}, {"TLS12", 0x303, 0, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			extensions := []tlsResumptionExt{{0xffa5, []byte{1, 2, 3}}}
			if c.selected {
				extensions = append(extensions, tlsResumptionExt{43, []byte{3, 4}})
			}
			body := tlsResumptionHello(false, []byte("observed"), 0x1302, extensions...)
			binary.BigEndian.PutUint16(body, c.version)
			body[37+len("observed")] = c.compression
			record := helloObservationRecord(body)
			original := bytes.Clone(record)
			o, err := ObserveTLSServerHelloRecord(record, 16)
			require.NoError(t, err)
			require.Equal(t, &TLSServerHelloRecordObservation{RecordVersion: 0x303, LegacyVersion: c.version, SelectedVersion: func() uint16 {
				if c.selected {
					return 0x304
				}
				return c.version
			}(), CipherSuite: 0x1302, Compression: c.compression, SessionIDLength: 8, Extensions: len(extensions)}, o)
			require.Equal(t, original, record, "observation must not edit original bytes")
		})
	}
}

func TestTLSServerHelloRecordObservationRejectsIncompleteAndAmbiguous(t *testing.T) {
	body := tlsResumptionHello(false, nil, 0x1302, tlsResumptionExt{43, []byte{3, 4}})
	valid := helloObservationRecord(body)
	for _, c := range []struct {
		name string
		edit func([]byte) []byte
	}{
		{"record-type", func(b []byte) []byte { b[0] = 23; return b }},
		{"record-version", func(b []byte) []byte { b[2] = 4; return b }},
		{"short-record", func(b []byte) []byte { return b[:4] }},
		{"short-body", func(b []byte) []byte { return b[:len(b)-1] }},
		{"trailing-wire", func(b []byte) []byte { return append(b, 0) }},
		{"fragmented-handshake", func(b []byte) []byte { b[8]++; return b }},
		{"client-hello-type", func(b []byte) []byte { b[5] = 1; return b }},
		{"session-overrun", func(b []byte) []byte { b[43] = 32; return b }},
		{"extension-overrun", func(b []byte) []byte { b[len(b)-3]++; return b }},
		{"coalesced-records", func(b []byte) []byte { return append(b, valid...) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			record := c.edit(bytes.Clone(valid))
			original := bytes.Clone(record)
			o, err := ObserveTLSServerHelloRecord(record, 16)
			require.Error(t, err)
			require.Nil(t, o)
			require.Equal(t, original, record)
		})
	}
	for _, extensions := range [][]tlsResumptionExt{
		{{43, []byte{3, 4}}, {43, []byte{3, 4}}},
		{{43, []byte{3}}}, {{41, []byte{0}}}, {{16, []byte{0, 3, 2, 'h'}}},
	} {
		record := helloObservationRecord(tlsResumptionHello(false, nil, 0x1302, extensions...))
		o, err := ObserveTLSServerHelloRecord(record, 16)
		require.Error(t, err)
		require.Nil(t, o)
	}
	_, err := ObserveTLSServerHelloRecord(valid, 0)
	var typed *ProtocolError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, ErrResourceExceeded, typed.Kind)
}

func FuzzTLSServerHelloRecordObservation(f *testing.F) {
	valid := helloObservationRecord(tlsResumptionHello(false, nil, 0x1302, tlsResumptionExt{43, []byte{3, 4}}))
	for _, seed := range [][]byte{nil, {22, 3, 3}, valid, valid[:len(valid)-1], append(bytes.Clone(valid), valid...), helloObservationRecord(tlsResumptionHello(false, nil, 0x1302, tlsResumptionExt{43, []byte{3}}))} {
		f.Add(seed, uint16(16))
	}
	f.Fuzz(func(t *testing.T, record []byte, budget uint16) {
		if len(record) > 20000 {
			return
		}
		original := bytes.Clone(record)
		o, err := ObserveTLSServerHelloRecord(record, int(budget)%32)
		require.Equal(t, original, record)
		if err != nil {
			require.Nil(t, o)
			return
		}
		require.NotNil(t, o)
		require.LessOrEqual(t, len(record)-5, 16384)
		require.LessOrEqual(t, o.SessionIDLength, 32)
		require.LessOrEqual(t, o.Extensions, int(budget)%32)
	})
}
