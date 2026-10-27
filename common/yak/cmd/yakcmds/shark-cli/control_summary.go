package sharkcli

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// Summaries project already accepted fields. They never parse base64, infer a
// method from a port or promote an incomplete observation to a complete PDU.
func mailRTSPEventSummary(e *pcaputil.ProtocolEvent) (string, bool) {
	if e.Error != "" || e.Status != "decoded" && e.Status != "deferred" {
		return "", false
	}
	v := eventValues(e)
	if e.Protocol == "imap" {
		tag := textValue(v["Tag"])
		parts := []string{}
		switch textValue(v["Profile"]) {
		case "imap-sasl-observed":
			parts = append(parts, "IMAP SASL "+textValue(v["Mechanism"]), textValue(v["SASL Stage"]))
			if result := textValue(v["Server Result Observed"]); result != "" {
				parts = append(parts, "server="+result)
			}
		case "imap-idle-observed":
			parts = append(parts, "IMAP IDLE", textValue(v["IDLE Stage"]))
			if result := textValue(v["Server Result Observed"]); result != "" {
				parts = append(parts, "server="+result)
			}
		case "imap-namespace-observed":
			parts = append(parts, "IMAP NAMESPACE", textValue(v["Namespace Stage"]))
			if textValue(v["Namespace Stage"]) == "response" {
				if fields, ok := v["IMAP Fields"].(map[string]any); ok {
					for _, group := range []struct{ key, label string }{{"Personal Namespaces", "personal"}, {"Other Users Namespaces", "other"}, {"Shared Namespaces", "shared"}} {
						if value, ok := fields[group.key].(map[string]any); ok {
							parts = append(parts, fmt.Sprintf("%s=%d", group.label, len(mapList(value["Entries"]))))
						}
					}
				}
			}
			if v["Association"] == "ambiguous-outstanding-namespace" {
				parts = append(parts, "request association ambiguous")
			}
		default:
			return "", false
		}
		if tag != "" {
			parts = append(parts, "tag="+tag)
		}
		return safeText(strings.Join(parts, " · ")), true
	}
	if e.Protocol == "rtsp" {
		method := textValue(v["Packet Name"])
		if method == "" {
			return "", false
		}
		if _, seen := v["CSeq"]; !seen {
			return "", false
		}
		parts := []string{}
		if method == "Response" {
			parts = append(parts, fmt.Sprintf("RTSP %d CSeq=%d", number(v["Status"]), number(v["CSeq"])))
			if request := textValue(v["In Reply To"]); request != "" {
				parts = append(parts, "reply to "+request)
			}
		} else {
			parts = append(parts, fmt.Sprintf("RTSP %s CSeq=%d", method, number(v["CSeq"])))
			if target := textValue(v["URI"]); target != "" {
				parts = append(parts, clipText(target, 120))
			}
			if method == "FLUSH" {
				if values := mapList(v["RTP Info"]); len(values) == 1 {
					for _, key := range []string{"seq", "rtptime"} {
						if value, seen := values[0][key]; seen {
							parts = append(parts, fmt.Sprintf("%s=%d", key, number(value)))
						}
					}
				}
			}
		}
		return safeText(strings.Join(parts, " · ")), true
	}
	return "", false
}
