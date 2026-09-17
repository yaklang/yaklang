package pcaputil

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func sipSDP() string {
	return "v=0\r\n" +
		"o=alice 2890844526 2890844526 IN IP4 pc33.atlanta.example.com\r\n" +
		"s=-\r\n" +
		"c=IN IP4 10.1.1.1\r\n" +
		"t=0 0\r\n" +
		"m=audio 49170 RTP/AVP 0\r\n"
}

func sipMsg(start string, headers [][2]string, body string) []byte {
	var b string
	b += start + "\r\n"
	for _, h := range headers {
		b += h[0] + ": " + h[1] + "\r\n"
	}
	if body != "" {
		hasCL := false
		for _, h := range headers {
			if h[0] == "Content-Length" || h[0] == "l" || h[0] == "L" {
				hasCL = true
				break
			}
		}
		if !hasCL {
			b += "Content-Length: " + strconv.Itoa(len(body)) + "\r\n"
		}
	} else {
		b += "Content-Length: 0\r\n"
	}
	b += "\r\n" + body
	return []byte(b)
}

func sipInvite() []byte {
	sdp := sipSDP()
	return sipMsg("INVITE sip:bob@biloxi.example.com SIP/2.0", [][2]string{
		{"Via", "SIP/2.0/UDP pc33.atlanta.example.com;branch=z9hG4bK776asdhds"},
		{"From", "Alice <sip:alice@atlanta.example.com>;tag=1928301774"},
		{"To", "Bob <sip:bob@biloxi.example.com>"},
		{"Call-ID", "a84b4c76e66710@pc33.atlanta.example.com"},
		{"CSeq", "314159 INVITE"},
		{"Content-Type", "application/sdp"},
		{"Content-Length", strconv.Itoa(len(sdp))},
	}, sdp)
}

func sipCompactInvite() []byte {
	sdp := sipSDP()
	return sipMsg("INVITE sip:bob@biloxi.example.com SIP/2.0", [][2]string{
		{"v", "SIP/2.0/UDP pc33.atlanta.example.com;branch=z9hG4bKcompact"},
		{"f", "Alice <sip:alice@atlanta.example.com>;tag=1928301774"},
		{"t", "Bob <sip:bob@biloxi.example.com>"},
		{"i", "compact-call@pc33.atlanta.example.com"},
		{"CSeq", "1 INVITE"},
		{"c", "application/sdp"},
		{"l", strconv.Itoa(len(sdp))},
	}, sdp)
}

func sipResp(code, reason, branch, cseq, callID, from, to string) []byte {
	return sipMsg("SIP/2.0 "+code+" "+reason, [][2]string{
		{"Via", "SIP/2.0/UDP pc33.atlanta.example.com;branch=" + branch},
		{"From", from},
		{"To", to},
		{"Call-ID", callID},
		{"CSeq", cseq},
	}, "")
}

