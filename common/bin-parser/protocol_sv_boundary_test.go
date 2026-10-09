package bin_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"testing"
)

// Fixed Wireshark 4.2.5 sv.asn and libiec61850 1.5.1 wire grammar.
// Publisher counters use fixed unsigned widths, including high-bit values.
func svBoundaryFrame(fields []byte, count []byte) []byte {
	pdu := iecOutTLV(0x60, append(iecOutTLV(0x80, count, 1), iecOutTLV(0xa2, iecOutTLV(0x30, fields, 1), 1)...), 1)
	wire := append([]byte{0x40, 0, 0, 0, 0, 0, 0, 0}, pdu...)
	binary.BigEndian.PutUint16(wire[2:], uint16(len(wire)))
	return wire
}
func svBoundaryFields(counter, revision []byte, synch bool) []byte {
	fields := iecOutTLV(0x80, []byte("MVP"), 1)
	fields = append(fields, iecOutTLV(0x82, counter, 1)...)
	fields = append(fields, iecOutTLV(0x83, revision, 1)...)
	if synch {
		fields = append(fields, iecOutTLV(0x85, []byte{2}, 1)...)
	}
	return append(fields, iecOutTLV(0x87, []byte{0, 1, 2, 3, 4, 5, 6, 7}, 1)...)
}
func TestSVOptionalSynchronizationAndScalarBounds(t *testing.T) {
	for _, tc := range []struct {
		name   string
		fields []byte
		valid  bool
	}{
		{"present-normal", svBoundaryFields([]byte{0, 3}, []byte{0, 0, 0, 4}, true), true},
		{"absent-optional-synchronization", svBoundaryFields([]byte{0, 3}, []byte{0, 0, 0, 4}, false), true},
		{"unsigned-publisher-max", svBoundaryFields([]byte{255, 255}, []byte{255, 255, 255, 255}, true), true},
		{"counter-outside-u16", svBoundaryFields([]byte{1, 0, 0}, []byte{0, 0, 0, 4}, true), false},
		{"revision-outside-u32", svBoundaryFields([]byte{0, 3}, []byte{1, 0, 0, 0, 0}, true), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseStructured(svBoundaryFrame(tc.fields, []byte{1}), "iec61850", "SampledValues")
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
func TestSVOptionalOctetWidths(t *testing.T) {
	for _, tag := range []byte{0x84, 0x89} {
		for _, n := range []int{0, 7, 8, 9} {
			f := svBoundaryFields([]byte{0, 3}, []byte{0, 0, 0, 4}, true)
			if tag == 0x84 {
				// Insert refrTm before smpSynch; never make an ordering error a width oracle.
				at := bytes.Index(f, iecOutTLV(0x85, []byte{2}, 1))
				f = append(append(bytes.Clone(f[:at]), iecOutTLV(tag, make([]byte, n), 1)...), f[at:]...)
			} else {
				f = append(f, iecOutTLV(tag, make([]byte, n), 1)...)
			}
			_, err := ParseStructured(svBoundaryFrame(f, []byte{1}), "iec61850", "SampledValues")
			if n == 8 {
				require.NoError(t, err)
			} else {
				require.Error(t, err, "tag%x width%d", tag, n)
			}
		}
	}
}
func TestSVSchemaEmptyASDUSequence(t *testing.T) {
	// noASDU INTEGER(0..65535), seqASDU SEQUENCE OF ASDU without SIZE lower bound.
	wire := []byte{0x40, 0, 0, 15, 0, 0, 0, 0, 0x60, 5, 0x80, 1, 0, 0xa2, 0}
	_, err := ParseStructured(wire, "iec61850", "SampledValues")
	require.NoError(t, err)
}

func TestEtherCATDatagramDataLimit(t *testing.T) {
	for _, tc := range []struct {
		name  string
		valid bool
	}{{"ecat-max-data1486", true}, {"ecat-over-data1487", false}} {
		t.Run(tc.name, func(t *testing.T) {
			answer, err := trafficfixture.ReadFile("../pcapx/pcaputil/industrial-link-core/answers/" + tc.name + ".json")
			require.NoError(t, err)
			var a struct{ Wire string }
			require.NoError(t, json.Unmarshal(answer, &a))
			wire, err := hex.DecodeString(a.Wire)
			require.NoError(t, err)
			_, err = ParseStructured(wire, "ethercat", "EtherCAT")
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
