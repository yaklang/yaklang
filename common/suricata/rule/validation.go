package rule

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/suricata/data/modifier"
)

// ValidationStage identifies which layer found a compatibility problem.
type ValidationStage string

const (
	ValidationStageHeader     ValidationStage = "header"
	ValidationStageCapability ValidationStage = "capability"
	ValidationStageModel      ValidationStage = "model"
	ValidationStageExtraFlag  ValidationStage = "extra-flag"
)

// ValidationDiagnostic describes one rule feature that cannot currently be
// evaluated without changing or dropping its matching semantics.
type ValidationDiagnostic struct {
	Stage      ValidationStage         `json:"stage"`
	Keyword    string                  `json:"keyword"`
	SID        int                     `json:"sid"`
	Reason     string                  `json:"reason"`
	Capability KeywordCapabilityStatus `json:"capability,omitempty"`
}

// ValidateRule reports capability and model compatibility problems without
// changing the rule. An empty result means all observed semantics are covered
// by the current matcher.
func ValidateRule(rule *Rule) []ValidationDiagnostic {
	if rule == nil {
		return []ValidationDiagnostic{{
			Stage:  ValidationStageModel,
			Reason: "rule is nil",
		}}
	}

	validator := &ruleValidator{rule: rule, seen: make(map[string]struct{})}
	validator.validateHeader()
	validator.validateKeywords()
	validator.validateModel()
	return validator.diagnostics
}

// ValidateRules validates multiple parsed rules in input order.
func ValidateRules(rules []*Rule) []ValidationDiagnostic {
	var diagnostics []ValidationDiagnostic
	for _, rule := range rules {
		diagnostics = append(diagnostics, ValidateRule(rule)...)
	}
	return diagnostics
}

// ParseAndValidate keeps Parse's tolerant parsing behavior while exposing
// unsupported semantics as structured diagnostics to callers that opt in.
func ParseAndValidate(data string, envs ...string) ([]*Rule, []ValidationDiagnostic, error) {
	rules, err := Parse(data, envs...)
	if err != nil {
		return rules, nil, err
	}
	return rules, ValidateRules(rules), nil
}

// UsesHTTPBuffers reports whether the rule selects HTTP sticky buffers or
// HTTP request/response configuration. Such TCP rules must use the HTTP
// parser instead of the raw TCP parser.
func UsesHTTPBuffers(r *Rule) bool {
	if r == nil || r.ContentRuleConfig == nil {
		return false
	}
	if r.ContentRuleConfig.HTTPConfig != nil {
		return true
	}
	for _, content := range r.ContentRuleConfig.ContentRules {
		if content != nil && modifier.IsHTTPModifier(content.Modifier) {
			return true
		}
	}
	return false
}

// UsesTLSBuffers reports whether the rule selects a TLS certificate, SNI,
// random or JA3 sticky buffer.
func UsesTLSBuffers(r *Rule) bool {
	if r == nil || r.ContentRuleConfig == nil {
		return false
	}
	for _, content := range r.ContentRuleConfig.ContentRules {
		if content != nil && content.Modifier >= modifier.TLSCertSubject && content.Modifier <= modifier.JA3SString {
			return true
		}
	}
	return false
}

// UsesSSHBuffers reports whether the rule selects the SSH protocol banner
// sticky buffer.
func UsesSSHBuffers(r *Rule) bool {
	if r == nil || r.ContentRuleConfig == nil {
		return false
	}
	for _, content := range r.ContentRuleConfig.ContentRules {
		if content != nil && content.Modifier == modifier.SSHProto {
			return true
		}
	}
	return false
}

func UsesDNSQueryBuffer(r *Rule) bool {
	if r == nil || r.ContentRuleConfig == nil {
		return false
	}
	for _, content := range r.ContentRuleConfig.ContentRules {
		if content != nil && content.Modifier == modifier.DNSQuery {
			return true
		}
	}
	return false
}

func UsesFileData(r *Rule) bool {
	if r == nil || r.ContentRuleConfig == nil {
		return false
	}
	for _, content := range r.ContentRuleConfig.ContentRules {
		if content != nil && content.Modifier == modifier.FileData {
			return true
		}
	}
	return false
}

func UsesRawBytes(r *Rule) bool {
	if r == nil || r.ContentRuleConfig == nil {
		return false
	}
	for _, content := range r.ContentRuleConfig.ContentRules {
		if content != nil && content.RawBytes {
			return true
		}
	}
	return false
}

func isTCPBasedProtocol(protocol string) bool {
	switch protocol {
	case "tcp", "tcp-pkt", "http", "tls", "smtp", "ftp", "ftp-data", "smb", "ssh", "dcerpc", "tcp-stream":
		return true
	default:
		return false
	}
}

