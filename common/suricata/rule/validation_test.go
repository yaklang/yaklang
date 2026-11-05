package rule

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKeywordCapabilityRegistry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		keyword string
		status  KeywordCapabilityStatus
	}{
		{keyword: "content", status: KeywordSupported},
		{keyword: "HTTP.URI", status: KeywordSupported},
		{keyword: "flowbits", status: KeywordStatefulRequired},
		{keyword: "threshold", status: KeywordStatefulRequired},
		{keyword: "byte_test", status: KeywordUnsupported},
		{keyword: "detection_filter", status: KeywordStatefulRequired},
		{keyword: "dataset", status: KeywordUnsupported},
		{keyword: "tls.sni", status: KeywordUnsupported},
	}
	for _, test := range tests {
		test := test
		t.Run(test.keyword, func(t *testing.T) {
			capability, ok := LookupKeywordCapability(test.keyword)
			require.True(t, ok)
			require.Equal(t, test.status, capability.Status)
			require.NotEmpty(t, capability.Canonical)
		})
	}

	capabilities := KeywordCapabilities()
	require.NotEmpty(t, capabilities)
	encoded, err := json.Marshal(capabilities)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"status":"stateful-required"`)

	capabilities[0].Status = "mutated"
	original, ok := LookupKeywordCapability(capabilities[0].Keyword)
	require.True(t, ok)
	require.NotEqual(t, KeywordCapabilityStatus("mutated"), original.Status)
}

func TestVisitorKeywordsHaveCapabilities(t *testing.T) {
	t.Parallel()

	keywords := []string{
		"sid", "rev", "gid", "classtype", "reference", "msg", "priority", "metadata",
		"content", "dns.opcode", "dns_opcode", "flow", "ttl", "sameip", "ipopts", "ip_proto",
		"id", "geoip", "fragbits", "fragoffset", "tos", "flags", "seq", "ack", "window",
		"threshold", "icode", "itype", "icmp_id", "icmp_seq", "urilen", "nocase", "depth",
		"offset", "startswith", "endswith", "distance", "within", "rawbytes", "isdataat",
		"bsize", "dsize", "byte_test", "byte_math", "byte_extract", "byte_jump", "rpc",
		"replace", "pcre", "tcp.mss", "fast_pattern", "flowbits", "noalert", "base64_decode",
		"base64_data", "flowint", "xbits", "stream_size", "flow.age", "flow.pkts", "flow.bytes",
		"detection_filter", "app-layer-event", "file_data", "http_uri",
		"http.header", "dns.query", "ja3.hash", "tls.cert_subject", "ipv4.hdr", "tcp.hdr",
	}
	for _, keyword := range keywords {
		_, ok := LookupKeywordCapability(keyword)
		require.Truef(t, ok, "missing capability for visitor keyword %q", keyword)
	}
}

func TestParseAndValidateReportsStructuredDiagnostics(t *testing.T) {
	t.Parallel()

	input := `alert tcp any any -> any any (msg:"validation"; flow:established,to_server; content:"abc"; byte_test:1,=,1,0; mystery_keyword:value; sid:4242;)`
	rules, diagnostics, err := ParseAndValidate(input)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	require.NotEmpty(t, diagnostics)

	flow := findDiagnostic(diagnostics, ValidationStageCapability, "flow")
	require.NotNil(t, flow)
	require.Equal(t, 4242, flow.SID)
	require.Equal(t, KeywordStatefulRequired, flow.Capability)
	require.NotEmpty(t, flow.Reason)

	byteTest := findDiagnostic(diagnostics, ValidationStageCapability, "byte_test")
	require.NotNil(t, byteTest)
	require.Equal(t, KeywordUnsupported, byteTest.Capability)

	unknown := findDiagnostic(diagnostics, ValidationStageCapability, "mystery_keyword")
	require.NotNil(t, unknown)
	require.Equal(t, 4242, unknown.SID)
	require.Contains(t, unknown.Reason, "not present")

	extra := findDiagnostic(diagnostics, ValidationStageExtraFlag, "mystery_keyword")
	require.NotNil(t, extra)
	require.Contains(t, extra.Reason, "ExtraFlags")
}

func TestValidateRuleDetectsUnimplementedModelFields(t *testing.T) {
	t.Parallel()

	rule := &Rule{
		Action:   "alert",
		Protocol: "tcp",
		Sid:      77,
		ContentRuleConfig: &ContentRuleConfig{
			ContentRules: []*ContentRule{{
				Content:    []byte("abc"),
				FlowBits:   "isset,test",
				ExtraFlags: []string{"made_up:value"},
			}},
		},
	}

	diagnostics := ValidateRule(rule)
	for _, expected := range []struct {
		stage   ValidationStage
		keyword string
	}{
		{stage: ValidationStageModel, keyword: "flowbits"},
		{stage: ValidationStageExtraFlag, keyword: "made_up"},
	} {
		diagnostic := findDiagnostic(diagnostics, expected.stage, expected.keyword)
		require.NotNilf(t, diagnostic, "missing diagnostic for %s/%s", expected.stage, expected.keyword)
		require.Equal(t, 77, diagnostic.SID)
		require.NotEmpty(t, diagnostic.Reason)
	}
}

func TestParseRemainsTolerant(t *testing.T) {
	t.Parallel()

	input := `alert tcp any any -> any any (msg:"compat"; content:"abc"; unsupported_option:value; sid:99;)`
	rules, err := Parse(input)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	require.Equal(t, 99, rules[0].Sid)

	diagnostics := ValidateRule(rules[0])
	require.NotNil(t, findDiagnostic(diagnostics, ValidationStageCapability, "unsupported_option"))
}

func findDiagnostic(diagnostics []ValidationDiagnostic, stage ValidationStage, keyword string) *ValidationDiagnostic {
	for i := range diagnostics {
		if diagnostics[i].Stage == stage && diagnostics[i].Keyword == keyword {
			return &diagnostics[i]
		}
	}
	return nil
}
