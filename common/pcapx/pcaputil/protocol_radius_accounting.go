package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// RFC2866 sections4/5 and RFC2869 sections2.1/5.1/5.2. These are observed
// unsigned counter values, not a verified accounting or billing decision.
var radiusAccountingUint32 = map[byte]string{
	40: "Acct-Status-Type", 41: "Acct-Delay-Time", 42: "Acct-Input-Octets", 43: "Acct-Output-Octets",
	45: "Acct-Authentic", 46: "Acct-Session-Time", 47: "Acct-Input-Packets", 48: "Acct-Output-Packets",
	49: "Acct-Terminate-Cause", 52: "Acct-Input-Gigawords", 53: "Acct-Output-Gigawords",
}

func radiusAccountingAttribute(out map[string]any, seen map[byte]bool, typ byte, v []byte) error {
	switch typ {
	case 2, 3, 18, 24, 60:
		return fmt.Errorf("radius: attribute %d forbidden in Accounting-Request", typ)
	}
	name, numeric := radiusAccountingUint32[typ]
	if !numeric && typ != 44 {
		return nil
	}
	if seen[typ] {
		return fmt.Errorf("radius: repeated singleton attribute %d", typ)
	}
	seen[typ] = true
	if typ == 44 {
		if len(v) == 0 {
			return fmt.Errorf("radius: empty Acct-Session-Id")
		}
		out["Acct-Session-Id"] = bytes.Clone(v)
		return nil
	}
	if len(v) != 4 {
		return fmt.Errorf("radius: %s width", name)
	}
	out[name] = binary.BigEndian.Uint32(v)
	return nil
}
func radiusAccountingComplete(out map[string]any, seen map[byte]bool) error {
	if !seen[40] || !seen[44] || !seen[4] && !seen[32] {
		return fmt.Errorf("radius: Accounting-Request requires status, session ID and NAS identity")
	}
	status := out["Acct-Status-Type"].(uint32)
	for _, typ := range []byte{42, 43, 46, 47, 48, 52, 53} {
		if seen[typ] && status != 2 && status != 3 {
			return fmt.Errorf("radius: %s requires Stop or Interim-Update", radiusAccountingUint32[typ])
		}
	}
	if seen[49] && status != 2 {
		return fmt.Errorf("radius: Acct-Terminate-Cause requires Stop")
	}
	// Do not invent an absent high/low half or round an integer through float64.
	for _, v := range []struct {
		low, high byte
		name      string
	}{{42, 52, "Acct-Input-Octets64"}, {43, 53, "Acct-Output-Octets64"}} {
		if seen[v.low] && seen[v.high] {
			out[v.name] = uint64(out[radiusAccountingUint32[v.high]].(uint32))<<32 | uint64(out[radiusAccountingUint32[v.low]].(uint32))
		}
	}
	return nil
}
