package pcaputil

import (
	"fmt"
	"strconv"
	"strings"
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

func TestProtocolSessionSIPRetainedIdentityBudgetPreflight(t *testing.T) {
	budget := DefaultParserBudget()
	budget.MaxMessageBytes = 4096
	budget.MaxFrameBytes = 4096
	budget.MaxBufferedBytes = 4096
	limited, err := NewProtocolSession(budget)
	require.NoError(t, err)
	t.Cleanup(func() { limited.Close("FIN") })
	request := sipMsg("OPTIONS sip:b@example.test SIP/2.0", [][2]string{
		{"Via", "SIP/2.0/UDP a;branch=z9hG4bK-small"},
		{"From", "<sip:a@example.test>;tag=a"},
		{"To", "<sip:b@example.test>"},
		{"Call-ID", "small"},
		{"CSeq", "1 OPTIONS"},
	}, "")
	result := limited.Feed(0, time.Unix(1, 0), request)
	require.Nil(t, result.Err, "%v", result.Err)

	budget = DefaultParserBudget()
	budget.MaxMessageBytes = 4096
	budget.MaxFrameBytes = 4096
	budget.MaxBufferedBytes = 4096
	limitedLarge, err := NewProtocolSession(budget)
	require.NoError(t, err)
	t.Cleanup(func() { limitedLarge.Close("FIN") })
	large := sipMsg("OPTIONS sip:b@example.test SIP/2.0", [][2]string{
		{"Via", "SIP/2.0/UDP a;branch=z9hG4bK-large"},
		{"From", "<sip:a@example.test>;tag=a"},
		{"To", "<sip:b@example.test>"},
		{"Call-ID", strings.Repeat("C", 2048)},
		{"CSeq", "1 OPTIONS"},
	}, "")
	result = limitedLarge.Feed(0, time.Unix(1, 0), large)
	require.NotNil(t, result.Err, "attacker-controlled transaction identifiers must be charged before retention")
	require.Equal(t, ErrResourceExceeded, result.Err.Kind)
	require.NotEmpty(t, result.Events)
	require.Equal(t, "context-required", result.Events[0].Status, "rejected key state must not be emitted as successfully parsed")
	require.Contains(t, result.Err.Message, "connection state exceeds capture memory budget")
	require.Zero(t, limitedLarge.Stats().BufferedBytes, "the failed frame must not leave retained transaction-key memory behind")
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

func TestProtocolSessionSIPConflictingContentLength(t *testing.T) {
	wire := []byte("OPTIONS sip:bob@example.test SIP/2.0\r\n" +
		"Via: SIP/2.0/TCP client.example.test;branch=z9hG4bK-conflict\r\n" +
		"From: <sip:alice@example.test>;tag=a\r\n" +
		"To: <sip:bob@example.test>\r\n" +
		"Call-ID: conflict-cl@example.test\r\n" +
		"CSeq: 1 OPTIONS\r\n" +
		"Content-Length: 0\r\n" +
		"l: 1\r\n\r\n")
	_, err := sipContentLength(wire[:len(wire)-4])
	require.Error(t, err, "conflicting full and compact Content-Length fields must not choose one value")
	_, err = sipParseHeaders(wire[:len(wire)-4])
	require.Error(t, err)
}

func TestProtocolSessionSIPFinalResponseRetransmission(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(2, 0)
	call := "retransmit-final@example.test"
	from := "<sip:alice@example.test>;tag=from"
	to := "<sip:bob@example.test>;tag=to"
	branch := "z9hG4bK-final"
	require.Nil(t, s.Feed(0, ts, sipMsg("OPTIONS sip:bob@example.test SIP/2.0", [][2]string{
		{"Via", "SIP/2.0/UDP client.example.test;branch=" + branch}, {"From", from}, {"To", "<sip:bob@example.test>"},
		{"Call-ID", call}, {"CSeq", "1 OPTIONS"},
	}, "")).Err)
	response := sipResp("200", "OK", branch, "1 OPTIONS", call, from, to)
	first := s.Feed(1, ts, response)
	require.Nil(t, first.Err, "%v", first.Err)
	require.Equal(t, "matched", first.Events[0].Session["Association Status"])
	second := s.Feed(1, ts.Add(10*time.Millisecond), response)
	require.Nil(t, second.Err, "%v", second.Err)
	require.Equal(t, "matched", second.Events[0].Session["Association Status"])
	require.Equal(t, true, second.Events[0].Session["Retransmission"])
	require.Equal(t, 1, second.Events[0].Session["Retransmission Count"])
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

func TestProtocolSessionSIPCancelAndInviteFinalResponseOrdering(t *testing.T) {
	ts := time.Unix(3, 0)
	call := "cancel-race@example.test"
	from := "<sip:alice@example.test>;tag=alice"
	to := "<sip:bob@example.test>"
	toTagged := "<sip:bob@example.test>;tag=bob"
	branch := "z9hG4bK-cancel-race"
	invite := sipMsg("INVITE sip:bob@example.test SIP/2.0", [][2]string{
		{"Via", "SIP/2.0/UDP alice.example.test;branch=" + branch},
		{"From", from}, {"To", to}, {"Call-ID", call}, {"CSeq", "77 INVITE"},
	}, "")
	cancel := sipMsg("CANCEL sip:bob@example.test SIP/2.0", [][2]string{
		{"Via", "SIP/2.0/UDP alice.example.test;branch=" + branch},
		{"From", from}, {"To", to}, {"Call-ID", call}, {"CSeq", "77 CANCEL"},
	}, "")

	t.Run("CANCEL accepted then INVITE 487", func(t *testing.T) {
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		require.Nil(t, s.Feed(0, ts, invite).Err)

		r := s.Feed(0, ts.Add(time.Millisecond), cancel)
		require.Nil(t, r.Err, "%v", r.Err)
		require.Equal(t, "INVITE", r.Events[0].Session["Cancels"])

		r = s.Feed(1, ts.Add(2*time.Millisecond), sipResp("200", "OK", branch, "77 CANCEL", call, from, to))
		require.Nil(t, r.Err, "%v", r.Err)
		require.Equal(t, "CANCEL", r.Events[0].Session["Matched Request"])
		require.Equal(t, "matched", r.Events[0].Session["Association Status"])

		r = s.Feed(1, ts.Add(3*time.Millisecond), sipResp("487", "Request Terminated", branch, "77 INVITE", call, from, toTagged))
		require.Nil(t, r.Err, "%v", r.Err)
		require.Equal(t, 487, r.Events[0].Session["Status"])
		require.Equal(t, "INVITE", r.Events[0].Session["Matched Request"])
		require.Equal(t, "matched", r.Events[0].Session["Association Status"])

		ack := sipMsg("ACK sip:bob@example.test SIP/2.0", [][2]string{
			{"Via", "SIP/2.0/UDP alice.example.test;branch=" + branch},
			{"From", from}, {"To", toTagged}, {"Call-ID", call}, {"CSeq", "77 ACK"},
		}, "")
		r = s.Feed(0, ts.Add(4*time.Millisecond), ack)
		require.Nil(t, r.Err, "%v", r.Err)
		require.Equal(t, 487, r.Events[0].Session["ACK For INVITE"])
		require.Equal(t, "non-2xx-invite-ack", r.Events[0].Session["Association Status"])
	})

	t.Run("INVITE 200 wins before CANCEL", func(t *testing.T) {
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		require.Nil(t, s.Feed(0, ts, invite).Err)

		r := s.Feed(1, ts.Add(time.Millisecond), sipResp("200", "OK", branch, "77 INVITE", call, from, toTagged))
		require.Nil(t, r.Err, "%v", r.Err)
		require.Equal(t, "INVITE", r.Events[0].Session["Matched Request"])
		require.Equal(t, "confirmed", r.Events[0].Session["Dialog State"])

		r = s.Feed(0, ts.Add(2*time.Millisecond), cancel)
		require.Nil(t, r.Err, "%v", r.Err)
		require.NotContains(t, r.Events[0].Session, "Cancels", "a completed INVITE cannot be canceled")
		r = s.Feed(1, ts.Add(3*time.Millisecond), sipResp("200", "OK", branch, "77 CANCEL", call, from, to))
		require.Nil(t, r.Err, "%v", r.Err)
		require.Equal(t, "CANCEL", r.Events[0].Session["Matched Request"])
		require.Equal(t, "matched", r.Events[0].Session["Association Status"])
	})
}

func TestProtocolSessionSIPDirectionAndReconnectIsolation(t *testing.T) {
	ts := time.Unix(4, 0)
	call := "direction@example.test"
	from := "<sip:alice@example.test>;tag=alice"
	to := "<sip:bob@example.test>"
	toTagged := "<sip:bob@example.test>;tag=bob"
	branch := "z9hG4bK-direction"
	request := sipMsg("OPTIONS sip:bob@example.test SIP/2.0", [][2]string{
		{"Via", "SIP/2.0/UDP alice.example.test;branch=" + branch},
		{"From", from}, {"To", to}, {"Call-ID", call}, {"CSeq", "5 OPTIONS"},
	}, "")
	response := sipResp("200", "OK", branch, "5 OPTIONS", call, from, toTagged)

	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s.Feed(0, ts, request).Err)

	wrongDirection := s.Feed(0, ts.Add(time.Millisecond), response)
	require.Nil(t, wrongDirection.Err, "%v", wrongDirection.Err)
	require.Equal(t, true, wrongDirection.Events[0].Session["Unmatched"], "a response in the request direction is not the transaction peer")
	require.Equal(t, "direction-mismatch", wrongDirection.Events[0].Session["Association Status"])

	correctDirection := s.Feed(1, ts.Add(2*time.Millisecond), response)
	require.Nil(t, correctDirection.Err, "%v", correctDirection.Err)
	require.Equal(t, "OPTIONS", correctDirection.Events[0].Session["Matched Request"], "the mismatched-direction response must leave the transaction pending")
	require.Equal(t, "matched", correctDirection.Events[0].Session["Association Status"])

	// A new session represents a new flow or a reconnected flow. Transaction
	// identifiers from the old connection must not leak into its response map.
	reconnected, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	r := reconnected.Feed(1, ts.Add(3*time.Millisecond), response)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Unmatched"])
	require.Equal(t, "missing-request", r.Events[0].Session["Association Status"], "event direction=%d session=%#v", r.Events[0].Direction, r.Events[0].Session)
}

func TestProtocolSessionSIPResponseFirstStaysUnmatched(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(4, 1)
	call := "response-first@example.test"
	from := "<sip:alice@example.test>;tag=alice"
	to := "<sip:bob@example.test>"
	toTagged := "<sip:bob@example.test>;tag=bob"
	branch := "z9hG4bK-response-first"
	request := sipMsg("OPTIONS sip:bob@example.test SIP/2.0", [][2]string{
		{"Via", "SIP/2.0/UDP alice.example.test;branch=" + branch},
		{"From", from}, {"To", to}, {"Call-ID", call}, {"CSeq", "6 OPTIONS"},
	}, "")
	response := sipResp("200", "OK", branch, "6 OPTIONS", call, from, toTagged)

	firstResponse := s.Feed(1, ts, response)
	require.Nil(t, firstResponse.Err, "%v", firstResponse.Err)
	require.Equal(t, true, firstResponse.Events[0].Session["Unmatched"])
	require.Equal(t, "missing-request", firstResponse.Events[0].Session["Association Status"])

	laterRequest := s.Feed(0, ts.Add(time.Millisecond), request)
	require.Nil(t, laterRequest.Err, "%v", laterRequest.Err)
	require.Equal(t, "OPTIONS", laterRequest.Events[0].Session["Method"])
	require.Equal(t, true, laterRequest.Events[0].Session["Outstanding"])
	require.NotContains(t, laterRequest.Events[0].Session, "Matched Response")
	// Events are immutable snapshots: seeing the request later does not rewrite
	// the earlier response's incomplete association.
	require.Equal(t, "missing-request", firstResponse.Events[0].Session["Association Status"])

	otherFlow, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	otherFlowRequest := otherFlow.Feed(0, ts, request)
	require.Nil(t, otherFlowRequest.Err, "%v", otherFlowRequest.Err)
	capturedElsewhere, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	otherFlowResponse := capturedElsewhere.Feed(1, ts.Add(2*time.Millisecond), response)
	require.Nil(t, otherFlowResponse.Err, "%v", otherFlowResponse.Err)
	require.Equal(t, true, otherFlowResponse.Events[0].Session["Unmatched"])
	require.Equal(t, "missing-request", otherFlowResponse.Events[0].Session["Association Status"], "another flow's transaction table must not associate this response")
}

func TestProtocolSessionSDPMediaAssociationExpiresAtTTL(t *testing.T) {
	budget := DefaultParserBudget()
	parser := &binParser{budget: budget, config: BinParserConfig{MaxMessageBytes: budget.MaxMessageBytes, MaxBufferedBytes: budget.MaxBufferedBytes}}
	ts := time.Unix(7, 0)
	domain := CaptureDomain{Interface: 1}
	sdp, err := sipParseSDP([]byte("v=0\r\n"+
		"o=- 1 1 IN IP4 127.0.0.1\r\n"+
		"s=-\r\n"+
		"c=IN IP4 127.0.0.1\r\n"+
		"t=0 0\r\n"+
		"m=audio 40000 RTP/AVP 0\r\n"), budget.MaxCollectionElements)
	require.NoError(t, err)
	offer := &ProtocolEvent{
		ID: 7, Protocol: "sip", Domain: domain, Timestamp: ts,
		SourceBytes: ByteSource{PacketRefs: []PacketReference{{Number: 7, Domain: domain}}},
		Session: map[string]any{
			"Packet Name": "INVITE", "Method": "INVITE", "Call-ID": "media-ttl@example.test",
			"From Tag": "alice", "To Tag": "", "SDP": sdp,
		},
	}
	parser.observeSIPMedia(offer)
	require.Len(t, parser.mediaEntries, 1)
	require.Equal(t, 1, offer.Session["SDP Media Candidates"])

	beforeExpiry := parser.matchSDPMedia(domain, "198.51.100.2:50000", "127.0.0.1:40000", 0, false, ts.Add(sipMediaAssociationTTL-time.Nanosecond))
	require.Equal(t, "observed-offer", beforeExpiry.status)
	require.Equal(t, "media-ttl@example.test", beforeExpiry.callID)
	require.ElementsMatch(t, []string{"127.0.0.1:40000", "127.0.0.1:40001"}, beforeExpiry.endpoints)

	expired := parser.matchSDPMedia(domain, "198.51.100.2:50000", "127.0.0.1:40000", 0, false, ts.Add(sipMediaAssociationTTL))
	require.Empty(t, expired.status, "the entry expires when now reaches its deadline")
	require.Empty(t, parser.mediaEntries)
	require.Empty(t, parser.mediaIndex, "RTP and paired RTCP endpoint indexes are pruned together")
	require.Zero(t, parser.buffered.Load(), "expiry releases the association's reserved memory")
}

func TestProtocolSessionSIPOfferlessInviteNegotiatesOnlyAfterACK(t *testing.T) {
	budget := DefaultParserBudget()
	parser := &binParser{budget: budget, config: BinParserConfig{MaxMessageBytes: budget.MaxMessageBytes, MaxBufferedBytes: budget.MaxBufferedBytes}}
	ts := time.Unix(8, 0)
	domain := CaptureDomain{Interface: 2}
	callID, fromTag, toTag := "offerless@example.test", "alice", "bob"
	parseSDP := func(address string, port int) map[string]any {
		wire := fmt.Sprintf("v=0\r\n"+
			"o=- 1 1 IN IP4 127.0.0.1\r\n"+
			"s=-\r\n"+
			"c=IN IP4 %s\r\n"+
			"t=0 0\r\n"+
			"m=audio %d RTP/AVP 0\r\n", address, port)
		sdp, err := sipParseSDP([]byte(wire), budget.MaxCollectionElements)
		require.NoError(t, err)
		return sdp
	}
	response := &ProtocolEvent{
		ID: 41, Protocol: "sip", Domain: domain, Timestamp: ts,
		SourceBytes: ByteSource{PacketRefs: []PacketReference{{Number: 41, Domain: domain}}},
		Session: map[string]any{
			"Packet Name": "Response", "Association Status": "matched", "Matched Request": "INVITE",
			"Call-ID": callID, "From Tag": fromTag, "To Tag": toTag, "SDP": parseSDP("198.51.100.20", 42000),
		},
	}
	parser.observeSIPMedia(response)
	require.Len(t, parser.mediaEntries, 1)
	beforeACK := parser.matchSDPMedia(domain, "198.51.100.20:42000", "203.0.113.40:53000", 0, false, ts)
	require.Equal(t, "observed-offer", beforeACK.status, "a 2xx SDP offer must wait for the ACK answer")
	require.Equal(t, []uint64{41}, beforeACK.eventIDs)

	ack := &ProtocolEvent{
		ID: 42, Protocol: "sip", Domain: domain, Timestamp: ts.Add(time.Millisecond),
		SourceBytes: ByteSource{PacketRefs: []PacketReference{{Number: 42, Domain: domain}}},
		Session: map[string]any{
			"Packet Name": "ACK", "Method": "ACK", "Association Status": "invite-2xx-ack",
			"Call-ID": callID, "From Tag": fromTag, "To Tag": toTag, "SDP": parseSDP("203.0.113.40", 53000),
		},
	}
	parser.observeSIPMedia(ack)

	for _, endpoint := range []string{"198.51.100.20:42000", "203.0.113.40:53000"} {
		matched := parser.matchSDPMedia(domain, endpoint, "192.0.2.1:60000", 0, false, ts.Add(2*time.Millisecond))
		require.Equal(t, "matched", matched.status, endpoint)
		require.Equal(t, callID, matched.callID)
		require.ElementsMatch(t, []uint64{41, 42}, matched.eventIDs)
		require.ElementsMatch(t, []PacketReference{{Number: 41, Domain: domain}, {Number: 42, Domain: domain}}, matched.packetRefs)
	}
}

func TestProtocolSessionSIPForkedDialogsHaveIndependentTeardown(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(5, 0)
	call := "forked-dialogs@example.test"
	from := "<sip:alice@example.test>;tag=alice"
	to := "<sip:bob@example.test>"
	branch := "z9hG4bK-fork"
	invite := sipMsg("INVITE sip:bob@example.test SIP/2.0", [][2]string{
		{"Via", "SIP/2.0/UDP alice.example.test;branch=" + branch},
		{"From", from}, {"To", to}, {"Call-ID", call}, {"CSeq", "9 INVITE"},
	}, "")
	require.Nil(t, s.Feed(0, ts, invite).Err)

	toA, toB := "<sip:bob@example.test>;tag=fork-a", "<sip:bob@example.test>;tag=fork-b"
	responseA := sipResp("200", "OK", branch, "9 INVITE", call, from, toA)
	r := s.Feed(1, ts.Add(time.Millisecond), responseA)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "confirmed", r.Events[0].Session["Dialog State"])
	r = s.Feed(1, ts.Add(2*time.Millisecond), responseA)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Retransmission"])
	require.Equal(t, 1, r.Events[0].Session["Retransmission Count"])

	r = s.Feed(1, ts.Add(3*time.Millisecond), sipResp("200", "OK", branch, "9 INVITE", call, from, toB))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "confirmed", r.Events[0].Session["Dialog State"])

	ack := func(to, ackBranch string) []byte {
		return sipMsg("ACK sip:bob@example.test SIP/2.0", [][2]string{
			{"Via", "SIP/2.0/UDP alice.example.test;branch=" + ackBranch},
			{"From", from}, {"To", to}, {"Call-ID", call}, {"CSeq", "9 ACK"},
		}, "")
	}
	for _, dialog := range []struct{ to, branch string }{{toA, "z9hG4bK-ack-a"}, {toB, "z9hG4bK-ack-b"}} {
		r = s.Feed(0, ts.Add(4*time.Millisecond), ack(dialog.to, dialog.branch))
		require.Nil(t, r.Err, "%v", r.Err)
		require.Equal(t, "2xx-dialog", r.Events[0].Session["ACK For INVITE"])
		require.Equal(t, "invite-2xx-ack", r.Events[0].Session["Association Status"])
		require.Equal(t, uint32(9), r.Events[0].Session["INVITE CSeq"])
		require.Equal(t, "confirmed", r.Events[0].Session["Dialog State"])
	}

	bye := func(to, byeBranch string) []byte {
		return sipMsg("BYE sip:bob@example.test SIP/2.0", [][2]string{
			{"Via", "SIP/2.0/UDP alice.example.test;branch=" + byeBranch},
			{"From", from}, {"To", to}, {"Call-ID", call}, {"CSeq", "10 BYE"},
		}, "")
	}
	byeA := bye(toA, "z9hG4bK-bye-a")
	r = s.Feed(0, ts.Add(5*time.Millisecond), byeA)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "terminating", r.Events[0].Session["Dialog State"])
	r = s.Feed(1, ts.Add(6*time.Millisecond), sipResp("200", "OK", "z9hG4bK-bye-a", "10 BYE", call, from, toA))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "terminated", r.Events[0].Session["Dialog State"])

	// A retransmitted fork-a final response and its ACK must not reopen the
	// terminated fork or affect fork-b's still-confirmed state.
	r = s.Feed(1, ts.Add(7*time.Millisecond), responseA)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Retransmission"])
	r = s.Feed(0, ts.Add(8*time.Millisecond), ack(toA, "z9hG4bK-ack-a"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "terminated", r.Events[0].Session["Dialog State"])

	r = s.Feed(0, ts.Add(9*time.Millisecond), bye(toB, "z9hG4bK-bye-b"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "terminating", r.Events[0].Session["Dialog State"], "fork-b remains live after fork-a teardown")
	r = s.Feed(1, ts.Add(10*time.Millisecond), sipResp("200", "OK", "z9hG4bK-bye-b", "10 BYE", call, from, toB))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "terminated", r.Events[0].Session["Dialog State"])
}

