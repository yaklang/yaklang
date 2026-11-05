package pcaputil

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// Message bodies here exercise observed order, not signatures or Finished MACs.
func serverOrderMessage(t *testing.T, kind byte, suite uint16) []byte {
	t.Helper()
	switch kind {
	case 8:
		return reliabilityHS(8, eeScopeBody())
	case 13:
		return certificateRequestFixture(t, true)
	case 11:
		return reliabilityHS(11, certificateExtensionsBody(nil, nil))
	case 15:
		return reliabilityHS(15, []byte{8, 7, 0, 1, 0xa5})
	case 20:
		n := 32
		if suite == 0x1302 {
			n = 48
		}
		return reliabilityHS(20, bytes.Repeat([]byte{0xa5}, n))
	default:
		t.Fatalf("unsupported test kind %d", kind)
		return nil
	}
}

func TestTLS13ObservedServerOrderRejectsRepetitionAndReversal(t *testing.T) {
	kinds := []byte{8, 13, 11, 15}
	for _, server := range []int{0, 1} {
		for i, first := range kinds {
			for _, next := range kinds[:i+1] {
				t.Run(fmt.Sprintf("server%d/%d-then-%d", server, first, next), func(t *testing.T) {
					state := newBinTLS()
					state.client, state.version, state.suite = 1-server, 0x304, 0x1301
					_, err := state.handshakeMessage(server, serverOrderMessage(t, first, state.suite), true)
					require.NoError(t, err)
					before := *state
					message, err := state.handshakeMessage(server, serverOrderMessage(t, next, state.suite), true)
					var typed *ProtocolError
					require.ErrorAs(t, err, &typed)
					require.Equal(t, ErrMalformedMessage, typed.Kind)
					require.Contains(t, err.Error(), "server handshake order")
					require.Nil(t, message)
					require.Equal(t, before, *state, "refusal cannot change negotiated, request, epoch or observation state")
				})
			}
		}
	}
}

func TestTLS13ObservedServerOrderAllowsMissingPrefixesAndSubsequences(t *testing.T) {
	kinds := []byte{8, 13, 11, 15, 20}
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, server := range []int{0, 1} {
			for mask := 1; mask < 1<<len(kinds); mask++ {
				t.Run(fmt.Sprintf("suite%04x/server%d/mask%02x", suite, server, mask), func(t *testing.T) {
					state := newBinTLS()
					state.client, state.version, state.suite = 1-server, 0x304, suite
					// A passive capture can lose any prefix or intermediate record.
					state.recordGap[server], state.handshakeGap[server] = true, true
					for i, kind := range kinds {
						if mask&(1<<i) == 0 {
							continue
						}
						_, err := state.handshakeMessage(server, serverOrderMessage(t, kind, suite), true)
						require.NoError(t, err, "missing messages do not prove wrong peer order")
					}
					fresh := newBinTLS()
					fresh.client, fresh.version, fresh.suite = 1-server, 0x304, suite
					_, err := fresh.handshakeMessage(server, serverOrderMessage(t, 8, suite), true)
					require.NoError(t, err, "another connection cannot inherit this flight")
				})
			}
		}
	}
}

func TestTLS13ObservedServerOrderOnlyCommitsAdmittedMessages(t *testing.T) {
	state := newBinTLS()
	state.client, state.version, state.suite = 0, 0x304, 0x1301
	_, err := state.handshakeMessage(1, reliabilityHS(8, []byte{0, 1}), true)
	require.Error(t, err)
	require.Zero(t, state.serverHandshakeStage)
	_, err = state.handshakeMessage(1, serverOrderMessage(t, 8, state.suite), true)
	require.NoError(t, err)
	_, err = state.handshakeMessage(1, reliabilityHS(15, []byte{8, 7, 0, 2, 0xa5}), true)
	require.Error(t, err)
	require.Equal(t, uint8(1), state.serverHandshakeStage)
	_, err = state.handshakeMessage(1, certificateRequestFixture(t, true), true, 0)
	var typed *ProtocolError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, ErrResourceExceeded, typed.Kind)
	require.Equal(t, uint8(1), state.serverHandshakeStage)
	require.Empty(t, state.requestContexts)
	_, err = state.handshakeMessage(1, certificateRequestFixture(t, true), true)
	require.NoError(t, err, "body and budget refusals cannot consume a later phase")
	require.Equal(t, uint8(2), state.serverHandshakeStage)
}

