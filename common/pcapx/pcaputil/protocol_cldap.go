package pcaputil

import (
	"errors"
	"fmt"

	binparser "github.com/yaklang/yaklang/common/bin-parser/parser"
)

const cldapRule = "application-layer.cldap"

// validCLDAPDatagram is deliberately only a transport hint. The complete BER
// envelope and SearchRequest grammar are validated by decodeCLDAPDatagram, so
// malformed UDP/389 data still produces an explicit CLDAP refusal rather than
// being silently reclassified as DNS, LDAP-over-TCP or unrecognized.
func validCLDAPDatagram(w []byte) bool {
	return len(w) > 0 && w[0] == 0x30
}

func decodeCLDAPDatagram(w []byte, maxElements int) (map[string]any, error) {
	budget := DefaultParserBudget()
	budget.MaxCollectionElements = sessionCollectionLimit(maxElements)
	return decodeCLDAPDatagramBudget(w, budget, budget.MaxMessageBytes)
}

func (a *binParser) decodeCLDAPDatagram(w []byte) (map[string]any, error) {
	limit := min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes)
	return decodeCLDAPDatagramBudget(w, a.budget, limit)
}

func decodeCLDAPDatagramBudget(w []byte, budget ParserBudget, maxBytes int) (map[string]any, error) {
	info := map[string]any{
		"Transport":             "UDP",
		"Observation Scope":     "datagram",
		"Response Decoded":      false,
		"Security Layer Parsed": false,
	}
	if maxBytes <= 0 {
		maxBytes = DefaultParserBudget().MaxMessageBytes
	}
	if len(w) > maxBytes {
		return info, protocolError(ErrResourceExceeded, "cldap: datagram exceeds configured byte limit")
	}
	used := 0
	if err := validateLDAPBER(w, 0, budget, &used); err != nil {
		var typed *ProtocolError
		if errors.As(err, &typed) {
			return info, protocolError(typed.Kind, "cldap: %v", err)
		}
		return info, protocolError(ErrMalformedMessage, "cldap: %v", err)
	}
	messages, messageFields, err := splitCLDAPMessages(w)
	if err != nil {
		return info, protocolError(ErrMalformedMessage, "cldap: %v", err)
	}
	info["Message Count"] = len(messages)
	if len(messageFields) > 0 {
		info["Message ID"] = messageFields[0]["Message ID"]
		info["Operation"] = messageFields[0]["Operation"]
		info["Message Name"] = messageFields[0]["Message Name"]
	}

	messageID, operation, _, _ := ldapOperation(messages[0])
	if len(messages) == 1 && operation == 0x63 {
		result, err := binparser.ParseStructured(w, cldapRule, "CLDAP")
		if err != nil {
			return info, protocolError(ErrMalformedMessage, "cldap: %v", err)
		}
		fields := protocolFields(result)
		if fields == nil {
			fields = map[string]any{}
		}
		fields["Message ID"] = messageID
		fields["Operation"] = uint64(operation)
		fields["Message Name"] = messageFields[0]["Message Name"]
		fields["Transport"] = "UDP"
		fields["Observation Scope"] = "datagram"
		fields["Response Decoded"] = false
		fields["Security Layer Parsed"] = false
		fields["Message Count"] = 1
		normalizeCLDAPSearchFields(fields)
		return fields, nil
	}

	info["Messages"] = messageFields
	info["Response Body Opaque"] = true
	names := make([]string, 0, len(messageFields))
	for _, message := range messageFields {
		name, _ := message["Message Name"].(string)
		if name != "" {
			names = append(names, name)
		}
		if result, ok := message["Result Code"]; ok {
			info["Result Code"] = result
		}
	}
	if len(names) == 1 {
		return info, protocolError(
			ErrUnsupportedFeature,
			"cldap: %s response operation is outside the SearchRequest profile",
			names[0],
		)
	}
	return info, protocolError(
		ErrUnsupportedFeature,
		"cldap: response operations %v are outside the SearchRequest profile",
		names,
	)
}

