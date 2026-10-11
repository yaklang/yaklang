package pcaputil

import "encoding/hex"

// nmeaAISEnvelope validates the shared bounded AIVDM/AIVDO representation.
func nmeaAISEnvelope(p []string) error {
	if len(p) == 0 {
		return protocolError(ErrMalformedMessage, "AIS envelope is empty")
	}
	if p[0] != "AIVDM" && p[0] != "AIVDO" {
		return protocolError(ErrUnsupportedFeature, "AIS requires AIVDM/AIVDO encapsulation")
	}
	if len(p) != 7 {
		return protocolError(ErrMalformedMessage, "AIS encapsulation requires seven fields")
	}
	if len(p[1]) != 1 || p[1][0] < '1' || p[1][0] > '9' || len(p[2]) != 1 || p[2][0] < '1' || p[2][0] > p[1][0] {
		return protocolError(ErrMalformedMessage, "AIS fragment count/index is invalid")
	}
	if p[3] != "" && (len(p[3]) != 1 || p[3][0] < '0' || p[3][0] > '9') {
		return protocolError(ErrMalformedMessage, "AIS sequence label must be absent or one decimal digit")
	}
	switch p[4] {
	case "", "A", "B", "1", "2":
	default:
		return protocolError(ErrUnsupportedFeature, "AIS channel label is unsupported")
	}
	if len(p[6]) != 1 || p[6][0] < '0' || p[6][0] > '5' || p[5] == "" {
		return protocolError(ErrMalformedMessage, "AIS fill bits or payload is invalid")
	}
	// Validate every armored byte, including unsupported/multipart payloads.
	for _, c := range []byte(p[5]) {
		if !(c >= 48 && c <= 87 || c >= 96 && c <= 119) {
			return protocolError(ErrMalformedMessage, "AIS payload contains invalid six-bit armoring")
		}
	}
	return nil
}

