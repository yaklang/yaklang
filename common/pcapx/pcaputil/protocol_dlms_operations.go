package pcaputil

import (
	"encoding/binary"
	"errors"
)

// These service envelopes are observations on an HDLC link, not a Wrapper
// association or evidence that a method/write was authorized or executed.
func dlmsOperationAPDU(p []byte) bool {
	return len(p) > 0 && (p[0] == 0xc1 || p[0] == 0xc5 || p[0] == 0xc3 || p[0] == 0xc7)
}
func dlmsOperationListAPDU(p []byte) bool {
	return len(p) > 1 && ((p[0] == 0xc1 && p[1] == 4) || (p[0] == 0xc5 && p[1] == 5) || ((p[0] == 0xc3 || p[0] == 0xc7) && p[1] == 3))
}
func dlmsOperationKind(kind string) bool    { return kind == "SET" || kind == "ACTION" }
func dlmsSequencedService(kind string) bool { return kind == "GET" || dlmsOperationKind(kind) }
func dlmsOperationName(kind string) string {
	if kind == "SET" {
		return "Set"
	}
	return "Action"
}
func decodeDLMSOperationInformation(m *dlmsMessage, p []byte, llc byte, limit, depth int) (*dlmsMessage, error) {
	if len(p) < 3 {
		return nil, dlmsError(ErrMalformedMessage, "operation service header is truncated")
	}
	if p[1] != 1 && !dlmsOperationListAPDU(p) {
		return nil, dlmsError(ErrUnsupportedFeature, "selected Set/Action normal or bounded list choice only")
	}
	if p[2]&0x30 != 0 {
		return nil, dlmsError(ErrMalformedMessage, "invoke-id-and-priority reserved bits are nonzero")
	}
	request := p[0] == 0xc1 || p[0] == 0xc3
	if request && llc != 0xe6 || !request && llc != 0xe7 {
		return nil, dlmsError(ErrMalformedMessage, "operation LLC direction differs from command")
	}
	// Reuse the bounded normal service codec. The synthetic transport envelope
	// is private and its carrier fields are removed before any observation.
	w := make([]byte, 8+len(p))
	w[1] = 1
	binary.BigEndian.PutUint16(w[6:8], uint16(len(p)))
	copy(w[8:], p)
	var decoded *wrapperMessage
	var err error
	if p[0] == 0xc1 || p[0] == 0xc5 {
		decoded, err = decodeWrapperSet(w, limit, depth)
		m.kind = "SET"
	} else {
		decoded, err = decodeWrapperAction(w, limit, depth)
		m.kind = "ACTION"
	}
	if err != nil {
		var pe *ProtocolError
		if errors.As(err, &pe) {
			return nil, dlmsError(pe.Kind, "operation: "+pe.Message)
		}
		return nil, err
	}
	fields := decoded.fields
	for _, k := range []string{"wrapper_version", "source_wport", "destination_wport", "wrapper_header_hex"} {
		delete(fields, k)
	}
	m.invoke, m.flags, m.choice, m.count, m.request = decoded.invoke, decoded.flags, decoded.choice, decoded.count, request
	f := m.fields
	f["Invoke ID and Priority"], f["Invoke ID"], f["High Priority"], f["Confirmed Service"] = m.flags, m.invoke, m.flags&128 != 0, m.flags&64 != 0
	name := dlmsOperationName(m.kind)
	direction := "Response"
	if request {
		direction = "Request"
	}
	form := " Normal"
	if dlmsOperationListAPDU(p) {
		form = " With List"
	}
	f["Frame Kind"], f[name] = name+" "+direction+form, fields
	return m, nil
}

func dlmsOperationProjection(p []byte, n int) int64 {
	if dlmsOperationListAPDU(p) {
		nodes := min(wrapperDataNodes, max(0, len(p)-4))
		items := min(wrapperListItems, max(0, len(p)-4))
		if p[0] == 0xc1 {
			items = min(wrapperListItems, max(0, len(p)-4)/10)
		} else if p[0] == 0xc3 {
			items = min(wrapperListItems, max(0, len(p)-4)/9)
		} else if p[0] == 0xc7 {
			items = min(wrapperListItems, max(0, len(p)-4)/2)
		}
		if wrapperSetListAPDUContainsCompact(p) || wrapperActionListAPDUContainsCompact(p) {
			nodes = wrapperDataNodes
		}
		return 32768 + 512*int64(n) + 2048*int64(nodes) + 8192*int64(items)
	}

	at := -1
	if len(p) > 1 && p[1] == 1 {
		if p[0] == 0xc1 && len(p) > 12 && p[12] <= 1 {
			at = 13
			if p[12] == 1 {
				at = 14
			}
		}
		if p[0] == 0xc3 && len(p) > 12 && p[12] == 1 {
			at = 13
		}
		if p[0] == 0xc7 && len(p) > 5 && p[4] == 1 && p[5] == 0 {
			at = 6
		}
	}
	nodes := 0
	if at >= 0 {
		nodes = min(wrapperDataNodes, max(0, len(p)-at))
		probe := wrapperCompactProbe{wire: p, at: at}

		found, complete := probe.data(0)
		if !found && complete && p[0] == 0xc1 && len(p) > 12 && p[12] == 1 {
			found, _ = probe.data(0)
		}
		if found {
			nodes = wrapperDataNodes
		}

	}
	return 32768 + 512*int64(n) + 2048*int64(nodes)
}
func dlmsOperationCloseFields(p *dlmsPending) map[string]any {
	fields := map[string]any{"Outstanding": 1, "Request Service": p.kind, "Invoke ID": p.invoke, "Source Address": p.source, "Destination Address": p.destination}
	if p.choice != 1 {
		fields["Service Choice"] = p.choice
		fields["List Count"] = uint64(p.count)
	}
	return fields
}

func dlmsOperationCloseName(p *dlmsPending) string {
	name := dlmsOperationName(p.kind)
	if p.choice != 1 {
		return name + "-with-list"
	}
	return name
}
