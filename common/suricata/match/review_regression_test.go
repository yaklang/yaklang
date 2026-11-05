package match

import (
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/suricata/rule"
)

func TestCompileRuleRejectsProtocolMismatch(t *testing.T) {
	for _, raw := range []string{
		`alert udp any any -> any any (flags:S; sid:10;)`,
		`alert tcp any any -> any any (dns.opcode:0; sid:11;)`,
		`alert udp any any -> any any (http.uri; content:"/"; sid:12;)`,
	} {
		rules, err := rule.Parse(raw)
		require.NoError(t, err)
		_, err = CompileRule(rules[0])
		require.Error(t, err)
	}
}

func TestStrictParserRejectsInvalidNumbers(t *testing.T) {
	for _, raw := range []string{
		`alert icmp any any -> any any (itype:bogus; sid:20;)`,
		`alert tcp any any -> any any (seq:bogus; sid:21;)`,
		`alert http any any -> any any (urilen:bogus; sid:22;)`,
		`alert tcp any any -> any any (content:"x"; offset:bogus; sid:23;)`,
	} {
		_, err := rule.ParseStrictRule(raw)
		require.Error(t, err)
	}
}

func TestStandaloneIsDataAt(t *testing.T) {
	packet := buildTCPPacket(t, []byte("abc"))
	require.True(t, compileTestRule(t, `alert tcp any any -> any any (isdataat:2; sid:30;)`).MatchPackage(packet))
	require.False(t, compileTestRule(t, `alert tcp any any -> any any (isdataat:3; sid:31;)`).MatchPackage(packet))
	require.True(t, compileTestRule(t, `alert tcp any any -> any any (isdataat:!3; sid:32;)`).MatchPackage(packet))
}

func TestNegativeWithinRejected(t *testing.T) {
	rules, err := rule.Parse(`alert tcp any any -> any any (content:"abc"; content:"XYZ"; within:-1; sid:33;)`)
	require.NoError(t, err)
	_, err = CompileRule(rules[0])
	require.Error(t, err)
}

func TestStickyBufferPCRE(t *testing.T) {
	packet := buildTCPPacket(t, []byte("GET /admin HTTP/1.1\r\nHost: example.com\r\n\r\n"))
	matcher := compileTestRule(t, `alert http any any -> any any (http.uri; pcre:"/^\/admin$/"; sid:40;)`)
	require.True(t, matcher.MatchPackage(packet))
}

func TestUnsupportedTLSBuffersRejected(t *testing.T) {
	for _, keyword := range []string{"ja3s.hash", "ja3s.string", "tls.cert_subject"} {
		rules, err := rule.Parse(`alert tls any any -> any any (` + keyword + `; content:"x"; sid:50;)`)
		require.NoError(t, err)
		_, err = CompileRule(rules[0])
		require.Error(t, err)
	}
}

func TestGroupCallbackMayClose(t *testing.T) {
	var group *Group
	closed := make(chan struct{})
	var once sync.Once
	group = NewGroup(WithGroupOnMatchedCallback(func(gopacket.Packet, *rule.Rule) {
		once.Do(func() {
			group.Close()
			close(closed)
		})
	}))
	rules, err := rule.Parse(`alert http any any -> any any (content:"close-me"; http.header; sid:60;)`)
	require.NoError(t, err)
	require.NoError(t, group.LoadRuleWithError(rules[0]))
	group.FeedHTTPRequestBytes([]byte("GET / HTTP/1.1\r\nX-Test: close-me\r\n\r\n"))
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("callback Close deadlocked")
	}
}
