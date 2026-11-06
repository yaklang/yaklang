package chaosmaker

import (
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/suricata/engine"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
)

func TestLoadStatefulRulesReportsRejectedRules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mixed.rules")
	raw := `alert tcp any any -> any any (msg:"supported"; flow:established,to_server; content:"GET"; sid:9100001; rev:1;)
alert tcp any any -> any any (msg:"byte test"; content:"x"; byte_test:1,=,1; sid:9100002; rev:1;)
alert http any any -> any any (msg:"stateless http"; content:"GET"; http_method; sid:9100003; rev:1;)
alert http any any -> any any (msg:"stateful http"; flow:established,to_server; content:"GET"; http_method; sid:9100004; rev:1;)
alert udp any any -> any any (msg:"udp threshold"; flow:to_server; content:"x"; threshold:type both,track by_src,count 1,seconds 60; sid:9100005; rev:1;)
`
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o600))

	loaded, err := LoadStatefulRules(path)
	require.NoError(t, err)
	require.Len(t, loaded.Rules, 4)
	require.Equal(t, 9100001, loaded.Rules[0].Sid)
	require.Equal(t, 9100003, loaded.Rules[1].Sid)
	require.Equal(t, 9100004, loaded.Rules[2].Sid)
	require.Equal(t, 9100005, loaded.Rules[3].Sid)
	require.Len(t, loaded.Rejected, 1)
	require.Equal(t, 9100002, loaded.Rejected[0].SID)
	require.Contains(t, loaded.Rejected[0].Reason, "byte_test")
}

func TestLoadStatefulRulesRejectsDuplicateIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "duplicate.rules")
	raw := `alert tcp any any -> any any (msg:"first"; sid:9100010; rev:1;)
alert tcp any any -> any any (msg:"duplicate"; sid:9100010; rev:2;)
`
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o600))

	loaded, err := LoadStatefulRules(path)
	require.NoError(t, err)
	require.Len(t, loaded.Rules, 1)
	require.Len(t, loaded.Rejected, 1)
	require.Contains(t, loaded.Rejected[0].Reason, "duplicate")
}

func TestStatefulAlertProjection(t *testing.T) {
	rule := &surirule.Rule{
		Sid:       9100020,
		Gid:       1,
		Rev:       2,
		Message:   "projection",
		ClassType: "attempted-recon",
		Action:    "alert",
	}
	projected := newStatefulAlert(engine.Alert{
		Timestamp: time.Unix(100, 0),
		Rule:      rule,
		Flow: engine.FlowSnapshot{
			Key:    engine.FlowKey{Generation: 7},
			First:  engine.Endpoint{Address: netip.MustParseAddr("10.0.0.1"), Port: 43210},
			Second: engine.Endpoint{Address: netip.MustParseAddr("198.51.100.20"), Port: 80},
		},
		Direction:  0,
		View:       engine.ViewStream,
		EventID:    11,
		FlowID:     12,
		TCPFlowID:  13,
		References: []pcaputil.PacketReference{{Number: 5}, {Number: 6}},
	})
	require.Equal(t, 9100020, projected.SID)
	require.Equal(t, 1, projected.GID)
	require.Equal(t, 2, projected.Rev)
	require.Equal(t, "projection", projected.Message)
	require.Equal(t, "stream", projected.View)
	require.Equal(t, "10.0.0.1", projected.Source)
	require.Equal(t, 43210, projected.SourcePort)
	require.Equal(t, "198.51.100.20", projected.Destination)
	require.Equal(t, 80, projected.DestinationPort)
	require.Equal(t, uint64(7), projected.FlowGeneration)
	require.Equal(t, uint64(11), projected.EventID)
	require.Equal(t, uint64(12), projected.FlowID)
	require.Equal(t, uint64(13), projected.TCPFlowID)
	require.Equal(t, []uint64{5, 6}, projected.PacketNumbers)
	require.Len(t, projected.References, 2)
}

func TestStatefulEngineOptions(t *testing.T) {
	config := engine.Config{}
	engineOnAlert(func(StatefulAlert) {})(&config)
	engineOnDiagnostic(func(StatefulDiagnostic) {})(&config)
	engineEpoch(9)(&config)
	engineLimits(map[string]int64{
		"maxFlows":            100,
		"maxStreamBytes":      2048,
		"maxTotalStreamBytes": 4096,
	})(&config)
	engineStateLimits(map[string]int64{
		"maxFlows":               17,
		"maxXBits":               23,
		"flowIdleTimeoutSeconds": 4,
	})(&config)

	require.NotNil(t, config.OnAlert)
	require.NotNil(t, config.OnDiagnostic)
	require.Equal(t, uint64(9), config.Epoch)
	require.Equal(t, 100, config.MaxFlows)
	require.Equal(t, 2048, config.MaxStreamBytes)
	require.Equal(t, int64(4096), config.MaxTotalStreamBytes)
	require.Equal(t, 17, config.State.MaxFlows)
	require.Equal(t, 23, config.State.MaxXBits)
	require.Equal(t, 4*time.Second, config.State.FlowIdleTimeout)
}
