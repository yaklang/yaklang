package pcaputil

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

// Only smb2Walk creates this marker, before it associates any command. Errors
// during association, framing or header validation must remain session-fatal.
type smb2BodyDecodeError struct {
	command int
	cause   error
}

func (e *smb2BodyDecodeError) Error() string { return e.cause.Error() }
func (e *smb2BodyDecodeError) Unwrap() error { return e.cause }

// The caller has independently checked the complete 64-byte fixed header.
func smb2CommandHeader(pdu []byte) map[string]any {
	cmd := binary.LittleEndian.Uint16(pdu[12:14])
	flags := binary.LittleEndian.Uint32(pdu[16:20])
	resp := flags&smb2FlagResp != 0
	info := map[string]any{
		"Command": cmd, "Packet Name": smb2CommandName(cmd), "Flags": flags,
		"Response": resp, "Message ID": binary.LittleEndian.Uint64(pdu[24:32]),
		"Credit": binary.LittleEndian.Uint16(pdu[14:16]),
	}
	smb2HeaderFields(info, pdu, resp, flags)
	info["Session ID"] = binary.LittleEndian.Uint64(pdu[40:48])
	if flags&smb2FlagAsync == 0 {
		info["Tree ID"] = binary.LittleEndian.Uint32(pdu[36:40])
	} else {
		info["Async ID"] = binary.LittleEndian.Uint64(pdu[32:40])
	}
	if flags&smb2FlagRelated != 0 {
		info["Related"] = true
	}
	return info
}

// Return a diagnostic projection only after proving the entire framed record's
// header chain, including headers after the failed body. There is no byte scan,
// re-synchronization, dialect inference or state publication here.
func smb2RecoverBodyRecord(raw []byte, max int, err error) map[string]any {
	var bodyError *smb2BodyDecodeError
	if !errors.As(err, &bodyError) || max <= 0 || len(raw) < 68 || raw[0] != 0 || smb2DirectLength(raw)+4 != len(raw) {
		return nil
	}
	var typed *ProtocolError
	if errors.As(err, &typed) && typed.Kind != ErrMalformedMessage && typed.Kind != ErrUnsupportedFeature && typed.Kind != ErrUnsupportedVersion {
		return nil // Resource, state and context failures are not body recovery.
	}
	var commands []map[string]any
	var bodies [][2]int
	var firstResponse, relatedStyle bool
	for at := 4; ; {
		if len(raw)-at < 64 || len(commands) >= max {
			return nil
		}
		h := raw[at:]
		if binary.LittleEndian.Uint32(h) != smb2Magic || binary.LittleEndian.Uint16(h[4:]) != 64 {
			return nil
		}
		next := uint64(binary.LittleEndian.Uint32(h[20:]))
		end := len(raw)
		if next != 0 {
			if next < 64 || next%8 != 0 || next+64 > uint64(len(h)) {
				return nil
			}
			end = at + int(next)
		}
		flags := binary.LittleEndian.Uint32(h[16:])
		if flags&smb2FlagAsync != 0 && binary.LittleEndian.Uint64(h[32:40]) == 0 {
			return nil
		}
		response, related := flags&smb2FlagResp != 0, flags&smb2FlagRelated != 0
		if len(commands) == 0 {
			if related {
				return nil
			}
			firstResponse = response
		} else {
			if response != firstResponse {
				return nil
			}
			if len(commands) == 1 {
				relatedStyle = related
			} else if !response && related != relatedStyle {
				return nil
			}
		}
		command := smb2CommandHeader(h[:end-at])
		smb2OffsetRanges(command, at)
		commands = append(commands, command)
		bodies = append(bodies, [2]int{at + 64, end})
		if next == 0 {
			break
		}
		at = end
	}
	if bodyError.command < 0 || bodyError.command >= len(commands) {
		return nil
	}
	for i, command := range commands {
		span := bodies[i]
		command["Body"] = map[string]any{
			"Unparsed Body":     bytes.Clone(raw[span[0]:span[1]]),
			"Wire Field Ranges": map[string]any{"Unparsed Body": []any{uint64(span[0]), uint64(span[1])}},
		}
		command["Body Decode Status"] = "not decoded: record body failed before association"
		command["Context Level"] = "partial"
	}
	info := commands[0]
	if len(commands) > 1 {
		info = cloneSession(info)
		info["Compound"], info["Compound Count"], info["Commands"] = true, len(commands), commands
	}
	info["Error Scope"] = "message"
	info["Body Decode Error"] = err.Error()
	info["Failed Command Index"] = bodyError.command
	info["State Updated"] = false
	info["Validated Envelope"] = "Direct TCP length and complete SMB2 fixed-header chain"
	return info
}

