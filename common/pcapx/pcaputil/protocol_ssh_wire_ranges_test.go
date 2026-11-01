package pcaputil

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func sshAssertLiteralRange(t *testing.T, owner map[string]any, name string, start, end uint64) {
	t.Helper()
	ranges, ok := owner["Wire Field Ranges"].(map[string]any)
	require.True(t, ok, "literal owner %s must retain positions", name)
	require.Equal(t, []any{start, end}, ranges[name], name)
}

func TestSSHIdentificationAndPreLineRangesExcludeTerminators(t *testing.T) {
	for _, version := range []string{"2.0", "1.99"} {
		for _, ending := range []string{"\r\n", "\n"} {
			t.Run(version+fmt.Sprintf("-%q", ending), func(t *testing.T) {
				state := &binSSH{client: 0, clientKnown: true}
				line := "SSH-" + version + "-range_test"
				info, err := state.consume(0, []byte(line+ending))
				require.NoError(t, err)
				sshAssertLiteralRange(t, info, "Identification", 0, uint64(len(line)))
				sshAssertLiteralRange(t, info, "Version", 4, uint64(4+len(version)))
				pre, err := state.consume(1, []byte("ready"+ending))
				require.NoError(t, err)
				sshAssertLiteralRange(t, pre, "Text", 0, 5)
				require.NotContains(t, pre["Wire Field Ranges"], "Line Number")
			})
		}
	}
}

func TestSSHNativeRangesKeepCurrentOffersSeparateFromSelectedContext(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprint(deferred), func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionClientDirection(0))
			require.NoError(t, err)
			defer s.Close("SSH owned range control")
			s.(*captureSession).f.a.config.Deferred = deferred
			for _, d := range []int{1, 0} {
				require.Nil(t, s.Feed(d, time.Unix(1, 0), sshIdent("SSH-2.0-range_test")).Err)
			}
			offer := "aes256-ctr,aes128-ctr"
			method := "curve25519-sha256"
			wire := sshKex(method, "ssh-ed25519", offer, "hmac-sha2-256", "none")
			require.Nil(t, s.Feed(0, time.Unix(1, 0), sshKex(method, "ssh-ed25519", "aes128-ctr,aes256-ctr", "hmac-sha2-256", "none")).Err)
			// Split inside the length field: metadata must be relative to the
			// completed PDU, never to this last arrival's start.
			r := s.Feed(1, time.Unix(1, 0), wire[:24])
			require.True(t, r.NeedMore)
			require.Empty(t, r.Events)
			r = s.Feed(1, time.Unix(1, 0), wire[24:])
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			e := r.Events[0]
			require.Equal(t, wire, e.Raw)
			sshAssertLiteralRange(t, e.Session, "Message Number", 5, 6)
			sshAssertLiteralRange(t, e.Session, "Kex Algorithms", 26, 43)
			sshAssertLiteralRange(t, e.Session, "Host Key Algorithms", 47, 58)
			sshAssertLiteralRange(t, e.Session, "Offered Encryption C2S", 62, 83)
			sshAssertLiteralRange(t, e.Session, "Offered Encryption S2C", 87, 108)
			require.Equal(t, offer, e.Session["Offered Encryption C2S"])
			require.Equal(t, "aes128-ctr", e.Session["Encryption C2S"])
			ranges := e.Session["Wire Field Ranges"].(map[string]any)
			for _, name := range []string{"Identification", "Kex Algorithm", "Host Key Algorithm", "Encryption C2S", "MAC C2S", "Compression C2S", "Kex Family"} {
				require.NotContains(t, ranges, name, "carried/selected/derived fields have no current literal span")
			}
			fields, err := e.GetFields()
			require.NoError(t, err)
			sshAssertLiteralRange(t, fields, "Packet Length", 0, 4)
			p := fields["Payload"].(map[string]any)
			sshAssertLiteralRange(t, p, "Encryption S2C", 87, 108)
			for _, value := range e.DisplayFieldValues().Values {
				switch value.Name {
				case "ssh.message_code":
					require.Equal(t, &DisplayFieldRange{Offset: 5, Length: 1}, value.Range)
				case "ssh.kex_algorithms":
					require.Equal(t, &DisplayFieldRange{Offset: 26, Length: 17}, value.Range)
				case "ssh.server_host_key_algorithms":
					require.Equal(t, &DisplayFieldRange{Offset: 47, Length: 11}, value.Range)
				default:
					if value.Origin != "pdu" {
						require.Nil(t, value.Range)
					}
				}
			}
			// The public caches and each returned metadata slice are owned.
			ranges["Message Number"].([]any)[0] = uint64(0)
			p["Wire Field Ranges"].(map[string]any)["Encryption S2C"].([]any)[0] = uint64(0)
			e.Fields, e.Structured = map[string]any{"edited": true}, nil
			again, err := e.GetFields()
			require.NoError(t, err)
			sshAssertLiteralRange(t, again["Payload"].(map[string]any), "Encryption S2C", 87, 108)
			for _, value := range e.DisplayFieldValues().Values {
				if value.Name == "ssh.message_code" {
					require.Equal(t, &DisplayFieldRange{Offset: 5, Length: 1}, value.Range)
					value.Range.Offset = 0
				}
			}
			for _, value := range e.DisplayFieldValues().Values {
				if value.Name == "ssh.message_code" {
					require.Equal(t, 5, value.Range.Offset)
				}
			}
		})
	}
}