func TestProtocolSessionSIPInviteAckBye(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	inv := sipInvite()
	p := s.Probe(inv)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "sip", p.Protocol)
	require.Equal(t, "2.0", p.Version)

	r := s.Feed(0, ts, inv)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "INVITE", r.Events[0].Session["Packet Name"])
	require.Equal(t, "a84b4c76e66710@pc33.atlanta.example.com", r.Events[0].Session["Call-ID"])
	require.Equal(t, uint32(314159), r.Events[0].Session["CSeq"])
	require.Equal(t, "INVITE", r.Events[0].Session["CSeq Method"])
	require.Equal(t, "1928301774", r.Events[0].Session["From Tag"])
	require.Equal(t, "z9hG4bK776asdhds", r.Events[0].Session["Via Branch"])
	sdp, _ := r.Events[0].Session["SDP"].(map[string]any)
	require.NotNil(t, sdp)
	require.Equal(t, "0", sdp["Version"])
	media, _ := sdp["Media"].([]map[string]any)
	require.NotEmpty(t, media)
	require.Equal(t, "audio", media[0]["Type"])
	require.Equal(t, "49170", media[0]["Port"])

	from := "Alice <sip:alice@atlanta.example.com>;tag=1928301774"
	to := "Bob <sip:bob@biloxi.example.com>;tag=a6c85cf"
	call := "a84b4c76e66710@pc33.atlanta.example.com"
	branch := "z9hG4bK776asdhds"
	r = s.Feed(1, ts, sipResp("100", "Trying", branch, "314159 INVITE", call, from, "Bob <sip:bob@biloxi.example.com>"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, 100, r.Events[0].Session["Status"])
	require.Equal(t, "INVITE", r.Events[0].Session["Matched Request"])

	r = s.Feed(1, ts, sipResp("180", "Ringing", branch, "314159 INVITE", call, from, to))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, 180, r.Events[0].Session["Status"])

	r = s.Feed(1, ts, sipResp("200", "OK", branch, "314159 INVITE", call, from, to))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, 200, r.Events[0].Session["Status"])
	require.Equal(t, "matched", r.Events[0].Session["Association Status"])

	ack := sipMsg("ACK sip:bob@biloxi.example.com SIP/2.0", [][2]string{
		{"Via", "SIP/2.0/UDP pc33.atlanta.example.com;branch=z9hG4bKack"},
		{"From", from},
		{"To", to},
		{"Call-ID", call},
		{"CSeq", "314159 ACK"},
	}, "")
	r = s.Feed(0, ts, ack)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "ACK", r.Events[0].Session["Packet Name"])

	bye := sipMsg("BYE sip:bob@biloxi.example.com SIP/2.0", [][2]string{
		{"Via", "SIP/2.0/UDP pc33.atlanta.example.com;branch=z9hG4bKbye"},
		{"From", from},
		{"To", to},
		{"Call-ID", call},
		{"CSeq", "314160 BYE"},
	}, "")
	r = s.Feed(0, ts, bye)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "BYE", r.Events[0].Session["Method"])
	r = s.Feed(1, ts, sipResp("200", "OK", "z9hG4bKbye", "314160 BYE", call, from, to))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "BYE", r.Events[0].Session["Matched Request"])
}

func TestProtocolSessionSIPRegisterCancelCompactRetransmit(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	reg := sipMsg("REGISTER sip:sip.provider.com SIP/2.0", [][2]string{
		{"Via", "SIP/2.0/UDP 10.0.0.1;branch=z9hG4bKreg"},
		{"From", "Alice <sip:alice@atlanta.example.com>;tag=regtag"},
		{"To", "Alice <sip:alice@atlanta.example.com>"},
		{"Call-ID", "reg-call@host"},
		{"CSeq", "1 REGISTER"},
	}, "")
	r := s.Feed(0, ts, reg)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "REGISTER", r.Events[0].Session["Method"])
	r = s.Feed(1, ts, sipResp("200", "OK", "z9hG4bKreg", "1 REGISTER", "reg-call@host",
		"Alice <sip:alice@atlanta.example.com>;tag=regtag", "Alice <sip:alice@atlanta.example.com>;tag=srv"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "REGISTER", r.Events[0].Session["Matched Request"])

	r = s.Feed(0, ts, sipCompactInvite())
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "compact-call@pc33.atlanta.example.com", r.Events[0].Session["Call-ID"])
	require.Equal(t, "1928301774", r.Events[0].Session["From Tag"])
	require.Equal(t, "z9hG4bKcompact", r.Events[0].Session["Via Branch"])
	require.NotNil(t, r.Events[0].Session["SDP"])

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	inv := sipInvite()
	require.Nil(t, s2.Feed(0, ts, inv).Err)
	r = s2.Feed(0, ts, inv)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Retransmission"])

	cancel := sipMsg("CANCEL sip:bob@biloxi.example.com SIP/2.0", [][2]string{
		{"Via", "SIP/2.0/UDP pc33.atlanta.example.com;branch=z9hG4bK776asdhds"},
		{"From", "Alice <sip:alice@atlanta.example.com>;tag=1928301774"},
		{"To", "Bob <sip:bob@biloxi.example.com>"},
		{"Call-ID", "a84b4c76e66710@pc33.atlanta.example.com"},
		{"CSeq", "314159 CANCEL"},
	}, "")
	r = s2.Feed(0, ts, cancel)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "CANCEL", r.Events[0].Session["Method"])
	require.Equal(t, "INVITE", r.Events[0].Session["Cancels"])
	r = s2.Feed(1, ts, sipResp("200", "OK", "z9hG4bK776asdhds", "314159 CANCEL",
		"a84b4c76e66710@pc33.atlanta.example.com",
		"Alice <sip:alice@atlanta.example.com>;tag=1928301774",
		"Bob <sip:bob@biloxi.example.com>"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "CANCEL", r.Events[0].Session["Matched Request"])
}

