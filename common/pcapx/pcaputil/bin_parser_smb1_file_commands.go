package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func smb1FileEntry(command byte, response bool) (entry, profile, name string, maxSize int) {
	suffix := "Request"
	if response {
		suffix = "Response"
	}
	switch command {
	case 0x75:
		entry, profile, name, maxSize = "CIFSDirectTCPTreeConnectAndX"+suffix, "cifs-tree-connect-andx-request", "SMB_COM_TREE_CONNECT_ANDX", 65582
		if response {
			profile, maxSize = "cifs-tree-connect-andx-response", 65580
		}
	case 0x06:
		entry, profile, name, maxSize = "CIFSDirectTCPDelete"+suffix, "cifs-delete-request", "SMB_COM_DELETE", 65576
		if response {
			profile, maxSize = "cifs-delete-response", 39
		}
	case 0xa2:
		entry, profile, name, maxSize = "CIFSDirectTCPNTCreateAndX"+suffix, "cifs-nt-create-andx-request", "SMB_COM_NT_CREATE_ANDX", 65582
		if response {
			profile, maxSize = "cifs-nt-create-andx-response", 107
		}
	case 0x2f:
		entry, profile, name, maxSize = "CIFSDirectTCPWriteAndX"+suffix, "cifs-write-andx-request", "SMB_COM_WRITE_ANDX", 65582
		if response {
			profile, maxSize = "cifs-write-andx-response", 51
		}
	case 0x22:
		entry, profile, name, maxSize = "CIFSDirectTCPSetInformation2"+suffix, "cifs-set-information2-request", "SMB_COM_SET_INFORMATION2", 53
		if response {
			profile, maxSize = "cifs-set-information2-response", 39
		}
	case 0x23:
		entry, profile, name, maxSize = "CIFSDirectTCPQueryInformation2"+suffix, "cifs-query-information2-request", "SMB_COM_QUERY_INFORMATION2", 41
		if response {
			profile, maxSize = "cifs-query-information2-response", 61
		}
	case 0x32:
		entry, profile, name, maxSize = "CIFSDirectTCPTransaction2"+suffix, "cifs-transaction2-request", "SMB_COM_TRANSACTION2", 65582
		if response {
			profile = "cifs-transaction2-response"
		}
	}
	return
}

func smb1SelectedFileCommand(command byte) bool {
	switch command {
	case 0x75, 0x06, 0xa2, 0x2f, 0x22, 0x23, 0x32:
		return true
	default:
		return false
	}
}

func isSMB1FileEntry(entry string) bool {
	switch entry {
	case "CIFSDirectTCPTreeConnectAndXRequest", "CIFSDirectTCPTreeConnectAndXResponse", "CIFSDirectTCPDeleteRequest", "CIFSDirectTCPDeleteResponse",
		"CIFSDirectTCPNTCreateAndXRequest", "CIFSDirectTCPNTCreateAndXResponse", "CIFSDirectTCPWriteAndXRequest", "CIFSDirectTCPWriteAndXResponse",
		"CIFSDirectTCPSetInformation2Request", "CIFSDirectTCPSetInformation2Response", "CIFSDirectTCPQueryInformation2Request", "CIFSDirectTCPQueryInformation2Response",
		"CIFSDirectTCPTransaction2Request", "CIFSDirectTCPTransaction2Response":
		return true
	}
	return false
}

