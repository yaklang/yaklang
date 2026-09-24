package pcaputil

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRound4PendingBudgets(t *testing.T) {
	cases := []struct {
		name                 string
		first, second, reply []byte
	}{
		{"smtp", mailCR("EHLO example.test"), mailCR("NOOP"), mailCR("250 OK")},
		{"imap", mailCR("a1 NOOP"), mailCR("a2 NOOP"), mailCR("a1 OK done")},
		{"coap", coapMsg(0, 1, 1, 1, []byte{0xab}, []byte{0xb1, 's'}, nil), coapMsg(0, 1, 2, 2, []byte{0xcd}, []byte{0xb1, 's'}, nil), coapMsg(2, 1, 69, 1, []byte{0xab}, []byte{0xb1, 's'}, nil)},
		{"modbus", mbap(1, 1, 3, []byte{0, 0, 0, 1}), mbap(2, 1, 3, []byte{0, 0, 0, 1}), mbap(1, 1, 3, []byte{2, 0, 7})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, release := range []bool{false, true} {
				t.Run(fmt.Sprint(release), func(t *testing.T) {
					s, err := NewProtocolSession(ParserBudget{MaxCollectionElements: 1})
					require.NoError(t, err)
					defer s.Close("test")
					ts := time.Unix(1, 0)
					r := s.Feed(0, ts, tc.first)
					require.Nil(t, r.Err, "%v", r.Err)
					if release {
						r = s.Feed(1, ts, tc.reply)
						require.Nil(t, r.Err, "%v", r.Err)
					}
					r = s.Feed(0, ts, tc.second)
					if release {
						require.Nil(t, r.Err, "%v", r.Err)
					} else {
						require.NotNil(t, r.Err)
						require.Equal(t, ErrResourceExceeded, r.Err.Kind)
					}
				})
			}
		})
	}
}

func TestRound4CoAPBidirectionalAssociation(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer s.Close("test")
	ts := time.Unix(1, 0)
	for dir, code := range []byte{1, 2} {
		r := s.Feed(dir, ts, coapMsg(0, 1, code, 7, []byte{0xab}, []byte{0xb1, 's'}, nil))
		require.Nil(t, r.Err, "%v", r.Err)
	}
	for _, tc := range []struct {
		dir  int
		name string
	}{{1, "GET"}, {0, "POST"}} {
		r := s.Feed(tc.dir, ts, coapMsg(2, 1, 69, 7, []byte{0xab}, []byte{0xb1, 's'}, nil))
		require.Nil(t, r.Err, "%v", r.Err)
		require.Len(t, r.Events, 1)
		require.Equal(t, tc.name, r.Events[0].Session["In Reply To"])
	}
}

func TestRound4IMAPLiteralFraming(t *testing.T) {
	for _, plus := range []string{"", "+"} {
		for _, size := range []int{0, 5} {
			for _, chunk := range []int{0, 1, 7} {
				for _, deferred := range []bool{false, true} {
					t.Run(fmt.Sprintf("plus=%s/size=%d/chunk=%d/deferred=%v", plus, size, chunk, deferred), func(t *testing.T) {
						steps := []sessionStep{{1, mailCR("* OK IMAP ready")}, {0, mailCR(fmt.Sprintf("a1 APPEND INBOX {%d%s}", size, plus))}}
						if plus == "" {
							steps = append(steps, sessionStep{1, mailCR("+")})
						}
						body := ""
						if size > 0 {
							body = "hello"
						}
						steps = append(steps, sessionStep{0, mailCR(body)}, sessionStep{1, mailCR("a1 OK done")})
						events, _ := sessionTestFlow(t, "imap", steps, chunk, deferred)
						literals := 0
						for _, e := range events {
							require.Contains(t, []string{"decoded", "deferred"}, e.Status, "%+v", e)
							require.NotEmpty(t, e.Session)
							if e.Session["Role"] == "literal" {
								literals++
								require.Equal(t, size, e.Session["Bytes"])
							}
						}
						if size > 0 {
							require.Equal(t, 1, literals)
						} else {
							require.Zero(t, literals)
						}
						require.Equal(t, "APPEND", events[len(events)-1].Session["In Reply To"])
					})
				}
			}
		}
	}
}

func TestRound4ModbusResponseContract(t *testing.T) {
	cases := []struct {
		name               string
		fc                 byte
		request, bad, good []byte
	}{
		{"coils", 1, []byte{0, 0, 0, 9}, []byte{1, 1}, []byte{2, 1, 0}},
		{"registers", 3, []byte{0, 0, 0, 2}, []byte{2, 0, 1}, []byte{4, 0, 1, 0, 2}},
		{"single-register", 6, []byte{0, 7, 0, 9}, []byte{0, 8, 0, 9}, []byte{0, 7, 0, 9}},
		{"multiple-registers", 16, []byte{0, 7, 0, 2, 4, 0, 1, 0, 2}, []byte{0, 7, 0, 1}, []byte{0, 7, 0, 2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &binModbus{}
			_, err := s.consume(0, mbap(7, 1, tc.fc, tc.request))
			require.NoError(t, err)
			_, err = s.consume(1, mbap(7, 1, tc.fc, tc.bad))
			require.Error(t, err)
			require.Len(t, s.pending, 1)
			out, err := s.consume(1, mbap(7, 1, tc.fc, tc.good))
			require.NoError(t, err)
			require.NotEmpty(t, out["In Reply To"])
			require.Empty(t, s.pending)
		})
	}
	t.Run("client-exception", func(t *testing.T) {
		s := &binModbus{}
		_, err := s.consume(0, mbap(7, 1, 3, []byte{0, 0, 0, 1}))
		require.NoError(t, err)
		_, err = s.consume(0, mbap(7, 1, 0x83, []byte{2}))
		require.Error(t, err)
		require.Len(t, s.pending, 1)
		_, err = s.consume(1, mbap(7, 1, 0x83, []byte{2}))
		require.NoError(t, err)
		require.Empty(t, s.pending)
	})
}