func TestProtocolSessionSIPFailClosedAndProbe(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.Equal(t, ProbeAccept, s.Probe([]byte("OPTIONS sip:bob@example.com SIP/2.0\r\n")).Verdict)
	require.Equal(t, "sip", s.Probe([]byte("OPTIONS sip:bob@example.com SIP/2.0\r\n")).Protocol)
	require.NotEqual(t, "sip", s.Probe([]byte("GET /index.html HTTP/1.1\r\n")).Protocol)
	require.Equal(t, ProbeNeedMore, s.Probe([]byte("INVITE sip:")).Verdict)

	cut := s.Feed(0, ts, sipInvite()[:20])
	require.True(t, cut.NeedMore)
	require.Equal(t, ErrNeedMore, cut.Err.Kind)

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	bad := sipMsg("INVITE sip:bob@example.com SIP/2.0", [][2]string{
		{"Via", "SIP/2.0/UDP h;branch=z9hG4bKx"},
		{"From", "Alice <sip:a@ex>;tag=1"},
		{"To", "Bob <sip:b@ex>"},
		{"CSeq", "1 INVITE"},
	}, "")
	r := s2.Feed(0, ts, bad)
	require.NotNil(t, r.Err)
	require.NotEqual(t, ErrNeedMore, r.Err.Kind)
}

func TestProtocolSessionSIPFragmentation(t *testing.T) {
	from := "Alice <sip:alice@atlanta.example.com>;tag=1928301774"
	to := "Bob <sip:bob@biloxi.example.com>;tag=a6c85cf"
	call := "a84b4c76e66710@pc33.atlanta.example.com"
	branch := "z9hG4bK776asdhds"
	steps := []sessionStep{
		{0, sipInvite()},
		{1, sipResp("100", "Trying", branch, "314159 INVITE", call, from, "Bob <sip:bob@biloxi.example.com>")},
		{1, sipResp("200", "OK", branch, "314159 INVITE", call, from, to)},
		{0, sipMsg("ACK sip:bob@biloxi.example.com SIP/2.0", [][2]string{
			{"Via", "SIP/2.0/UDP pc33.atlanta.example.com;branch=z9hG4bKack"},
			{"From", from}, {"To", to}, {"Call-ID", call}, {"CSeq", "314159 ACK"},
		}, "")},
		{0, sipMsg("BYE sip:bob@biloxi.example.com SIP/2.0", [][2]string{
			{"Via", "SIP/2.0/UDP pc33.atlanta.example.com;branch=z9hG4bKbye"},
			{"From", from}, {"To", to}, {"Call-ID", call}, {"CSeq", "314160 BYE"},
		}, "")},
		{1, sipResp("200", "OK", "z9hG4bKbye", "314160 BYE", call, from, to)},
	}
	assertFragmentation(t, steps, func(chunk int) []string {
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
						names = append(names, fmt.Sprintf("%v:%v", e.Session["Packet Name"], e.Session["CSeq"]))
					}
				}
				w = w[n:]
			}
		}
		return names
	})
}
