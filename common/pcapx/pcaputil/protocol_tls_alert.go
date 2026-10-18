package pcaputil

// TLS through 1.2 permits same-type messages to be coalesced or split across
// records (RFC5246 6.2.1). The two-octet Alert stream has at most one retained
// octet per direction, copied into the existing fixed-size session storage.
// TLS1.3 has a different explicit record-alignment rule (RFC8446 5.1).
func (t *binTLS) observeAlert(dir int, plain []byte, info map[string]any, max int) error {
	pending := t.alertPending[dir]
	if len(plain) == 0 || t.version == 0x304 && (len(plain) != 2 || pending) {
		return protocolError(ErrMalformedMessage, "invalid TLS alert record alignment")
	}
	extra := 0
	if pending {
		extra = 1
	}
	count := (len(plain) + extra) / 2
	if count > max {
		return protocolError(ErrResourceExceeded, "TLS alert count limit")
	}
	if !pending && len(plain) == 2 {
		// Preserve the existing field shape for the unchanged single-message
		// case. Fragmented/coalesced messages have an unambiguous ordered list.
		info["Alert Level"], info["Alert Description"] = plain[0], plain[1]
		return nil
	}
	at := 0
	var rows []map[string]any
	if count > 0 {
		rows = make([]map[string]any, 0, count)
	}
	if pending {
		rows = append(rows, map[string]any{"Level": t.alert[dir], "Description": plain[0], "Raw": []byte{t.alert[dir], plain[0]}})
		at = 1
	}
	for len(plain)-at >= 2 {
		level, description := plain[at], plain[at+1]
		rows = append(rows, map[string]any{"Level": level, "Description": description, "Raw": []byte{level, description}})
		at += 2
	}
	t.alertPending[dir] = at < len(plain)
	if t.alertPending[dir] {
		t.alert[dir] = plain[at]
		info["Alert Buffered Bytes"] = 1
	} else {
		t.alert[dir] = 0
	}
	if len(rows) > 0 {
		info["Alert Messages"] = rows
	}
	return nil
}
