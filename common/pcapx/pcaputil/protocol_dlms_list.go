package pcaputil

import "errors"

// Locate only the bounded HDLC/LLC header. The semantic decoder still validates
// the exact frame, addresses, CRCs, direction and APDU before admitting fields.
// This walk selects a reservation before a repeated Data description expands.
func dlmsAPDU(w []byte) []byte {
	if len(w) < 12 || w[0] != 0x7e {
		return nil
	}
	end := len(w) - 3
	at := 3
	if _, _, err := dlmsAddress(w[:end], &at); err != nil {
		return nil
	}
	if _, _, err := dlmsAddress(w[:end], &at); err != nil {
		return nil
	}
	if at >= end || w[at]&1 != 0 || end-at < 8 {
		return nil
	}
	p := w[at+6 : end] // control, HCS(2), LLC(3)
	if len(p) < 2 || p[0] != 0xc0 && p[0] != 0xc4 {
		return nil
	}
	return p
}
func dlmsListAPDU(w []byte) []byte {
	p := dlmsAPDU(w)
	if len(p) >= 2 && p[1] == 3 {
		return p
	}
	return nil
}
func dlmsNormalExtended(p []byte) bool {
	return len(p) > 4 && p[0] == 0xc4 && p[1] == 1 && p[3] == 0 &&
		(p[4] == 1 || p[4] == 2 || wrapperExtendedScalarTag(p[4]))
}
func dlmsProfile(w []byte) string {
	if dlmsListAPDU(w) != nil {
		return "dlms-hdlc-get-list"
	}
	return "dlms-hdlc-get-normal"
}
func dlmsProjection(w []byte) int64 {
	if p := dlmsListAPDU(w); p != nil {
		// One complete APDU has at most 64 list records and 256 Data/schema/
		// expanded maps. Do not infer node counts from compact payload tag bytes.
		nodes := min(wrapperDataNodes, max(0, len(p)-4))
		if wrapperListContainsCompact(p) {
			nodes = wrapperDataNodes
		}
		return 32768 + 512*int64(len(w)) + 2048*int64(nodes) + 8192*int64(min(wrapperListItems, max(0, len(p)-4)/2))
	}
	if p := dlmsAPDU(w); dlmsNormalExtended(p) {
		// Every ordinary descendant needs a tag byte. Compact rows reuse their
		// description, so reserve the entire bounded expansion pool when found.
		nodes := min(wrapperDataNodes, len(p)-4)
		probe := wrapperCompactProbe{wire: p, at: 4}
		if compact, _ := probe.data(0); compact {
			nodes = wrapperDataNodes
		}
		return 32768 + 512*int64(len(w)) + 2048*int64(nodes)
	}
	return int64(len(w)) * 6
}
func dlmsNormalData(p []byte, limit, depth int) (map[string]any, error) {
	c := wrapperListCursor{wire: p, at: 4, maxDepth: depth}
	data, err := c.data(limit, 0)
	if err == nil && c.at != len(p) {
		err = dlmsError(ErrMalformedMessage, "Get-normal Data has trailing bytes")
	}
	var pe *ProtocolError
	if errors.As(err, &pe) {
		return nil, dlmsError(pe.Kind, "Get-normal Data: "+pe.Message)
	}
	return data, err
}
func dlmsListFields(m *dlmsMessage, p []byte, limit, depth int) error {
	list := &wrapperMessage{fields: make(map[string]any), request: m.request, choice: 3}
	if err := decodeWrapperList(list, p, limit, depth); err != nil {
		var pe *ProtocolError
		if errors.As(err, &pe) {
			return dlmsError(pe.Kind, "Get-list: "+pe.Message)
		}
		return err
	}
	m.count = list.count
	m.fields["Get List"] = list.fields
	if m.request {
		m.fields["Frame Kind"] = "Get Request With List"
	} else {
		m.fields["Frame Kind"] = "Get Response With List"
	}
	return nil
}