func TestSSHRoleRefusalRangesDoNotInventMethodFieldsAndRecoveryStaysOwned(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionClientDirection(0))
	require.NoError(t, err)
	defer s.Close("SSH role range recovery")
	for d := 0; d < 2; d++ {
		require.Nil(t, s.Feed(d, time.Unix(1, 0), sshIdent("SSH-2.0-range_test")).Err)
		require.Nil(t, s.Feed(d, time.Unix(1, 0), sshKex("curve25519-sha256", "ssh-ed25519", "aes128-ctr", "hmac-sha2-256", "none")).Err)
	}
	r := s.Feed(0, time.Unix(1, 0), sshValidCurveReply())
	require.NotNil(t, r.Err)
	require.Len(t, r.Events, 1)
	e := r.Events[0]
	sshAssertLiteralRange(t, e.Session, "Message Number", 5, 6)
	require.Len(t, e.Session["Wire Field Ranges"], 1)
	fields, err := e.GetFields()
	require.Error(t, err)
	require.Nil(t, fields)
	e.Session["Wire Field Ranges"].(map[string]any)["Message Number"].([]any)[0] = uint64(0)
	for _, value := range e.DisplayFieldValues().Values {
		if value.Name == "ssh.message_code" {
			require.Equal(t, &DisplayFieldRange{Offset: 5, Length: 1}, value.Range)
		}
	}
	r = s.Feed(1, time.Unix(1, 0), sshValidCurveReply())
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	e = r.Events[0]
	sshAssertLiteralRange(t, e.Session, "Host Key Format", 14, 25)
	ranges := e.Session["Wire Field Ranges"].(map[string]any)
	require.NotContains(t, ranges, "Host Key Fingerprint")
	require.NotContains(t, ranges, "Signature Verified")
	fields, err = e.GetFields()
	require.NoError(t, err)
	payload := fields["Payload"].(map[string]any)
	sshAssertLiteralRange(t, payload["Host Key"].(map[string]any), "Ed25519 Public Key", 29, 61)
	sshAssertLiteralRange(t, payload, "Server Curve25519 Public Key", 65, 97)
	sshAssertLiteralRange(t, payload["Signature"].(map[string]any), "Signature Format", 105, 116)
	for d := 0; d < 2; d++ {
		require.Nil(t, s.Feed(d, time.Unix(1, 0), sshNewKeys()).Err)
	}
	r = s.Feed(1, time.Unix(1, 0), make([]byte, 48))
	require.NotNil(t, r.Err)
	require.Len(t, r.Events, 1)
	require.NotContains(t, r.Events[0].Session, "Wire Field Ranges", "encrypted stream context cannot inherit the cleartext header")
}