func (f *binFlow) consumeSMB2(dir int, e *ProtocolEvent) error {
	max := f.a.budget.MaxCollectionElements
	info, err := f.smb2.consumeFrom(dir, e.Raw, max)
	if err == nil {
		e.Session = info
		return nil
	}
	info = smb2RecoverBodyRecord(e.Raw, max, err)
	if info == nil || !f.smb2.bodyRecoveryIdentitySafe(info, dir) {
		return err
	}
	if dialect := f.smb2.dialect; dialect != 0 {
		info["Connection Dialect"] = dialect // Observation only, not authenticated identity.
	}
	// These old public captures use a 16-byte SESSION_SETUP request (17 in
	// StructureSize), rather than the modern 24-byte layout (25). Keep the
	// unsupported layout visible; do not reinterpret security-token bytes as
	// PreviousSessionId or invent an exact historical dialect specification.
	if info["Compound"] != true && info["Command"] == uint16(1) && info["Response"] == false && len(e.Raw) >= 70 && binary.LittleEndian.Uint16(e.Raw[68:70]) == 17 {
		err = protocolError(ErrUnsupportedVersion, "SESSION_SETUP observed StructureSize 17; supported modern request layout requires 25")
		info["Body Decode Error"] = err.Error()
		info["Body Decode Status"] = "unsupported historical-shaped SESSION_SETUP layout"
	} else {
		_, classified := classifySessionError(err)
		if classified == nil {
			err = protocolError(ErrMalformedMessage, "%s", err)
		}
	}
	info["Recovery Diagnostic"] = fmt.Sprintf("command %d refused; continue at the next complete Direct TCP record", info["Failed Command Index"])
	e.Session = info
	e.nativeSessionMessage = true
	e.semanticSession = cloneSession(info)
	e.semanticFields = cloneSession(info)
	return err
}

// Header-visible pending conflicts must not be hidden by a later bad body.
// This pass is read-only and applies the same related-request effective IDs.
func (s *binSMB2) bodyRecoveryIdentitySafe(info map[string]any, dir int) bool {
	commands := []map[string]any{info}
	if info["Compound"] == true {
		commands = info["Commands"].([]map[string]any)
	}
	seen := map[smb2PendingKey]bool{}
	aliases := map[smb2AsyncKey]smb2PendingKey{}
	requestAliases := map[smb2PendingKey]smb2AsyncKey{}
	var previousSID uint64
	var previousTID uint32
	for _, c := range commands {
		sid, tid := smb2EffectiveIDs(c)
		if c["Response"] == false && c["Related"] == true {
			sid, tid = previousSID, previousTID
		}
		previousSID, previousTID = sid, tid
		mid := c["Message ID"].(uint64)
		key := smb2PendingKey{sessionID: sid, treeID: tid, messageID: mid}
		name := c["Packet Name"].(string)
		if c["Response"] == false {
			if name == "CANCEL" {
				continue
			}
			if _, ok := s.pending[key]; ok || seen[key] {
				return false
			}
			seen[key] = true
		} else if c["Status"] == uint32(0x103) {
			async, _ := c["Async ID"].(uint64)
			if async != 0 {
				pendingKey, pending, matched := s.findResponse(key, async, name, c)
				if matched && !(dir >= 0 && pending.hasDirection && pending.direction == dir) {
					alias := smb2AsyncKey{sessionID: sid, asyncID: async}
					if pending.async && pending.asyncID != async {
						return false
					}
					if oldAlias, ok := requestAliases[pendingKey]; ok && oldAlias != alias {
						return false
					}
					if oldKey, ok := s.async[alias]; ok && oldKey != pendingKey {
						return false
					}
					if oldKey, ok := aliases[alias]; ok && oldKey != pendingKey {
						return false
					}
					aliases[alias] = pendingKey
					requestAliases[pendingKey] = alias
				}
			}
		}
	}
	return true
}
