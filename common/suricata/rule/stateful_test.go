package rule

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseFlowTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want FlowRule
	}{
		{name: "aliases to server", raw: "to_server,from_client,established,only_stream", want: FlowRule{ToServer: true, Established: true, OnlyStream: true}},
		{name: "aliases to client", raw: "to_client,from_server,not_established,no_stream", want: FlowRule{ToClient: true, NotEstablished: true, NoStream: true}},
		{name: "stateless", raw: "stateless,to_server", want: FlowRule{Stateless: true, ToServer: true}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			rules, err := ParseStrictRule(`alert tcp any any -> any any (flow:` + test.raw + `; sid:1;)`)
			require.NoError(t, err)
			require.Len(t, rules, 1)
			flow := rules[0].ContentRuleConfig.Flow
			require.NotNil(t, flow)
			require.Equal(t, test.want.ToServer, flow.ToServer)
			require.Equal(t, test.want.ToClient, flow.ToClient)
			require.Equal(t, test.want.Established, flow.Established)
			require.Equal(t, test.want.NotEstablished, flow.NotEstablished)
			require.Equal(t, test.want.Stateless, flow.Stateless)
			require.Equal(t, test.want.OnlyStream, flow.OnlyStream)
			require.Equal(t, test.want.NoStream, flow.NoStream)
			require.Equal(t, len(parseCommaParts(test.raw)), len(flow.Tokens))
		})
	}
}

func TestParseFlowRejectsConflicts(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"to_server,to_client",
		"established,not_established",
		"stateless,established",
		"only_stream,no_stream",
		"to_server,made_up",
	} {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			rules, err := ParseStrictRule(`alert tcp any any -> any any (flow:` + raw + `; sid:1;)`)
			require.Error(t, err)
			require.Empty(t, rules)
		})
	}
}

func TestStateOptionsPreserveOrderAndStructure(t *testing.T) {
	t.Parallel()

	input := `alert tcp any any -> any any (` +
		`flow:to_server,established; ` +
		`flowbits:set,alpha; ` +
		`flowint:counter,+,1; ` +
		`xbits:set,seen,track ip_src,expire 60; ` +
		`flowbits:isnotset,beta; ` +
		`flowbits:noalert; ` +
		`noalert; sid:7;)`
	rules, err := ParseStrictRule(input)
	require.NoError(t, err)
	require.Len(t, rules, 1)

	options := rules[0].StateOptions
	require.Len(t, options, 7)
	require.Equal(t, []string{"flow", "flowbits", "flowint", "xbits", "flowbits", "flowbits", "noalert"}, stateOptionKeywords(options))
	require.Equal(t, FlowBitsSet, options[1].FlowBits.Operation)
	require.Equal(t, "alpha", options[1].FlowBits.Name)
	require.Equal(t, "counter", options[2].FlowInt.Name)
	require.Equal(t, "+", options[2].FlowInt.Operation)
	require.Equal(t, []string{"1"}, options[2].FlowInt.Arguments)
	require.Equal(t, "set", options[3].XBits.Operation)
	require.Equal(t, "seen", options[3].XBits.Name)
	require.Equal(t, "ip_src", options[3].XBits.Track)
	require.NotNil(t, options[3].XBits.Expire)
	require.Equal(t, 60, *options[3].XBits.Expire)
	require.Equal(t, FlowBitsIsNotSet, options[4].FlowBits.Operation)
	require.Equal(t, FlowBitsNoAlert, options[5].FlowBits.Operation)
	require.True(t, options[6].NoAlert)
}

func TestStateOnlyRuleIsPreserved(t *testing.T) {
	t.Parallel()

	rules, err := ParseStrictRule(`alert tcp any any -> any any (flowbits:noalert; noalert; sid:8;)`)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	require.Len(t, rules[0].StateOptions, 2)
	require.Equal(t, FlowBitsNoAlert, rules[0].StateOptions[0].FlowBits.Operation)
	require.True(t, rules[0].StateOptions[1].NoAlert)

	diagnostics := ValidateRule(rules[0])
	require.NotNil(t, findDiagnostic(diagnostics, ValidationStageCapability, "flowbits"))
	noAlert := findDiagnostic(diagnostics, ValidationStageCapability, "noalert")
	require.NotNil(t, noAlert)
	require.Equal(t, KeywordStatefulRequired, noAlert.Capability)
}

