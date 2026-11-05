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
		keys := []struct{ key, label string }{{"EA Size", "EA size"}}
		if profile == "FileStandardInformation" {
			keys = []struct{ key, label string }{{"End Of File", "EOF"}, {"Allocation Size", "allocation"}, {"Number Of Links", "links"}}
		}
		for _, field := range keys {
			value, ok := smbSummaryInteger(fields[field.key])
			if !ok {
				return "", false
			}
			parts = append(parts, field.label+"="+value)
		}
		if profile == "FileStandardInformation" {
			if directory, ok := fields["Directory State"].(bool); ok {
				parts = append(parts, fmt.Sprintf("directory=%t", directory))
			}
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
