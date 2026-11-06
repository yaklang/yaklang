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

func TestETHTTPStatefulRuleMatches(t *testing.T) {
	etRule := `alert http $EXTERNAL_NET any -> $HOME_NET any (msg:"ET SCAN Acunetix scan in progress acunetix_wvs_security_test in http_uri"; flow:established,to_server; threshold: type limit, count 1, seconds 60, track by_src; http.uri; content:"acunetix_wvs_security_test"; fast_pattern; reference:url,www.acunetix.com/; classtype:web-application-attack; sid:2023687; rev:7; metadata:affected_product Any, attack_target Web_Server, created_at 2016_12_28, deployment Datacenter, performance_impact Low, signature_severity Major, updated_at 2020_12_03;)`
	rules, err := rule.ParseStrictRule(etRule, "HOME_NET=192.0.2.2", "EXTERNAL_NET=192.0.2.1")
	require.NoError(t, err)
	require.Len(t, rules, 1)
	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 19, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
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

	base := time.Unix(1700010000, 0)
	clientSeq, serverSeq := uint32(100), uint32(900)
	feedTCP(t, analyzer, base, 41000, 80, clientSeq, 0, "syn", nil)
	feedTCP(t, analyzer, base.Add(time.Millisecond), 80, 41000, serverSeq, clientSeq+1, "syn,ack", nil)
	feedTCP(t, analyzer, base.Add(2*time.Millisecond), 41000, 80, clientSeq+1, serverSeq+1, "ack", nil)
	request := []byte("GET /acunetix_wvs_security_test HTTP/1.1\r\nHost: example.com\r\n\r\n")
	feedTCP(t, analyzer, base.Add(3*time.Millisecond), 41000, 80, clientSeq+1, serverSeq+1, "ack,psh", request)
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 1)
	require.Equal(t, 2023687, alerts[0].Rule.Sid)
	require.Equal(t, ViewStream, alerts[0].View)
	require.Equal(t, 0, alerts[0].Direction)
}

func TestHTTPStatefulChunkedBody(t *testing.T) {
	rules, err := rule.ParseStrictRule(`alert http any any -> any any (msg:"http chunked body"; flow:established,to_server,only_stream; content:"payload"; http_client_body; sid:9700018; rev:1;)`)
	require.NoError(t, err)
	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 20, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
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

	base := time.Unix(1700010100, 0)
	clientSeq, serverSeq := uint32(100), uint32(900)
	feedTCP(t, analyzer, base, 41000, 80, clientSeq, 0, "syn", nil)
	feedTCP(t, analyzer, base.Add(time.Millisecond), 80, 41000, serverSeq, clientSeq+1, "syn,ack", nil)
	feedTCP(t, analyzer, base.Add(2*time.Millisecond), 41000, 80, clientSeq+1, serverSeq+1, "ack", nil)

	headers := []byte("POST /upload HTTP/1.1\r\nHost: example.com\r\nTransfer-Encoding: chunked\r\n\r\n")
	firstPayload := append(append([]byte(nil), headers...), []byte("4\r\npayl\r\n")...)
	feedTCP(t, analyzer, base.Add(3*time.Millisecond), 41000, 80, clientSeq+1, serverSeq+1, "ack,psh", firstPayload)
	feedTCP(t, analyzer, base.Add(4*time.Millisecond), 41000, 80, clientSeq+1+uint32(len(firstPayload)), serverSeq+1, "ack,psh", []byte("4\r\noade\r\n0\r\n\r\n"))
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 1, "chunked body must be decoded before matching")
	require.Equal(t, 9700018, alerts[0].Rule.Sid)
}