func probeSMB1FileCommand(w []byte, limit int) ProbeResult {
	reject := ProbeResult{Verdict: ProbeReject}
	if len(w) < 8 || w[0] != 0 || binary.LittleEndian.Uint32(w[4:8]) != 0x424d53ff {
		return reject
	}
	total := smb2DirectLength(w) + 4
	if total < 39 || total > min(limit, 65582) {
		return reject
	}
	if len(w) < 9 {
		return probeNeed("smb", "1-file-command", len(w), 9)
	}
	if !smb1SelectedFileCommand(w[8]) {
		return reject
	}
	if len(w) < 16 {
		return probeNeed("smb", "1-file-command", len(w), 16)
	}
	response := w[13]&0x80 != 0
	_, profile, _, maxSize := smb1FileEntry(w[8], response)
	base, wc := 47, byte(4)
	if w[8] == 0x75 && response {
		base, wc = 45, 3
	}
	if w[8] == 0x06 {
		base, wc = 41, 1
		if response {
			base, wc = 39, 0
		}
	}
	if w[8] == 0xa2 {
		base, wc = 87, 24
		if response {
			base, wc = 107, 34
		}
	}
	if w[8] == 0x2f {
		base, wc = 67, 14
		if response {
			base, wc = 51, 6
		}
	}
	exact := false
	if w[8] == 0x22 {
		base, wc, exact = 53, 7, true
		if response {
			base, wc = 39, 0
		}
	}
	if w[8] == 0x23 {
		base, wc, exact = 41, 1, true
		if response {
			base, wc = 61, 11
		}
	}
	if w[8] == 0x32 {
		if len(w) < 37 {
			return probeNeed("smb", profile, len(w), 37)
		}
		wc = w[36]
		minimum := byte(14)
		if response {
			minimum = 10
		}
		if wc < minimum {
			return reject
		}
		base = 39 + 2*int(wc)
	}
	if exact && total != base || total < base || total > maxSize {
		return reject
	}
	if response && binary.LittleEndian.Uint32(w[9:13]) != 0 {
		return reject
	}
	// SET/QUERY_INFORMATION2 have no strings. TRANSACTION2 decodes an
	// SMB_STRING only when FLAGS2_UNICODE is set and leaves OEM opaque.
	if (w[8] == 0x75 || !response && w[8] != 0x22 && w[8] != 0x23 && w[8] != 0x32) && binary.LittleEndian.Uint16(w[14:16])&0x8000 == 0 {
		return reject
	}
	if len(w) > 36 && w[36] != wc || (w[8] == 0x75 || w[8] == 0xa2 || w[8] == 0x2f) && len(w) > 37 && w[37] != 0xff {
		return reject
	}
	if len(w) < base {
		return probeNeed("smb", profile, len(w), base)
	}
	if int(binary.LittleEndian.Uint16(w[base-2:base])) != total-base {
		return reject
	}
	if len(w) < total {
		return probeNeed("smb", profile, len(w), total)
	}
	return probeAccept("smb", profile, 99)
}

// This is a private framing proof, not a successful body parse. Only an exact
// DirectTCP boundary and complete WC/BC window may retain a body diagnostic.
func boundedSMB1FileEnvelope(w []byte) bool {
	if len(w) < 39 || w[0] != 0 || smb2DirectLength(w)+4 != len(w) || binary.LittleEndian.Uint32(w[4:8]) != 0x424d53ff || !smb1SelectedFileCommand(w[8]) {
		return false
	}
	fixed := 39 + 2*int(w[36])
	return fixed <= len(w) && int(binary.LittleEndian.Uint16(w[fixed-2:fixed])) == len(w)-fixed
}

func decodeSMB1FileFields(raw []byte, budget ParserBudget) (map[string]any, error) {
	defaults := DefaultParserBudget()
	for _, pair := range [][2]*int{
		{&budget.MaxFrameBytes, &defaults.MaxFrameBytes}, {&budget.MaxMessageBytes, &defaults.MaxMessageBytes}, {&budget.MaxBufferedBytes, &defaults.MaxBufferedBytes},
		{&budget.MaxRecursionDepth, &defaults.MaxRecursionDepth}, {&budget.MaxCollectionElements, &defaults.MaxCollectionElements}, {&budget.ProbeBytes, &defaults.ProbeBytes},
	} {
		if *pair[0] < 0 {
			return nil, protocolError(ErrResourceExceeded, "CIFS file command: negative parser budget")
		}
		if *pair[0] == 0 {
			*pair[0] = *pair[1]
		}
	}
	if len(raw) < 39 {
		return nil, fmt.Errorf("CIFS file command: truncated fixed header")
	}
	response := raw[13]&0x80 != 0
	entry, _, _, maximum := smb1FileEntry(raw[8], response)
	if entry == "" {
		return nil, protocolError(ErrUnsupportedFeature, "CIFS: no selected file command profile")
	}
	if len(raw) > maximum || len(raw) > budget.MaxFrameBytes || len(raw) > budget.MaxMessageBytes || len(raw) > budget.MaxBufferedBytes || budget.MaxRecursionDepth < 3 {
		return nil, protocolError(ErrResourceExceeded, "CIFS file command: bounded profile exceeds parser budget")
	}
	r := &smb1NegotiateBoundedReader{Reader: bytes.NewReader(raw), bits: uint64(len(raw)) * 8}
	node, err := parser.ParseBinary(r, "application-layer.cifs", entry)
	if err != nil {
		var profileErr error
		switch raw[8] {
		case 0x75:
			profileErr = stream_parser.ValidateCIFSTreeConnectAndXProfile(raw, response)
		case 0xa2, 0x2f:
			if response {
				_, profileErr = stream_parser.DecodeCIFSNTCreateAndX(raw, true)
			}
			if raw[8] == 0x2f {
				_, profileErr = stream_parser.DecodeCIFSWriteAndX(raw, response)
			} else if !response {
				_, profileErr = stream_parser.DecodeCIFSNTCreateAndX(raw, false)
			}
		case 0x22:
			_, profileErr = stream_parser.DecodeCIFSSetInformation2(raw, response)
		case 0x23:
			_, profileErr = stream_parser.DecodeCIFSQueryInformation2(raw, response)
		case 0x32:
			_, profileErr = stream_parser.DecodeCIFSTransaction2(raw, response)
		default:
			profileErr = stream_parser.ValidateCIFSDeleteProfile(raw, response)
		}
		if errors.Is(profileErr, stream_parser.ErrCIFSFileCommandUnsupported) {
			return nil, protocolError(ErrUnsupportedFeature, "%s", profileErr)
		}
		return nil, protocolError(ErrMalformedMessage, "CIFS file command: %s", err)
	}
	if r.Len() != 0 {
		return nil, protocolError(ErrMalformedMessage, "CIFS file command: unconsumed bounded record bytes")
	}
	var fields map[string]any
	switch raw[8] {
	case 0x75:
		fields, err = parser.ProjectCIFSTreeConnectAndXFields(node, raw)
	case 0xa2:
		fields, err = stream_parser.DecodeCIFSNTCreateAndX(raw, response)
	case 0x2f:
		fields, err = stream_parser.DecodeCIFSWriteAndX(raw, response)
	case 0x22:
		fields, err = stream_parser.DecodeCIFSSetInformation2(raw, response)
	case 0x23:
		fields, err = stream_parser.DecodeCIFSQueryInformation2(raw, response)
	case 0x32:
		fields, err = stream_parser.DecodeCIFSTransaction2(raw, response)
	default:
		fields, err = parser.ProjectCIFSDeleteFields(node, raw)
	}
	if err != nil {
		return nil, protocolError(ErrMalformedMessage, "CIFS file command: %s", err)
	}
	if sessionSnapshotSize(fields, false) > budget.MaxBufferedBytes {
		return nil, protocolError(ErrResourceExceeded, "CIFS file command: owned field projection exceeds buffer budget")
	}
	return fields, nil
}

