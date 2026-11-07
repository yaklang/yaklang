package match

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx"
	"github.com/yaklang/yaklang/common/suricata/rule"
)

func TestRuleHeaderDirectionAndVariables(t *testing.T) {
	forward := buildHeaderTCPPacket(t, "192.168.1.10", "8.8.8.8", 40000, 80, "syn", "yak")
	reverse := buildHeaderTCPPacket(t, "8.8.8.8", "192.168.1.10", 80, 40000, "syn", "yak")
	wrongPort := buildHeaderTCPPacket(t, "192.168.1.10", "8.8.8.8", 40000, 81, "syn", "yak")

	rules, err := rule.Parse(`alert tcp $HOME_NET any <> $EXTERNAL_NET $HTTP_PORTS (content:"yak"; sid:1;)`)
	require.NoError(t, err)
	require.Equal(t, "<>", rules[0].Direction)
	matcher, err := CompileRule(rules[0])
	require.NoError(t, err)
	require.True(t, matcher.Match(forward))
	require.True(t, matcher.Match(reverse))
	require.False(t, matcher.Match(wrongPort))

	oneWay, err := rule.Parse(`alert tcp $HOME_NET any -> $EXTERNAL_NET $HTTP_PORTS (content:"yak"; sid:2;)`)
	require.NoError(t, err)
	matcher, err = CompileRule(oneWay[0])
	require.NoError(t, err)
	require.True(t, matcher.Match(forward))
	require.False(t, matcher.Match(reverse))
}

func TestIPAndTCPHeaderFields(t *testing.T) {
	packet := buildHeaderTCPPacket(t, "10.0.0.1", "10.0.0.1", 12345, 443, "syn", "yak")
	matching, err := rule.Parse(`alert tcp any any -> any any (ttl:64; sameip; ip_proto:tcp; id:1; tos:0; fragoffset:0; flags:S; content:"yak"; sid:3;)`)
	require.NoError(t, err)
	matcher, err := CompileRule(matching[0])
	require.NoError(t, err)
	require.True(t, matcher.Match(packet))

	notMatching, err := rule.Parse(`alert tcp any any -> any any (ttl:63; flags:A; content:"yak"; sid:4;)`)
	require.NoError(t, err)
	matcher, err = CompileRule(notMatching[0])
	require.NoError(t, err)
	require.False(t, matcher.Match(packet))
}

func TestUnknownVariablesFailClosed(t *testing.T) {
	packet := buildHeaderTCPPacket(t, "192.168.1.10", "8.8.8.8", 40000, 80, "syn", "yak")
	rules, err := rule.Parse(`alert tcp $UNKNOWN_NET $UNKNOWN_PORT -> any any (content:"yak"; sid:5;)`)
	require.NoError(t, err)
	matcher, err := CompileRule(rules[0])
	require.NoError(t, err)
	require.False(t, matcher.Match(packet))
}

func TestHTTPUriLen(t *testing.T) {
	packet := buildHeaderTCPPacket(t, "192.168.1.10", "8.8.8.8", 40000, 80, "ack", "GET /abc HTTP/1.1\r\nHost: example.com\r\n\r\n")
	rules, err := rule.Parse(`alert http any any -> any any (urilen:4; sid:6;)`)
	require.NoError(t, err)
	matcher, err := CompileRule(rules[0])
	require.NoError(t, err)
	require.True(t, matcher.Match(packet))
}

func TestCompileRuleRejectsUnsupportedSemantics(t *testing.T) {
	rules, err := rule.Parse(`alert tcp any any -> any any (flow:established; content:"yak"; flowbits:isset,test; sid:99;)`)
	require.NoError(t, err)
	_, err = CompileRule(rules[0])
	var compileErr *RuleCompileError
	require.ErrorAs(t, err, &compileErr)
	require.NotEmpty(t, compileErr.Diagnostics)
}

func buildHeaderTCPPacket(t *testing.T, src, dst string, srcPort, dstPort int, flags, payload string) []byte {
	t.Helper()
	packet, err := pcapx.PacketBuilder(
		pcapx.WithEthernet_NextLayerType("ip"),
		pcapx.WithEthernet_SrcMac("00:11:22:33:44:55"),
		pcapx.WithEthernet_DstMac("00:11:22:33:44:66"),
		pcapx.WithIPv4_SrcIP(src),
		pcapx.WithIPv4_DstIP(dst),
		pcapx.WithIPv4_ID(1),
		pcapx.WithIPv4_TTL(64),
		pcapx.WithTCP_SrcPort(srcPort),
		pcapx.WithTCP_DstPort(dstPort),
		pcapx.WithTCP_Flags(flags),
		pcapx.WithPayload([]byte(payload)),
	)
	require.NoError(t, err)
	return packet
}