// nmeaAIS observes ITU-R M.1371-5's 168-bit Class A (types1/2/3) and Class B
// (Type18/19) position reports and independent Type24 static report parts.
// Each selected schema validates its own exact payload size.
// A checksum establishes representation, not ship identity or a physical fix.
// Sequence/channel labels are literal; this stateless profile never assembles
// multipart reports or associates observations across UDP datagrams.
func nmeaAIS(p []string, out map[string]any) error {
	if err := nmeaAISEnvelope(p); err != nil {
		return err
	}
	if p[1] != "1" {
		return protocolError(ErrUnsupportedFeature, "AIS multipart assembly is unsupported")
	}
	first := p[5][0] - 48
	if first > 40 {
		first -= 8
	}
	if first == 18 {
		return nmeaAISClassB(p, out)
	}
	if first == 19 {
		return nmeaAISExtendedClassB(p, out)
	}
	if first == 24 {
		return nmeaAISStatic(p, out)
	}
	if first < 1 || first > 3 {
		return protocolError(ErrUnsupportedFeature, "AIS message is outside selected position report profiles")
	}
	if len(p[5]) != 28 || p[6] != "0" {
		return protocolError(ErrMalformedMessage, "AIS Class A report requires 168 bits and zero fill")
	}
	var payload [21]byte
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
	u := func(start, width int) int {
		value := 0
		for i := start; i < start+width; i++ {
			value = value<<1 | int((payload[i/8]>>uint(7-i%8))&1)
		}
		return value
	}
	signed := func(start, width int) int {
		v := u(start, width)
		if v&(1<<uint(width-1)) != 0 {
			v -= 1 << uint(width)
		}
		return v
	}
	out["Report Context"] = "received"
	if p[0] == "AIVDO" {
		out["Report Context"] = "own-ship-reported"
	}
	out["Fragment Count"], out["Fragment Number"] = 1, 1
	out["Sequence ID Present"], out["Sequence ID Wire"] = p[3] != "", p[3]
	out["Radio Channel Present"], out["Radio Channel Wire"] = p[4] != "", p[4]
	out["Armored Payload Wire"], out["Fill Bits"] = p[5], 0
	out["Payload Bit Count"], out["Payload Hex"] = 168, hex.EncodeToString(payload[:])
	out["Message Type"], out["Repeat Indicator"], out["MMSI"] = int(first), u(6, 2), u(8, 30)
	out["Navigational Status"] = u(38, 4)
	rot, sog := signed(42, 8), u(50, 10)
	out["Rate Of Turn Raw"], out["Rate Of Turn Available"] = rot, rot != -128
	out["Rate Of Turn Over Range"] = rot == -127 || rot == 127
	if rot >= -126 && rot <= 126 {
		value := float64(rot*rot) * 1000000 / 22401289 // (raw / 4.733)^2, avoiding 32-bit multiplication overflow.
		if rot < 0 {
			value = -value
		}
		out["Rate Of Turn Degrees Per Minute"] = value
	}
	out["Speed Over Ground Raw"], out["Speed Over Ground Available"] = sog, sog != 1023
	out["Speed Over Ground Lower Bound"] = sog == 1022
	if sog != 1023 {
		out["Speed Knots"] = float64(sog) / 10
	}
	out["Position Accuracy"] = u(60, 1) != 0
	lon, lat := signed(61, 28), signed(89, 27)
	out["Longitude Raw"], out["Latitude Raw"] = lon, lat
	out["Longitude Available"], out["Latitude Available"] = lon != 108600000, lat != 54600000
	out["Position Available"] = lon != 108600000 && lat != 54600000
	if lon != 108600000 {
		if lon < -108000000 || lon > 108000000 {
			return protocolError(ErrMalformedMessage, "AIS longitude is neither in range nor unavailable")
		}
		out["Longitude"] = float64(lon) / 600000
	}
	if lat != 54600000 {
		if lat < -54000000 || lat > 54000000 {
			return protocolError(ErrMalformedMessage, "AIS latitude is neither in range nor unavailable")
		}
		out["Latitude"] = float64(lat) / 600000
	}
	cog, heading := u(116, 12), u(128, 9)
	if cog > 3600 || heading > 359 && heading != 511 {
		return protocolError(ErrMalformedMessage, "AIS course or heading is reserved, not unavailable")
	}
	out["Course Over Ground Raw"], out["Course Over Ground Available"] = cog, cog != 3600
	if cog != 3600 {
		out["Course Degrees True"] = float64(cog) / 10
	}
	out["True Heading Raw"], out["True Heading Available"] = heading, heading != 511
	if heading != 511 {
		out["True Heading Degrees"] = heading
	}
	stamp := u(137, 6)
	out["Time Stamp Raw"], out["UTC Second Available"] = stamp, stamp < 60
	if stamp < 60 {
		out["UTC Second Status"], out["UTC Second"] = "reported", stamp
	} else {
		out["UTC Second Status"] = []string{"unavailable", "manual", "dead-reckoning", "inoperative"}[stamp-60]
	}
	out["Special Maneuver"], out["Spare Bits"], out["RAIM"] = u(143, 2), u(145, 3), u(148, 1) != 0
	out["Communication State Raw"], out["Sync State"] = u(149, 19), u(149, 2)
	if first == 3 {
		out["Communication State Format"] = "ITDMA"
		out["Slot Increment"], out["Slots To Allocate"], out["Keep Flag"] = u(151, 13), u(164, 3), u(167, 1) != 0
	} else {
		out["Communication State Format"] = "SOTDMA"
		timeout := u(151, 3)
		out["Slot Timeout"] = timeout
		switch timeout {
		case 0:
			out["Slot Offset"] = u(154, 14)
		case 1:
			hour, minute := u(154, 5), u(159, 7)
			out["Radio UTC Hour"], out["Radio UTC Minute"], out["Radio UTC Spare"] = hour, minute, u(166, 2)
			out["Radio UTC Values In Range"] = hour < 24 && minute < 60
		case 2, 4, 6:
			out["Slot Number"] = u(154, 14)
		default:
			out["Received Stations"] = u(154, 14)
		}
	}
	return nil
}
