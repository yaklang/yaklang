package pcaputil

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestSTOMPNegotiationMustSelectOfferedVersion(t *testing.T) {
	s := &binSTOMP{clientDir: 0}
	_, err := s.consume(0, []byte(stomp12Connect), 4096)
	require.NoError(t, err)
	_, err = s.consume(1, []byte("CONNECTED\nversion:1.1\n\n\x00"), 4096)
	require.ErrorContains(t, err, "offered")
}

func TestSTOMPRejectsRepeatedConnectedWithoutRewritingState(t *testing.T) {
	for _, observedConnect := range []bool{false, true} {
		for _, repeatedVersion := range []string{"1.1", "1.2"} {
			name := "response-first/"
			if observedConnect {
				name = "complete-handshake/"
			}
			t.Run(name+repeatedVersion, func(t *testing.T) {
				s := &binSTOMP{clientDir: 0}
				feed := func(dir int, wire string) map[string]any {
					t.Helper()
					info, err := s.consume(dir, []byte(wire), 4096)
					require.NoError(t, err)
					return info
				}
				if observedConnect {
					feed(0, "CONNECT\naccept-version:1.1,1.2\nhost:broker.example\n\n\x00")
				}
				info := feed(1, "CONNECTED\nversion:1.2\n\n\x00")
				if !observedConnect {
					require.Equal(t, "partial", info["Context Level"])
				}
				feed(0, "SUBSCRIBE\nid:s\ndestination:/q\nack:client-individual\n\n\x00")
				feed(1, "MESSAGE\nsubscription:s\nmessage-id:m\nack:a\ndestination:/q\n\nx\x00")
				ack := s.state.messages["a"]
				_, err := s.consume(1, []byte("CONNECTED\nversion:"+repeatedVersion+"\n\n\x00"), 4096)
				require.ErrorContains(t, err, "repeated CONNECTED")
				require.Equal(t, "1.2", s.version)
				require.Equal(t, "client-individual", s.state.subscriptions["s"])
				require.Equal(t, ack, s.state.messages["a"])
			})
		}
	}
}

func TestSTOMPRequiredHeadersByVersion(t *testing.T) {
	for _, tc := range []struct{ v, wire string }{
		{"1.2", "SUBSCRIBE\ndestination:/q\n\n\x00"},
		{"1.2", "SUBSCRIBE\nid:s\n\n\x00"},
		{"1.2", "UNSUBSCRIBE\n\n\x00"},
		{"1.0", "UNSUBSCRIBE\n\n\x00"},
		{"1.2", "ACK\nmessage-id:m\n\n\x00"},
		{"1.1", "ACK\nmessage-id:m\n\n\x00"},
		{"1.0", "ACK\nid:m\n\n\x00"},
		{"1.2", "NACK\n\n\x00"},
		{"1.2", "BEGIN\n\n\x00"},
		{"1.2", "COMMIT\n\n\x00"},
		{"1.2", "ABORT\n\n\x00"},
		{"1.2", "MESSAGE\nmessage-id:m\ndestination:/q\n\n\x00"},
		{"1.0", "MESSAGE\ndestination:/q\n\n\x00"},
		{"1.2", "RECEIPT\n\n\x00"},
	} {
		t.Run(tc.v+"/"+tc.wire, func(t *testing.T) {
			s := &binSTOMP{clientDir: 0, version: tc.v}
			dir := 0
			if tc.wire[0] == 'M' || tc.wire[0] == 'R' {
				dir = 1
			}
			_, err := s.consume(dir, []byte(tc.wire), 4096)
			require.Error(t, err)
		})
	}
}

func TestSTOMPVersionSpecificHeaderEncoding(t *testing.T) {
	s := &binSTOMP{clientDir: 0, version: "1.0"}
	info, err := s.consume(0, []byte("SEND\ndestination:/q\nx-label:a:b\\r\\t\n\nx\x00"), 4096)
	require.NoError(t, err)
	require.Equal(t, "a:b\\r\\t", info["Headers"].(map[string]any)["x-label"])
	s = &binSTOMP{clientDir: 0, version: "1.1"}
	_, err = s.consume(0, []byte("SEND\ndestination:/q\nx-label:a\\r\n\nx\x00"), 4096)
	require.Error(t, err)
	s = &binSTOMP{clientDir: 0, version: "1.1"}
	_, err = s.consume(0, []byte("SEND\r\ndestination:/q\r\n\r\nx\x00"), 4096)
	require.Error(t, err)
}

func TestSTOMPResponseFirstHasPartialContext(t *testing.T) {
	s := &binSTOMP{clientDir: 0}
	info, err := s.consume(0, []byte("CONNECTED\nversion:1.2\n\n\x00"), 4096)
	require.NoError(t, err)
	require.Equal(t, "partial", info["Context Level"])
}

