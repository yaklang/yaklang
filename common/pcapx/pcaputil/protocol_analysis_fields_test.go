package pcaputil

import (
	"encoding/base64"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAnalysisHTTPFieldsUseAdmittedLiteralHeaders(t *testing.T) {
	request := "GET /path/resource HTTP/1.1\r\nHost: example.test\r\nUser-Agent: curl/9.0\r\nAuthorization: Basic YWRtaW46cGFzc3dvcmQ=\r\nCookie: session=deadbeef; role=admin\r\nContent-Length: 0\r\n\r\n"
	response := "HTTP/1.1 302 Found\r\nLocation: /other\r\nContent-Type: text/plain\r\nContent-Length: 4\r\n\r\nbody"
	for _, deferred := range []bool{false, true} {
		events, _, err := binReplay(t, binTestPcap(t, []tcpStep{{seq: 0, syn: true}, {seq: 0, syn: true, reverse: true}, {seq: 1, data: request}, {seq: 1, data: response, reverse: true}}, 80, false, false), 1, WithBinParserDeferred(deferred))
		require.NoError(t, err)
		require.Len(t, events, 2)
		fields := events[0].DisplayFields()
		for key, want := range map[string]any{"http.host": "example.test", "http.request.uri": "/path/resource", "http.request.version": "HTTP/1.1", "http.user_agent": "curl/9.0", "http.request.authorization": "Basic YWRtaW46cGFzc3dvcmQ=", "http.cookie": "session=deadbeef; role=admin", "http.request.header.length": uint64(0)} {
			require.Equal(t, []any{want}, fields[key], key)
		}
		for _, expr := range []string{`http.host == "example.test"`, `http.user_agent contains "curl"`, `http.cookie contains "role=admin"`} {
			filter, err := CompileDisplayFilter(expr)
			require.NoError(t, err)
			require.True(t, filter.Match(events[0]), expr)
		}
		require.Equal(t, []any{"/other"}, events[1].DisplayFields()["http.response.location"])
		require.NotContains(t, events[1].DisplayFields(), "http.host")
		body, length, ok := events[1].HTTPResponseBody()
		require.True(t, ok)
		require.EqualValues(t, 4, *length)
		wire, err := io.ReadAll(body)
		require.NoError(t, err)
		require.Equal(t, "body", string(wire))
		events[1].Raw[len(events[1].Raw)-1] ^= 1
		_, _, ok = events[1].HTTPResponseBody()
		require.False(t, ok, "edited bytes cannot borrow the admitted object's source")
		events[0].Session = map[string]any{}
		events[0].Session["Host"] = "forged"
		events[0].Session["Headers"] = map[string]any{"Authorization": []string{"forged"}}
		require.Equal(t, fields, events[0].DisplayFields(), "public context cannot overwrite accepted headers")
	}
	// A response-only observation cannot borrow the previous request's host.
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	events := append(s.Feed(1, time.Unix(1, 0), []byte(response)).Events, s.Close("eof")...)
	require.NotEmpty(t, events)
	for _, e := range events {
		require.NotContains(t, e.DisplayFields(), "http.host")
		_, _, ok := e.HTTPResponseBody()
		require.False(t, ok)
	}
}

func TestAnalysisHTTPVariantsAndProjectionBudget(t *testing.T) {
	headers := []map[string]any{{"Name": ":authority", "Value": "example.test"}, {"Name": ":path", "Value": "/h2"}, {"Name": "user-agent", "Value": "curl/h2"}, {"Name": "cookie", "Value": "a=b"}}
	for _, protocol := range []string{"http2", "grpc", "doh", "http3"} {
		e := &ProtocolEvent{Protocol: protocol, Session: map[string]any{"Stream ID": uint32(1), "Headers": headers}}
		if protocol == "http3" {
			e.Session = map[string]any{"QPACK Resumed Streams": []map[string]any{{"HTTP3 Frames": []map[string]any{{"Headers": headers}}}}}
		}
		fields := e.DisplayFields()
		require.Equal(t, []any{"example.test"}, fields["http.host"], protocol)
		require.Equal(t, []any{"curl/h2"}, fields["http.user_agent"], protocol)
		e.nativeDiagnosticError = "opaque refusal"
		require.NotContains(t, e.DisplayFields(), "http.host")
	}
	e := &ProtocolEvent{Protocol: "http", nativeHTTPHeaderObservation: true, semanticFields: map[string]any{"Status Code": 200, "Headers": map[string]any{"Content-Type": []string{"text/plain"}}}, Session: map[string]any{"Host": "forged"}}
	require.Equal(t, []any{"text/plain"}, e.DisplayFields()["http.response.content_type"])
	require.NotContains(t, e.DisplayFields(), "http.host")
	e = &ProtocolEvent{Protocol: "http", Session: map[string]any{"Headers": map[string][]string{"Cookie": make([]string, maxDisplayFieldValues+100)}}}
	for i := range e.Session["Headers"].(map[string][]string)["Cookie"] {
		e.Session["Headers"].(map[string][]string)["Cookie"][i] = "a=b"
	}
	snapshot := e.DisplayFieldValues()
	require.True(t, snapshot.Truncated)
	require.LessOrEqual(t, len(snapshot.Values), maxDisplayFieldValues)
}

func TestAnalysisDNSAnswersAreTypedRepeatedAndRecordBounded(t *testing.T) {
	answers := []map[string]any{{"Name": "example.test", "Type": uint16(1), "TTL": uint32(60), "Address": "93.184.216.34"}, {"Name": "example.test", "Type": uint16(1), "Address": "192.0.2.2"}, {"Name": "example.test", "Type": uint16(28), "Address": "2001:db8::1"}, {"Name": "example.test", "Type": uint16(16), "Text": []string{"example", "second"}}, {"Name": "other.test", "Type": uint16(16), "Text": []string{"third"}}, {"Type": uint16(5), "Target": "alias.test"}, {"Type": uint16(15), "Target": "mail.test"}, {"Type": uint16(33), "Target": "srv.test", "Port": uint16(443)}, {"Type": uint16(6), "MNAME": "ns.test"}}
	for _, protocol := range []string{"dns", "mdns", "llmnr", "dot", "doh", "doq"} {
		e := &ProtocolEvent{Protocol: protocol, Session: map[string]any{"DNS": map[string]any{"Answers": answers, "RCODE": uint16(0)}}}
		fields := e.DisplayFields()
		require.Equal(t, []any{"93.184.216.34", "192.0.2.2"}, fields["dns.a"])
		require.Equal(t, []any{"2001:db8::1"}, fields["dns.aaaa"])
		require.Equal(t, []any{"example", "second", "third"}, fields["dns.txt"])
		require.Equal(t, []any{uint64(443)}, fields["dns.srv.port"])
		for _, expr := range []string{`dns.txt contains "example"`, `dns.a == "93.184.216.34"`, `dns.rcode == 0`} {
			f, err := CompileDisplayFilter(expr)
			require.NoError(t, err)
			require.True(t, f.Match(e), protocol+expr)
		}
	}
	f, err := CompileDisplayFilter(`not (dns.a == "93.184.216.34")`)
	require.NoError(t, err)
	result := f.EvaluateEvents([]*ProtocolEvent{{Protocol: "dns"}})
	require.False(t, result.Complete)
	require.True(t, result.Partial)
	require.False(t, result.Matched)
	_, err = DecodeDNSMessage([]byte{0, 1, 0}, 100)
	require.Error(t, err)
}

func TestAnalysisLANTextEncodingAndProtocolSeparation(t *testing.T) {
	for _, hostname := range []string{"DESKTOP-TEST", base64.StdEncoding.EncodeToString([]byte("DESKTOP-TEST"))} {
		e := &ProtocolEvent{Protocol: "dhcp", Session: map[string]any{"Message Type": byte(3), "Requested IP": "192.0.2.5", "CHADDR": []byte{0, 1, 2, 3, 4, 5}, "Hostname": []byte(hostname), "Vendor Class": []byte("MSFT 5.0"), "Lease Seconds": uint32(3600)}}
		fields := e.DisplayFields()
		require.Equal(t, []any{"00:01:02:03:04:05"}, fields["dhcp.client_mac"])
		require.Equal(t, []any{uint64(3600)}, fields["dhcp.lease_time"])
		if strings.HasPrefix(hostname, "DESKTOP") {
			f, err := CompileDisplayFilter(`dhcp.hostname contains "DESKTOP"`)
			require.NoError(t, err)
			require.True(t, f.Match(e))
		} else {
			require.NotContains(t, fields, "dhcp.hostname")
			require.Equal(t, []any{"base64"}, fields["dhcp.hostname.encoding"])
			require.Equal(t, []any{hostname}, fields["dhcp.hostname.encoded"])
		}
		e.Protocol = "dhcpv6"
		require.NotContains(t, e.DisplayFields(), "dhcp.hostname")
		require.NotContains(t, e.DisplayFields(), "dhcp.requested_ip")
	}
	fields, err := decodeNBNS(nbnsTestQuery(), 100)
	require.NoError(t, err)
	e := &ProtocolEvent{Protocol: "nbns", Session: fields}
	require.NotEmpty(t, e.DisplayFields()["nbns.name"])
}

func TestAnalysisPacketFieldsAreEventScoped(t *testing.T) {
	e := &ProtocolEvent{Transport: "tcp", Source: "10.0.2.15:12345", Destination: "192.0.2.1:80", TCPFlowID: 7, Frame: &DisplayFrameObservation{Number: 5, Time: time.Unix(100, 123), CapturedLength: 60}}
	for _, expr := range []string{`ip.src == 10.0.2.15 and tcp.dstport == 80`, `frame.number <= 5`, `tcp.stream == 7`, `tcp.port == 80`} {
		f, err := CompileDisplayFilter(expr)
		require.NoError(t, err)
		require.True(t, f.EvaluateSnapshots([]DisplayFieldSnapshot{e.DisplayFieldValues()}).Matched, expr)
	}
	for _, name := range []string{"ip.src", "tcp.srcport", "frame.number", "frame.len", "frame.time_epoch"} {
		d, ok := LookupDisplayField(name)
		require.True(t, ok)
		require.Equal(t, "event", d.Origin)
		require.Empty(t, d.Protocols)
	}
	require.Equal(t, []any{"100.000000123"}, e.DisplayFields()["frame.time_epoch"])
	e.Frame.Time = time.Unix(-1, 500000000)
	require.Equal(t, []any{"-0.500000000"}, e.DisplayFields()["frame.time_epoch"])
}

func TestAnalysisKerberosFactsRemainStructuredAndUnverified(t *testing.T) {
	e := &ProtocolEvent{Protocol: "kerberos", Status: "decoded", kerberosSummaryFacts: map[string]any{"Message Type": int64(10), "Realm": "EXAMPLE.TEST", "Client Principal": "desktop$", "Server Principal": "krbtgt/EXAMPLE.TEST", "Requested Encryption Types": []int64{18, 17, -133}}}
	fields := e.DisplayFields()
	for key, want := range map[string]any{"kerberos.msg_type": uint64(10), "kerberos.realm": "EXAMPLE.TEST", "kerberos.client": "desktop$", "kerberos.server": "krbtgt/EXAMPLE.TEST", "kerberos.decrypt_status": "no", "kerberos.identity_verified": false} {
		require.Equal(t, []any{want}, fields[key], key)
	}
	require.Equal(t, []any{"18", "17", "-133"}, fields["kerberos.etypes"])
	e.Status = "malformed"
	require.NotContains(t, e.DisplayFields(), "kerberos.realm", "rejected messages cannot publish successful identity facts")
}
