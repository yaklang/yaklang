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
	case "dns", "http", "tcp", "udp", "icmp", "tls":
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
	if config.Flow != nil {
		v.add(ValidationStageModel, "flow", "parsed flow constraints require a stateful flow tracker", KeywordStatefulRequired)
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
		v.add(ValidationStageModel, "tls-config", "TLSConfig fields are not attached to the matcher pipeline", KeywordUnsupported)
	}

	protocol := normalizeKeyword(v.rule.Protocol)
	if config.DNS != nil && protocol != "dns" {
		v.add(ValidationStageModel, "dns", "DNS constraints require the dns protocol", KeywordUnsupported)
	}
	if config.TcpConfig != nil && protocol != "tcp" && protocol != "http" && protocol != "tls" {
		v.add(ValidationStageModel, "tcp", "TCP constraints require a TCP-based protocol", KeywordUnsupported)
	}
	if config.IcmpConfig != nil && protocol != "icmp" {
		v.add(ValidationStageModel, "icmp", "ICMP constraints require the icmp protocol", KeywordUnsupported)
	}
	if config.HTTPConfig != nil && protocol != "http" {
		v.add(ValidationStageModel, "http", "HTTP constraints require the http protocol", KeywordUnsupported)
	}

	for _, contentRule := range config.ContentRules {
		if contentRule == nil {
			continue
		}
		if modifier.IsHTTPModifier(contentRule.Modifier) && protocol != "http" {
			v.add(ValidationStageModel, "http-buffer", "HTTP sticky buffers require the http protocol", KeywordUnsupported)
		}
		if contentRule.Modifier == modifier.DNSQuery && protocol != "dns" {
			v.add(ValidationStageModel, "dns.query", "DNS query buffers require the dns protocol", KeywordUnsupported)
		}
		if contentRule.Modifier == modifier.TCPHDR && protocol != "tcp" && protocol != "http" && protocol != "tls" {
			v.add(ValidationStageModel, "tcp.hdr", "TCP header buffers require a TCP-based protocol", KeywordUnsupported)
		}
		if contentRule.Modifier >= modifier.TLSCertSubject && protocol != "tls" {
			v.add(ValidationStageModel, "tls-buffer", "TLS buffers require the tls protocol", KeywordUnsupported)
		}
		if contentRule.Within != nil && *contentRule.Within < 0 {
			v.add(ValidationStageModel, "within", "negative within ranges are not implemented", KeywordUnsupported)
		}
		if contentRule.RawBytes {
			v.add(ValidationStageModel, "rawbytes", "rawbytes does not change the selected payload buffer", KeywordUnsupported)
		}
		for keyword, value := range map[string]string{
			"byte_test":    contentRule.ByteTest,
			"byte_math":    contentRule.ByteMath,
			"byte_jump":    contentRule.ByteJump,
			"byte_extract": contentRule.ByteExtract,
			"rpc":          contentRule.RPC,
		} {
			if value != "" {
				v.add(ValidationStageModel, keyword, "parsed field is not evaluated by the matcher", KeywordUnsupported)
			}
		}
		if len(contentRule.Replace) > 0 {
			v.add(ValidationStageModel, "replace", "payload replacement is not implemented", KeywordUnsupported)
		}
		for keyword, value := range map[string]string{
			"flowbits":      contentRule.FlowBits,
			"flowint":       contentRule.FlowInt,
			"xbits":         contentRule.XBits,
			"base64_decode": contentRule.Base64Decode,
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
		if contentRule.Base64Data {
			v.add(ValidationStageModel, "base64_data", "base64 data buffers are not implemented", KeywordUnsupported)
		}
	}
}