type ruleValidator struct {
	rule        *Rule
	diagnostics []ValidationDiagnostic
	seen        map[string]struct{}
}

func (v *ruleValidator) add(stage ValidationStage, keyword, reason string, capability KeywordCapabilityStatus) {
	key := fmt.Sprintf("%s\x00%s\x00%s", stage, keyword, reason)
	if _, ok := v.seen[key]; ok {
		return
	}
	v.seen[key] = struct{}{}
	v.diagnostics = append(v.diagnostics, ValidationDiagnostic{
		Stage:      stage,
		Keyword:    keyword,
		SID:        v.rule.Sid,
		Reason:     reason,
		Capability: capability,
	})
}

func (v *ruleValidator) validateHeader() {
	switch normalizeKeyword(v.rule.Action) {
	case "", "alert":
	default:
		v.add(ValidationStageHeader, "action", fmt.Sprintf("action %q is not enforced by the matcher", v.rule.Action), KeywordUnsupported)
	}

	switch normalizeKeyword(v.rule.Protocol) {
	case "ip", "dns", "http", "tcp", "tcp-pkt", "udp", "icmp", "icmpv6", "tls",
		"smtp", "ftp", "ftp-data", "smb", "ssh", "dcerpc", "tcp-stream":
	default:
		v.add(ValidationStageHeader, "protocol", fmt.Sprintf("protocol %q is not supported by the matcher", v.rule.Protocol), KeywordUnsupported)
	}
}

func (v *ruleValidator) validateKeywords() {
	keywords := make([]string, 0, len(v.rule.SettingMap))
	for keyword := range v.rule.SettingMap {
		keywords = append(keywords, keyword)
	}
	sort.Strings(keywords)
	for _, keyword := range keywords {
		capability, ok := LookupKeywordCapability(keyword)
		if !ok {
			v.add(ValidationStageCapability, keyword, "keyword is not present in the capability registry", KeywordUnsupported)
			continue
		}
		if capability.Status != KeywordSupported {
			v.add(ValidationStageCapability, keyword, capability.Reason, capability.Status)
		}
	}

	if v.rule.ContentRuleConfig == nil {
		return
	}
	for _, contentRule := range v.rule.ContentRuleConfig.ContentRules {
		if contentRule == nil {
			continue
		}
		for _, extraFlag := range contentRule.ExtraFlags {
			keyword, _, _ := strings.Cut(extraFlag, ":")
			keyword = strings.TrimSpace(keyword)
			reason := "keyword was preserved in ExtraFlags but was not interpreted by the parser"
			capability := KeywordUnsupported
			if registered, ok := LookupKeywordCapability(keyword); ok {
				capability = registered.Status
				if registered.Reason != "" {
					reason += ": " + registered.Reason
				}
			}
			v.add(ValidationStageExtraFlag, keyword, reason, capability)
		}
	}
}

