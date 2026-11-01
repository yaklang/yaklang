package sharkcli

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// Bind the retained fixed-header observation to its owned Direct TCP bytes.
// This is not a body parser, a port hint, or evidence of authentication.
func smbSummaryOwnedRecord(e *pcaputil.ProtocolEvent) bool {
	if e == nil || e.Protocol != "smb2" || len(e.Raw) < 68 || e.Raw[0] != 0 {
		return false
	}
	n := int(e.Raw[1])<<16 | int(e.Raw[2])<<8 | int(e.Raw[3])
	if n+4 != len(e.Raw) || binary.LittleEndian.Uint32(e.Raw[4:8]) != 0x424d53fe || binary.LittleEndian.Uint16(e.Raw[8:10]) != 64 {
		return false
	}
	h, _ := eventValues(e)["Header"].(map[string]any)
	span := detailWireRanges(h["Wire Field Ranges"])["Protocol ID"]
	return number(h["Protocol ID"]) == 0x424d53fe && len(span) == 2 && span[0] == 4 && span[1] == 8
}

func smbBodyDiagnostic(e *pcaputil.ProtocolEvent) bool {
	return e != nil && e.Profile == "smb2-record-body-diagnostic" && e.Completeness == "headers" &&
		(e.Status == "malformed" || e.Status == "context-required") && e.Error != "" &&
		eventValues(e)["Error Scope"] == "message" && eventValues(e)["State Updated"] == false &&
		eventValues(e)["Validated Envelope"] == "Direct TCP length and complete SMB2 fixed-header chain" && smbSummaryOwnedRecord(e)
}

func smbRefusedRecordSummary(e *pcaputil.ProtocolEvent) string {
	kind := "invalid body"
	if e.ExpertCode == string(pcaputil.ErrUnsupportedVersion) {
		kind = "unsupported body layout"
	}
	return fmt.Sprintf("%s · %s · %s", textValue(eventValues(e)["Packet Name"]), kind, safeText(e.Error))
}

// Preserve integer spelling across the isolated JSON worker. In particular,
// never round an int64 file size through float64, or turn an absent value into0.
func smbSummaryInteger(value any) (string, bool) {
	switch value := value.(type) {
	case uint8, uint16, uint32, uint64, int8, int16, int32, int64, int:
		return fmt.Sprint(value), true
	case json.Number:
		if _, err := strconv.ParseInt(string(value), 10, 64); err == nil {
			return string(value), true
		}
		if _, err := strconv.ParseUint(string(value), 10, 64); err == nil {
			return string(value), true
		}
	}
	return "", false
}

// smbCommandSummary is the list line for a single decoded SMB2 command.
// Cached connection dialect is not substituted for this packet's own fields.
func smbCommandSummary(e *pcaputil.ProtocolEvent) (string, bool) {
	if e == nil || e.Error != "" || e.Status != "decoded" && e.Status != "deferred" || !smbSummaryOwnedRecord(e) {
		return "", false
	}
	v := eventValues(e)
	if v["Compound"] == true {
		return "", false
	}
	name, _ := v["Packet Name"].(string)
	if name == "" || name == "QUERY_INFO" {
		return "", false
	}
	h, _ := v["Header"].(map[string]any)
	role := "request"
	if v["Response"] == true {
		role = "response"
	}
	parts := []string{"SMB2", name, role}
	body, _ := v["Body"].(map[string]any)
	switch name {
	case "NEGOTIATE":
		if role == "response" {
			if _, ok := body["Dialect"]; ok {
				parts = append(parts, fmt.Sprintf("dialect=0x%04x", number(body["Dialect"])))
			}
		} else if n, ok := smbSummaryInteger(body["Dialect Count"]); ok {
			parts = append(parts, "dialects="+n)
		}
	case "TREE_CONNECT":
		if path, ok := body["Path"].(string); ok && path != "" {
			parts = append(parts, path)
		} else if share, ok := smbSummaryInteger(body["Share Type"]); ok {
			parts = append(parts, "shareType="+share)
		}
	case "READ":
		if n, ok := smbSummaryInteger(body["Length"]); ok && role == "request" {
			parts = append(parts, "len="+n)
		}
		if n, ok := smbSummaryInteger(body["Data Length"]); ok && role == "response" {
			parts = append(parts, "data="+n)
		}
	case "WRITE":
		if n, ok := smbSummaryInteger(body["Length"]); ok && role == "request" {
			parts = append(parts, "len="+n)
		}
		if n, ok := smbSummaryInteger(body["Count"]); ok && role == "response" {
			parts = append(parts, "count="+n)
		}
	case "SESSION_SETUP":
		if auth, ok := v["Authentication"].(map[string]any); ok {
			if mechanism, ok := auth["Mechanism"].(map[string]any); ok {
				if name, ok := mechanism["Message Name"].(string); ok && name != "" {
					parts = append(parts, name)
				}
			}
		}
	case "CREATE":
		if file, ok := body["File Name"].(string); ok && file != "" {
			parts = append(parts, file)
		}
	case "LOGOFF", "TREE_DISCONNECT":
		if v["Error Response"] == true {
			parts = append(parts, "error")
		}
	}
	if status, ok := smbSummaryInteger(h["Status"]); ok && role == "response" && status != "0" {
		parts = append(parts, "status="+status)
	}
	if mid, ok := smbSummaryInteger(h["Message ID"]); ok {
		parts = append(parts, "MID="+mid)
	}
	return safeText(strings.Join(parts, " · ")), true
}

