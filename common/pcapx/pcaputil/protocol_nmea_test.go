package pcaputil

import (
	"bytes"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func nmeaTestSentence(body string) []byte {
	var c byte
	for _, b := range []byte(body) {
		c ^= b
	}
	hex := "0123456789ABCDEF"
	return append(append([]byte("$"+body+"*"), hex[c>>4], hex[c&15]), '\r', '\n')
}

const nmeaTestGGA = "GPGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,"
const nmeaTestRMC = "GPRMC,123519,A,4807.038,N,01131.000,E,022.4,084.4,230394,003.1,W"

// Independent answers for the standard GGA/RMC examples, not generated from
// parser outputs. Century remains unspecified for the two-digit RMC date.
func TestNMEACompleteGGAAndRMCFields(t *testing.T) {
	for _, tc := range []struct {
		body   string
		fields map[string]any
	}{
		{nmeaTestGGA, map[string]any{"Packet Name": "GGA", "Talker": "GP", "UTC": "123519", "UTC Hour": 12, "UTC Minute": 35, "UTC Second": 19, "Fix Quality": 1, "Satellites Used": 8, "HDOP": 0.9, "Altitude Meters": 545.4, "Geoid Separation Meters": 46.9, "Checksum": 0x47, "Position Valid": true}},
		{nmeaTestRMC, map[string]any{"Packet Name": "RMC", "Talker": "GP", "UTC": "123519", "Status": "A", "Speed Knots": 22.4, "Course Degrees True": 84.4, "Date": "230394", "Date Day": 23, "Date Month": 3, "Date Year Two Digits": 94, "Magnetic Variation Degrees": -3.1, "Magnetic Variation Direction": "W", "Checksum": 0x6a, "Position Valid": true}},
	} {
		w := nmeaTestSentence(tc.body)
		require.True(t, validNMEADatagram(w))
		fields, err := decodeNMEADatagram(w, 2)
		require.NoError(t, err)
		for key, want := range tc.fields {
			require.Equal(t, want, fields[key], key)
		}
		require.InDelta(t, 48.1173, fields["Latitude"], 1e-10)
		require.InDelta(t, 11.516666666666667, fields["Longitude"], 1e-10)
		for i := range w {
			w[i] = 0
		}
		require.Equal(t, "123519", fields["UTC"])
		require.Equal(t, "GP", fields["Talker"])
	}
	fields, err := decodeNMEADatagram(nmeaTestSentence("GNRMC,235960.250,A,9000.000,S,18000.000,W,0,0,290224,,,A"), 1)
	require.NoError(t, err)
	require.Equal(t, -90.0, fields["Latitude"])
	require.Equal(t, -180.0, fields["Longitude"])
	require.Equal(t, 60, fields["UTC Second"])
	require.Equal(t, "250", fields["UTC Fraction"])
}

func TestNMEANegativesAndNoFix(t *testing.T) {
	for _, body := range []string{
		strings.Replace(nmeaTestGGA, "4807.038", "4860.000", 1),
		strings.Replace(nmeaTestGGA, "4807.038", "9000.001", 1),
		strings.Replace(nmeaTestGGA, "01131.000", "18000.001", 1),
		strings.Replace(nmeaTestGGA, ",N,", ",Q,", 1),
		strings.Replace(nmeaTestGGA, "0.9", "NaN", 1),
		strings.Replace(nmeaTestGGA, "545.4", "-.4", 1),
		strings.Replace(nmeaTestGGA, "545.4", "1e20", 1),
		strings.Replace(nmeaTestRMC, "022.4", "+2.0", 1),
		strings.Replace(nmeaTestRMC, "084.4", "360.0", 1),
		strings.Replace(nmeaTestRMC, "230394", "310294", 1),
		strings.Replace(nmeaTestRMC, "123519", "246001", 1),
		"GPGGA,123519,4807.038,N,01131.000,E,1",
		"GPRMC,123519,A,4807.038,N",
		"GPGSV,1,1,04,01,40,083,46,02,17,308,42,03,22,123,45,04,30,234,43",
	} {
		w := nmeaTestSentence(body)
		require.False(t, validNMEADatagram(w), body)
		_, err := decodeNMEADatagram(w, 8)
		require.Error(t, err, body)
	}
	w := nmeaTestSentence(nmeaTestGGA)
	w[len(w)-4] ^= 1
	require.False(t, validNMEADatagram(w))
	require.False(t, validNMEADatagram(nmeaTestSentence(nmeaTestRMC)[:30]))
	require.False(t, validNMEADatagram(append(nmeaTestSentence(nmeaTestGGA), 0)))
	fields, err := decodeNMEADatagram(nmeaTestSentence("GPGGA,,,,,,0,00,,,,,,,"), 1)
	require.NoError(t, err)
	require.Equal(t, false, fields["Position Valid"])
	require.Equal(t, false, fields["Position Present"])
	fields, err = decodeNMEADatagram(nmeaTestSentence("GPRMC,,V,,,,,,,,,"), 1)
	require.NoError(t, err)
	require.Equal(t, false, fields["Position Valid"])
}

func TestNMEAEmptyOptionalMode(t *testing.T) {
	// GPSD release-3.25 accepts a transmitted empty FAA mode. The field
	// supplies no additional status, so the observed A/V flag is preserved.
	for _, body := range []string{nmeaTestRMC + ",", "GPRMC,,V,,,,,,,,,,"} {
		fields, err := decodeNMEADatagram(nmeaTestSentence(body), 2)
		require.NoError(t, err)
		require.Equal(t, "", fields["Mode"])
		require.Equal(t, fields["Status"] == "A", fields["Position Valid"])
	}
	for _, mode := range []string{"AA", "Q", " "} {
		_, err := decodeNMEADatagram(nmeaTestSentence(nmeaTestRMC+","+mode), 2)
		require.Error(t, err, mode)
	}
}

func TestNMEAMultipleSentencesBudgetAndOwnership(t *testing.T) {
	w := append(nmeaTestSentence(nmeaTestGGA), nmeaTestSentence(nmeaTestRMC)...)
	fields, err := decodeNMEADatagram(w, 2)
	require.NoError(t, err)
	require.Equal(t, 2, fields["Message Count"])
	_, err = decodeNMEADatagram(w, 1)
	require.Error(t, err)
	_, err = decodeNMEADatagram(w, 0)
	require.Error(t, err)
	for i := range w {
		w[i] = 0
	}
	ms := fields["Messages"].([]map[string]any)
	require.Equal(t, "GGA", ms[0]["Packet Name"])
	require.Equal(t, "RMC", ms[1]["Packet Name"])
	for _, term := range []string{"nan", "Inf", "-Inf", "1e99", "+1", ".1", "-.1", "1.", "1..0", "--1"} {
		_, err = nmeaNumber(term, true)
		require.Error(t, err, term)
	}
}

func TestNMEANativeDatagramIngressAndBoundary(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		w := nmeaTestSentence(nmeaTestGGA)
		pcap := t20UDPCapture(t, net.ParseIP("192.0.2.1"), net.ParseIP("192.0.2.2"), 41707, 36198, w)
		var es []*ProtocolEvent
		var stats ProtocolStats
		require.NoError(t, ReplayPcap(bytes.NewReader(pcap), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
		require.Zero(t, stats.BufferedBytes)
		var matches []*ProtocolEvent
		for _, e := range es {
			if e.Protocol == "nmea" && (e.Status == "decoded" || e.Status == "deferred") {
				matches = append(matches, e)
			}
		}
		require.Len(t, matches, 1, "complete nonstandard-port NMEA must decode")
		d, err := matches[0].Decode()
		require.NoError(t, err)
		f := d["fields"].(map[string]any)
		require.Equal(t, 1, f["Fix Quality"])
		require.InDelta(t, 48.1173, f["Latitude"], 1e-10)
		for i := range pcap {
			pcap[i] = 0
		}
		require.Equal(t, "123519", f["UTC"])
		partial := t20UDPCapture(t, net.ParseIP("192.0.2.1"), net.ParseIP("192.0.2.2"), 41707, 10110, w[:20], w[20:])
		es = nil
		require.NoError(t, ReplayPcap(bytes.NewReader(partial), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) })))
		for _, e := range es {
			require.NotEqual(t, "nmea", e.Protocol, "independent UDP datagrams must not be concatenated")
		}
	}
}