func (v *ruleValidator) validateModel() {
	config := v.rule.ContentRuleConfig
	if config == nil {
		return
	}
	switch v.rule.Target {
	case "", "src_ip", "dest_ip":
	default:
		v.add(ValidationStageModel, "target", fmt.Sprintf("target %q must be src_ip or dest_ip", v.rule.Target), KeywordUnsupported)
	}
	if config.Tag != nil {
		switch {
		case config.Tag.Kind != "session":
			v.add(ValidationStageModel, "tag", "tag only supports session scope", KeywordUnsupported)
		case config.Tag.Count <= 0:
			v.add(ValidationStageModel, "tag", "tag count must be positive", KeywordUnsupported)
		case config.Tag.Unit != "packets" && config.Tag.Unit != "seconds":
			v.add(ValidationStageModel, "tag", "tag unit must be packets or seconds", KeywordUnsupported)
		case !isTCPBasedProtocol(normalizeKeyword(v.rule.Protocol)):
			v.add(ValidationStageModel, "tag", "tag requires a TCP-based flow", KeywordUnsupported)
		}
	}
	if config.Flow != nil {
		v.add(ValidationStageModel, "flow", "parsed flow constraints require a stateful flow tracker", KeywordStatefulRequired)
		if config.Flow.ToClient && UsesDNSQueryBuffer(v.rule) {
			v.add(ValidationStageModel, "dns.query", "DNS query buffers conflict with flow:to_client", KeywordUnsupported)
		}
	}
	if config.Thresholding != nil {
		keyword := config.Thresholding.Keyword
		if keyword == "" {
			keyword = "threshold"
		}
		v.add(ValidationStageModel, keyword, "parsed threshold constraints require counters shared across packets", KeywordStatefulRequired)
	}
	for keyword, metric := range map[string]*StateMetricRule{
		"stream_size": config.StreamSize,
		"flow.age":    config.FlowAge,
		"flow.pkts":   config.FlowPkts,
		"flow.bytes":  config.FlowBytes,
	} {
		if metric != nil {
			capability, _ := LookupKeywordCapability(keyword)
			v.add(ValidationStageModel, keyword, capability.Reason, capability.Status)
		}
	}
	for _, option := range v.rule.StateOptions {
		if option.FlowBits != nil || option.FlowInt != nil || option.XBits != nil || option.NoAlert {
			capability, ok := LookupKeywordCapability(option.Keyword)
			if ok {
				v.add(ValidationStageModel, option.Keyword, capability.Reason, capability.Status)
			}
		}
	}
	if config.IPConfig != nil && config.IPConfig.Geoip != "" {
		v.add(ValidationStageModel, "geoip", "GeoIP matching requires an external geolocation database", KeywordUnsupported)
	}
	if config.HTTPConfig != nil && config.HTTPConfig.Uricontent != "" {
		v.add(ValidationStageModel, "uricontent", "deprecated uricontent matching is not implemented", KeywordUnsupported)
	}
	if config.TLSConfig != nil {
		if config.TLSConfig.Version != "" {
			switch config.TLSConfig.Version {
			case "1.0", "1.1", "1.2", "1.3":
			default:
				v.add(ValidationStageModel, "tls.version", "tls.version only supports 1.0, 1.1, 1.2 and 1.3", KeywordUnsupported)
			}
		}
		if config.TLSConfig.CertExpired != nil || config.TLSConfig.CertValid != nil ||
			config.TLSConfig.Store || config.TLSConfig.CertSubject != "" ||
			config.TLSConfig.CertIssuer != "" || config.TLSConfig.CertSerial != "" ||
			config.TLSConfig.CertFingerprint != "" || config.TLSConfig.SNI != "" {
			v.add(ValidationStageModel, "tls-config", "the populated TLSConfig field is not evaluated by the matcher", KeywordUnsupported)
		}
	}

	protocol := normalizeKeyword(v.rule.Protocol)
	if config.DNS != nil && protocol != "dns" {
		v.add(ValidationStageModel, "dns", "DNS constraints require the dns protocol", KeywordUnsupported)
	}
	if config.TcpConfig != nil && !isTCPBasedProtocol(protocol) {
		v.add(ValidationStageModel, "tcp", "TCP constraints require a TCP-based protocol", KeywordUnsupported)
	}
	if config.IcmpConfig != nil {
		switch protocol {
		case "icmp":
		case "icmpv6":
			if config.IcmpConfig.ICMPId != nil || config.IcmpConfig.ICMPSeq != nil {
				v.add(ValidationStageModel, "icmp", "icmp_id/icmp_seq require the icmp protocol", KeywordUnsupported)
			}
		default:
			v.add(ValidationStageModel, "icmp", "ICMP constraints require the icmp or icmpv6 protocol", KeywordUnsupported)
		}
	}
	if config.HTTPConfig != nil && protocol != "http" && protocol != "tcp" && protocol != "tcp-pkt" {
		v.add(ValidationStageModel, "http", "HTTP constraints require the http protocol", KeywordUnsupported)
	}
	if config.AppLayerProtocol != "" {
		if config.AppLayerProtocol != "http" {
			v.add(ValidationStageModel, "app-layer-protocol", "app-layer-protocol only supports http", KeywordUnsupported)
		}
		if protocol != "http" && protocol != "tcp" && protocol != "tcp-pkt" {
			v.add(ValidationStageModel, "app-layer-protocol", "app-layer-protocol requires the http, tcp or tcp-pkt protocol", KeywordUnsupported)
		}
	}
	if len(config.AppLayerEvents) > 0 {
		if protocol != "http" && protocol != "tcp" && protocol != "tcp-pkt" {
			v.add(ValidationStageModel, "app-layer-event", "app-layer-event requires an HTTP-capable protocol", KeywordUnsupported)
		}
		for _, event := range config.AppLayerEvents {
			switch event.Name {
			case "http.invalid_transfer_encoding_value_in_request":
			default:
				v.add(ValidationStageModel, "app-layer-event", fmt.Sprintf("app-layer-event %q is not implemented", event.Name), KeywordUnsupported)
			}
		}
	}
	if config.FTPBounce && protocol != "tcp" && protocol != "ftp" {
		v.add(ValidationStageModel, "ftpbounce", "ftpbounce requires the tcp or ftp protocol", KeywordUnsupported)
	}

	for _, contentRule := range config.ContentRules {
		if contentRule == nil {
			continue
		}
		if modifier.IsHTTPModifier(contentRule.Modifier) && protocol != "http" && protocol != "tcp" && protocol != "tcp-pkt" {
			v.add(ValidationStageModel, "http-buffer", "HTTP sticky buffers require the http protocol", KeywordUnsupported)
		}
		if (protocol == "tcp" || protocol == "tcp-pkt") && UsesHTTPBuffers(v.rule) && contentRule.Modifier == modifier.TCPHDR {
			v.add(ValidationStageModel, "tcp.hdr", "TCP header buffers cannot be combined with HTTP sticky buffers", KeywordUnsupported)
		}
		if contentRule.Modifier == modifier.DNSQuery && protocol != "dns" {
			v.add(ValidationStageModel, "dns.query", "DNS query buffers require the dns protocol", KeywordUnsupported)
		}
		if contentRule.Modifier == modifier.UDPHDR && protocol != "udp" {
			v.add(ValidationStageModel, "udp.hdr", "UDP header buffers require the udp protocol", KeywordUnsupported)
		}
		if contentRule.Modifier == modifier.ICMPV4HDR && protocol != "icmp" {
			v.add(ValidationStageModel, "icmpv4.hdr", "ICMPv4 header buffers require the icmp protocol", KeywordUnsupported)
		}
		if contentRule.Modifier == modifier.ICMPV6HDR && protocol != "icmpv6" {
			v.add(ValidationStageModel, "icmpv6.hdr", "ICMPv6 header buffers require the icmpv6 protocol", KeywordUnsupported)
		}
		if contentRule.Modifier == modifier.FileData {
			switch protocol {
			case "http", "tcp", "tcp-pkt", "smtp":
			default:
				v.add(ValidationStageModel, "file_data", "file_data requires HTTP, TCP payload or SMTP DATA body matching", KeywordUnsupported)
			}
		}
		if contentRule.Modifier == modifier.TCPHDR && !isTCPBasedProtocol(protocol) {
			v.add(ValidationStageModel, "tcp.hdr", "TCP header buffers require a TCP-based protocol", KeywordUnsupported)
		}
		if contentRule.Modifier == modifier.SSHProto && protocol != "ssh" && protocol != "tcp" && protocol != "tcp-pkt" {
			v.add(ValidationStageModel, "ssh.proto", "SSH protocol buffers require the ssh, tcp or tcp-pkt protocol", KeywordUnsupported)
		}
		if contentRule.Modifier >= modifier.TLSCertSubject && contentRule.Modifier <= modifier.JA3SString &&
			protocol != "tls" && protocol != "tcp" && protocol != "tcp-pkt" {
			v.add(ValidationStageModel, "tls-buffer", "TLS buffers require the tls, tcp or tcp-pkt protocol", KeywordUnsupported)
		}
		if (protocol == "tcp" || protocol == "tcp-pkt") && UsesHTTPBuffers(v.rule) && UsesTLSBuffers(v.rule) {
			v.add(ValidationStageModel, "tls-buffer", "HTTP and TLS sticky buffers cannot be combined in one TCP rule", KeywordUnsupported)
		}
		if contentRule.DotPrefix && contentRule.Modifier == modifier.Default {
			v.add(ValidationStageModel, "dotprefix", "dotprefix requires a sticky buffer", KeywordUnsupported)
		}
		if contentRule.Within != nil && *contentRule.Within < 0 {
			v.add(ValidationStageModel, "within", "negative within ranges are not implemented", KeywordUnsupported)
		}
		for keyword, value := range map[string]string{
			"byte_math": contentRule.ByteMath,
			"rpc":       contentRule.RPC,
		} {
			if value != "" {
				v.add(ValidationStageModel, keyword, "parsed field is not evaluated by the matcher", KeywordUnsupported)
			}
		}
		if len(contentRule.Replace) > 0 {
			v.add(ValidationStageModel, "replace", "payload replacement is not implemented", KeywordUnsupported)
		}
		for keyword, value := range map[string]string{
			"flowbits": contentRule.FlowBits,
			"flowint":  contentRule.FlowInt,
			"xbits":    contentRule.XBits,
		} {
			if value == "" {
				continue
			}
			capability, _ := LookupKeywordCapability(keyword)
			v.add(ValidationStageModel, keyword, capability.Reason, capability.Status)
		}
		if contentRule.NoAlert {
			capability, _ := LookupKeywordCapability("noalert")
			v.add(ValidationStageModel, "noalert", capability.Reason, capability.Status)
		}
	}
}