func TestSSHWireRangesSurviveReorderingLossAndTupleReuse(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		for _, workers := range []int{1, 2} {
			for _, loseMiddle := range []bool{false, true} {
				t.Run(fmt.Sprintf("deferred%v/workers%d/loss%v", deferred, workers, loseMiddle), func(t *testing.T) {
					banner := sshIdent("SSH-2.0-reorder_test")
					kex := sshKex("curve25519-sha256", "ssh-ed25519", "aes128-ctr", "hmac-sha2-256", "none")
					reply := sshValidCurveReply()
					base := uint32(500 + len(banner) + len(kex))
					steps := []tcpStep{
						{seq: 99, syn: true}, {seq: 499, ack: 100, syn: true, synack: true, reverse: true}, {seq: 100, ack: 500},
						{seq: 100, data: string(banner)}, {seq: 500, data: string(banner), reverse: true},
						{seq: uint32(100 + len(banner)), data: string(kex)}, {seq: uint32(500 + len(banner)), data: string(kex), reverse: true},
						{seq: base + 20, data: string(reply[20:]), reverse: true},
						{seq: base, data: string(reply[:10]), reverse: true},
					}
					if !loseMiddle {
						steps = append(steps, tcpStep{seq: base + 10, data: string(reply[10:20]), reverse: true})
					}
					// Repeating the suffix cannot manufacture a second PDU or fill
					// the missing bytes. A new SYN reuses the same tuple afterward.
					steps = append(steps, tcpStep{seq: base + 20, data: string(reply[20:]), reverse: true})
					secondStart := uint64(len(steps) + 1)
					private := sshKex("private@example.test", "ssh-ed25519", "aes128-ctr", "hmac-sha2-256", "none")
					steps = append(steps,
						tcpStep{seq: 9999, syn: true}, tcpStep{seq: 19999, ack: 10000, syn: true, synack: true, reverse: true}, tcpStep{seq: 10000, ack: 20000},
						tcpStep{seq: 10000, data: string(banner)}, tcpStep{seq: 20000, data: string(banner), reverse: true},
						tcpStep{seq: uint32(10000 + len(banner)), data: string(private)}, tcpStep{seq: uint32(20000 + len(banner)), data: string(private), reverse: true},
						tcpStep{seq: uint32(20000 + len(banner) + len(private)), data: string(reply), reverse: true})
					var mu sync.Mutex
					var events []*ProtocolEvent
					err := ReplayPcap(bytes.NewReader(binTestPcap(t, steps, 50022, false, true)), WithTCPReassemblyStream(64), WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred),
						WithOnProtocolMessage(func(e *ProtocolEvent) { mu.Lock(); defer mu.Unlock(); events = append(events, e) }))
					if loseMiddle {
						require.ErrorContains(t, err, "unfilled sequence gap")
					} else {
						require.NoError(t, err)
					}
					selected, opaque := 0, 0
					var selectedFlow, opaqueFlow uint64
					for _, e := range events {
						if e.Protocol != "ssh" {
							continue
						}
						if e.Session["Packet Name"] == "KEX_ECDH_REPLY" {
							selected++
							selectedFlow = e.TCPFlowID
							fields, err := e.GetFields()
							require.NoError(t, err)
							require.Equal(t, reply, e.Raw)
							payload := fields["Payload"].(map[string]any)
							sshAssertLiteralRange(t, payload["Host Key"].(map[string]any), "Ed25519 Public Key", 29, 61)
							sshAssertLiteralRange(t, payload, "Server Curve25519 Public Key", 65, 97)
							refs := map[uint64]bool{}
							for _, ref := range e.SourceBytes.PacketRefs {
								refs[ref.Number] = true
							}
							require.Equal(t, map[uint64]bool{8: true, 9: true, 10: true}, refs, "the duplicate arrival does not own new PDU bytes")
							require.NotNil(t, e.SourceBytes.CompletedIn)
							require.Equal(t, uint64(10), e.SourceBytes.CompletedIn.Number)
						}
						if e.Session["Packet Name"] == "KEY_EXCHANGE_31 (layout unconfirmed)" {
							opaque++
							opaqueFlow = e.TCPFlowID
							require.Equal(t, "private@example.test", e.Session["Kex Algorithm"])
							require.NotContains(t, e.Session, "Host Key Fingerprint")
							require.NotContains(t, e.Session["Wire Field Ranges"], "Host Key Format")
							fields, err := e.GetFields()
							require.NoError(t, err)
							payload := fields["Payload"].(map[string]any)
							require.NotContains(t, payload, "Host Key")
							sshAssertLiteralRange(t, payload, "Opaque Message Data", 6, 184)
							for _, ref := range e.SourceBytes.PacketRefs {
								require.GreaterOrEqual(t, ref.Number, secondStart)
							}
						}
						if e.Status == "incomplete" {
							require.NotContains(t, e.Session, "Host Key Fingerprint")
							require.NotContains(t, e.Session, "Wire Field Ranges", "loss does not certify any plaintext message field")
						}
					}
					if loseMiddle {
						require.Zero(t, selected, "missing middle cannot be repaired from a repeated suffix")
					} else {
						require.Equal(t, 1, selected)
						require.NotZero(t, selectedFlow)
						require.NotEqual(t, selectedFlow, opaqueFlow)
					}
					require.NotZero(t, opaqueFlow)
					require.Equal(t, 1, opaque, "the reused tuple must negotiate independently")
				})
			}
		}
	}
}
