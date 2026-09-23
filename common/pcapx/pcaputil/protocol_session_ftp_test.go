package pcaputil

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestProtocolSessionFTPUserPassPasv(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	greet := mailCR("220 ftp.example FTP server ready")
	p := s.Probe(greet)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "ftp", p.Protocol)

	r := s.Feed(1, ts, greet)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, 220, r.Events[0].Session["Reply Code"])

	r = s.Feed(0, ts, mailCR("USER anonymous"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "USER", r.Events[0].Session["Packet Name"])
	r = s.Feed(1, ts, mailCR("331 Password required"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "USER", r.Events[0].Session["In Reply To"])

	r = s.Feed(0, ts, mailCR("PASS x"))
	require.Nil(t, r.Err, "%v", r.Err)
	r = s.Feed(1, ts, mailCR("230 Login successful"))
	require.Nil(t, r.Err, "%v", r.Err)

	r = s.Feed(0, ts, mailCR("PASV"))
	require.Nil(t, r.Err, "%v", r.Err)
	r = s.Feed(1, ts, mailCR("227 Entering Passive Mode (192,0,2,1,20,21)."))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, 227, r.Events[0].Session["Reply Code"])
	require.Equal(t, "PASV", r.Events[0].Session["In Reply To"])
}

func TestProtocolSessionFTPAUTHTLSEncrypted(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.Nil(t, s.Feed(1, ts, mailCR("220 ftp.example FTP server ready")).Err)
	require.Nil(t, s.Feed(0, ts, mailCR("AUTH TLS")).Err)
	r := s.Feed(1, ts, mailCR("234 Proceed with negotiation"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Encrypted"])
	require.Equal(t, "ftp->tls", r.Events[0].Session["Protocol Transition"])
	r = s.Feed(0, ts, mailCR("USER anonymous"))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrEncrypted, r.Err.Kind)
}

func TestProtocolSessionFTPFailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.NotEqual(t, "ftp", s.Probe(mailCR("220 smtp.example ESMTP ready")).Protocol)
	require.Equal(t, "smtp", s.Probe(mailCR("220 smtp.example ESMTP ready")).Protocol)

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s2.Feed(1, ts, mailCR("220 ftp.example FTP server ready")).Err)
	r := s2.Feed(0, ts, mailCR("FOO bar"))
	require.NotNil(t, r.Err)
	require.NotEqual(t, ErrNeedMore, r.Err.Kind)
}

func TestProtocolSessionFTPFragmentation(t *testing.T) {
	steps := []sessionStep{
		{1, mailCR("220 ftp.example FTP server ready")},
		{0, mailCR("USER anonymous")},
		{1, mailCR("331 Password required")},
		{0, mailCR("PASS x")},
		{1, mailCR("230 Login successful")},
	}
	assertFragmentation(t, steps, func(chunk int) []string { return runMailNames(t, steps, chunk) })
}

func TestProtocolSessionFTPReplayWinlabCorpus(t *testing.T) {
	path := filepath.Join("..", "..", "bin-parser", "testdata", "winlab5013", "captures", "ctf-01-ftp.pcapng")
	var events []*ProtocolEvent
	var stats ProtocolStats
	err := ReplayPcapFile(path,
		WithOnProtocolMessage(func(event *ProtocolEvent) { events = append(events, event) }),
		WithOnProtocolStats(func(value ProtocolStats) { stats = value }),
	)
	require.NoError(t, err)

	var ftpEvents, dataEvents []*ProtocolEvent
	for _, event := range events {
		if event.Protocol == "ftp" {
			ftpEvents = append(ftpEvents, event)
			continue
		}
		if strings.HasSuffix(event.Source, ":1025") || strings.HasSuffix(event.Destination, ":1025") {
			dataEvents = append(dataEvents, event)
			require.Empty(t, event.Protocol, "passive data is not an FTP control channel")
			require.Contains(t, []string{"incomplete", "unrecognized"}, event.Status)
			continue
		}
		require.Empty(t, event.Protocol, "only the negotiated control flow should be FTP")
	}
	require.Len(t, ftpEvents, 14)
	require.EqualValues(t, 14, stats.Decoded)
	require.EqualValues(t, 1, stats.Incomplete)
	require.Zero(t, stats.Unknown)
	var packetNames []string
	var ftpBytes uint64
	for _, event := range ftpEvents {
		require.Equal(t, "decoded", event.Status)
		require.True(t, strings.HasSuffix(event.Source, ":21") || strings.HasSuffix(event.Destination, ":21"), "FTP event must belong to control port 21: %s -> %s", event.Source, event.Destination)
		require.NotNil(t, event.Session)
		name, ok := event.Session["Packet Name"].(string)
		require.True(t, ok)
		packetNames = append(packetNames, name)
		ftpBytes += uint64(event.Length)
	}
	require.Equal(t, []string{"Reply", "USER", "Reply", "PASS", "Reply", "TYPE", "Reply", "PASV", "Reply", "RETR", "Reply", "Reply", "QUIT", "Reply"}, packetNames)
	require.Len(t, dataEvents, 1)
	require.Equal(t, ftpBytes, stats.MessageBytes, "only the control messages count as decoded; passive data stays opaque")
}

func TestProtocolSessionFTPRejectsDecoyBannerAndCommandPrefixes(t *testing.T) {
	for _, command := range []string{"USER lab", "PASS x", "PASV", "RETR flag.txt"} {
		require.Equal(t, ProbeReject, probeFTP(mailCR(command), DefaultParserBudget().ProbeBytes).Verdict, command)
	}

	decoyGreeting := mailCR("220 flag{not-the-ftp-flag} lab-ftp ready")
	for _, tc := range []struct {
		name  string
		port  layers.TCPPort
		steps []sessionStep
	}{
		{
			name: "wrong USER reply on control port",
			port: 21,
			steps: []sessionStep{
				{1, decoyGreeting}, {0, mailCR("USER lab")}, {1, mailCR("500 unknown command")},
			},
		},
		{
			name: "FTP-like exchange on nonstandard port",
			port: 2121,
			steps: []sessionStep{
				{1, decoyGreeting}, {0, mailCR("USER lab")}, {1, mailCR("331 Password required")},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := sessionTestPCAP(t, tc.steps, tc.port, 0, false, true)
			var events []*ProtocolEvent
			err := ReplayPcap(bytes.NewReader(capture), WithOnProtocolMessage(func(event *ProtocolEvent) { events = append(events, event) }))
			require.NoError(t, err)
			require.NotEmpty(t, events)
			for _, event := range events {
				require.Empty(t, event.Protocol, "ambiguous banner or partial exchange must not identify FTP")
				require.Contains(t, []string{"unrecognized", "incomplete"}, event.Status)
			}
		})
	}
}