func TestSTOMP10HandshakeAdmission(t *testing.T) {
	p := probeSTOMP([]byte("CONNECT\nlogin:fixture\npasscode:fixture\n\n\x00"), 4096)
	require.Equal(t, ProbeAccept, p.Verdict)
}

func TestSTOMPStateAcknowledgementsTransactionsAndReceipts(t *testing.T) {
	s := &binSTOMP{clientDir: 0, version: "1.2"}
	feed := func(dir int, wire string) map[string]any {
		t.Helper()
		info, err := s.consume(dir, []byte(wire), 4096)
		require.NoError(t, err)
		return info
	}
	feed(0, "SUBSCRIBE\nid:s\ndestination:/q\nack:client\nreceipt:subscription\n\n\x00")
	info := feed(1, "RECEIPT\nreceipt-id:subscription\n\n\x00")
	require.Equal(t, "SUBSCRIBE", info["Receipt Command"])
	for _, id := range []string{"a", "b"} {
		feed(1, "MESSAGE\nmessage-id:"+id+"\nack:"+id+"\nsubscription:s\ndestination:/q\n\nx\x00")
	}
	feed(0, "BEGIN\ntransaction:t\n\n\x00")
	feed(0, "ACK\nid:b\ntransaction:t\n\n\x00")
	require.Len(t, s.state.messages, 2)
	feed(0, "ABORT\ntransaction:t\n\n\x00")
	require.Len(t, s.state.messages, 2)
	feed(0, "ACK\nid:b\n\n\x00")
	require.Empty(t, s.state.messages, "client acknowledgement is cumulative")
	info = feed(0, "COMMIT\ntransaction:unobserved\n\n\x00")
	require.Equal(t, "partial", info["Context Level"])
	info = feed(0, "ACK\nid:unobserved\n\n\x00")
	require.Equal(t, "missing-context", info["Association Status"])
	info = feed(1, "RECEIPT\nreceipt-id:unobserved\n\n\x00")
	require.Equal(t, "partial", info["Context Level"])
	feed(0, "UNSUBSCRIBE\nid:s\n\n\x00")
	require.Empty(t, s.state.subscriptions)
}

func TestSTOMPStateRejectsObservedContradictions(t *testing.T) {
	for _, wire := range []string{"BEGIN\ntransaction:t\n\n\x00", "SUBSCRIBE\nid:s\ndestination:/q\n\n\x00"} {
		s := &binSTOMP{clientDir: 0, version: "1.2"}
		_, err := s.consume(0, []byte(wire), 4096)
		require.NoError(t, err)
		_, err = s.consume(0, []byte(wire), 4096)
		require.Error(t, err)
	}
	s := &binSTOMP{clientDir: 0, version: "1.2"}
	_, err := s.consume(0, []byte("SUBSCRIBE\nid:s\ndestination:/q\nack:client-individual\n\n\x00"), 4096)
	require.NoError(t, err)
	_, err = s.consume(1, []byte("MESSAGE\nmessage-id:m\nsubscription:s\ndestination:/q\n\nx\x00"), 4096)
	require.ErrorContains(t, err, "requires ack")
	_, err = s.consume(1, []byte("SEND\ndestination:/q\n\nx\x00"), 4096)
	require.ErrorContains(t, err, "wrong direction")
}

func TestSTOMP10ExplicitAndAnonymousAdmission(t *testing.T) {
	for _, wire := range []string{"CONNECT\naccept-version:1.0\nlogin:fixture\npasscode:fixture\n\n\x00", "CONNECT\n\n\x00"} {
		require.Equal(t, ProbeAccept, probeSTOMP([]byte(wire), 4096).Verdict)
	}
}
func TestSTOMP10SubscribeBodyAndUnknownVersion(t *testing.T) {
	for _, version := range []string{"1.0", ""} {
		s := &binSTOMP{clientDir: 0, version: version}
		_, err := s.consume(0, []byte("SUBSCRIBE\ndestination:/q\n\nignored\x00"), 4096)
		require.NoError(t, err)
	}
}

