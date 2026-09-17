package pcaputil

import (
	"testing"
	"time"

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