func splitCLDAPMessages(w []byte) ([][]byte, []map[string]any, error) {
	var messages [][]byte
	var fields []map[string]any
	for at := 0; at < len(w); {
		if w[at] != 0x30 {
			return nil, nil, fmt.Errorf("expected LDAPMessage SEQUENCE at datagram offset %d", at)
		}
		n, h, err := berLength(w[at+1:])
		if err != nil || h == 0 {
			return nil, nil, fmt.Errorf("invalid LDAPMessage length at datagram offset %d", at)
		}
		total := 1 + h + n
		if total > len(w)-at {
			return nil, nil, fmt.Errorf("LDAPMessage at datagram offset %d exceeds datagram", at)
		}
		message := w[at : at+total]
		messageID, operation, body, err := ldapOperation(message)
		if err != nil {
			return nil, nil, err
		}
		name := ldapMessageName[operation]
		if name == "" {
			name = fmt.Sprintf("Operation0x%02x", operation)
		}
		field := map[string]any{
			"Message ID":            messageID,
			"Operation":             uint64(operation),
			"Message Name":          name,
			"Operation Body Length": len(body),
			"Operation Body Opaque": true,
		}
		if code, ok := ldapResultCode(body); ok {
			field["Result Code"] = code
		}
		messages = append(messages, message)
		fields = append(fields, field)
		at += total
	}
	if len(messages) == 0 {
		return nil, nil, fmt.Errorf("empty LDAPMessage datagram")
	}
	return messages, fields, nil
}

func cldapDatagramSummary(fields map[string]any) string {
	if fields == nil {
		return "CLDAP"
	}
	name, _ := fields["Message Name"].(string)
	if name == "" {
		name = "Message"
	}
	id, _ := displayUint(fields["Message ID"])
	summary := fmt.Sprintf("CLDAP %s id=%d", name, id)
	if count, ok := displayUint(fields["Message Count"]); ok && count > 1 {
		summary += fmt.Sprintf(" messages=%d", count)
	}
	if base, ok := fields["Base Object"].(string); ok {
		if base == "" {
			base = "<root>"
		}
		summary += " base=" + base
	}
	if scope, ok := displayUint(fields["Scope"]); ok {
		summary += fmt.Sprintf(" scope=%d", scope)
	}
	if count, ok := cldapAttributeCount(fields); ok {
		summary += fmt.Sprintf(" attributes=%d", count)
	}
	if code, ok := displayUint(fields["Result Code"]); ok {
		summary += fmt.Sprintf(" result=%d", code)
	}
	return summary
}

func normalizeCLDAPSearchFields(fields map[string]any) {
	message, _ := fields["Message"].(map[string]any)
	search, _ := message["Search"].(map[string]any)
	if search == nil {
		return
	}
	if value, ok := cldapNestedString(search["Base Object"]); ok {
		fields["Base Object"] = value
	}
	for _, key := range []string{"Scope", "Alias Mode", "Size Limit", "Time Limit", "Types Only"} {
		if value, ok := cldapNestedUnsigned(search[key]); ok {
			fields[key] = value
		}
	}
	if filter, ok := search["Filter"].(map[string]any); ok {
		fields["Filter"] = filter
		if present, ok := filter["Present Attribute"].(string); ok {
			fields["Present Attribute"] = present
		}
	}
	attributes, _ := search["Attributes"].([]any)
	names := make([]string, 0, len(attributes))
	for _, attribute := range attributes {
		if name, ok := cldapNestedString(attribute); ok {
			names = append(names, name)
		}
	}
	fields["Requested Attributes"] = names
}

func cldapNestedString(value any) (string, bool) {
	switch value := value.(type) {
	case string:
		return value, true
	case []byte:
		return string(value), true
	case map[string]any:
		nested, exists := value["Value"]
		if !exists {
			return "", true
		}
		return cldapNestedString(nested)
	default:
		return "", false
	}
}

func cldapNestedUnsigned(value any) (uint64, bool) {
	if object, ok := value.(map[string]any); ok {
		value = object["Value"]
	}
	var out uint64
	switch value := value.(type) {
	case []byte:
		if len(value) == 0 || len(value) > 8 {
			return 0, false
		}
		for _, b := range value {
			out = out<<8 | uint64(b)
		}
	case []any:
		if len(value) == 0 || len(value) > 8 {
			return 0, false
		}
		for _, item := range value {
			b, ok := displayUint(item)
			if !ok || b > 0xff {
				return 0, false
			}
			out = out<<8 | b
		}
	default:
		return displayUint(value)
	}
	return out, true
}

func cldapAttributeCount(fields map[string]any) (int, bool) {
	if attributes, ok := fields["Requested Attributes"].([]string); ok {
		return len(attributes), true
	}
	attributes, ok := fields["Attributes"].(map[string]any)
	if !ok {
		return 0, false
	}
	switch values := attributes["Attribute"].(type) {
	case []map[string]any:
		return len(values), true
	case []any:
		return len(values), true
	case map[string]any:
		return 1, true
	case []string:
		return len(values), true
	default:
		return 0, false
	}
}
