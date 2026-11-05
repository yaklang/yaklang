package pcaputil

// TLS through 1.2 permits same-type messages to be coalesced or split across
// records (RFC5246 6.2.1). The two-octet Alert stream has at most one retained
// octet per direction, copied into the existing fixed-size session storage.
// TLS1.3 has a different explicit record-alignment rule (RFC8446 5.1).
func (t *binTLS) observeAlert(dir int, plain []byte, info map[string]any, max int, authenticated bool) (bool, error) {
	pending := t.alertPending[dir]
	if len(plain) == 0 || t.version == 0x304 && (len(plain) != 2 || pending) {
		return false, protocolError(ErrMalformedMessage, "invalid TLS alert record alignment")
	}
	extra := 0
	if pending {
		extra = 1
	}
	count := (len(plain) + extra) / 2
	if count > max {
		return false, protocolError(ErrResourceExceeded, "TLS alert count limit")
	}
	closed := func(level, description byte, verified bool) bool {
		if verified && (t.version == 0x304 && description != 0 && description != 90 || t.version != 0x304 && level == 2) {
			t.fatalAlert[dir] = true
			info["Authenticated Fatal Alert"] = true
		}
		// TLS 1.3 ignores AlertLevel; earlier close_notify is a warning.
		return verified && description == 0 && (t.version == 0x304 || level == 1) && !t.fatalAlert[0] && !t.fatalAlert[1]
	}
	if !pending && len(plain) == 2 {
		// Preserve the existing field shape for the unchanged single-message
		// case. Fragmented/coalesced messages have an unambiguous ordered list.
		info["Alert Level"], info["Alert Description"] = plain[0], plain[1]
		return closed(plain[0], plain[1], authenticated), nil
	}
	at := 0
	var rows []map[string]any
	closeNotify := false
	if count > 0 {
		rows = make([]map[string]any, 0, count)
	}
	if pending {
		rows = append(rows, map[string]any{"Level": t.alert[dir], "Description": plain[0], "Raw": []byte{t.alert[dir], plain[0]}})
		closeNotify = closed(t.alert[dir], plain[0], authenticated && t.alertAuthenticated[dir])
		at = 1
	}
	for len(plain)-at >= 2 {
		level, description := plain[at], plain[at+1]
		rows = append(rows, map[string]any{"Level": level, "Description": description, "Raw": []byte{level, description}})
		if !closeNotify { // Later alert bytes are ignored after sender closure.
			closeNotify = closed(level, description, authenticated)
		}
		at += 2
	}
	t.alertPending[dir] = at < len(plain)
	if t.alertPending[dir] {
		t.alert[dir] = plain[at]
		t.alertAuthenticated[dir] = authenticated
		info["Alert Buffered Bytes"] = 1
	} else {
		t.alert[dir] = 0
		t.alertAuthenticated[dir] = false
	}
	if len(rows) > 0 {
		info["Alert Messages"] = rows
	}
	if closeNotify {
		// A following alert fragment belongs after closure and cannot remain
		// live or generate a spurious incomplete-alert event at transport EOF.
		t.alert[dir], t.alertPending[dir], t.alertAuthenticated[dir] = 0, false, false
		delete(info, "Alert Buffered Bytes")
	}
	return closeNotify, nil
}