func FuzzProtocolSessionSIPMediaStateSequence(f *testing.F) {
	f.Add([]byte{0, 1, 2})             // INVITE, CANCEL, 487
	f.Add([]byte{0, 3, 4, 3, 5, 6, 7}) // forked 2xx, replay, ACK, BYE
	f.Add([]byte{9, 10, 11, 12, 13})   // RTP wrap, out-of-order, duplicate, RTCP
	f.Add([]byte{8, 0, 0, 1, 2, 3, 4, 5, 6, 7, 9, 10, 11, 12, 13})
	f.Fuzz(func(t *testing.T, actions []byte) {
		if len(actions) > 32 {
			actions = actions[:32]
		}
		budget := DefaultParserBudget()
		budget.MaxFrameBytes = 4096
		budget.MaxMessageBytes = 4096
		budget.MaxBufferedBytes = 16 << 10
		budget.MaxCollectionElements = 64
		sip, err := NewProtocolSession(budget)
		require.NoError(t, err)
		media, err := NewProtocolSession(budget)
		require.NoError(t, err)
		baseline := media.Feed(0, time.Unix(99, 0), rtpPkt(0, 1, 0, 0x12345678))
		require.Nil(t, baseline.Err, "initial media packet: %v", baseline.Err)
		require.False(t, baseline.NeedMore)
		require.Len(t, baseline.Events, 1)
		require.Equal(t, "decoded", baseline.Events[0].Status)

		call := "sequence-fuzz@example.test"
		from := "<sip:alice@example.test>;tag=alice"
		to := "<sip:bob@example.test>"
		toA, toB := "<sip:bob@example.test>;tag=fork-a", "<sip:bob@example.test>;tag=fork-b"
		branch := "z9hG4bK-sequence-fuzz"
		invite := sipMsg("INVITE sip:bob@example.test SIP/2.0", [][2]string{
			{"Via", "SIP/2.0/UDP alice.example.test;branch=" + branch},
			{"From", from}, {"To", to}, {"Call-ID", call}, {"CSeq", "1 INVITE"},
		}, "")
		cancel := sipMsg("CANCEL sip:bob@example.test SIP/2.0", [][2]string{
			{"Via", "SIP/2.0/UDP alice.example.test;branch=" + branch},
			{"From", from}, {"To", to}, {"Call-ID", call}, {"CSeq", "1 CANCEL"},
		}, "")
		ackA := sipMsg("ACK sip:bob@example.test SIP/2.0", [][2]string{
			{"Via", "SIP/2.0/UDP alice.example.test;branch=z9hG4bK-ack-a"},
			{"From", from}, {"To", toA}, {"Call-ID", call}, {"CSeq", "1 ACK"},
		}, "")
		byeA := sipMsg("BYE sip:bob@example.test SIP/2.0", [][2]string{
			{"Via", "SIP/2.0/UDP alice.example.test;branch=z9hG4bK-bye-a"},
			{"From", from}, {"To", toA}, {"Call-ID", call}, {"CSeq", "2 BYE"},
		}, "")
		sipOps := [][]byte{
			invite,
			cancel,
			sipResp("487", "Request Terminated", branch, "1 INVITE", call, from, toA),
			sipResp("200", "OK", branch, "1 INVITE", call, from, toA),
			sipResp("200", "OK", branch, "1 INVITE", call, from, toB),
			ackA,
			byeA,
			sipResp("200", "OK", "z9hG4bK-bye-a", "2 BYE", call, from, toA),
			invite,
		}
		mediaOps := [][]byte{
			rtpPkt(0, 65534, 0, 0x12345678),
			rtpPkt(0, 0, 160, 0x12345678),
			rtpPkt(0, 65535, 320, 0x12345678),
			rtpPkt(0, 0, 160, 0x12345678),
			rtcpRRWithExtendedHighest(0xabcdef01, 0x12345678, 0, 0x7fffff, 0x00010000),
		}
		for i, action := range actions {
			ts := time.Unix(100, int64(i)*int64(time.Millisecond))
			var session ProtocolSession
			var wire []byte
			var dir int
			var protocol, packetName string
			if action%14 < 9 {
				op := int(action % 14)
				wire, session, protocol = sipOps[op], sip, "sip"
				dir = 0
				if op == 2 || op == 3 || op == 4 || op == 7 {
					dir, packetName = 1, "Response"
				} else {
					packetName = []string{"INVITE", "CANCEL", "", "", "", "ACK", "BYE", "", "INVITE"}[op]
				}
			} else {
				op := int(action%14) - 9
				wire, session, protocol = mediaOps[op], media, "rtp"
				dir = 0
				if op == 4 {
					dir, packetName = 1, "RR"
				} else {
					packetName = "RTP"
				}
			}
			r := session.Feed(dir, ts, wire)
			require.Nil(t, r.Err, "%s op %d: %v", protocol, action, r.Err)
			require.False(t, r.NeedMore, "%s op %d left an incomplete message", protocol, action)
			require.Equal(t, protocol, r.State)
			require.Len(t, r.Events, 1)
			e := r.Events[0]
			require.Equal(t, "decoded", e.Status)
			require.Equal(t, protocol, e.Protocol)
			require.Equal(t, packetName, e.Session["Packet Name"])
			if protocol == "sip" {
				require.Equal(t, call, e.Session["Call-ID"])
				require.NotEmpty(t, e.Session["CSeq Method"])
			} else if packetName == "RTP" {
				require.Equal(t, uint32(0x12345678), e.Session["SSRC"])
				require.IsType(t, uint16(0), e.Session["Sequence"])
			} else {
				blocks, ok := e.Session["Report Blocks"].([]map[string]any)
				require.True(t, ok)
				require.Len(t, blocks, 1)
				require.Equal(t, int32(0x7fffff), blocks[0]["Cumulative Packets Lost"])
				require.Equal(t, uint32(0x00010000), blocks[0]["Extended Highest Sequence"])
			}
		}
	})
}
