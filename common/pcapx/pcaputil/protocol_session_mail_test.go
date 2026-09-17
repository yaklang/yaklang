package pcaputil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func mailCR(s string) []byte { return []byte(s + "\r\n") }

func TestProtocolSessionSMTPHelloMailData(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	greet := mailCR("220 smtp.example ESMTP ready")
	p := s.Probe(greet)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "smtp", p.Protocol)

	r := s.Feed(1, ts, greet)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Reply", r.Events[0].Session["Packet Name"])
	require.Equal(t, 220, r.Events[0].Session["Reply Code"])

	r = s.Feed(0, ts, mailCR("EHLO client.example"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "EHLO", r.Events[0].Session["Packet Name"])

	r = s.Feed(1, ts, []byte("250-PIPELINING\r\n250-STARTTLS\r\n250 HELP\r\n"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Reply", r.Events[0].Session["Packet Name"])
	require.Equal(t, 250, r.Events[0].Session["Reply Code"])
	require.Equal(t, true, r.Events[0].Session["Multiline"])
	require.Equal(t, "EHLO", r.Events[0].Session["In Reply To"])

	r = s.Feed(0, ts, mailCR("MAIL FROM:<a@b>"))
	require.Nil(t, r.Err, "%v", r.Err)
	r = s.Feed(1, ts, mailCR("250 OK"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "MAIL", r.Events[0].Session["In Reply To"])

	r = s.Feed(0, ts, mailCR("RCPT TO:<c@d>"))
	require.Nil(t, r.Err, "%v", r.Err)
	r = s.Feed(1, ts, mailCR("250 OK"))
	require.Nil(t, r.Err, "%v", r.Err)

	r = s.Feed(0, ts, mailCR("DATA"))
	require.Nil(t, r.Err, "%v", r.Err)
	r = s.Feed(1, ts, mailCR("354 Start mail input"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, 354, r.Events[0].Session["Reply Code"])

	r = s.Feed(0, ts, []byte("From: a@b\r\n\r\nhello\r\n.\r\n"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "DATA Body", r.Events[0].Session["Packet Name"])
	r = s.Feed(1, ts, mailCR("250 OK"))
	require.Nil(t, r.Err, "%v", r.Err)
}

func TestProtocolSessionSMTPSTARTTLSEncrypted(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.Nil(t, s.Feed(1, ts, mailCR("220 smtp.example ESMTP ready")).Err)
	require.Nil(t, s.Feed(0, ts, mailCR("STARTTLS")).Err)
	r := s.Feed(1, ts, mailCR("220 Ready to start TLS"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Encrypted"])
	require.Equal(t, "smtp->tls", r.Events[0].Session["Protocol Transition"])
	r = s.Feed(0, ts, []byte("EHLO after tls\r\n"))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrEncrypted, r.Err.Kind)
}

func TestProtocolSessionSMTPFailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.NotEqual(t, "smtp", s.Probe([]byte("GET / HTTP/1.1")).Protocol)
	require.Equal(t, ProbeReject, s.Probe([]byte{0xff, 0x00}).Verdict)
	r := s.Feed(0, ts, []byte("EHLO "))
	require.True(t, r.NeedMore || r.Err != nil && r.Err.Kind == ErrNeedMore)

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s2.Feed(1, ts, mailCR("220 smtp.example ESMTP ready")).Err)
	r = s2.Feed(0, ts, []byte("NOTACMD foo\r\n"))
	require.NotNil(t, r.Err)
	require.NotEqual(t, ErrNeedMore, r.Err.Kind)
}

func TestProtocolSessionSMTPFragmentation(t *testing.T) {
	steps := []sessionStep{
		{1, mailCR("220 smtp.example ESMTP ready")},
		{0, mailCR("EHLO client.example")},
		{1, []byte("250-PIPELINING\r\n250 HELP\r\n")},
		{0, mailCR("MAIL FROM:<a@b>")},
		{1, mailCR("250 OK")},
	}
	assertFragmentation(t, steps, func(chunk int) []string { return runMailNames(t, steps, chunk) })
}

func TestProtocolSessionIMAPTaggedAndLiteral(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	greet := mailCR("* OK IMAP4rev1 ready")
	p := s.Probe(greet)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "imap", p.Protocol)

	r := s.Feed(1, ts, greet)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Untagged OK", r.Events[0].Session["Packet Name"])

	r = s.Feed(0, ts, mailCR("a1 CAPABILITY"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "CAPABILITY", r.Events[0].Session["Packet Name"])
	require.Equal(t, "a1", r.Events[0].Session["Tag"])

	r = s.Feed(1, ts, mailCR("* CAPABILITY IMAP4rev1 STARTTLS"))
	require.Nil(t, r.Err, "%v", r.Err)
	r = s.Feed(1, ts, mailCR("a1 OK CAPABILITY completed"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Tagged OK", r.Events[0].Session["Packet Name"])
	require.Equal(t, "CAPABILITY", r.Events[0].Session["In Reply To"])

	r = s.Feed(0, ts, mailCR("a2 APPEND INBOX {5}"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, 5, r.Events[0].Session["Literal Size"])
	r = s.Feed(1, ts, mailCR("+ Ready for literal"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Continuation", r.Events[0].Session["Packet Name"])
	r = s.Feed(0, ts, []byte("hello"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "Literal", r.Events[0].Session["Packet Name"])
	r = s.Feed(1, ts, mailCR("a2 OK APPEND completed"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "APPEND", r.Events[0].Session["In Reply To"])
}

func TestProtocolSessionIMAPSTARTTLSEncrypted(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.Nil(t, s.Feed(1, ts, mailCR("* OK IMAP4rev1 ready")).Err)
	require.Nil(t, s.Feed(0, ts, mailCR("a1 STARTTLS")).Err)
	r := s.Feed(1, ts, mailCR("a1 OK Begin TLS"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Encrypted"])
	r = s.Feed(0, ts, mailCR("a2 LOGIN u p"))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrEncrypted, r.Err.Kind)
}

func TestProtocolSessionIMAPFailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.NotEqual(t, "imap", s.Probe([]byte("GET / HTTP/1.1")).Protocol)
	r := s.Feed(1, ts, []byte("* OK"))
	require.True(t, r.NeedMore || r.Err != nil && r.Err.Kind == ErrNeedMore)

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s2.Feed(1, ts, mailCR("* OK IMAP4rev1 ready")).Err)
	r = s2.Feed(0, ts, mailCR("a1 ZZ"))
	require.NotNil(t, r.Err)
	require.NotEqual(t, ErrNeedMore, r.Err.Kind)
}

func TestProtocolSessionIMAPFragmentation(t *testing.T) {
	steps := []sessionStep{
		{1, mailCR("* OK IMAP4rev1 ready")},
		{0, mailCR("a1 CAPABILITY")},
		{1, mailCR("* CAPABILITY IMAP4rev1 STARTTLS")},
		{1, mailCR("a1 OK CAPABILITY completed")},
	}
	assertFragmentation(t, steps, func(chunk int) []string { return runMailNames(t, steps, chunk) })
}

func TestProtocolSessionPOP3CapaStatAndSTLS(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	greet := mailCR("+OK POP3 ready")
	p := s.Probe(greet)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "pop3", p.Protocol)

	r := s.Feed(1, ts, greet)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "+OK", r.Events[0].Session["Packet Name"])

	r = s.Feed(0, ts, mailCR("CAPA"))
	require.Nil(t, r.Err, "%v", r.Err)
	r = s.Feed(1, ts, []byte("+OK Capability list follows\r\nSTLS\r\nUSER\r\n.\r\n"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.GreaterOrEqual(t, len(r.Events), 1)
	require.Equal(t, "CAPA", r.Events[0].Session["In Reply To"])

	r = s.Feed(0, ts, mailCR("STAT"))
	require.Nil(t, r.Err, "%v", r.Err)
	r = s.Feed(1, ts, mailCR("+OK 1 20"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "STAT", r.Events[0].Session["In Reply To"])
	require.Equal(t, "+OK", r.Events[0].Session["Packet Name"])

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s2.Feed(1, ts, greet).Err)
	require.Nil(t, s2.Feed(0, ts, mailCR("STLS")).Err)
	r = s2.Feed(1, ts, mailCR("+OK Begin TLS"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Encrypted"])
	r = s2.Feed(0, ts, mailCR("USER n"))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrEncrypted, r.Err.Kind)
}

func TestProtocolSessionPOP3FailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.NotEqual(t, "pop3", s.Probe([]byte("*1\r\n$4\r\nPING\r\n")).Protocol)
	require.Equal(t, "redis", s.Probe([]byte("+PONG\r\n")).Protocol)

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s2.Feed(1, ts, mailCR("+OK POP3 ready")).Err)
	r := s2.Feed(0, ts, mailCR("NOPE x"))
	require.NotNil(t, r.Err)
	require.NotEqual(t, ErrNeedMore, r.Err.Kind)
}

func TestProtocolSessionPOP3Fragmentation(t *testing.T) {
	steps := []sessionStep{
		{1, mailCR("+OK POP3 ready")},
		{0, mailCR("STAT")},
		{1, mailCR("+OK 1 20")},
	}
	assertFragmentation(t, steps, func(chunk int) []string { return runMailNames(t, steps, chunk) })
}

func runMailNames(t *testing.T, steps []sessionStep, chunk int) []string {
	t.Helper()
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	var names []string
	for _, st := range steps {
		for w := st.wire; len(w) > 0; {
			n := len(w)
			if chunk > 0 {
				n = min(n, chunk)
			}
			r := s.Feed(st.dir, ts, w[:n])
			for _, e := range r.Events {
				if e.Status == "decoded" || e.Status == "deferred" {
					if name, ok := e.Session["Packet Name"].(string); ok {
						names = append(names, name)
					}
				}
			}
			w = w[n:]
		}
	}
	s.Close("FIN")
	return names
}