func TestSTOMPTransactionalACKAtEntryBudget(t *testing.T) {
	s := newReviewSession(t, ParserBudget{MaxCollectionElements: 3})
	for _, step := range []sessionStep{
		{0, []byte(stomp12Connect)}, {1, []byte("CONNECTED\nversion:1.2\n\n\x00")},
		{0, []byte("SUBSCRIBE\nid:s\ndestination:/q\nack:client-individual\n\n\x00")},
		{0, []byte("BEGIN\ntransaction:t\n\n\x00")},
		{1, []byte("MESSAGE\nsubscription:s\nmessage-id:m\nack:a\ndestination:/q\n\nx\x00")},
		{0, []byte("ACK\nid:a\ntransaction:t\n\n\x00")},
		{0, []byte("COMMIT\ntransaction:t\n\n\x00")},
	} {
		require.Nil(t, s.Feed(step.dir, time.Unix(1, 0), step.wire).Err)
	}
}
func TestSTOMPMidstreamLeadingHeartbeat(t *testing.T) {
	wire := []byte("\n\r\nMESSAGE\nsubscription:s\nmessage-id:m\ndestination:/q\n\nx\x00")
	require.Equal(t, ProbeAccept, probeSTOMP(wire, 4096).Verdict)
	for _, chunk := range []int{0, 1, 7} {
		events, _ := sessionTestFlow(t, "stomp", []sessionStep{{1, wire}, {0, []byte("ACK\nid:unobserved\n\n\x00")}}, chunk, false)
		require.NotEmpty(t, events)
		for _, event := range events {
			require.Empty(t, event.Error)
			require.Equal(t, "partial", event.Session["Context Level"])
		}
	}
}

func TestSTOMPReusedReceiptDoesNotGrowEntryBudget(t *testing.T) {
	s := newReviewSession(t, ParserBudget{MaxCollectionElements: 1})
	require.Nil(t, s.Feed(0, time.Unix(1, 0), []byte(stomp12Connect)).Err)
	for i := 0; i < 2; i++ {
		require.Nil(t, s.Feed(0, time.Unix(1, 0), []byte("SEND\ndestination:/q\nreceipt:r\n\nx\x00")).Err)
	}
}

func TestSTOMPTransactionCompletionNeverConsumesUnboundMessages(t *testing.T) {
	for _, transaction := range []string{"", "unobserved"} {
		for _, command := range []string{"COMMIT", "ABORT"} {
			t.Run(command+"/"+transaction, func(t *testing.T) {
				s := &binSTOMP{clientDir: 0}
				feed := func(dir int, wire string) map[string]any {
					t.Helper()
					info, err := s.consume(dir, []byte(wire), 4096)
					require.NoError(t, err)
					return info
				}
				feed(0, stomp12Connect)
				feed(1, "CONNECTED\nversion:1.2\n\n\x00")
				feed(0, "SUBSCRIBE\nid:s\ndestination:/q\nack:client-individual\n\n\x00")
				feed(1, "MESSAGE\nmessage-id:m\nack:a\nsubscription:s\ndestination:/q\n\nx\x00")
				info := feed(0, command+"\ntransaction:"+transaction+"\n\n\x00")
				require.Len(t, s.state.messages, 1, "completion without a transactional ACK must preserve outstanding messages")
				require.Equal(t, "missing-context", info["Transaction Status"])
				require.Equal(t, "partial", info["Context Level"])
				info = feed(0, "ACK\nid:a\n\n\x00")
				require.Equal(t, "matched", info["Association Status"])
			})
		}
	}
}

func TestSTOMPEmptyTransactionIdentifierIsDistinctFromNoTransaction(t *testing.T) {
	for _, command := range []string{"COMMIT", "ABORT"} {
		t.Run(command, func(t *testing.T) {
			s := &binSTOMP{clientDir: 0, version: "1.2", sawConnect: true}
			feed := func(dir int, wire string) map[string]any {
				t.Helper()
				info, err := s.consume(dir, []byte(wire), 4096)
				require.NoError(t, err)
				return info
			}
			feed(0, "SUBSCRIBE\nid:s\ndestination:/q\nack:client-individual\n\n\x00")
			for _, id := range []string{"a", "b"} {
				feed(1, "MESSAGE\nmessage-id:"+id+"\nack:"+id+"\nsubscription:s\ndestination:/q\n\nx\x00")
			}
			feed(0, "BEGIN\ntransaction:\n\n\x00")
			info := feed(0, "ACK\nid:b\ntransaction:\n\n\x00")
			require.Equal(t, "observed", info["Transaction Status"])
			require.Len(t, s.state.messages, 2, "transactional ACK must wait for completion even with an empty identifier")
			info = feed(0, command+"\ntransaction:\n\n\x00")
			require.Equal(t, "observed", info["Transaction Status"])
			require.Contains(t, s.state.messages, "a")
			if command == "COMMIT" {
				require.NotContains(t, s.state.messages, "b")
			} else {
				require.Contains(t, s.state.messages, "b")
				// A later commit without a new transactional ACK cannot consume
				// the message restored by ABORT.
				feed(0, "COMMIT\ntransaction:\n\n\x00")
				require.Len(t, s.state.messages, 2)
			}
			info = feed(0, "ACK\nid:a\n\n\x00")
			require.Equal(t, "matched", info["Association Status"])
		})
	}
}
