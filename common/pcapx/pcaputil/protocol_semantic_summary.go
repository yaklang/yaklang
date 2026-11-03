package pcaputil

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

const protocolSummaryLimit = 240

func (e *ProtocolEvent) kerberosSummary() string {
	var facts map[string]any
	if metadata, ok := e.Metadata.(map[string]any); ok {
		facts, _ = metadata["Semantic Summary"].(map[string]any)
	}
	if facts == nil {
		var err error
		facts, err = stream_parser.DecodeKerberosSummaryFacts(e.Raw, e.Entry == "KerberosTCPFields")
		if err != nil {
			typeValue := int64(0)
			switch {
			case e.Transport == "tcp" && len(e.Raw) >= 5:
				typeValue = int64(e.Raw[4] & 31)
			case len(e.Raw) != 0:
				typeValue = int64(e.Raw[0] & 31)
			}
			return limitSummary("Kerberos "+kerberosMessageName(typeValue)+" decrypt=unknown identity=unverified", protocolSummaryLimit)
		}
	}
	return formatKerberosSummary(facts)
}

func formatKerberosSummary(facts map[string]any) string {
	if facts == nil {
		return "Kerberos decrypt=unknown identity=unverified"
	}
	name := summaryString(facts["Message Name"])
	if name == "" {
		name = kerberosMessageName(summaryInt64(facts["Message Type"]))
	}
	summary := "Kerberos " + sanitizeSummaryText(name, 48)
	if code, ok := summaryInteger(facts["Error Code"]); ok {
		summary += fmt.Sprintf(" error=%d", code)
	}
	etypes := summaryInt64Slice(facts["Requested Encryption Types"])
	etypes = mergeSummaryIntegers(etypes, summaryInt64Slice(facts["Observed Encryption Types"]))
	if formatted := formatSummaryIntegers(etypes, 4); formatted != "" {
		summary += " etypes=" + formatted
	}
	if count, ok := summaryInteger(facts["Ticket Count"]); ok && count != 0 {
		summary += " ticket=opaque"
	}
	if realm := firstSummaryString(facts, "Realm", "Client Realm", "Ticket Realm"); realm != "" {
		summary += " realm=" + sanitizeSummaryText(realm, 80)
	}
	if client := summaryString(facts["Client Principal"]); client != "" {
		summary += " client=" + sanitizeSummaryText(client, 80)
	}
	if server := summaryString(facts["Server Principal"]); server != "" {
		summary += " server=" + sanitizeSummaryText(server, 80)
	}
	if ticketServer := summaryString(facts["Ticket Server"]); ticketServer != "" && ticketServer != summaryString(facts["Server Principal"]) {
		summary += " ticket_server=" + sanitizeSummaryText(ticketServer, 80)
	}
	const limitations = " decrypt=no identity=unverified"
	summary = limitSummary(summary, protocolSummaryLimit-len([]rune(limitations)))
	return summary + limitations
}

func kerberosMessageName(value int64) string {
	switch value {
	case 10:
		return "AS-REQ"
	case 11:
		return "AS-REP"
	case 12:
		return "TGS-REQ"
	case 13:
		return "TGS-REP"
	case 14:
		return "AP-REQ"
	case 15:
		return "AP-REP"
	case 30:
		return "KRB-ERROR"
	default:
		return fmt.Sprintf("message-%d", value)
	}
}

