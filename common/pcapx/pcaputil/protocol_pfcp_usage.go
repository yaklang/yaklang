package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Selected finite observation limits, not PFCP wire maxima. All report child
// collections in one datagram share the caller's aggregate allowance.
const pfcpUsageChildren = 64
const pfcpUsageNodes = 256
const pfcpUsageBytes = 4096

// TS29.244 v16.12.1, Table7.5.7.2-1 and sections8.2.41/52/53/54/71.
// This validates only the selected child requirements assuming their condition
// applies. A captured conditional group cannot establish provisioned URRs or
// available measurements; the parent observation keeps that distinction explicit.
func decodePFCPUsage(v []byte, childLimit, byteLimit int) (map[string]any, error) {
	bad := func(why string) (map[string]any, error) { return nil, discoveryError(ErrMalformedMessage, why) }
	if len(v) > min(byteLimit, pfcpUsageBytes) {
		return nil, discoveryError(ErrResourceExceeded, "Usage group exceeds selected byte budget")
	}
	out := map[string]any{"raw_hex": hex.EncodeToString(v), "ies": []map[string]any{}, "urr_id": nil, "ur_sequence": nil, "trigger": nil, "start_time": nil, "end_time": nil, "actual_usage_verified": false, "requires_qualified_conditional_context": true}
	type entry struct {
		value  []byte
		fields map[string]any
	}
	first := map[uint16]entry{}
	seen := map[uint16]bool{}
	items := []map[string]any{}
	for at := 0; at < len(v); {
		if len(v)-at < 4 {
			return bad("usage child IE header incomplete")
		}
		t, n := binary.BigEndian.Uint16(v[at:]), int(binary.BigEndian.Uint16(v[at+2:]))
		at += 4
		if n > len(v)-at {
			return bad("usage child IE overruns group boundary")
		}
		if len(items) >= min(childLimit, pfcpUsageChildren) {
			return nil, discoveryError(ErrResourceExceeded, "Usage child IE collection budget")
		}
		w := v[at : at+n]
		at += n
		f := map[string]any{"type": t, "length": n, "value_hex": hex.EncodeToString(w), "duplicate": seen[t]}
		items = append(items, f)
		if seen[t] {
			f["handling"] = "ignored repetition (first wins)"
			continue
		}
		seen[t] = true
		switch t {
		case 81, 104, 63, 75, 76:
			first[t] = entry{w, f}
		default:
			f["handling"] = "raw extension not interpreted in minimal subfield oracle"
		}
	}
	out["ies"] = items
	for _, t := range []uint16{81, 104, 63} {
		if _, ok := first[t]; !ok {
			return bad(fmt.Sprintf("qualified UsageReport missing mandatory child %d", t))
		}
	}
	e := first[81]
	if len(e.value) < 4 {
		return bad("URRID fixed prefix incomplete")
	}
	out["urr_id"] = map[string]any{"unsigned32_raw": binary.BigEndian.Uint32(e.value), "predefined_in_up": e.value[0]&128 != 0, "extension_hex": hex.EncodeToString(e.value[4:])}
	e.fields["handling"] = "handled first"
	e = first[104]
	if len(e.value) != 4 {
		return bad("URSEQN fixed length must4")
	}
	out["ur_sequence"] = binary.BigEndian.Uint32(e.value)
	e.fields["handling"] = "handled first"
	e = first[63]
	w := e.value
	if len(w) < 2 {
		return bad("UsageTrigger minimum fixed prefix2")
	}
	names := []string{"PERIO", "VOLTH", "TIMTH", "QUHTI", "START", "STOPT", "DROTH", "IMMER", "VOLQU", "TIMQU", "LIUSA", "TERMR", "MONIT", "ENVCL", "MACAR", "EVETH", "EVEQU", "TEBUR", "IPMJL", "QUVTI", "EMRRE"}
	flags := map[string]any{}
	anyKnown := false
	for i, name := range names {
		on := i/8 < len(w) && w[i/8]&(1<<uint(i%8)) != 0
		flags[name] = on
		anyKnown = anyKnown || on
	}
	if !anyKnown {
		for _, b := range w[min(3, len(w)):] {
			if b != 0 {
				return nil, discoveryError(ErrUnsupportedFeature, "unknown-only future trigger extension; no verified known trigger meaning")
			}
		}
		return bad("no known report trigger set; zero/spare-only trigger invalid after ignoring sparebits")
	}
	var spare any
	if len(w) > 2 {
		spare = w[2] & 224
	}
	out["trigger"] = map[string]any{"flags": flags, "raw_hex": hex.EncodeToString(w), "third_spare_bits_raw": spare, "extension_hex": hex.EncodeToString(w[min(3, len(w)):])}
	e.fields["handling"] = "handled first"
	required := !(flags["START"].(bool) || flags["STOPT"].(bool) || flags["MACAR"].(bool))
	out["start_end_required_for_trigger"] = required
	for _, timeField := range []struct {
		typ  uint16
		name string
	}{{75, "start_time"}, {76, "end_time"}} {
		e, ok := first[timeField.typ]
		if !ok {
			if required {
				return bad(fmt.Sprintf("qualified UsageReport missing verifiable time child %d", timeField.typ))
			}
			continue
		}
		if len(e.value) < 4 {
			if required {
				return bad("required timestamp fixed prefix incomplete")
			}
			e.fields["handling"] = "ignored invalid non-required time"
			continue
		}
		out[timeField.name] = map[string]any{"ntp_seconds_1900": binary.BigEndian.Uint32(e.value), "extension_hex": hex.EncodeToString(e.value[4:])}
		e.fields["handling"] = "observed first UTC seconds; no era inference"
	}
	return out, nil
}

