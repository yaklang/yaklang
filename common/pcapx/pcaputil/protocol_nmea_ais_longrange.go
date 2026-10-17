package pcaputil

import "encoding/hex"

// nmeaAISLongRange observes the 96-bit Type27 schema in ITU-R M.1371-5,
// Annex8 Table84. Bit94 is position latency, not GNSS validity. Its coordinate
// units are 1/10 minute; the Class A/B high-resolution scale does not apply.
// Reserved wire values are retained explicitly without claiming conformance.
func nmeaAISLongRange(p []string, out map[string]any) error {
	if len(p[5]) != 16 || p[6] != "0" {
		return protocolError(ErrUnsupportedFeature, "AIS Type27 requires 96 bits and zero fill")
	}
	var payload [12]byte
	for i, c := range []byte(p[5]) {
		v := c - 48
		if v > 40 {
			v -= 8
		}
		for j := 0; j < 6; j++ {
			bit := i*6 + j
			payload[bit/8] |= ((v >> uint(5-j)) & 1) << uint(7-bit%8)
		}
	}
	u := func(at, width int) int {
		v := 0
		for i := at; i < at+width; i++ {
			v = v<<1 | int((payload[i/8]>>uint(7-i%8))&1)
		}
		return v
	}
	signed := func(at, width int) int {
		v := u(at, width)
		if v&(1<<uint(width-1)) != 0 {
			v -= 1 << uint(width)
		}
		return v
	}
	lon, lat, cog := signed(44, 18), signed(62, 17), u(85, 9)
	if lon != 108600 && (lon < -108000 || lon > 108000) || lat != 54600 && (lat < -54000 || lat > 54000) || cog > 359 && cog != 511 {
		return protocolError(ErrMalformedMessage, "AIS Type27 position/course is neither defined nor unavailable")
	}
	out["Report Context"] = "received"
	if p[0] == "AIVDO" {
		out["Report Context"] = "own-ship-reported"
	}
	out["Fragment Count"], out["Fragment Number"] = 1, 1
	out["Sequence ID Present"], out["Sequence ID Wire"] = p[3] != "", p[3]
	out["Radio Channel Present"], out["Radio Channel Wire"] = p[4] != "", p[4]
	out["Armored Payload Wire"], out["Fill Bits"] = p[5], 0
	out["Payload Bit Count"], out["Payload Hex"] = 96, hex.EncodeToString(payload[:])
	out["Message Type"], out["Repeat Indicator"], out["MMSI"] = 27, u(6, 2), u(8, 30)
	out["Repeat Indicator Conforms"] = u(6, 2) == 3
	out["Position Accuracy"], out["RAIM"], out["Navigational Status"] = u(38, 1) != 0, u(39, 1) != 0, u(40, 4)
	out["Longitude Raw"], out["Latitude Raw"] = lon, lat
	out["Longitude Available"], out["Latitude Available"] = lon != 108600, lat != 54600
	out["Position Available"] = lon != 108600 && lat != 54600
	if lon != 108600 {
		out["Longitude"] = float64(lon) / 600
	}
	if lat != 54600 {
		out["Latitude"] = float64(lat) / 600
	}
	sog := u(79, 6)
	out["Speed Over Ground Raw"], out["Speed Over Ground Available"] = sog, sog != 63
	if sog != 63 {
		out["Speed Knots"] = sog // Table84 defines 0..62; no saturation flag.
	}
	out["Course Over Ground Raw"], out["Course Over Ground Available"] = cog, cog != 511
	if cog != 511 {
		out["Course Degrees True"] = cog
	}
	latency := u(94, 1)
	out["Position Latency Raw"] = latency
	out["Position Latency Status"] = "less-than-5-seconds"
	if latency != 0 {
		out["Position Latency Status"] = "greater-than-5-seconds"
	}
	out["Spare Bits Raw"], out["Spare Bits Conform"] = u(95, 1), u(95, 1) == 0
	return nil
}
