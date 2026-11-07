package engine

import (
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/suricata/match"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/suricata/state"
)

// TestEngineCompilationContainsMatcher locks the merge invariant: every rule
// the legacy stateless matcher can execute must also compile in the engine.
func TestEngineCompilationContainsMatcher(t *testing.T) {
	data, err := os.ReadFile("../rules/emerging-scan.rules")
	require.NoError(t, err)
	rules, err := rule.Parse(string(data))
	require.NoError(t, err)

	matcherExecutable, engineExecutable := 0, 0
	for _, current := range rules {
		_, matcherErr := match.CompileRule(current)
		_, engineErr := CompileStatefulRule(current)
		if matcherErr == nil {
			matcherExecutable++
			require.NoErrorf(t, engineErr, "engine rejected matcher rule sid=%d", current.Sid)
		}
		if engineErr == nil {
			engineExecutable++
		}
	}
	require.GreaterOrEqual(t, engineExecutable, matcherExecutable)
	t.Logf("matcher_executable=%d engine_executable=%d", matcherExecutable, engineExecutable)
}

func TestEngineCompilationCoversMatcherProtocols(t *testing.T) {
	cases := []string{
		`alert tcp any any -> any any (msg:"tcp"; content:"x"; sid:9310001;)`,
		`alert udp any any -> any any (msg:"udp"; content:"x"; sid:9310002;)`,
		`alert icmp any any -> any any (msg:"icmp"; itype:8; sid:9310003;)`,
		`alert dns any any -> any any (msg:"dns"; dns_query; content:"example.com"; sid:9310004;)`,
		`alert http any any -> any any (msg:"http"; content:"GET"; http_method; sid:9310005;)`,
		`alert tls any any -> any any (msg:"tls"; content:"x"; sid:9310006;)`,
	}
	for _, raw := range cases {
		rules, err := rule.ParseStrictRule(raw)
		require.NoError(t, err)
		_, matcherErr := match.CompileRule(rules[0])
		require.NoErrorf(t, matcherErr, "matcher rejected %s", raw)
		_, engineErr := CompileStatefulRule(rules[0])
		require.NoErrorf(t, engineErr, "engine rejected matcher rule %s", raw)
	}
}

func TestEngineMatchesStatelessHTTPRule(t *testing.T) {
	rules, err := rule.ParseStrictRule(`alert http any any -> any any (msg:"stateless http"; content:"GET"; http_method; content:"/stateless-proof"; http_uri; sid:9300001; rev:1;)`)
	require.NoError(t, err)
	matcher, err := match.CompileRule(rules[0])
	require.NoError(t, err)
	engine, err := NewStatefulEngine(rules, Config{State: state.DefaultConfig()})
	require.NoError(t, err)
	defer engine.Close()

	payload := []byte("GET /stateless-proof HTTP/1.1\r\nHost: example.com\r\n\r\n")
	raw := serializeTCP(t, 47000, 80, 1, 1, "ack,psh", payload)
	packet := gopacket.NewPacket(raw, layers.LayerTypeEthernet, gopacket.Default)
	require.True(t, matcher.MatchPackage(packet), "legacy matcher must match")

	matched, err := engine.MatchPacket(packet)
	require.NoError(t, err)
	require.True(t, matched, "engine must contain the matcher's HTTP packet behavior")

	matched, err = engine.MatchHTTPFlow(&match.HttpFlow{Req: payload})
	require.NoError(t, err)
	require.True(t, matched, "engine must contain MatchHTTPFlow")
}

func TestEngineAcceptsStatefulHTTPRuleMatcherDoesNot(t *testing.T) {
	rules, err := rule.ParseStrictRule(`alert http any any -> any any (msg:"stateful http"; flow:established,to_server; content:"GET"; http_method; sid:9300002; rev:1;)`)
	require.NoError(t, err)
	_, matcherErr := match.CompileRule(rules[0])
	require.Error(t, matcherErr, "legacy stateless matcher rejects stateful requirements")
	engine, err := NewStatefulEngine(rules, Config{State: state.DefaultConfig()})
	require.NoError(t, err)
	require.NoError(t, engine.Close())
}

func TestEngineMatchPacketConcurrent(t *testing.T) {
	rules, err := rule.ParseStrictRule(`alert tcp any any -> any any (msg:"concurrent"; content:"benchmark"; sid:9300003; rev:1;)`)
	require.NoError(t, err)
	engine, err := NewStatefulEngine(rules, Config{State: state.DefaultConfig()})
	require.NoError(t, err)
	defer engine.Close()
	packet := buildTCPPacketForBenchmark(t)

	var failures atomic.Int64
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				matched, err := engine.MatchPacket(packet)
				if err != nil || !matched {
					failures.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	require.Zero(t, failures.Load())
}
