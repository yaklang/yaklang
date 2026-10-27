package pcaputil

import (
	"bytes"
	"strings"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

type imapIdle struct {
	tag                          string
	client                       int
	continued, done, quarantined bool
	commandID, continuationID    uint64
}
type imapNamespaceRequest struct {
	client int
	id     uint64
}

func imapExtensionError(profile, reason string) (map[string]any, bool, error) {
	return map[string]any{"Profile": profile, "Packet Name": "Refused extension message", "Error Scope": "message"}, true, protocolError(ErrMalformedMessage, "IMAP: %s", reason)
}

func (s *binIMAP) addExtensionCommand(tag, command string) error {
	if _, exists := s.pending[tag]; exists {
		return protocolError(ErrDesynchronized, "IMAP command reused an outstanding tag")
	}
	if len(s.pending) >= sessionCollectionLimit(s.maxPending) {
		return protocolError(ErrResourceExceeded, "IMAP pending tag budget exceeded")
	}
	if s.pending == nil {
		s.pending = map[string]string{}
	}
	s.pending[strings.Clone(tag)] = command
	s.pendingBytes += len(tag) + len(command)
	return nil
}
func (s *binIMAP) retireExtension(tag string) {
	if command, ok := s.pending[tag]; ok {
		delete(s.pending, tag)
		s.pendingBytes -= len(tag) + len(command)
	}
}

func (s *binIMAP) consumeNamespace(dir int, raw []byte, words []string) (map[string]any, bool, error) {
	const profile = "imap-namespace-observed"
	if len(words) < 2 {
		return nil, false, nil
	}
	tag, command := words[0], strings.ToUpper(words[1])
	if owner := s.namespaces[tag]; owner != nil {
		if dir == owner.client || command != "OK" && command != "NO" && command != "BAD" {
			return imapExtensionError(profile, "NAMESPACE completion direction/status differs")
		}
		info, err := stream_parser.DecodeIMAPMessageFields(raw, "response")
		if err != nil {
			return imapExtensionError(profile, err.Error())
		}
		s.retireExtension(tag)
		delete(s.namespaces, tag)
		return map[string]any{"Profile": profile, "Namespace Stage": "result", "Packet Name": "Tagged " + command, "Role": "tagged-reply", "Tag": tag, "Command": command, "In Reply To": "NAMESPACE", "IMAP Fields": info}, true, nil
	}
	if command != "NAMESPACE" {
		return nil, false, nil
	}
	if tag == "*" {
		info, err := stream_parser.DecodeIMAPMessageFields(raw, "response")
		if err != nil {
			if _, _, literal := imapLiteralSuffix(bytes.TrimSuffix(raw, []byte("\r\n"))); literal {
				return map[string]any{"Profile": profile, "Packet Name": "NAMESPACE literal context required"}, true, protocolError(ErrUnsupportedFeature, "native NAMESPACE literal reconstruction is unavailable; explicit imap_fields accepts complete literal messages")
			}
			return imapExtensionError(profile, err.Error())
		}
		return map[string]any{"Profile": profile, "Namespace Stage": "response", "Packet Name": "Untagged NAMESPACE", "Role": "untagged", "Tag": "*", "Command": "NAMESPACE", "IMAP Fields": info}, true, nil
	}
	info, err := stream_parser.DecodeIMAPMessageFields(raw, "command")
	if err != nil {
		return imapExtensionError(profile, err.Error())
	}
	if err = s.addExtensionCommand(tag, "NAMESPACE"); err != nil {
		return nil, true, err
	}
	if s.namespaces == nil {
		s.namespaces = map[string]*imapNamespaceRequest{}
	}
	s.namespaces[strings.Clone(tag)] = &imapNamespaceRequest{client: dir}
	return map[string]any{"Profile": profile, "Namespace Stage": "command", "Packet Name": "NAMESPACE", "Role": "command", "Tag": tag, "Command": "NAMESPACE", "IMAP Fields": info}, true, nil
}

func (s *binIMAP) consumeIdle(dir int, raw []byte, words []string) (map[string]any, bool, error) {
	const profile = "imap-idle-observed"
	if owner := s.idle; owner != nil {
		out := map[string]any{"Profile": profile, "Tag": owner.tag, "Command": "IDLE", "Session State Validated": false, "Capability Authorization Verified": false}
		if dir == owner.client {
			if !owner.continued || owner.done || owner.quarantined || !bytes.EqualFold(raw, []byte("DONE\r\n")) {
				owner.quarantined = true
				return imapExtensionError(profile, "IDLE client data has no valid outstanding DONE continuation")
			}
			owner.done = true
			out["Packet Name"], out["Role"], out["IDLE Stage"] = "IDLE DONE", "continuation-response", "done"
			return out, true, nil
		}
		if bytes.HasPrefix(raw, []byte("+")) {
			if owner.continued || owner.done || owner.quarantined || !bytes.HasPrefix(raw, []byte("+ ")) || !bytes.HasSuffix(raw, []byte("\r\n")) {
				owner.quarantined = true
				return imapExtensionError(profile, "IDLE server continuation overlaps or has invalid syntax")
			}
			for _, b := range raw[2 : len(raw)-2] {
				if b == 0 || b == '\r' || b == '\n' {
					owner.quarantined = true
					return imapExtensionError(profile, "invalid IDLE continuation text")
				}
			}
			owner.continued = true
			out["Packet Name"], out["Role"], out["IDLE Stage"], out["Continuation Text"] = "IDLE Continuation", "continuation", "continuation", string(raw[2:len(raw)-2])
			return out, true, nil
		}
		if len(words) >= 2 && words[0] == owner.tag {
			status := strings.ToUpper(words[1])
			if status != "OK" && status != "NO" && status != "BAD" || status == "OK" && (!owner.done || owner.quarantined) {
				owner.quarantined = true
				return imapExtensionError(profile, "IDLE completion status precedes a valid DONE")
			}
			info, err := stream_parser.DecodeIMAPMessageFields(raw, "response")
			if err != nil {
				return imapExtensionError(profile, err.Error())
			}
			out["Packet Name"], out["Role"], out["IDLE Stage"], out["In Reply To"] = "Tagged "+status, "tagged-reply", "result", "IDLE"
			out["Server Result Observed"], out["IMAP Fields"] = status, info
			s.retireExtension(owner.tag)
			s.idle = nil
			return out, true, nil
		}
		return nil, false, nil
	}
	if len(words) < 2 || !strings.EqualFold(words[1], "IDLE") || words[0] == "*" {
		return nil, false, nil
	}
	info, err := stream_parser.DecodeIMAPMessageFields(raw, "command")
	if err != nil {
		return imapExtensionError(profile, err.Error())
	}
	if s.literal[0] > 0 || s.literal[1] > 0 || s.literalTail[0] || s.literalTail[1] {
		return imapExtensionError(profile, "IDLE overlaps a literal")
	}
	if err = s.addExtensionCommand(words[0], "IDLE"); err != nil {
		return nil, true, err
	}
	s.idle = &imapIdle{tag: strings.Clone(words[0]), client: dir}
	return map[string]any{"Profile": profile, "Tag": words[0], "Command": "IDLE", "Packet Name": "IDLE", "Role": "command", "IDLE Stage": "command", "IMAP Fields": info, "Session State Validated": false, "Capability Authorization Verified": false}, true, nil
}