func (f *binFlow) consumeSMB1FileCommand(dir int, e *ProtocolEvent) (resultErr error) {
	e.nativeSessionMessage = true
	if e.ID == 0 {
		e.ID = f.a.ids.Add(1)
	}
	if f.smb2 == nil {
		return sessionContext("CIFS file command: carrier unavailable")
	}
	if len(e.Raw) < 39 {
		return protocolError(ErrMalformedMessage, "CIFS file command: truncated record")
	}
	response := e.Raw[13]&0x80 != 0
	_, e.Profile, _, _ = smb1FileEntry(e.Raw[8], response)
	e.Completeness = "message"
	fields, err := decodeSMB1FileFields(e.Raw, f.a.budget)
	if err != nil {
		var typed *ProtocolError
		if errors.As(err, &typed) && (typed.Kind == ErrMalformedMessage || typed.Kind == ErrUnsupportedFeature) && boundedSMB1FileEnvelope(e.Raw) {
			return f.refuseSMB1FileBody(dir, e, err)
		}
		return smb1ResourceRefusal(err)
	}
	tx, err := f.beginSMB1Legacy(dir, e, fields)
	if err != nil {
		return err
	}
	defer tx.finish(&resultErr)
	header := fields["Header"].(map[string]any)
	uid, tid := header["User ID"].(uint16), header["Tree ID"].(uint16)
	_, _, name, _ := smb1FileEntry(tx.command, response)
	fields["Association Policy"] = "unique observed opposite-direction PID32/MID command owner; ambiguity is not a wire validity or authentication verdict"
	fields["Association Status"] = "request context unavailable"
	if !tx.allowed {
		fields["Association Status"] = tx.reason
	} else if !response {
		ledger := f.smb2.smb1Legacy
		if len(ledger.pending) >= f.a.budget.MaxCollectionElements {
			return protocolError(ErrResourceExceeded, "CIFS file command: bounded pending collection exceeded")
		}
		body := fields["Body"].(map[string]any)
		pathKey := "Path"
		if tx.command == 0x06 || tx.command == 0xa2 {
			pathKey = "File Name"
		}
		path, _ := body[pathKey].(string)
		service, _ := body["Service"].(string)
		digest := sha256.Sum256(e.Raw)
		pending := smb1OperationPending{id: e.ID, command: tx.command, uid: uid, tid: tid, hash: hex.EncodeToString(digest[:]), path: strings.Clone(path), service: strings.Clone(service)}
		if tx.command == 0x32 {
			if setup, ok := body["Setup"].(uint16); ok {
				pending.setup, pending.hasSetup = setup, true
			}
			if level, ok := body["Information Level"].(uint16); ok {
				pending.level, pending.hasLevel = level, true
			}
		}
		ledger.pending[tx.key] = pending
		fields["Association Status"] = "request observed; response required"
	} else if ledger := f.smb2.smb1Legacy; ledger != nil {
		pending, ok := ledger.pending[tx.key]
		// TREE_CONNECT allocates a response TID; DELETE must echo its own
		// request TID. Neither may borrow a previous connection's numeric UID.
		if ok && pending.command == tx.command && pending.uid == uid && (tx.command == 0x75 || pending.tid == tid) {
			e.ResponseTo = pending.id
			fields["Matched Request"], fields["Request Event ID"], fields["Request PDU SHA256"] = name, pending.id, pending.hash
			fields["Association Status"] = "matched observed request; authentication unverified"
			if tx.command == 0x32 && pending.hasSetup {
				stream_parser.AnnotateCIFSTransaction2Response(fields, pending.setup, pending.level, pending.hasLevel)
			}
			if tx.command == 0x06 || tx.command == 0xa2 {
				fields["Request File Name"] = pending.path
			} else if tx.command == 0x75 {
				fields["Request Path"], fields["Request Service"] = pending.path, pending.service
			}
			if tx.command == 0xa2 {
				if fid, ok := fields["Body"].(map[string]any)["FID"].(uint16); ok {
					fields["Observed FID"] = fid
					fields["FID Status"] = "allocated by this successful response; not an authentication result"
				}
			}
			if tx.command == 0x2f {
				fields["Write Count Status"] = "literal count only; request FID, offset and cached path have no response wire range"
			}
			delete(ledger.pending, tx.key)
		} else {
			fields["Association Status"] = "response command, UID or TID has no matching observed request"
		}
	}
	e.Session = fields
	e.semanticFields, e.semanticSession = cloneSession(fields), cloneSession(fields)
	return nil
}

