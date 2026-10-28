package pcaputil

import (
	"encoding/hex"
	"strings"
)

// Type24 parts are independent reports, not NMEA multipart fragments. Observe
// each literal Part Number without rewriting a short PartB as PartA or joining
// parts merely because their reported MMSIs match. Both 160/168-bit Part A forms
// are observed; Part B requires 168 bits. The caller validated the envelope.
func nmeaAISStatic(p []string, out map[string]any) error {
	n := len(p[5])*6 - int(p[6][0]-'0')
	if n != 160 && n != 168 || len(p[5]) > 28 {
		return protocolError(ErrMalformedMessage, "AIS Type24 requires 160 or 168 payload bits")
	}
	var payload [21]byte
	for i, c := range []byte(p[5]) {
		v := c - 48
		if v > 40 {
			v -= 8
		}
		for j := 0; j < 6 && i*6+j < n; j++ {
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
	part := u(38, 2)
	if part > 1 {
		return protocolError(ErrUnsupportedFeature, "AIS Type24 part is undefined")
	}
	if part == 1 && n != 168 {
		return protocolError(ErrMalformedMessage, "AIS Type24 Part B is truncated; Part Number cannot be rewritten")
	}
	out["Report Context"] = "received"
	if p[0] == "AIVDO" {
		out["Report Context"] = "own-ship-reported"
	}
	out["Fragment Count"], out["Fragment Number"] = 1, 1
	out["Sequence ID Present"], out["Sequence ID Wire"] = p[3] != "", p[3]
	out["Radio Channel Present"], out["Radio Channel Wire"] = p[4] != "", p[4]
	out["Armored Payload Wire"], out["Fill Bits"] = p[5], int(p[6][0]-'0')
	out["Payload Bit Count"], out["Payload Hex"] = n, hex.EncodeToString(payload[:(n+7)/8])
	out["Message Type"], out["Repeat Indicator"], out["MMSI"] = 24, u(6, 2), u(8, 30)
	out["Part Number"], out["Association Scope"] = part, "individual-report"
	fill := p[5][len(p[5])-1] - 48
	if fill > 40 {
		fill -= 8
	}
	out["Armoring Fill Value"] = int(fill) & ((1 << uint(p[6][0]-'0')) - 1)
	text := func(at, count int) string {
		const alphabet = "@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_ !\"#$%&'()*+,-./0123456789:;<=>?"
		var b strings.Builder
		for i := 0; i < count; i++ {
			b.WriteByte(alphabet[u(at+i*6, 6)])
		}
		return b.String()
	}
	addText := func(key, wire string) {
		v, _, _ := strings.Cut(wire, "@")
		v = strings.TrimRight(v, " ")
		out[key+" Wire"], out[key], out[key+" Available"] = wire, v, v != ""
	}
	if part == 0 {
		addText("Ship Name", text(40, 20))
		out["Spare Bits Present"] = n == 168
		if n == 168 {
			out["Spare Bits Raw"] = u(160, 8)
		}
		return nil
	}
	out["Ship Type Raw"] = u(40, 8)
	// M.1371-4/5 split the former seven-character vendor field into a
	// three-character manufacturer plus numeric model/serial. Preserve both
	// representations, without inferring the equipment's actual version.
	out["Vendor Legacy Text Wire"] = text(48, 7)
	addText("Manufacturer", text(48, 3))
	out["Unit Model Code Raw"], out["Serial Number Raw"] = u(66, 4), u(70, 20)
	addText("Call Sign", text(90, 7))
	out["Spare Bits Present"], out["Spare Bits Raw"] = true, u(162, 6)
	aux := u(8, 30)/10000000 == 98
	out["Auxiliary MMSI Prefix"] = aux
	if aux {
		out["Dimensions Applicable"] = false
		out["Mothership MMSI Raw"], out["Mothership MMSI Nonzero"] = u(132, 30), u(132, 30) != 0
		return nil
	}
	out["Dimensions Applicable"] = true
	for _, d := range []struct {
		name      string
		at, width int
	}{{"Bow", 132, 9}, {"Stern", 141, 9}, {"Port", 150, 6}, {"Starboard", 156, 6}} {
		v := u(d.at, d.width)
		key := "Dimension To " + d.name
		out[key+" Raw"], out[key+" Available"], out[key+" Lower Bound"] = v, v != 0, v == (1<<uint(d.width))-1
		if v != 0 {
			out[key+" Meters"] = v
		}
	}
	return nil
}