func TestHTTPFlowbitsAcrossRequestResponse(t *testing.T) {
	rules, err := rule.ParseStrictRule(`
alert http any any -> any any (msg:"http set"; flow:established,to_server,only_stream; content:"/one"; http_uri; flowbits:set,http_one; flowbits:noalert; sid:9700016; rev:1;)
alert http any any -> any any (msg:"http isset"; flow:established,to_client,only_stream; content:"200"; http_stat_code; flowbits:isset,http_one; sid:9700017; rev:1;)
`)
	require.NoError(t, err)
	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 17, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
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

	base := time.Unix(1700009000, 0)
	clientSeq, serverSeq := uint32(100), uint32(900)
	feedTCP(t, analyzer, base, 41000, 80, clientSeq, 0, "syn", nil)
	feedTCP(t, analyzer, base.Add(time.Millisecond), 80, 41000, serverSeq, clientSeq+1, "syn,ack", nil)
	feedTCP(t, analyzer, base.Add(2*time.Millisecond), 41000, 80, clientSeq+1, serverSeq+1, "ack", nil)

	request := []byte("GET /one HTTP/1.1\r\nHost: example.com\r\n\r\n")
	feedTCP(t, analyzer, base.Add(3*time.Millisecond), 41000, 80, clientSeq+1, serverSeq+1, "ack,psh", request)
	response := []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
	feedTCP(t, analyzer, base.Add(4*time.Millisecond), 80, 41000, serverSeq+1, clientSeq+1+uint32(len(request)), "ack,psh", response)
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 1, "request must set the bit without alerting")
	require.Equal(t, 9700017, alerts[0].Rule.Sid)
	require.Equal(t, 1, alerts[0].Direction)
}

func TestHTTPStatefulKeepAliveMessages(t *testing.T) {
	rules, err := rule.ParseStrictRule(`
alert http any any -> any any (msg:"http one"; flow:established,to_server,only_stream; content:"/one"; http_uri; sid:9700014; rev:1;)
alert http any any -> any any (msg:"http two"; flow:established,to_server,only_stream; content:"/two"; http_uri; sid:9700015; rev:1;)
`)
	require.NoError(t, err)
	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 16, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
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

	base := time.Unix(1700008000, 0)
	clientSeq, serverSeq := uint32(100), uint32(900)
	feedTCP(t, analyzer, base, 41000, 80, clientSeq, 0, "syn", nil)
	feedTCP(t, analyzer, base.Add(time.Millisecond), 80, 41000, serverSeq, clientSeq+1, "syn,ack", nil)
	feedTCP(t, analyzer, base.Add(2*time.Millisecond), 41000, 80, clientSeq+1, serverSeq+1, "ack", nil)

	requestOne := []byte("GET /one HTTP/1.1\r\nHost: example.com\r\n\r\n")
	requestTwo := []byte("GET /two HTTP/1.1\r\nHost: example.com\r\n\r\n")
	feedTCP(t, analyzer, base.Add(3*time.Millisecond), 41000, 80, clientSeq+1, serverSeq+1, "ack,psh", requestOne)
	feedTCP(t, analyzer, base.Add(4*time.Millisecond), 41000, 80, clientSeq+1+uint32(len(requestOne)), serverSeq+1, "ack,psh", requestTwo)
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 2)
	seen := map[int]bool{}
	for _, alert := range alerts {
		seen[alert.Rule.Sid] = true
	}
	require.True(t, seen[9700014], "first keep-alive message must match")
	require.True(t, seen[9700015], "second keep-alive message must match")
}

func TestHTTPStatefulBodyCompleteness(t *testing.T) {
	rules, err := rule.ParseStrictRule(`alert http any any -> any any (msg:"http body"; flow:established,to_server,only_stream; content:"payload"; http_client_body; sid:9700013; rev:1;)`)
	require.NoError(t, err)
	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 15, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
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

	base := time.Unix(1700007000, 0)
	clientSeq, serverSeq := uint32(100), uint32(900)
	feedTCP(t, analyzer, base, 41000, 80, clientSeq, 0, "syn", nil)
	feedTCP(t, analyzer, base.Add(time.Millisecond), 80, 41000, serverSeq, clientSeq+1, "syn,ack", nil)
	feedTCP(t, analyzer, base.Add(2*time.Millisecond), 41000, 80, clientSeq+1, serverSeq+1, "ack", nil)

	headers := []byte("POST /upload HTTP/1.1\r\nHost: example.com\r\nContent-Length: 12\r\n\r\n")
	firstPayload := append(append([]byte(nil), headers...), []byte("payload-")...)
	feedTCP(t, analyzer, base.Add(3*time.Millisecond), 41000, 80, clientSeq+1, serverSeq+1, "ack,psh", firstPayload)
	feedTCP(t, analyzer, base.Add(4*time.Millisecond), 41000, 80, clientSeq+1+uint32(len(firstPayload)), serverSeq+1, "ack,psh", []byte("body"))
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 1, "body rule must wait for the declared Content-Length")
	require.Equal(t, 9700013, alerts[0].Rule.Sid)
}