func smb1OwnedHeader(w []byte) map[string]any {
	header := map[string]any{
		"Protocol ID": binary.LittleEndian.Uint32(w[4:8]), "Command": w[8], "Status": binary.LittleEndian.Uint32(w[9:13]), "Flags": w[13], "Flags 2": binary.LittleEndian.Uint16(w[14:16]),
		"PID High": binary.LittleEndian.Uint16(w[16:18]), "Security Features": bytes.Clone(w[18:26]), "Reserved": binary.LittleEndian.Uint16(w[26:28]), "Tree ID": binary.LittleEndian.Uint16(w[28:30]),
		"Process ID Low": binary.LittleEndian.Uint16(w[30:32]), "User ID": binary.LittleEndian.Uint16(w[32:34]), "Multiplex ID": binary.LittleEndian.Uint16(w[34:36]),
	}
	ranges := map[string]any{}
	for _, field := range []struct {
		name       string
		start, end uint64
	}{
		{"Protocol ID", 4, 8}, {"Command", 8, 9}, {"Status", 9, 13}, {"Flags", 13, 14}, {"Flags 2", 14, 16}, {"PID High", 16, 18}, {"Security Features", 18, 26},
		{"Reserved", 26, 28}, {"Tree ID", 28, 30}, {"Process ID Low", 30, 32}, {"User ID", 32, 34}, {"Multiplex ID", 34, 36},
	} {
		ranges[field.name] = []any{field.start, field.end}
	}
	header["Wire Field Ranges"] = ranges
	return header
}

func (f *binFlow) refuseSMB1FileBody(dir int, e *ProtocolEvent, cause error) (resultErr error) {
	if !boundedSMB1FileEnvelope(e.Raw) {
		return cause
	}
	response := e.Raw[13]&0x80 != 0
	_, _, name, _ := smb1FileEntry(e.Raw[8], response)
	fields := map[string]any{
		"Header": smb1OwnedHeader(e.Raw), "Packet Name": name, "Response": response,
		"Body Decode Error": cause.Error(), "Decoded Body Published": false, "Error Scope": "message",
		"Context Eligibility": "refused command owner retired; no accepted body committed",
		"Direct TCP":          map[string]any{"Message Type": e.Raw[0], "Payload Length": uint32(len(e.Raw) - 4), "Wire Field Ranges": map[string]any{"Message Type": []any{uint64(0), uint64(1)}, "Payload Length": []any{uint64(1), uint64(4)}}},
		"Wire Field Ranges":   map[string]any{},
	}
	tx, err := f.beginSMB1Legacy(dir, e, fields)
	if err != nil {
		return err
	}
	tx.refused = true
	defer tx.finish(&resultErr)
	if !tx.refusalEligible {
		fields["Context Eligibility"] = "unmatched refused response; existing request context unchanged"
	}
	e.ResponseTo = 0
	e.Profile, e.Completeness = "smb1-file-command-body-diagnostic", "headers"
	e.Summary = name + " · refused body · exact record boundary retained"
	e.Session = fields
	e.semanticFields, e.semanticSession = cloneSession(fields), cloneSession(fields)
	e.smb1FileBodyRefusal = true
	return cause
}
