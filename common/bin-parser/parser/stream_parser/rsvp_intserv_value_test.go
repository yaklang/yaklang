package stream_parser

import (
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func rsvpIntservTestWords(number, flags byte, data []byte) []byte {
	w := append([]byte{number, flags, 0, 0}, data...)
	binary.BigEndian.PutUint16(w[2:], uint16(len(data)/4))
	return w
}
func rsvpIntservTestBody(services ...[]byte) []byte {
	w := make([]byte, 4)
	for _, service := range services {
		w = append(w, service...)
	}
	binary.BigEndian.PutUint16(w[2:], uint16((len(w)-4)/4))
	return w
}
func rsvpIntservTestToken() []byte {
	data := make([]byte, 20)
	for i := 0; i < 3; i++ {
		binary.BigEndian.PutUint32(data[4*i:], 0x45bb8000) // 6000 exactly.
	}
	binary.BigEndian.PutUint32(data[12:], 1)
	binary.BigEndian.PutUint32(data[16:], 1500)
	return rsvpIntservTestWords(127, 0, data)
}
func rsvpIntservTestParameter(t *testing.T, result map[string]any) map[string]any {
	t.Helper()
	return rsvpFirstBody(t, result)["Services"].([]map[string]any)[0]["Parameters"].([]map[string]any)[0]
}

func TestRSVPIntservNestedOwnersAndClassServiceNamespaces(t *testing.T) {
	for _, tc := range []struct{ class, service byte }{{12, 1}, {9, 5}} {
		body := rsvpIntservTestBody(rsvpIntservTestWords(tc.service, 0x80, rsvpIntservTestToken()))
		w := rsvpTestMessage(1, rsvpTestObject(tc.class, 2, body))
		f, err := DecodeRSVPWire(w, 3)
		require.NoError(t, err)
		require.True(t, f["Field Layouts Complete"].(bool))
		services := rsvpFirstBody(t, f)["Services"].([]map[string]any)
		require.True(t, services[0]["Header"].(map[string]any)["Break Bit Observed"].(bool))
		parameter := rsvpIntservTestParameter(t, f)
		rate := parameter["Body"].(map[string]any)["Token Bucket Rate"].(map[string]any)
		require.Equal(t, uint64(0x45bb8000), rate["IEEE754 Bits"])
		require.Equal(t, "6000", rate["Value"])
		require.Equal(t, []any{uint64(24), uint64(28)}, rate["Wire Field Ranges"].(map[string]any)["Value"])
		require.NotContains(t, parameter["Body"], "Observed Value Domain Issues")
		require.False(t, f["Reservation State Inferred"].(bool))
	}
	// The same service1 bytes have no selected token-bucket meaning in class9.
	f, err := DecodeRSVPWire(rsvpTestMessage(1, rsvpTestObject(9, 2, rsvpIntservTestBody(rsvpIntservTestWords(1, 0, rsvpIntservTestToken())))), 3)
	require.NoError(t, err)
	require.False(t, f["Field Layouts Complete"].(bool))
	service := rsvpFirstBody(t, f)["Services"].([]map[string]any)[0]
	require.NotContains(t, service, "Parameters")
	require.Equal(t, rsvpIntservTestToken(), service["Body"].(map[string]any)["Opaque Service Data"])
}

func TestRSVPIntservIEEE32SpecialsAndExactDecimalAreJSONSafe(t *testing.T) {
	for _, tc := range []struct {
		bits            uint32
		value, category string
	}{
		{0x3dcccccd, "0.100000001490116119384765625", "finite"}, {0x45bb8000, "6000", "finite"},
		{0, "0", "positive-zero"}, {0x80000000, "-0", "negative-zero"},
		{0x7f800000, "+Infinity", "positive-infinity"}, {0xff800000, "-Infinity", "negative-infinity"},
		{0x7f800001, "NaN", "nan"}, {0xffc00001, "NaN", "nan"},
	} {
		value, category := RSVPIntservIEEE32(tc.bits)
		require.Equal(t, tc.value, value)
		require.Equal(t, tc.category, category)
	}
	for _, tc := range []struct {
		field int
		bits  uint32
		issue bool
	}{{0, 0, true}, {1, 0x80000000, true}, {0, 0x7f800000, true}, {2, 0x7f800000, false}, {2, 0x7fc00001, true}} {
		p := rsvpIntservTestToken()
		binary.BigEndian.PutUint32(p[4+4*tc.field:], tc.bits)
		w := rsvpTestMessage(1, rsvpTestObject(12, 2, rsvpIntservTestBody(rsvpIntservTestWords(1, 0, p))))
		f, err := DecodeRSVPWire(w, 3)
		require.NoError(t, err, "value-domain warnings do not discard observed wire fields")
		_, found := rsvpIntservTestParameter(t, f)["Body"].(map[string]any)["Observed Value Domain Issues"]
		require.Equal(t, tc.issue, found)
		_, err = json.Marshal(f)
		require.NoError(t, err)
	}
}

func TestRSVPIntservPartialPrimitiveAndWordBoundaries(t *testing.T) {
	body := rsvpIntservTestBody(rsvpIntservTestWords(1, 0, rsvpIntservTestToken()))
	w := rsvpTestMessage(1, rsvpTestObject(12, 2, body))
	for cut := 0; cut < len(w); cut++ {
		f, err := DecodeRSVPWire(w[:cut], 64)
		require.Error(t, err, "cut=%d", cut)
		require.NotErrorIs(t, err, ErrRSVPWireFormat, "capture shortage alone is not a declared-length contradiction; cut=%d", cut)
		if cut >= 24 {
			b := rsvpIntservTestParameter(t, f)["Body"].(map[string]any)
			_, rate := b["Token Bucket Rate"]
			_, bucket := b["Token Bucket Size"]
			require.Equal(t, cut >= 28, rate)
			require.Equal(t, cut >= 32, bucket)
			if cut == 27 {
				require.Equal(t, w[24:27], b["Unparsed Parameter Data"])
				require.Equal(t, []any{uint64(24), uint64(27)}, b["Wire Field Ranges"].(map[string]any)["Unparsed Parameter Data"])
				require.NotContains(t, f, "Unparsed Body")
			}
		}
	}
	for _, position := range []int{14, 18, 22} {
		bad := append([]byte(nil), w...)
		bad[2], bad[3] = 0, 0 // Omitted checksum isolates length evidence.
		binary.BigEndian.PutUint16(bad[position:], 0x100)
		_, err := DecodeRSVPWire(bad, 64)
		require.ErrorIs(t, err, ErrRSVPWireFormat, "position=%d", position)
	}
	// Parameter says only one word: the remaining words belong to its service,
	// and must never complete the bucket/peak/integer fields of this parameter.
	bad := append([]byte(nil), w...)
	bad[2], bad[3] = 0, 0
	binary.BigEndian.PutUint16(bad[22:], 1)
	f, err := DecodeRSVPWire(bad, 64)
	require.ErrorIs(t, err, ErrRSVPWireFormat)
	b := rsvpIntservTestParameter(t, f)["Body"].(map[string]any)
	require.Contains(t, b, "Token Bucket Rate")
	require.NotContains(t, b, "Token Bucket Size")
	require.NotContains(t, b, "Maximum Packet Size")
}

func TestRSVPIntservUnknownChildrenFutureVersionAndSharedBudget(t *testing.T) {
	unknown := rsvpIntservTestWords(250, 0, []byte{1, 2, 3, 4})
	body := rsvpIntservTestBody(rsvpIntservTestWords(1, 0, append(unknown, rsvpIntservTestToken()...)))
	f, err := DecodeRSVPWire(rsvpTestMessage(1, rsvpTestObject(12, 2, body)), 4)
	require.NoError(t, err)
	require.False(t, f["Field Layouts Complete"].(bool))
	parameters := rsvpFirstBody(t, f)["Services"].([]map[string]any)[0]["Parameters"].([]map[string]any)
	require.Len(t, parameters, 2)
	require.Equal(t, []byte{1, 2, 3, 4}, parameters[0]["Body"].(map[string]any)["Opaque Parameter Data"])
	require.Contains(t, parameters[1]["Body"], "Token Bucket Rate")
	body[0] = 0x10
	f, err = DecodeRSVPWire(rsvpTestMessage(1, rsvpTestObject(12, 2, body)), 64)
	require.ErrorIs(t, err, ErrObservedWireContext)
	require.NotErrorIs(t, err, ErrRSVPWireFormat)
	require.False(t, f["Field Layouts Complete"].(bool))
	require.NotContains(t, rsvpFirstBody(t, f), "Services")
	require.Contains(t, rsvpFirstBody(t, f), "Opaque Intserv Data")
	binary.BigEndian.PutUint16(body[2:], 0xffff)
	f, err = DecodeRSVPWire(rsvpTestMessage(1, rsvpTestObject(12, 2, body)), 64)
	require.ErrorIs(t, err, ErrObservedWireContext)
	require.NotErrorIs(t, err, ErrRSVPWireFormat, "future format does not select v0 word-count validation")
	require.Equal(t, body[4:], rsvpFirstBody(t, f)["Opaque Intserv Data"])
	body = rsvpIntservTestBody(rsvpIntservTestWords(1, 0, rsvpIntservTestToken()))
	for budget := 0; budget < 3; budget++ {
		_, err = DecodeRSVPWire(rsvpTestMessage(1, rsvpTestObject(12, 2, body)), budget)
		require.ErrorIs(t, err, ErrObservedWireBudget)
	}
	// ADSPEC Controlled-Load service may contain no parameters.
	f, err = DecodeRSVPWire(rsvpTestMessage(1, rsvpTestObject(13, 2, rsvpIntservTestBody(rsvpIntservTestWords(5, 0, nil)))), 2)
	require.NoError(t, err)
	require.True(t, f["Field Layouts Complete"].(bool))
}
