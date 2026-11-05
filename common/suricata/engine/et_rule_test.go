package engine

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/suricata/state"
)

func TestETFTPThresholdRuleEndToEnd(t *testing.T) {
	rules, err := rule.ParseStrictRule(`alert tcp $EXTERNAL_NET any -> $HOME_NET 21 (msg:"ET SCAN Multiple FTP Root Login Attempts"; flow:established,to_server; content:"USER "; nocase; depth:5; content:"root"; within:15; nocase; threshold:type threshold,track by_src,count 5,seconds 60; sid:2010642; rev:3;)`, "HOME_NET=192.0.2.2", "EXTERNAL_NET=!$HOME_NET")
	require.NoError(t, err)
	var mu sync.Mutex
	var alerts []Alert
	var diagnostics []Diagnostic
	engine, err := NewStatefulEngine(rules, Config{State: state.DefaultConfig(), OnDiagnostic: func(diagnostic Diagnostic) {
		mu.Lock()
		diagnostics = append(diagnostics, diagnostic)
		mu.Unlock()
	}, OnAlert: func(alert Alert) {
		mu.Lock()
		alerts = append(alerts, alert)
		mu.Unlock()
	}})
	require.NoError(t, err)
	defer engine.Close()
	adapter, err := NewPcapAdapter(128, func(event InspectionEvent) { engine.Handle(event) })
	require.NoError(t, err)
	analyzer, err := pcaputil.NewPacketAnalyzer(adapter.Options()...)
	require.NoError(t, err)
	base := time.Unix(1700004000, 0)
	for i := 0; i < 10; i++ {
		at := base.Add(time.Duration(i) * time.Second)
		srcPort := uint16(44000 + i)
		clientSeq, serverSeq := uint32(1000+i*100), uint32(2000+i*100)
		feedTCP(t, analyzer, at, srcPort, 21, clientSeq, 0, "syn", nil)
		feedTCP(t, analyzer, at.Add(time.Millisecond), 21, srcPort, serverSeq, clientSeq+1, "syn,ack", nil)
		feedTCP(t, analyzer, at.Add(2*time.Millisecond), srcPort, 21, clientSeq+1, serverSeq+1, "ack", nil)
		payload := []byte("USER root\r\n")
		feedTCP(t, analyzer, at.Add(3*time.Millisecond), srcPort, 21, clientSeq+1, serverSeq+1, "ack,psh", payload)
		feedTCP(t, analyzer, at.Add(4*time.Millisecond), srcPort, 21, clientSeq+1+uint32(len(payload)), serverSeq+1, "rst,ack", nil)
	}
	require.NoError(t, analyzer.Close())
	mu.Lock()
	defer mu.Unlock()
	// threshold emits every fifth qualifying event.
	require.Len(t, alerts, 2, "diagnostics=%+v engine=%+v adapter=%+v", diagnostics, engine.Stats(), adapter.Stats())
	for _, alert := range alerts {
		require.Equal(t, 2010642, alert.Rule.Sid)
		require.Equal(t, 0, alert.Direction)
	}
}