func smbQueryInfoSummary(e *pcaputil.ProtocolEvent) (string, bool) {
	if e == nil || e.Error != "" || e.Status != "decoded" && e.Status != "deferred" || !smbSummaryOwnedRecord(e) {
		return "", false
	}
	v := eventValues(e)
	h, _ := v["Header"].(map[string]any)
	if v["Compound"] == true || v["Packet Name"] != "QUERY_INFO" || number(h["Command"]) != 16 {
		return "", false
	}
	body, _ := v["Body"].(map[string]any)
	role := "request"
	typ, class := number(body["Info Type"]), number(body["File Info Class"])
	if v["Response"] == true {
		role = "response"
		if v["Association Status"] != "matched" {
			return "", false
		}
		typ, class = number(v["Request Info Type"]), number(v["Request File Info Class"])
	}
	if typ > 255 || class > 255 {
		return "", false
	}
	profile, supported := stream_parser.SMB2QueryInfoOutputProfile(uint8(typ), uint8(class))
	if !supported {
		return "", false
	}
	parts := []string{"QUERY_INFO " + role, profile}
	if role == "response" {
		if v["Output Information Type"] != profile {
			return "", false
		}
		fields, ok := body[profile].(map[string]any)
		if !ok {
			return "", false
		}
		switch profile {
		case "FileEaInformation":
			value, ok := smbSummaryInteger(fields["EA Size"])
			if !ok {
				return "", false
			}
			parts = append(parts, "EA size="+value)
		case "FileStandardInformation":
			for _, field := range []struct{ key, label string }{{"End Of File", "EOF"}, {"Allocation Size", "allocation"}, {"Number Of Links", "links"}} {
				value, ok := smbSummaryInteger(fields[field.key])
				if !ok {
					return "", false
				}
				parts = append(parts, field.label+"="+value)
			}
			if directory, ok := fields["Directory State"].(bool); ok {
				parts = append(parts, fmt.Sprintf("directory=%t", directory))
			}
		case "FileNetworkOpenInformation":
			value, ok := smbSummaryInteger(fields["End Of File"])
			if !ok {
				return "", false
			}
			parts = append(parts, "EOF="+value)
		case "FileStreamInformation":
			name, ok := fields["Stream Name"].(string)
			if !ok || name == "" {
				return "", false
			}
			parts = append(parts, "stream="+name)
		case "FileFsVolumeInformation":
			label, ok := fields["Volume Label"].(string)
			if !ok || label == "" {
				return "", false
			}
			parts = append(parts, "volume="+label)
		case "FileFsAttributeInformation":
			name, ok := fields["File System Name"].(string)
			if !ok || name == "" {
				return "", false
			}
			parts = append(parts, "fs="+name)
		case "SecurityDescriptor":
			revision, ok := smbSummaryInteger(fields["Revision"])
			if !ok || fields["Security Body Status"] == nil || fields["Identity Verified"] == true {
				return "", false
			}
			if owner, ok := fields["Owner SID"].(string); ok && owner != "" {
				parts = append(parts, "revision="+revision, "owner="+owner)
			} else {
				parts = append(parts, "revision="+revision, "opaque SID/ACL")
			}
		default:
			return "", false
		}
		if v["Output Content Partial"] == true {
			parts = append(parts, "partial output")
		}
	}
	if mid, ok := smbSummaryInteger(h["Message ID"]); ok {
		parts = append(parts, "MID="+mid)
	}
	return safeText(strings.Join(parts, " · ")), true
}