func ldapSessionSummary(session, fields map[string]any) string {
	if session == nil {
		return "LDAP"
	}
	name := summaryString(session["Message Name"])
	if name == "" {
		return "LDAP"
	}
	name = sanitizeSummaryText(name, 48)
	if name == "SASLProtectedRecord" {
		return limitSummary("LDAP SASLProtectedRecord payload=opaque authentication=unverified", protocolSummaryLimit)
	}
	id, _ := summaryUnsigned(session["Message ID"])
	summary := fmt.Sprintf("LDAP %s id=%d", name, id)

	switch name {
	case "BindRequest":
		choice := summaryString(semanticSummaryField(fields, "Choice"))
		if choice != "" {
			summary += " bind=" + sanitizeSummaryText(choice, 24)
		}
		if target := ldapObservedTarget(fields, name); target != "" {
			summary += " name=" + sanitizeSummaryText(target, 80)
		}
		summary += " credentials=opaque"
	case "BindResponse":
		summary += ldapResultSummary(session)
		if status := summaryString(session["Authentication Status"]); status != "" {
			summary += " auth=" + sanitizeSummaryText(status, 48)
		}
		summary += " identity=unverified"
	case "SearchRequest":
		if target := ldapObservedTarget(fields, name); target != "" {
			summary += " base=" + sanitizeSummaryText(target, 80)
		}
		if scope, ok := summaryUnsigned(semanticSummaryField(fields, "Scope")); ok {
			summary += " scope=" + ldapScopeName(scope)
		}
		if count, ok := semanticSummaryListLen(fields, "Attributes"); ok {
			summary += fmt.Sprintf(" attrs=%d", count)
		}
	case "SearchResultEntry":
		if target := ldapObservedTarget(fields, name); target != "" {
			summary += " object=" + sanitizeSummaryText(target, 80)
		}
		if count, ok := semanticSummaryListLen(fields, "Attributes"); ok {
			summary += fmt.Sprintf(" attrs=%d", count)
		}
	case "SearchResultDone":
		summary += ldapResultSummary(session)
		if session["Search Complete"] == true {
			summary += " complete"
			if entries, ok := summaryUnsigned(session["Entries Returned"]); ok {
				summary += fmt.Sprintf(" entries=%d", entries)
			}
			if references, ok := summaryUnsigned(session["References Returned"]); ok {
				summary += fmt.Sprintf(" refs=%d", references)
			}
		}
	case "ModifyRequest", "AddRequest", "DelRequest", "CompareRequest", "ModifyDNRequest":
		if target := ldapObservedTarget(fields, name); target != "" {
			summary += " target=" + sanitizeSummaryText(target, 80)
		}
	case "AbandonRequest":
		if abandoned, ok := summaryUnsigned(session["Abandoned Message ID"]); ok {
			summary += fmt.Sprintf(" abandoned_id=%d", abandoned)
		}
	case "ExtendedRequest":
		if oid := ldapObservedTarget(fields, name); oid != "" {
			summary += " oid=" + sanitizeSummaryText(oid, 96)
		}
		if session["StartTLS"] == true {
			summary += " starttls=request"
		}
	case "ExtendedResponse":
		summary += ldapResultSummary(session)
		if session["StartTLS"] == true {
			summary += " starttls=accepted-result-observed"
		}
	case "SearchResultReference":
		summary += " references=opaque"
	default:
		if _, present := session["Result Code"]; present {
			summary += ldapResultSummary(session)
		}
	}

	if session["Matched Request"] == true {
		summary += " matched"
	} else if session["Unsolicited"] == true {
		summary += " unsolicited"
	} else if ldapResponseName(name) {
		summary += " unmatched"
	}
	return limitSummary(summary, protocolSummaryLimit)
}

func ldapResultSummary(session map[string]any) string {
	code, ok := summaryUnsigned(session["Result Code"])
	if !ok {
		return " result=unavailable"
	}
	return fmt.Sprintf(" result=%d", code)
}

func ldapObservedTarget(fields map[string]any, name string) string {
	if fields == nil {
		return ""
	}
	keys := []string{"Object Name", "Entry", "Directory Name"}
	switch name {
	case "SearchRequest":
		keys = []string{"Base Object"}
	case "ExtendedRequest":
		keys = []string{"Request Name"}
	}
	for _, key := range keys {
		value := semanticSummaryField(fields, key)
		if value == nil {
			continue
		}
		if text := summaryBytesOrString(value); text != "" {
			return text
		}
		switch name {
		case "SearchRequest", "SearchResultEntry":
			return "<root>"
		case "BindRequest":
			return "<anonymous>"
		}
	}
	return ""
}

func ldapScopeName(scope uint64) string {
	switch scope {
	case 0:
		return "base"
	case 1:
		return "one"
	case 2:
		return "subtree"
	default:
		return fmt.Sprintf("%d", scope)
	}
}

func ldapResponseName(name string) bool {
	switch name {
	case "BindResponse", "SearchResultEntry", "SearchResultDone", "SearchResultReference",
		"ModifyResponse", "AddResponse", "DelResponse", "ModifyDNResponse",
		"CompareResponse", "ExtendedResponse":
		return true
	default:
		return false
	}
}

