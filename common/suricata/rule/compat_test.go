package rule

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
)

func TestParseTargetAndCompatibilityAliases(t *testing.T) {
	rules, err := ParseStrictRule(`
alert tls any any -> any any (msg:"tls aliases"; ja3_hash; content:"abc"; tls_sni; content:"example.com"; endswith; target:src_ip; sid:9700101; rev:1;)
alert dns any any -> any any (msg:"dotprefix"; dns.query; dotprefix; content:".example.com"; content:".example.org"; endswith; sid:9700102; rev:1;)
alert tcp-pkt any any -> any any (msg:"tcp packet"; content:"hello"; sid:9700103; rev:1;)
`)
	require.NoError(t, err)
	require.Len(t, rules, 3)

	tlsRule := rules[0]
	require.Equal(t, "src_ip", tlsRule.Target)
	require.Len(t, tlsRule.ContentRuleConfig.ContentRules, 2)
	require.Equal(t, modifier.JA3Hash, tlsRule.ContentRuleConfig.ContentRules[0].Modifier)
	require.Equal(t, modifier.TLSSNI, tlsRule.ContentRuleConfig.ContentRules[1].Modifier)
	require.True(t, tlsRule.ContentRuleConfig.ContentRules[1].EndsWith)
	require.Empty(t, ValidateRule(tlsRule))

	dnsRule := rules[1]
	require.Equal(t, "dns", dnsRule.Protocol)
	require.Len(t, dnsRule.ContentRuleConfig.ContentRules, 2)
	for _, content := range dnsRule.ContentRuleConfig.ContentRules {
		require.Equal(t, modifier.DNSQuery, content.Modifier, "sticky DNS buffer must survive both content matches")
		require.True(t, content.DotPrefix, "dotprefix must apply to both content matches")
	}
	require.True(t, dnsRule.ContentRuleConfig.ContentRules[1].EndsWith)
	require.Empty(t, ValidateRule(dnsRule))

	require.Equal(t, "tcp-pkt", rules[2].Protocol)
	require.Empty(t, ValidateRule(rules[2]))
}

func TestDotPrefixHTTPHostPersistence(t *testing.T) {
	rules, err := ParseStrictRule(`alert http any any -> any any (msg:"host"; http.host; dotprefix; content:!".surveymonkey.com"; endswith; content:!"cisco.com"; endswith; sid:9700104; rev:1;)`)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	require.Len(t, rules[0].ContentRuleConfig.ContentRules, 2)
	for _, content := range rules[0].ContentRuleConfig.ContentRules {
		require.Equal(t, modifier.HTTPHost, content.Modifier)
		require.True(t, content.DotPrefix)
		require.True(t, content.Negative)
		require.True(t, content.EndsWith)
	}
}

func TestMalformedQuotedRuleDoesNotSwallowFollowingRules(t *testing.T) {
	rules, err := Parse(`
alert http any any -> any any (msg:"unterminated; sid:1;)
alert http any any -> any any (msg:"valid"; content:"good"; sid:2;)
`)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	require.Equal(t, 2, rules[0].Sid)

	_, err = ParseStrictRule(`
alert http any any -> any any (msg:"unterminated; sid:1;)
alert http any any -> any any (msg:"valid"; content:"good"; sid:2;)
`)
	require.Error(t, err)
}

func TestCommentOnlyInputIsEmptyRuleSet(t *testing.T) {
	rules, err := Parse("# license\n// history\n\n")
	require.NoError(t, err)
	require.Empty(t, rules)
	rules, err = ParseStrictRule("# disabled example\n")
	require.NoError(t, err)
	require.Empty(t, rules)
}

func TestEscapedQuotesRemainBalanced(t *testing.T) {
	rules, err := ParseStrictRule(`alert http any any -> any any (msg:"a \"quoted\" message"; content:"good"; sid:3;)`)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	require.Equal(t, `a "quoted" message`, rules[0].Message)
}

func TestPCREInnerQuotesAndAsn1Options(t *testing.T) {
	rules, err := ParseStrictRule(`
alert http any any -> any any (msg:"inner quotes"; pcre:"/^["']\s*:/Ri"; sid:4;)
alert smtp any any -> any any (content:"x|3a 20|\\"; content:"y"; sid:5;)
alert tcp any any -> any any (Content:"hello"; SID:6;)
alert tcp any any -> any any (content:"x"; asn1:double_overflow, relative_offset 54, oversize_length 2048; sid:7;)
`)
	require.NoError(t, err)
	require.Len(t, rules, 4)
	require.Contains(t, rules[0].ContentRuleConfig.ContentRules[0].PCRE, `["']`)
	require.NotNil(t, rules[0].ContentRuleConfig.ContentRules[0].PCREParsed)
	require.Len(t, rules[1].ContentRuleConfig.ContentRules, 2)
	require.Equal(t, "x: \\", string(rules[1].ContentRuleConfig.ContentRules[0].Content))
	require.Equal(t, 6, rules[2].Sid)
	require.Equal(t, []byte("hello"), rules[2].ContentRuleConfig.ContentRules[0].Content)

	asn1 := rules[3].ContentRuleConfig.ASN1
	require.NotNil(t, asn1)
	require.True(t, asn1.DoubleOverflow)
	require.NotNil(t, asn1.RelativeOffset)
	require.Equal(t, 54, *asn1.RelativeOffset)
	require.NotNil(t, asn1.OversizeLength)
	require.EqualValues(t, 2048, *asn1.OversizeLength)
}

func TestValidateTargetAndDotPrefixConstraints(t *testing.T) {
	rules, err := ParseStrictRule(`
alert dns any any -> any any (msg:"invalid target"; dns.query; content:"example.com"; target:host; sid:9700111;)
alert dns any any -> any any (msg:"no sticky buffer"; content:"example.com"; dotprefix; sid:9700112;)
`)
	require.NoError(t, err)
	require.Len(t, rules, 2)

	require.Len(t, ValidateRule(rules[0]), 1)
	require.Equal(t, "target", ValidateRule(rules[0])[0].Keyword)
	require.Len(t, ValidateRule(rules[1]), 1)
	require.Equal(t, "dotprefix", ValidateRule(rules[1])[0].Keyword)
}
