package bin_parser

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

// This checks the exact historical delivery scope, not an exemption based on
// a protocol name. Unsupported discovery/proxy/body semantics remain explicit
// in both the catalog and the actual parsed metadata.
func requireP0WPADRetrievalScope(t *testing.T) {
	t.Helper()
	roadmapMatches := 0
	for _, item := range ProtocolRoadmap {
		if item.Name == "WPAD proxy" {
			roadmapMatches++
			require.Equal(t, priP0, item.Priority)
			require.Equal(t, stDone, item.Status)
			require.Equal(t, "HTTP GET /wpad.dat", item.Notes)
		}
	}
	require.Equal(t, 1, roadmapMatches)
	var info ProtocolInfo
	catalogMatches := 0
	for _, item := range ProtocolCatalog {
		if item.Name == "WPAD proxy" {
			catalogMatches++
			info = item
		}
	}
	require.Equal(t, 1, catalogMatches)
	require.Equal(t, statusPartial, info.Status)
	require.Equal(t, "application-layer/wpad.yaml", info.RuleFile)
	require.Equal(t, "WPADRequest", info.EntryNode)
	require.Contains(t, info.Notes, "do not establish actual proxy use or a complete discovery exchange")
	require.Equal(t, protocolCorpusParseContract{RuleFile: info.RuleFile, EntryNode: info.EntryNode, Layer: "L7"}, protocolCorpusParseContracts[info.Name])
	score, ok := ResolveP0Scorecard(info.Name)
	require.True(t, ok)
	require.Equal(t, "application-layer/http.yaml", score.Rule)
	require.Contains(t, score.Evidence, "HTTP framing only")

	rule := strings.ReplaceAll(strings.TrimSuffix(info.RuleFile, ".yaml"), "/", ".")
	for _, fixture := range wpadTestFixtures()[:2] {
		wire := fixture.wire()
		// Preserve the original HTTP/Ethernet delivery evidence separately.
		http := parseRule(t, wire, "application-layer.http", "HTTP")
		request := mustChild(t, http, "HTTP Request")
		require.Equal(t, "GET", request.Child("Method").Value)
		require.Equal(t, fixture.target, request.Child("Path").Value)
		ethernet := parseEthernet(t, ipv4TCPFrame(t, 50000, 80, wire))
		request = mustChild(t, ethernet, "IP", "TCP", "HTTP", "HTTP Request")
		require.Equal(t, fixture.target, request.Child("Path").Value)
		// Resolve through the catalog entry, not a hardcoded generic HTTP rule.
		node := protocolCorpusRequireBoundedRuleParse(t, wire, rule, info.EntryNode)
		require.Equal(t, wire, NodeToBytes(node))
		wpadTestCheck(t, node, fixture, 0)
	}
	wrongPath := []byte("GET /other.dat HTTP/1.1\r\nHost: wpad\r\n\r\n")
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wrongPath), rule, info.EntryNode)
	require.ErrorContains(t, err, "wpad: outside default configuration path profile")
}