func semanticSummaryField(value any, name string) any {
	nodes := 4096
	return semanticSummaryFieldBudget(value, name, 0, &nodes)
}

func semanticSummaryFieldBudget(value any, name string, depth int, nodes *int) any {
	if value == nil || depth > 128 || *nodes <= 0 {
		return nil
	}
	*nodes--
	switch current := value.(type) {
	case map[string]any:
		if found, ok := current[name]; ok {
			return found
		}
		for _, child := range current {
			if found := semanticSummaryFieldBudget(child, name, depth+1, nodes); found != nil {
				return found
			}
		}
	case []any:
		for _, child := range current {
			if found := semanticSummaryFieldBudget(child, name, depth+1, nodes); found != nil {
				return found
			}
		}
	case []map[string]any:
		for _, child := range current {
			if found := semanticSummaryFieldBudget(child, name, depth+1, nodes); found != nil {
				return found
			}
		}
	}
	return nil
}

func semanticSummaryListLen(value any, name string) (int, bool) {
	found := semanticSummaryField(value, name)
	switch current := found.(type) {
	case []any:
		return len(current), true
	case []map[string]any:
		return len(current), true
	case nil:
		return 0, false
	default:
		return 1, true
	}
}

func summaryBytesOrString(value any) string {
	switch current := value.(type) {
	case string:
		return current
	case []byte:
		return string(current)
	case map[string]any:
		for _, key := range []string{"Value", "Bytes", "Octets"} {
			if text := summaryBytesOrString(current[key]); text != "" {
				return text
			}
		}
	}
	return ""
}

func summaryString(value any) string {
	switch current := value.(type) {
	case string:
		return current
	case []byte:
		return string(current)
	default:
		return ""
	}
}

func firstSummaryString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := summaryString(values[key]); value != "" {
			return value
		}
	}
	return ""
}

func summaryUnsigned(value any) (uint64, bool) {
	switch current := value.(type) {
	case uint64:
		return current, true
	case uint32:
		return uint64(current), true
	case uint16:
		return uint64(current), true
	case uint8:
		return uint64(current), true
	case uint:
		return uint64(current), true
	case int:
		if current >= 0 {
			return uint64(current), true
		}
	case int64:
		if current >= 0 {
			return uint64(current), true
		}
	case int32:
		if current >= 0 {
			return uint64(current), true
		}
	}
	return 0, false
}

func summaryInteger(value any) (int64, bool) {
	switch current := value.(type) {
	case int:
		return int64(current), true
	case int64:
		return current, true
	case int32:
		return int64(current), true
	case uint:
		if uint64(current) <= 1<<63-1 {
			return int64(current), true
		}
	case uint64:
		if current <= 1<<63-1 {
			return int64(current), true
		}
	case uint32:
		return int64(current), true
	}
	return 0, false
}

func summaryInt64(value any) int64 {
	valueInt, _ := summaryInteger(value)
	return valueInt
}

func summaryInt64Slice(value any) []int64 {
	switch current := value.(type) {
	case []int64:
		return current
	case []int:
		out := make([]int64, len(current))
		for i, item := range current {
			out[i] = int64(item)
		}
		return out
	case []any:
		out := make([]int64, 0, len(current))
		for _, item := range current {
			if number, ok := summaryInteger(item); ok {
				out = append(out, number)
			}
		}
		return out
	default:
		return nil
	}
}

func formatSummaryIntegers(values []int64, maximum int) string {
	if len(values) == 0 || maximum <= 0 {
		return ""
	}
	limit := min(len(values), maximum)
	parts := make([]string, 0, limit+1)
	for _, value := range values[:limit] {
		parts = append(parts, fmt.Sprintf("%d", value))
	}
	if len(values) > limit {
		parts = append(parts, fmt.Sprintf("+%d", len(values)-limit))
	}
	return strings.Join(parts, ",")
}

func mergeSummaryIntegers(groups ...[]int64) []int64 {
	seen := make(map[int64]bool)
	var out []int64
	for _, values := range groups {
		for _, value := range values {
			if seen[value] {
				continue
			}
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}
