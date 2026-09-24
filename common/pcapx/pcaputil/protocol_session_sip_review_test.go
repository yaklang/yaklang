package pcaputil

import (
	"fmt"
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestSIPHTTPOptionsAdmission(t *testing.T) {
	for _, target := range []string{"sip:bob@example.test", "sips:bob@example.test", "SIP:bob@example.test", "tel:+12025550123", "urn:service:sos", "http://example.test/call", "*"} {
		for _, chunk := range []int{0, 1, 2, 7} {
			t.Run(fmt.Sprintf("%s/chunk=%d", target, chunk), func(t *testing.T) {
				req := sipMsg("OPTIONS "+target+" SIP/2.0", [][2]string{
					{"Via", "SIP/2.0/TCP example.test;branch=z9hG4bKoptions"},
					{"From", "<sip:alice@example.test>;tag=1"},
					{"To", "<sip:bob@example.test>"},
					{"Call-ID", "options@example.test"},
					{"CSeq", "1 OPTIONS"},
				}, "")
				resp := sipResp("200", "OK", "z9hG4bKoptions", "1 OPTIONS", "options@example.test", "<sip:alice@example.test>;tag=1", "<sip:bob@example.test>;tag=2")
				events, _ := sessionTestFlow(t, "sip", []sessionStep{{0, req}, {1, resp}}, chunk, false)
				require.Len(t, events, 2)
				assertSessionEvents(t, events, "sip", false)
			})
		}
	}
	for _, target := range []string{"*", "/path", "http://example.test/path"} {
		events, _ := sessionTestFlow(t, "http", []sessionStep{
			{0, []byte("OPTIONS " + target + " HTTP/1.1\r\nHost: example.test\r\n\r\n")},
			{1, []byte("HTTP/1.1 204 No Content\r\n\r\n")},
		}, 1, true)
		require.Len(t, events, 2)
		for _, e := range events {
			require.Equal(t, "http", e.Protocol)
			require.Equal(t, "deferred", e.Status)
			require.Empty(t, e.Error)
			result, err := e.Decode()
			require.NoError(t, err)
			require.NotEmpty(t, result)
		}
	}
}

func TestSIPRequestURIBoundaries(t *testing.T) {
	for _, line := range []string{
		"OPTIONS bob@example.test SIP/2.0",
		"OPTIONS <sip:bob@example.test> SIP/2.0",
		"OPTIONS sip:%ZZ SIP/2.0",
		"INVITE * SIP/2.0",
		"OPTIONS tel:\tbob SIP/2.0",
	} {
		require.False(t, sipRequestLine(line), line)
		require.NotEqual(t, ProbeAccept, probeSIP([]byte(line+"\r\n"), len(line)+2).Verdict, line)
	}
}

func TestSIPResponseStatusLineBoundary(t *testing.T) {
	for _, line := range []string{
		"SIP/2.0 2000 OK",
		"SIP/2.0 200",
		"SIP/2.0 20x OK",
	} {
		require.False(t, sipResponseLine(line), line)
		require.NotEqual(t, ProbeAccept, probeSIP([]byte(line+"\r\n"), len(line)+2).Verdict, line)
	}
	require.True(t, sipResponseLine("SIP/2.0 204 "), "empty reason phrase still has its delimiter")
}

func TestSIPAdmissionSurvivesGopherPortHint(t *testing.T) {
	req := sipMsg("OPTIONS sip:bob@example.test SIP/2.0", [][2]string{
		{"Via", "SIP/2.0/TCP client.example.test;branch=z9hG4bKoptions"},
		{"From", "<sip:alice@example.test>;tag=1"},
		{"To", "<sip:bob@example.test>"},
		{"Call-ID", "options@example.test"},
		{"CSeq", "1 OPTIONS"},
	}, "")
	resp := sipResp("200", "OK", "z9hG4bKoptions", "1 OPTIONS", "options@example.test", "<sip:alice@example.test>;tag=1", "<sip:bob@example.test>;tag=2")
	steps := []tcpStep{
		{seq: 100, syn: true},
		{seq: 300, syn: true, synack: true, reverse: true},
		{seq: 101, ack: 301},
	}
	clientNext, serverNext := uint32(101), uint32(301)
	for at := 0; at < len(req); at++ {
		steps = append(steps, tcpStep{seq: clientNext, ack: serverNext, data: string(req[at : at+1])})
		clientNext++
	}
	for at := 0; at < len(resp); at++ {
		steps = append(steps, tcpStep{seq: serverNext, ack: clientNext, reverse: true, data: string(resp[at : at+1])})
		serverNext++
	}
	steps = append(steps, tcpStep{seq: clientNext, ack: serverNext, fin: true})
	pcap := binTestPcap(t, steps, layers.TCPPort(70), false, false)
	events, _, err := binReplay(t, pcap, 1)
	require.NoError(t, err)
	require.Len(t, events, 2)
	for _, event := range events {
		require.Equal(t, "sip", event.Protocol, "%s: %s", event.Status, event.Summary)
		require.Equal(t, "decoded", event.Status, "%s: %s", event.Status, event.Summary)
	}
}
