package pcaputil

import "encoding/hex"

// nmeaAISClassB observes M.1371-5 Type18 without inventing receiver or ship
// identity. The caller has already validated the complete AIVDM/AIVDO envelope.
// CS units do not carry the SOTDMA/ITDMA state implied by bit148 alone.
func nmeaAISClassB(p []string, out map[string]any) error {
	if len(p[5]) != 28 || p[6] != "0" {
		return protocolError(ErrMalformedMessage, "AIS Type18 requires 168 bits and zero fill")
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
		// Undefined navigation values remain outside the admitted Type18
		// profile, preserving its prior UnsupportedFeature boundary. Defined
		// unavailable sentinels are explicitly admitted above.
		return protocolError(ErrUnsupportedFeature, "AIS Type18 navigation values are outside defined values and sentinels")
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
	out["Message Type"], out["Repeat Indicator"], out["MMSI"] = 18, u(6, 2), u(8, 30)
	out["Reserved Header Bits"], out["Reserved Regional Bits"] = u(38, 8), u(139, 2)
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
	for _, f := range []struct {
		name string
		at   int
	}{{"CS Unit", 141}, {"Display Flag", 142}, {"DSC Flag", 143}, {"Band Flag", 144}, {"Message 22 Flag", 145}, {"Assigned Mode Flag", 146}, {"RAIM", 147}} {
		out[f.name] = u(f.at, 1) != 0
	}
	state := u(149, 19)
	out["Communication State Raw"], out["Communication State Selector Raw"] = state, u(148, 1)
	if u(141, 1) == 1 {
		out["Communication State Format"] = "CS-fill"
		out["CS Fill Matches Standard"] = state == 393222
		return nil
	}
	out["Sync State"] = u(149, 2)
	if u(148, 1) == 1 {
		out["Communication State Format"] = "ITDMA"
		out["Slot Increment"], out["Slots To Allocate"], out["Keep Flag"] = u(151, 13), u(164, 3), u(167, 1) != 0
		return nil
	}
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
	return nil
}
