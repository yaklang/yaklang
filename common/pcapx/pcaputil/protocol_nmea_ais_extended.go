package pcaputil

import (
	"encoding/hex"
	"strings"
)

// nmeaAISExtendedClassB observes a complete Type19 extended Class B report.
// Shapes outside its exact 312-bit admission profile remain UnsupportedFeature,
// preserving historical mutated-message boundaries. Envelope failures stay
// MalformedMessage. The caller validated the full AIVDM/AIVDO envelope.
func nmeaAISExtendedClassB(p []string, out map[string]any) error {
	if len(p[5]) != 52 || p[6] != "0" {
		return protocolError(ErrUnsupportedFeature, "AIS Type19 requires 312 bits and zero fill")
	}
	var payload [39]byte
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
	u := func(at, n int) int {
		v := 0
		for i := at; i < at+n; i++ {
			v = v<<1 | int((payload[i/8]>>uint(7-i%8))&1)
		}
		return v
	}
	signed := func(at, n int) int {
		v := u(at, n)
		if v&(1<<uint(n-1)) != 0 {
			v -= 1 << uint(n)
		}
		return v
	}
	lat, lon := signed(85, 27), signed(57, 28)
	cog, heading := u(112, 12), u(124, 9)
	if lat != 54600000 && (lat < -54000000 || lat > 54000000) || lon != 108600000 && (lon < -108000000 || lon > 108000000) || cog > 3600 || heading > 359 && heading != 511 {
		// Undefined navigation values remain outside the admitted Type19
		// profile, preserving its prior UnsupportedFeature boundary. Defined
		// unavailable sentinels are explicitly admitted above.
		return protocolError(ErrUnsupportedFeature, "AIS Type19 navigation values are outside defined values and sentinels")
	}
	out["Report Context"] = "received"
	if p[0] == "AIVDO" {
		out["Report Context"] = "own-ship-reported"
	}
	out["Fragment Count"], out["Fragment Number"] = 1, 1
	out["Sequence ID Present"], out["Sequence ID Wire"] = p[3] != "", p[3]
	out["Radio Channel Present"], out["Radio Channel Wire"] = p[4] != "", p[4]
	out["Armored Payload Wire"], out["Fill Bits"] = p[5], 0
	out["Payload Bit Count"], out["Payload Hex"] = 312, hex.EncodeToString(payload[:])
	out["Message Type"], out["Repeat Indicator"], out["MMSI"] = 19, u(6, 2), u(8, 30)
	out["Reserved Header Bits"], out["Reserved Regional Bits"] = u(38, 8), u(139, 4)
	sog := u(46, 10)
	out["Speed Over Ground Raw"], out["Speed Over Ground Available"], out["Speed Over Ground Lower Bound"] = sog, sog != 1023, sog == 1022
	if sog != 1023 {
		out["Speed Knots"] = float64(sog) / 10
	}
	out["Position Accuracy"] = u(56, 1) != 0
	out["Longitude Raw"], out["Latitude Raw"] = lon, lat
	out["Longitude Available"], out["Latitude Available"] = lon != 108600000, lat != 54600000
	out["Position Available"] = lon != 108600000 && lat != 54600000
	if lon != 108600000 {
		out["Longitude"] = float64(lon) / 600000
	}
	if lat != 54600000 {
		out["Latitude"] = float64(lat) / 600000
	}
	out["Course Over Ground Raw"], out["Course Over Ground Available"] = cog, cog != 3600
	if cog != 3600 {
		out["Course Degrees True"] = float64(cog) / 10
	}
	out["True Heading Raw"], out["True Heading Available"] = heading, heading != 511
	if heading != 511 {
		out["True Heading Degrees"] = heading
	}
	stamp := u(133, 6)
	out["Time Stamp Raw"], out["UTC Second Available"] = stamp, stamp < 60
	if stamp < 60 {
		out["UTC Second Status"], out["UTC Second"] = "reported", stamp
	} else {
		out["UTC Second Status"] = []string{"unavailable", "manual", "dead-reckoning", "inoperative"}[stamp-60]
	}
	const alphabet = "@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_ !\"#$%&'()*+,-./0123456789:;<=>?"
	var name strings.Builder
	for i := 0; i < 20; i++ {
		name.WriteByte(alphabet[u(143+i*6, 6)])
	}
	wire := name.String()
	v, _, _ := strings.Cut(wire, "@")
	v = strings.TrimRight(v, " ")
	out["Ship Name Wire"], out["Ship Name"], out["Ship Name Available"] = wire, v, v != ""
	out["Ship Type Raw"], out["Position Fix Type Raw"] = u(263, 8), u(301, 4)
	out["RAIM"], out["DTE Not Ready"], out["Assigned Mode Flag"] = u(305, 1) != 0, u(306, 1) != 0, u(307, 1) != 0
	out["Spare Bits Raw"] = u(308, 4)
	for _, d := range []struct {
		name      string
		at, width int
	}{{"Bow", 271, 9}, {"Stern", 280, 9}, {"Port", 289, 6}, {"Starboard", 295, 6}} {
		v := u(d.at, d.width)
		key := "Dimension To " + d.name
		out[key+" Raw"], out[key+" Available"], out[key+" Lower Bound"] = v, v != 0, v == (1<<uint(d.width))-1
		if v != 0 {
			out[key+" Meters"] = v
		}
	}
	return nil
}
