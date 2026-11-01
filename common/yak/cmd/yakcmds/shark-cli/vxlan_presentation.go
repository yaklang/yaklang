package sharkcli

import (
	"encoding/json"
	"strconv"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func gpeHeaderInteger(v any, max uint64) (uint64, bool) {
	n := number(v)
	if n > max {
		return 0, false
	}
	switch value := v.(type) {
	case uint8, uint16, uint32, uint64:
		return n, true
	case int:
		return n, value >= 0
	case int64:
		return n, value >= 0
	case float64:
		return n, value == float64(n)
	case json.Number:
		parsed, err := strconv.ParseUint(string(value), 10, 64)
		return parsed, err == nil && parsed <= max
	}
	return 0, false
}

func validVXLANGPEHeaderObservation(e *pcaputil.ProtocolEvent) bool {
	if e.Protocol != "ip" || e.Profile != "network-encapsulation-refusal" || e.Status != "limited" || e.Completeness != "headers" || e.ExpertCode != "VXLANProfileUnselected" || e.Session["Unselected Carrier"] != "VXLAN-GPE" || e.Session["Child Header Identity"] != "unconfirmed" || e.Session["Payload Profile Implemented"] != false {
		return false
	}
	for name, max := range map[string]uint64{"VXLAN-GPE Version": 0, "VXLAN-GPE Flags": 255, "VXLAN-GPE VNI": 0xffffff, "VXLAN-GPE Next Protocol": 255} {
		if _, valid := gpeHeaderInteger(e.Session[name], max); !valid {
			return false
		}
	}
	flags, next := number(e.Session["VXLAN-GPE Flags"]), number(e.Session["VXLAN-GPE Next Protocol"])
	return number(e.Session["VXLAN-GPE Version"]) == 0 && flags <= 255 && flags&0x08 != 0 && flags&0x30 == 0 && number(e.Session["VXLAN-GPE VNI"]) <= 0xffffff && next <= 255 && e.Session["VXLAN-GPE Next Protocol Present"] == (flags&4 != 0) && (flags&4 == 0 && next == 0 || flags&4 != 0 && next != 0)
}