func TestTLS13ObservedServerOrderDoesNotBorrowUnknownOrClientHistory(t *testing.T) {
	for _, profile := range []string{"unknown-role", "client-direction", "opaque-compressed-certificate"} {
		t.Run(profile, func(t *testing.T) {
			state := newBinTLS()
			state.client, state.version, state.suite = 0, 0x304, 0x1301
			dir, kind := 1, byte(15)
			message := serverOrderMessage(t, kind, state.suite)
			if profile == "unknown-role" {
				state.client = -1
			} else if profile == "client-direction" {
				dir = 0
			} else {
				message = reliabilityHS(25, []byte{0xa5}) // currently uninterpreted, not a validated Certificate
			}
			_, err := state.handshakeMessage(dir, message, true)
			require.NoError(t, err)
			require.Zero(t, state.serverHandshakeStage)
			state.client = 0
			_, err = state.handshakeMessage(1, serverOrderMessage(t, 8, state.suite), true)
			require.NoError(t, err)
		})
	}
}

func TestTLS13ObservedServerOrderLeavesPostHandshakeRequestsIndependent(t *testing.T) {
	state := newBinTLS()
	state.client, state.version, state.suite = 0, 0x304, 0x1301
	_, err := state.handshakeMessage(1, serverOrderMessage(t, 20, state.suite), true)
	require.NoError(t, err)
	require.Equal(t, uint8(5), state.serverHandshakeStage)
	for _, context := range []byte{1, 2} {
		message := certificateRequest13([]byte{context}, certificateRequestExtension(13, []byte{0, 2, 8, 7}))
		_, err = state.handshakeMessage(1, message, true)
		require.NoError(t, err, "independent PHA requests do not restart the initial flight")
		require.Equal(t, uint8(5), state.serverHandshakeStage)
	}
}

func TestTLS13ObservedServerOrderAuthenticatedRecordRefusal(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, server := range []int{0, 1} {
			for _, profile := range []string{"duplicate-EE", "CR-then-EE", "CV-then-Certificate", "fragmented-duplicate-EE"} {
				t.Run(fmt.Sprintf("suite%04x/server%d/%s", suite, server, profile), func(t *testing.T) {
					flow, state := eeRecordFlow(t, suite, server, true)
					first, next := byte(8), byte(8)
					if profile == "CR-then-EE" {
						first = 13
					} else if profile == "CV-then-Certificate" {
						first, next = 15, 11
					}
					encoded := serverOrderMessage(t, first, suite)
					event := &ProtocolEvent{Protocol: "tls", Raw: boundaryTLSRecord(t, state, server, state.seq[server], encoded, 22, 2)}
					_, err := flow.consumeTLS(server, event)
					require.NoError(t, err)
					require.Equal(t, true, event.Session["Authentication Verified"])
					stage, epochs, peerSequence := state.serverHandshakeStage, state.app, state.seq[1-server]
					key, iv, secret := state.keys[server], bytes.Clone(state.iv[server]), bytes.Clone(state.secret[server])
					encoded = serverOrderMessage(t, next, suite)
					pieces := [][]byte{encoded}
					if profile == "fragmented-duplicate-EE" {
						pieces = [][]byte{encoded[:3], encoded[3:]}
					}
					for i, piece := range pieces {
						sequence := state.seq[server]
						event = &ProtocolEvent{Protocol: "tls", Raw: boundaryTLSRecord(t, state, server, sequence, piece, 22, 2)}
						_, err = flow.consumeTLS(server, event)
						require.Equal(t, true, event.Session["Authentication Verified"])
						require.Equal(t, sequence+1, state.seq[server])
						if i < len(pieces)-1 {
							require.NoError(t, err)
							require.Equal(t, true, event.Session["Handshake Incomplete"])
						} else {
							var typed *ProtocolError
							require.ErrorAs(t, err, &typed)
							require.Equal(t, ErrMalformedMessage, typed.Kind)
							require.NotContains(t, event.Session, "Handshake Messages")
						}
						require.Equal(t, stage, state.serverHandshakeStage)
						require.Equal(t, epochs, state.app)
						require.Equal(t, peerSequence, state.seq[1-server])
						require.Same(t, key, state.keys[server])
						require.Equal(t, iv, state.iv[server])
						require.Equal(t, secret, state.secret[server])
						require.Nil(t, state.child)
					}
				})
			}
		}
	}
}

func FuzzTLS13ObservedServerOrder(f *testing.F) {
	for _, seed := range [][]byte{{0, 1, 2, 3}, {3, 2, 1, 0}, {0, 0}, {1, 0}, {2, 1}, {3, 3}} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 128 {
			return
		}
		// Independent ordered subsequence model from the supported server
		// authentication flight; no requirement to observe every message.
		order := []byte{8, 13, 11, 15}
		state, last := newBinTLS(), -1
		state.client, state.version, state.suite = 0, 0x304, 0x1301
		for _, value := range data {
			position := int(value) % len(order)
			before := *state
			_, err := state.handshakeMessage(1, serverOrderMessage(t, order[position], state.suite), true)
			if position <= last {
				var typed *ProtocolError
				require.ErrorAs(t, err, &typed)
				require.Equal(t, ErrMalformedMessage, typed.Kind)
				require.Equal(t, before, *state)
			} else {
				require.NoError(t, err)
				last = position
			}
		}
	})
}