func BenchmarkRound4SMTPCommandReply(b *testing.B) {
	s := &binSMTP{client: 0}
	cmd, reply := mailCR("NOOP"), mailCR("250 OK")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.consume(0, cmd); err != nil {
			b.Fatal(err)
		}
		if _, err := s.consume(1, reply); err != nil {
			b.Fatal(err)
		}
	}
}

func TestRound4CoAPIndexesAndOptions(t *testing.T) {
	s := &binCoAP{maxPending: 1}
	request := func(mid uint16, token byte) []byte { return coapMsg(0, 1, 1, mid, []byte{token}, nil, nil) }
	for _, wire := range [][]byte{request(7, 1), request(7, 1), request(7, 2), request(8, 2)} {
		_, err := s.consume(0, wire)
		require.NoError(t, err)
		require.Len(t, s.pending, 1)
		require.Len(t, s.tokens, 1)
	}
	// Replaced token and old message ID cannot consume the active transaction.
	for _, wire := range [][]byte{coapMsg(2, 1, 69, 7, []byte{1}, nil, nil), coapMsg(3, 0, 0, 7, nil, nil, nil)} {
		out, err := s.consume(1, wire)
		require.NoError(t, err)
		require.Equal(t, "missing-request", out["Association"])
		require.Len(t, s.pending, 1)
	}
	_, err := s.consume(0, request(9, 3))
	require.Error(t, err)
	require.Len(t, s.pending, 1)
	require.Len(t, s.tokens, 1)
	out, err := s.consume(1, coapMsg(3, 0, 0, 8, nil, nil, nil))
	require.NoError(t, err)
	require.Equal(t, "GET", out["In Reply To"])
	require.Empty(t, s.tokens)
	require.Empty(t, s.pending)
	// Count even unexported options; a payload is not an additional option.
	_, err = s.consume(0, coapMsg(0, 0, 1, 1, nil, []byte{0x00, 0x00}, nil))
	require.ErrorContains(t, err, "option budget")
	_, err = s.consume(0, coapMsg(0, 0, 1, 1, nil, []byte{0x00}, []byte("ok")))
	require.NoError(t, err)
}

func TestRound4IMAPLimitsAndDuplicateTag(t *testing.T) {
	for _, plus := range []string{"", "+"} {
		s, err := NewProtocolSession(ParserBudget{MaxFrameBytes: 64, MaxMessageBytes: 64})
		require.NoError(t, err)
		r := s.Feed(0, time.Unix(1, 0), mailCR(fmt.Sprintf("a1 APPEND INBOX {65%s}", plus)))
		require.NotNil(t, r.Err)
		require.Equal(t, ErrResourceExceeded, r.Err.Kind)
		s.Close("test")
	}
	s := &binIMAP{}
	_, err := s.consume(0, mailCR("a1 NOOP"))
	require.NoError(t, err)
	_, err = s.consume(0, mailCR("a1 LOGOUT"))
	require.ErrorContains(t, err, "outstanding tag")
	require.Equal(t, "NOOP", s.pending["a1"])
	_, err = s.consume(1, mailCR("a1 OK done"))
	require.NoError(t, err)
	require.Empty(t, s.pending)
	require.Zero(t, s.pendingBytes)
}

func TestRound4SMTPQueueReuseAndSnapshot(t *testing.T) {
	s := &binSMTP{client: 0, maxPending: 3}
	for _, cmd := range []string{"EHLO example.test", "MAIL FROM:<a@b>", "RCPT TO:<c@d>"} {
		_, err := s.consume(0, mailCR(cmd))
		require.NoError(t, err)
	}
	for _, cmd := range []string{"EHLO", "MAIL"} {
		out, err := s.consume(1, mailCR("250 OK"))
		require.NoError(t, err)
		require.Equal(t, cmd, out["In Reply To"])
	}
	for _, cmd := range []string{"NOOP", "QUIT"} {
		_, err := s.consume(0, mailCR(cmd))
		require.NoError(t, err)
	}
	owned := cloneSession(map[string]any{"Outstanding": s.pending[s.pendingHead:]})
	for _, cmd := range []string{"RCPT", "NOOP", "QUIT"} {
		out, err := s.consume(1, mailCR("250 OK"))
		require.NoError(t, err)
		require.Equal(t, cmd, out["In Reply To"])
	}
	require.Equal(t, []string{"RCPT", "NOOP", "QUIT"}, owned["Outstanding"])
	require.Zero(t, s.pendingCount())
	require.Zero(t, s.pendingHead)
	for _, slot := range s.pending[:cap(s.pending)] {
		require.Empty(t, slot)
	}
}
