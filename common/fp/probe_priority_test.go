package fp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProbeRarityOverridesMatchEmbeddedProbes(t *testing.T) {
	rules, err := GetDefaultNmapServiceProbeRules()
	require.NoError(t, err)
	embedded := map[string]int{}
	for probe := range rules {
		if probe == nil {
			continue
		}
		embedded[probe.Name] = probe.Rarity
	}
	for name, rarity := range probeRarityOverride {
		got, ok := embedded[name]
		require.Truef(t, ok, "override %s is not an embedded probe", name)
		require.NotEqualf(t, got, rarity, "override %s does not change embedded rarity %d", name, got)
		require.Equal(t, rarity, effectiveProbeRarity(&NmapProbe{Name: name, Rarity: got}))
	}
}

func TestPortMatchedProbeSurvivesSmallGenericBudget(t *testing.T) {
	rare := &NmapProbe{Name: "RareDB", Proto: TCP, Index: 1, Rarity: 9, DefaultPorts: []int{27017}, Payload: "rare"}
	generic := &NmapProbe{Name: "GenericA", Proto: TCP, Index: 2, Rarity: 1, Payload: "generic"}
	rules := map[*NmapProbe][]*NmapMatch{rare: nil, generic: nil}
	config := NewConfig(WithActiveMode(true), WithNmapRule(rules), WithProbesMax(1), WithRarityMax(1))

	_, blocks, portSpecific := GetRuleBlockByConfig(27017, config)
	require.True(t, portSpecific)
	require.Equal(t, []string{"RareDB", "GenericA"}, probeNames(blocks))
}

func TestPortProbeBudgetDoesNotGrowWithoutLimit(t *testing.T) {
	rules := map[*NmapProbe][]*NmapMatch{}
	for i := 0; i < 30; i++ {
		rules[&NmapProbe{Name: "Port" + string(rune('A'+i)), Proto: TCP, Index: i, Rarity: 9, DefaultPorts: []int{4242}, Payload: "p"}] = nil
	}
	config := NewConfig(WithActiveMode(true), WithNmapRule(rules), WithProbesMax(1), WithRarityMax(1))
	_, blocks, _ := GetRuleBlockByConfig(4242, config)
	require.LessOrEqual(t, len(blocks), defaultPortProbeBudget)
	require.NotEmpty(t, blocks)
}

func TestLoweredProtocolsEnterDefaultSweep(t *testing.T) {
	config := NewConfig(WithActiveMode(true))
	_, blocks, _ := GetRuleBlockByConfig(59999, config)
	names := map[string]bool{}
	for _, name := range probeNames(blocks) {
		names[name] = true
	}
	for _, want := range []string{"mqtt", "mongodb", "ms-sql-s", "Memcache", "redis-server", "oracle-tns", "LDAPBindReq", "LDAPSearchReq"} {
		require.Truef(t, names[want], "default sweep for an unlisted port missed %s", want)
	}
	for _, demoted := range []string{"RdpStandard", "RdpSSLHybridAndHybridEx", "ORACLE", "SMBProgNeg"} {
		require.Falsef(t, names[demoted], "demoted probe %s still occupies the generic sweep", demoted)
	}
	require.LessOrEqual(t, len(blocks), selectionCap(config))
	require.Contains(t, probeNames(blocks), "GetRequest")
}

func TestDemotedProbesStayOnTheirPorts(t *testing.T) {
	config := NewConfig(WithActiveMode(true))
	for _, tc := range []struct {
		port int
		name string
	}{
		{3389, "RdpStandard"},
		{1521, "ORACLE"},
		{445, "SMBProgNeg"},
	} {
		_, blocks, portSpecific := GetRuleBlockByConfig(tc.port, config)
		require.Truef(t, portSpecific, "port %d", tc.port)
		require.Containsf(t, probeNames(blocks), tc.name, "port %d dropped %s", tc.port, tc.name)
		require.LessOrEqualf(t, len(blocks), selectionCap(config), "port %d", tc.port)
	}
}

func TestHighRarityPortProbeStaysOnItsPort(t *testing.T) {
	config := NewConfig(WithActiveMode(true), WithRarityMax(1), WithProbesMax(3))
	_, blocks, portSpecific := GetRuleBlockByConfig(1883, config)
	require.True(t, portSpecific)
	require.Contains(t, probeNames(blocks), "mqtt")
	require.LessOrEqual(t, len(blocks), selectionCap(config))
}

func TestScanProtocolBudgetStopsFurtherDecoding(t *testing.T) {
	ctx := withScanProtocolBudget(context.Background())
	budget := scanProtocolBudgetFrom(ctx)
	require.NotNil(t, budget)
	budget.bytes.Store(scanProtocolByteBudget)
	got := parseProtocolFields(ctx, nil, [][]byte{[]byte("SSH-2.0-OpenSSH_9.6\r\n")}, TCP, 22, true)
	require.Empty(t, got)

	fresh := ParseProtocolFields([]byte("SSH-2.0-OpenSSH_9.6\r\n"), TCP, 22)
	require.Contains(t, fresh, "ssh")
}

func probeNames(blocks []*RuleBlock) []string {
	names := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block != nil && block.Probe != nil {
			names = append(names, block.Probe.Name)
		}
	}
	return names
}