func pfcpUsageObservations(out map[string]any, limit int) error {
	groups := out["opaque_groups"].([]map[string]any)
	observations := []map[string]any{}
	remaining := min(limit, pfcpUsageNodes)
	accepted := out["cause"].(map[string]any)["accepted"].(bool)
	for _, g := range groups {
		if g["type"].(uint16) != 79 {
			continue
		}
		if limit < 35 {
			return discoveryError(ErrResourceExceeded, "Usage observation field map exceeds collection budget")
		}
		raw := g["raw_hex"].(string)
		o := map[string]any{"repeat_index": g["repeat_index"], "raw_hex": raw, "qualified_selected_children": false, "report_semantics_verified": false, "urr_configuration_verified": false, "actual_usage_verified": false, "selected_fields": nil, "qualified_error": nil}
		if !accepted {
			o["handling"] = "ignored non-required rejection IE; raw only"
			o["conditional_status"] = "not applicable on rejection"
		} else {
			w, err := hex.DecodeString(raw)
			if err != nil {
				return err
			}
			count, err := pfcpUsageChildAdmission(w, remaining)
			if err != nil {
				return err
			}
			remaining -= count
			fields, err := decodePFCPUsage(w, min(limit, pfcpUsageChildren), pfcpUsageBytes)
			o["handling"] = "selected scalar observation; other conditional measurements not verified"
			o["conditional_status"] = "URR provisioning/availability context absent; no mandatory-receiver judgement inferred"
			if err != nil {
				var pe *ProtocolError
				if !errors.As(err, &pe) {
					return err
				}
				if pe.Kind == ErrResourceExceeded {
					return err
				}
				// Diagnostic of the selected conditional syntax, not a forged parent
				// rejection or operational success. No partial field tree is published.
				o["qualified_error"] = map[string]any{"code": string(pe.Kind), "detail": pfcpUsageDetail(err)}
			} else {
				o["selected_fields"] = fields
			}
		}
		observations = append(observations, o)
	}
	if len(observations) > 0 {
		out["usage_report_observations"] = observations
	}
	return nil
}

func pfcpUsageDetail(err error) string {
	var pe *ProtocolError
	if errors.As(err, &pe) {
		return strings.TrimPrefix(pe.Message, "discovery: ")
	}
	return err.Error()
}

// Count every visited child, including unknown/repeated or semantically invalid
// values, before allocating its field tree. Invalid conditional syntax remains
// a qualified diagnostic, but cannot reset the aggregate resource allowance.
func pfcpUsageChildAdmission(v []byte, remaining int) (int, error) {
	if len(v) > pfcpUsageBytes {
		return 0, discoveryError(ErrResourceExceeded, "Usage group exceeds selected byte budget")
	}
	count := 0
	for at := 0; len(v)-at >= 4; {
		n := int(binary.BigEndian.Uint16(v[at+2:]))
		at += 4
		if n > len(v)-at {
			break
		}
		if count >= min(remaining, pfcpUsageChildren) {
			return 0, discoveryError(ErrResourceExceeded, "Usage child IE collection budget")
		}
		count++
		at += n
	}
	return count, nil
}

// Bounded header-only scan used for pre-allocation projection/depth admission.
// Full top-level and child boundaries are still checked by the decoders.
func pfcpHasUsage(w []byte) bool {
	if len(w) < 16 || w[1] != 55 || w[0]&1 == 0 {
		return false
	}
	for at := 16; len(w)-at >= 4; {
		t, n := binary.BigEndian.Uint16(w[at:]), int(binary.BigEndian.Uint16(w[at+2:]))
		at += 4
		if t == 79 {
			return true
		}
		if n > len(w)-at {
			return false
		}
		at += n
	}
	return false
}
