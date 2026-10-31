package pcaputil

func smb1DisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	add := func(name, key, owner string, typ DisplayFieldType, bits int) {
		b := displayRoot("smb", name, key, typ, bits, "pdu", key)
		if owner != "" {
			b.path = []string{owner, key}
			b.rangeFor = func(e *ProtocolEvent, ordinal int) *DisplayFieldRange {
				m, _ := e.Session[owner].(map[string]any)
				return (&ProtocolEvent{Raw: e.Raw, Session: m}).displayPDUFieldRange(key)
			}
		}
		out = append(out, b)
	}
	for _, f := range []struct {
		name, key string
		bits      int
	}{
		{"smb.cmd", "Command", 8}, {"smb.status_raw", "Status", 32},
		{"smb.flags", "Flags", 8}, {"smb.flags2", "Flags 2", 16},
		{"smb.tid", "Tree ID", 16}, {"smb.uid", "User ID", 16},
		{"smb.mid", "Multiplex ID", 16},
		{"smb.pid.high", "PID High", 16}, {"smb.pid.low", "Process ID Low", 16},
	} {
		add(f.name, f.key, "Header", DisplayUnsigned, f.bits)
	}
	// MS-CIFS 2.2.3.1: in a request NT_STATUS asks for the response error
	// format; it does not turn the request's raw Status into a server error.
	// A response uses NTSTATUS only when both REPLY and NT_STATUS are set.
	// Header aliases do not establish request association or authentication.
	// https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-cifs/69a29f73-de0c-45a6-a1aa-8ceeea42217f
	ntStatus := displayCustom("smb", "smb.nt_status", "Response NTSTATUS", DisplayUnsigned, 32, "pdu", func(e *ProtocolEvent, emit func(any), _ *displayProjection) {
		flags, flagsOK := smb1DisplayHeaderUint(e, "Flags", 8)
		flags2, flags2OK := smb1DisplayHeaderUint(e, "Flags 2", 16)
		if flagsOK && flags2OK && flags&0x80 != 0 && flags2&0x4000 != 0 {
			if status, ok := smb1DisplayHeaderUint(e, "Status", 32); ok {
				emit(status)
			}
		}
	})
	ntStatus.descriptor.Description = "Literal server response Status with REPLY and NT_STATUS set; request format preferences and SMBSTATUS bytes are not NTSTATUS errors"
	ntStatus.rangeFor = func(e *ProtocolEvent, _ int) *DisplayFieldRange {
		header, _ := e.Session["Header"].(map[string]any)
		return (&ProtocolEvent{Raw: e.Raw, Session: header}).displayPDUFieldRange("Status")
	}
	out = append(out, ntStatus)
	requested := displayCustom("smb", "smb.nt_status_requested", "NTSTATUS response format requested", DisplayBoolean, 0, "derived", func(e *ProtocolEvent, emit func(any), _ *displayProjection) {
		flags, flagsOK := smb1DisplayHeaderUint(e, "Flags", 8)
		flags2, flags2OK := smb1DisplayHeaderUint(e, "Flags 2", 16)
		if flagsOK && flags2OK && flags&0x80 == 0 {
			emit(flags2&0x4000 != 0)
		}
	})
	requested.descriptor.Description = "Client request NT_STATUS preference from Flags2; not a server error or evidence that a response was observed"
	out = append(out, requested)
	pid := displayCustom("smb", "smb.pid", "Process ID (high and low halves)", DisplayUnsigned, 32, "derived", func(e *ProtocolEvent, emit func(any), _ *displayProjection) {
		high, highOK := smb1DisplayHeaderUint(e, "PID High", 16)
		low, lowOK := smb1DisplayHeaderUint(e, "Process ID Low", 16)
		if highOK && lowOK {
			emit(high<<16 | low)
		}
	})
	// The two literal uint16 fields are not contiguous in an SMB header.
	// Their combined opaque identifier cannot claim a single source range.
	pid.descriptor.Description = "32-bit opaque process identifier composed from observed PIDHigh and PIDLow; literal halves retain their separate PDU ranges"
	out = append(out, pid)
	for _, field := range []struct {
		name, key string
		bits      int
	}{{"smb.wct", "Word Count", 8}, {"smb.bcc", "Byte Count", 16}} {
		field := field
		binding := displayCustom("smb", field.name, field.key, DisplayUnsigned, field.bits, "pdu", func(e *ProtocolEvent, emit func(any), _ *displayProjection) {
			if value, ok := smb1WordCountOwner(e, field.key)[field.key]; ok {
				emit(value)
			}
		})
		binding.rangeFor = func(e *ProtocolEvent, _ int) *DisplayFieldRange {
			return (&ProtocolEvent{Raw: e.Raw, Session: smb1WordCountOwner(e, field.key)}).displayPDUFieldRange(field.key)
		}
		out = append(out, binding)
	}
	for _, field := range []struct {
		name, key string
		bits      int
	}{
		{"smb.dialect.index", "Dialect Index", 16}, {"smb.security_mode", "Security Mode", 8},
		{"smb.max_mpx_count", "Max Mpx Count", 16}, {"smb.max_vcs", "Max Number Vcs", 16},
		{"smb.max_buf", "Max Buffer Size", 32}, {"smb.max_raw", "Max Raw Size", 32},
		{"smb.session_key", "Session Key", 32}, {"smb.capabilities", "Capabilities", 32},
		{"smb.system_time", "System Time", 64}, {"smb.challenge_length", "Challenge Length", 8},
	} {
		add(field.name, field.key, "Body", DisplayUnsigned, field.bits)
	}
	for _, field := range []struct {
		name, key string
		bits      int
	}{
		{"smb.andx.cmd", "AndX Command", 8}, {"smb.andx.offset", "AndX Offset", 16},
		{"smb.andxoffset", "AndX Offset", 16},
		{"smb.security_blob_len", "Security Blob Length", 16}, {"smb.action", "Action", 16}, {"smb.vc", "VC Number", 16},
	} {
		add(field.name, field.key, "Body", DisplayUnsigned, field.bits)
	}
	for _, field := range []struct{ name, key string }{{"smb.native_os", "Native OS"}, {"smb.native_lanman", "Native LAN Manager"}} {
		add(field.name, field.key, "Body", DisplayString, 0)
		out[len(out)-1].descriptor.AllowEmpty = true
	}
	for _, field := range []struct {
		name, key string
		bits      int
	}{
		{"smb.pwlen", "Password Length", 16}, {"smb.connect.flags", "Flags", 16}, {"smb.connect.support", "Optional Support", 16},
		{"smb.search.attribute", "Search Attributes", 16}, {"smb.buffer_format", "Buffer Format", 8},
	} {
		add(field.name, field.key, "Body", DisplayUnsigned, field.bits)
	}
	for _, field := range []struct{ name, key string }{
		{"smb.path", "Path"}, {"smb.service", "Service"}, {"smb.native_fs", "Native File System"}, {"smb.file", "File Name"},
	} {
		add(field.name, field.key, "Body", DisplayString, 0)
		out[len(out)-1].descriptor.AllowEmpty = true
	}
	out = append(out, displayRoot("smb", "smb.request.file", "File name from observed matched request", DisplayString, 0, "context", "Request File Name"))
	out = append(out, displayRoot("smb", "smb.request.path", "Share path from observed matched request", DisplayString, 0, "context", "Request Path"))
	// Signed timezone and opaque bytes remain their correct typed Body values;
	// the current public alias schema has neither signed nor bytes field types.
	out = append(out, displayRoot("smb", "smb.selected_dialect", "Dialect selected from observed matched request", DisplayString, 0, "context", "Selected Dialect Name"))
	b := displayRoot("smb", "smb.dialect", "Advertised OEM dialect (escaped if needed)", DisplayString, 0, "pdu", "Name")
	b.path, b.descriptor.AllowEmpty = []string{"Dialects", "*", "Name"}, true
	b.rangeFor = func(e *ProtocolEvent, ordinal int) *DisplayFieldRange {
		var m map[string]any
		switch entries := e.Session["Dialects"].(type) {
		case []map[string]any:
			if ordinal >= 0 && ordinal < len(entries) {
				m = entries[ordinal]
			}
		case []any:
			if ordinal >= 0 && ordinal < len(entries) {
				m, _ = entries[ordinal].(map[string]any)
			}
		}
		return (&ProtocolEvent{Raw: e.Raw, Session: m}).displayPDUFieldRange("Name")
	}
	out = append(out, b)
	out = append(out, displayRoot("smb", "smb.smb2_upgrade_offered", "SMB2 upgrade offered, not selected", DisplayBoolean, 0, "derived", "SMB2 Upgrade Offered"))
	return out
}

func smb1WordCountOwner(e *ProtocolEvent, key string) map[string]any {
	if _, ok := e.Session[key]; ok {
		return e.Session
	}
	body, _ := e.Session["Body"].(map[string]any)
	return body
}

func smb1DisplayHeaderUint(e *ProtocolEvent, key string, bits uint) (uint64, bool) {
	header, _ := e.Session["Header"].(map[string]any)
	value, ok := displayUint(header[key])
	return value, ok && value < uint64(1)<<bits
}