func TestHTTPStatefulRulesOverReassembledTCP(t *testing.T) {
	rules, err := rule.ParseStrictRule(`
alert http any any -> any any (msg:"http request"; flow:established,to_server,only_stream; content:"GET"; http_method; content:"/pcap-skill-proof"; http_uri; sid:9700011; rev:1;)
alert http any any -> any any (msg:"http response"; flow:established,to_client,only_stream; content:"200"; http_stat_code; sid:9700012; rev:1;)
`)
	require.NoError(t, err)
	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 11, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
		mu.Lock()
		alerts = append(alerts, alert)
		mu.Unlock()
	}, OnDiagnostic: func(d Diagnostic) {
		sid := 0
		if d.Rule != nil {
			sid = d.Rule.Sid
		}
		t.Logf("diag kind=%s sid=%d msg=%s", d.Kind, sid, d.Message)
	}})
	require.NoError(t, err)
	defer engine.Close()

	adapter, err := NewPcapAdapter(128, func(event InspectionEvent) {
		if event.View == ViewStream {
			t.Logf("stream dir=%d payload=%q", event.Direction, event.Payload)
		}
		engine.Handle(event)
	})
	require.NoError(t, err)
	analyzer, err := pcaputil.NewPacketAnalyzer(adapter.Options()...)
	require.NoError(t, err)

	base := time.Unix(1700005000, 0)
	clientSeq, serverSeq := uint32(100), uint32(900)
	feedTCP(t, analyzer, base, 41000, 80, clientSeq, 0, "syn", nil)
	feedTCP(t, analyzer, base.Add(time.Millisecond), 80, 41000, serverSeq, clientSeq+1, "syn,ack", nil)
	feedTCP(t, analyzer, base.Add(2*time.Millisecond), 41000, 80, clientSeq+1, serverSeq+1, "ack", nil)

	// Split the request across two TCP segments. HTTP rules must see the
	// reassembled stream, not just the first segment.
	first := []byte("GET /pcap-skill-pr")
	second := []byte("oof HTTP/1.1\r\nHost: example.com\r\n\r\n")
	feedTCP(t, analyzer, base.Add(3*time.Millisecond), 41000, 80, clientSeq+1, serverSeq+1, "ack,psh", first)
	feedTCP(t, analyzer, base.Add(4*time.Millisecond), 41000, 80, clientSeq+1+uint32(len(first)), serverSeq+1, "ack,psh", second)

	nextClientSeq := clientSeq + 1 + uint32(len(first)+len(second))
	response := []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
	feedTCP(t, analyzer, base.Add(5*time.Millisecond), 80, 41000, serverSeq+1, nextClientSeq, "ack,psh", response)
	feedTCP(t, analyzer, base.Add(6*time.Millisecond), 41000, 80, nextClientSeq, serverSeq+1+uint32(len(response)), "rst,ack", nil)
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 2, "expected one request and one response alert")
	bySID := map[int]Alert{}
	for _, alert := range alerts {
		bySID[alert.Rule.Sid] = alert
		require.Equal(t, ViewStream, alert.View)
	}
	requestAlert, ok := bySID[9700011]
	require.True(t, ok, "missing request alert")
	require.Equal(t, 0, requestAlert.Direction)
	responseAlert, ok := bySID[9700012]
	require.True(t, ok, "missing response alert")
	require.Equal(t, 1, responseAlert.Direction)
}