func TestParseFlowBitsOperations(t *testing.T) {
	t.Parallel()

	for _, operation := range []FlowBitsOperation{
		FlowBitsSet, FlowBitsUnset, FlowBitsToggle, FlowBitsIsSet, FlowBitsIsNotSet,
	} {
		operation := operation
		t.Run(string(operation), func(t *testing.T) {
			rules, err := ParseStrictRule(`alert tcp any any -> any any (flowbits:` + string(operation) + `,test.bit; sid:1;)`)
			require.NoError(t, err)
			require.Len(t, rules, 1)
			require.Len(t, rules[0].StateOptions, 1)
			require.Equal(t, operation, rules[0].StateOptions[0].FlowBits.Operation)
			require.Equal(t, "test.bit", rules[0].StateOptions[0].FlowBits.Name)
		})
	}
}

func TestParseThresholdAndDetectionFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		keyword string
		value   string
		typeVal string
		limit   bool
		thresh  bool
	}{
		{name: "threshold both", keyword: "threshold", value: "type both, track by_src, count 5, seconds 60", typeVal: "both", limit: true, thresh: true},
		{name: "threshold limit", keyword: "threshold", value: "type limit,track by_dst,count 1,seconds 600", typeVal: "limit", limit: true},
		{name: "detection filter", keyword: "detection_filter", value: "track by_dst, count 4, seconds 20"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			rules, err := ParseStrictRule(`alert tcp any any -> any any (` + test.keyword + `:` + test.value + `; sid:9;)`)
			require.NoError(t, err)
			config := rules[0].ContentRuleConfig.Thresholding
			require.NotNil(t, config)
			require.Equal(t, test.keyword, config.Keyword)
			require.Equal(t, test.typeVal, config.Type)
			require.Equal(t, test.limit, config.LimitMode)
			require.Equal(t, test.thresh, config.ThresholdMode)
			require.NotZero(t, config.Count)
			require.NotZero(t, config.Seconds)
			require.NotEmpty(t, config.Track)
			require.Equal(t, config, rules[0].StateOptions[0].Threshold)
		})
	}
}

func TestParseThresholdRejectsMalformedFields(t *testing.T) {
	t.Parallel()

	for _, option := range []string{
		`threshold:track by_src,count 1,seconds 10`,
		`threshold:type unknown,track by_src,count 1,seconds 10`,
		`threshold:type limit,track nowhere,count 1,seconds 10`,
		`detection_filter:type limit,track by_src,count 1,seconds 10`,
		`detection_filter:track by_src,count nope,seconds 10`,
	} {
		option := option
		t.Run(option, func(t *testing.T) {
			rules, err := ParseStrictRule(`alert tcp any any -> any any (` + option + `; sid:1;)`)
			require.Error(t, err)
			require.Empty(t, rules)
		})
	}
}

func TestParseStateMetrics(t *testing.T) {
	t.Parallel()

	input := `alert tcp any any -> any any (` +
		`stream_size:client,>,6; flow.age:>60; flow.pkts:server,10-20; flow.bytes:!0; sid:10;)`
	rules, err := ParseStrictRule(input)
	require.NoError(t, err)
	require.Len(t, rules, 1)

	config := rules[0].ContentRuleConfig
	require.Equal(t, "client", config.StreamSize.Direction)
	require.True(t, config.StreamSize.Range.Match(7))
	require.True(t, config.FlowAge.Range.Match(61))
	require.Equal(t, "server", config.FlowPkts.Direction)
	require.True(t, config.FlowPkts.Range.Match(15))
	require.True(t, config.FlowBytes.Range.Match(1))
	require.Len(t, rules[0].StateOptions, 4)

	for _, keyword := range []string{"stream_size", "flow.age", "flow.pkts", "flow.bytes"} {
		capability, ok := LookupKeywordCapability(keyword)
		require.True(t, ok)
		require.Equal(t, KeywordStatefulRequired, capability.Status)
		require.NotNil(t, findDiagnostic(ValidateRule(rules[0]), ValidationStageCapability, keyword))
	}
}

func FuzzParseStateOptions(f *testing.F) {
	for _, seed := range []string{
		`alert tcp any any -> any any (flow:to_server,established; sid:1;)`,
		`alert tcp any any -> any any (flowbits:set,seed; noalert; sid:2;)`,
		`alert tcp any any -> any any (threshold:type both,track by_src,count 2,seconds 5; sid:3;)`,
		`alert tcp any any -> any any (detection_filter:track by_dst,count 4,seconds 20; sid:4;)`,
		`alert tcp any any -> any any (stream_size:client,>,6; flow.age:>60; sid:5;)`,
		`alert tcp any any -> any any (xbits:set,seen,track ip_src,expire 60; sid:6;)`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		_, _ = ParseStrictRule(input)
	})
}

func stateOptionKeywords(options []StateOption) []string {
	keywords := make([]string, len(options))
	for i := range options {
		keywords[i] = options[i].Keyword
	}
	return keywords
}
