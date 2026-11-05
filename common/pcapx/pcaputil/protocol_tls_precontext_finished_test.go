package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// These are deterministic public test keys, not captured session secrets.
type precontextRecoveryTestKeys struct{ random, master []byte }

func (p precontextRecoveryTestKeys) LookupTLSSecret(label string, random []byte) ([]byte, bool) {
	if label != "CLIENT_RANDOM" || !bytes.Equal(random, p.random) {
		return nil, false
	}
	return bytes.Clone(p.master), true
}

func precontextRecoveryTestRecord(t *testing.T, s *binTLS, dir int, seq uint64, typ byte, plain []byte) []byte {
	t.Helper()
	explicit := 8
	if s.suite == 0xcca8 {
		explicit = 0
	}
	header := []byte{typ, 3, 3, 0, 0}
	binary.BigEndian.PutUint16(header[3:], uint16(len(plain)+16+explicit))
	aad := make([]byte, 13)
	binary.BigEndian.PutUint64(aad, seq)
	copy(aad[8:11], header[:3])
	binary.BigEndian.PutUint16(aad[11:], uint16(len(plain)))
	nonce := bytes.Clone(s.iv[dir])
	var suffix []byte
	if explicit == 8 {
		suffix = binary.BigEndian.AppendUint64(nil, seq)
		nonce = append(nonce, suffix...)
	} else {
		for i := 0; i < 8; i++ {
			nonce[11-i] ^= byte(seq >> uint(i*8))
		}
	}
	ciphertext := s.keys[dir].Seal(nil, nonce, plain, aad)
	return append(append(header, suffix...), ciphertext...)
}

func TestTLS12PrecontextFinishedRecovery(t *testing.T) {
	for _, suite := range []uint16{0xc02f, 0xc030, 0xcca8} {
		for dir := 0; dir < 2; dir++ {
			t.Run(fmt.Sprintf("%04x/dir%d", suite, dir), func(t *testing.T) {
				a, _ := remainingParser(t)
				provider := precontextRecoveryTestKeys{random: bytes.Repeat([]byte{0x11}, 32), master: bytes.Repeat([]byte{0x33}, 48)}
				a.tlsSecrets = provider
				sender := newBinTLS()
				sender.version, sender.suite, sender.client = 0x303, suite, 0
				sender.random, sender.serverRandom = bytes.Clone(provider.random), bytes.Repeat([]byte{0x22}, 32)
				ok, err := sender.loadKey(dir, provider)
				require.NoError(t, err)
				require.True(t, ok)
				receiver := newBinTLS()
				receiver.client = 0
				receiver.random = bytes.Clone(provider.random)
				receiver.ccs[dir] = true
				f := &binFlow{a: a, id: 1, protocol: "tls", tls: receiver, transportFlow: &TrafficFlow{}}
				defer f.closeSession()
				finished := reliabilityHS(20, bytes.Repeat([]byte{0x44}, 12))
				previous := &ProtocolEvent{ID: 7, Raw: precontextRecoveryTestRecord(t, sender, dir, 0, 22, finished)}
				_, err = f.consumeTLS(dir, previous)
				require.NoError(t, err)
				require.Equal(t, "missing-context", previous.Session["Secret Lookup Status"])
				require.False(t, previous.Session["Authentication Verified"].(bool))
				original := cloneSession(previous.Session)
				// A later admitted Hello supplies version/suite/server random.
				receiver.version, receiver.suite = sender.version, sender.suite
				receiver.serverRandom = bytes.Clone(sender.serverRandom)
				app := []byte("GET /precontext HTTP/1.1\r\nHost: owned.invalid\r\n\r\n")
				current := &ProtocolEvent{ID: 9, Raw: precontextRecoveryTestRecord(t, sender, dir, 1, 23, app)}
				_, err = f.consumeTLS(dir, current)
				require.NoError(t, err, "a fully observed prior Finished must be authenticated before the first later application record")
				require.Equal(t, true, current.Session["Authentication Verified"])
				require.Equal(t, true, current.Session["Precontext Finished Recovery Verified"])
				require.Equal(t, app, receiver.plaintext)
				require.False(t, f.plaintextGap[dir], "no application bytes were missing: the preceding authenticated content was handshake")
				require.EqualValues(t, 2, receiver.seq[dir])
				require.Equal(t, original, previous.Session, "recovery must not mutate a previously published event")
			})
		}
	}
}

func TestTLS12PrecontextFinishedBoundsFragmentsAndRefusals(t *testing.T) {
	for _, suite := range []uint16{0xc02f, 0xc030, 0xcca8} {
		for dir := 0; dir < 2; dir++ {
			for _, mode := range []string{"whole", "two-records", "four-records", "bad-tag", "short-finished", "trailing-finished", "other-handshake", "application-skipped", "collection-limit", "session-limit", "record-limit", "byte-limit", "earlier-fragment"} {
				t.Run(fmt.Sprintf("%04x/dir%d/%s", suite, dir, mode), func(t *testing.T) {
					a, _ := remainingParser(t)
					provider := precontextRecoveryTestKeys{random: bytes.Repeat([]byte{0x11}, 32), master: bytes.Repeat([]byte{0x33}, 48)}
					a.tlsSecrets = provider
					sender := newBinTLS()
					sender.version, sender.suite, sender.client = 0x303, suite, 0
					sender.random, sender.serverRandom = bytes.Clone(provider.random), bytes.Repeat([]byte{0x22}, 32)
					for d := 0; d < 2; d++ {
						ok, err := sender.loadKey(d, provider)
						require.NoError(t, err)
						require.True(t, ok)
					}
					receiver := newBinTLS()
					receiver.client = 0
					receiver.random = bytes.Clone(provider.random)
					receiver.ccs[dir] = true
					f := &binFlow{a: a, id: 1, protocol: "tls", tls: receiver, transportFlow: &TrafficFlow{}}
					defer func() { f.closeSession(); require.Zero(t, a.stats().BufferedBytes) }()
					finished := reliabilityHS(20, bytes.Repeat([]byte{0x44}, 12))
					parts := [][]byte{finished}
					valid := mode == "whole" || mode == "two-records" || mode == "four-records"
					switch mode {
					case "two-records":
						parts = [][]byte{finished[:7], finished[7:]}
					case "four-records":
						parts = [][]byte{finished[:4], finished[4:8], finished[8:12], finished[12:]}
					case "short-finished":
						parts = [][]byte{reliabilityHS(20, bytes.Repeat([]byte{0x44}, 11))}
					case "trailing-finished":
						parts = [][]byte{append(bytes.Clone(finished), 0)}
					case "other-handshake":
						parts = [][]byte{reliabilityHS(13, []byte{1, 1, 0, 0})}
					case "application-skipped":
						parts = [][]byte{finished, []byte("unknown earlier application bytes")}
					case "collection-limit":
						a.budget.MaxCollectionElements = 0
					case "session-limit":
						a.config.MaxBufferedBytes = 2048
					case "record-limit":
						parts = [][]byte{finished[:3], finished[3:6], finished[6:9], finished[9:12], finished[12:]}
					case "byte-limit":
						parts = [][]byte{make([]byte, 16384), make([]byte, 16384), make([]byte, 16384), make([]byte, 16384)}
					case "earlier-fragment":
						receiver.handshake[dir] = []byte{15, 0}
					}
					var prior []*ProtocolEvent
					var original []map[string]any
					for i, plain := range parts {
						kind := byte(22)
						if mode == "application-skipped" && i == 1 {
							kind = 23
						}
						raw := precontextRecoveryTestRecord(t, sender, dir, uint64(i), kind, plain)
						if mode == "bad-tag" {
							raw[len(raw)-1] ^= 1
						}
						e := &ProtocolEvent{ID: uint64(7 + i), Raw: raw}
						_, err := f.consumeTLS(dir, e)
						require.NoError(t, err)
						prior = append(prior, e)
						original = append(original, cloneSession(e.Session))
						clear(raw)
					}
					receiver.version, receiver.suite = sender.version, sender.suite
					receiver.serverRandom = bytes.Clone(sender.serverRandom)
					app := []byte("GET /complete HTTP/1.1\r\nHost: owned.invalid\r\n\r\n")
					current := &ProtocolEvent{ID: 42, Raw: precontextRecoveryTestRecord(t, sender, dir, uint64(len(parts)), 23, app)}
					_, err := f.consumeTLS(dir, current)
					require.NoError(t, err)
					require.Equal(t, true, current.Session["Authentication Verified"])
					require.Equal(t, app, receiver.plaintext)
					require.Equal(t, !valid, f.plaintextGap[dir])
					if valid {
						require.Equal(t, true, current.Session["Precontext Finished Recovery Verified"])
						messages := current.Session["Recovered Context Handshake Messages"].([]map[string]any)
						require.Len(t, messages, 1)
						require.Equal(t, finished, messages[0]["Encoded Finished"])
						require.Equal(t, []any{uint64(20)}, (&ProtocolEvent{Protocol: "tls", Session: current.Session}).DisplayFields()["tls.handshake.type"])
					} else {
						require.NotContains(t, current.Session, "Recovered Context Handshake Messages")
					}
					for i, e := range prior {
						require.Equal(t, original[i], e.Session)
					}
					require.Empty(t, receiver.precontext[dir])
					require.Zero(t, receiver.precontextCharged[dir])
					opposite := &ProtocolEvent{Raw: precontextRecoveryTestRecord(t, sender, 1-dir, 0, 23, []byte("reverse remains available"))}
					_, err = f.consumeTLS(1-dir, opposite)
					require.NoError(t, err)
					require.Equal(t, true, opposite.Session["Authentication Verified"])
					require.False(t, f.plaintextGap[1-dir])
				})
			}
		}
	}
}

func FuzzTLS12PrecontextFinishedRecovery(f *testing.F) {
	for selector := byte(0); selector < 6; selector++ {
		f.Add(bytes.Repeat([]byte{0x44}, 12), selector, false)
	}
	for _, payload := range [][]byte{nil, {1}, bytes.Repeat([]byte{0x44}, 13)} {
		f.Add(payload, byte(0), false)
	}
	for _, selector := range []byte{0, 1, 5} {
		f.Add(bytes.Repeat([]byte{0x44}, 12), selector, true)
	}
	f.Fuzz(func(t *testing.T, payload []byte, selector byte, badTag bool) {
		if len(payload) > 128 {
			return
		}
		a, _ := remainingParser(t)
		provider := precontextRecoveryTestKeys{random: bytes.Repeat([]byte{0x11}, 32), master: bytes.Repeat([]byte{0x33}, 48)}
		a.tlsSecrets = provider
		suite := []uint16{0xc02f, 0xc030, 0xcca8}[int(selector/2)%3]
		dir := int(selector & 1)
		sender := newBinTLS()
		sender.version, sender.suite, sender.client = 0x303, suite, 0
		sender.random, sender.serverRandom = bytes.Clone(provider.random), bytes.Repeat([]byte{0x22}, 32)
		ok, err := sender.loadKey(dir, provider)
		require.NoError(t, err)
		require.True(t, ok)
		receiver := newBinTLS()
		receiver.client = 0
		receiver.random = bytes.Clone(provider.random)
		receiver.ccs[dir] = true
		flow := &binFlow{a: a, id: 1, protocol: "tls", tls: receiver, transportFlow: &TrafficFlow{}}
		defer func() { flow.closeSession(); require.Zero(t, a.stats().BufferedBytes) }()
		encoded := reliabilityHS(20, payload)
		parts := [][]byte{encoded}
		if selector >= 6 {
			at := 1 + int(selector)%(len(encoded)-1)
			parts = [][]byte{encoded[:at], encoded[at:]}
		}
		for i, part := range parts {
			raw := precontextRecoveryTestRecord(t, sender, dir, uint64(i), 22, part)
			if badTag && i == 0 {
				raw[len(raw)-1] ^= 1
			}
			_, err := flow.consumeTLS(dir, &ProtocolEvent{ID: uint64(7 + i), Raw: raw})
			require.NoError(t, err)
			clear(raw)
		}
		receiver.version, receiver.suite = sender.version, sender.suite
		receiver.serverRandom = bytes.Clone(sender.serverRandom)
		app := []byte("GET /fuzz HTTP/1.1\r\nHost: owned.invalid\r\n\r\n")
		current := &ProtocolEvent{ID: 42, Raw: precontextRecoveryTestRecord(t, sender, dir, uint64(len(parts)), 23, app)}
		_, err = flow.consumeTLS(dir, current)
		require.NoError(t, err)
		require.Equal(t, true, current.Session["Authentication Verified"])
		valid := len(payload) == 12 && !badTag
		require.Equal(t, !valid, flow.plaintextGap[dir])
		if valid {
			require.Equal(t, true, current.Session["Precontext Finished Recovery Verified"])
		} else {
			require.NotContains(t, current.Session, "Recovered Context Handshake Messages")
		}
		require.Empty(t, receiver.precontext[dir])
	})
}

func TestTLS12PrecontextIdentityReservedBeforePublication(t *testing.T) {
	a, _ := remainingParser(t)
	state := newBinTLS()
	state.ccs[0] = true
	f := &binFlow{a: a, id: 1, protocol: "tls", tls: state, transportFlow: &TrafficFlow{}}
	defer func() { f.closeSession(); require.Zero(t, a.stats().BufferedBytes) }()
	// This observer cannot authenticate the record yet. Its identity must
	// already be stable when it is retained, before the ordinary emit callback.
	record := append([]byte{22, 3, 3, 0, 24}, make([]byte, 24)...)
	event := &ProtocolEvent{Raw: record}
	_, err := f.consumeTLS(0, event)
	require.NoError(t, err)
	require.Equal(t, false, event.Session["Authentication Verified"])
	require.NotZero(t, event.ID)
	require.Len(t, state.precontext[0], 1)
	identity := event.ID
	require.Equal(t, identity, state.precontext[0][0].id)
	event.ID += 100 // A public callback cannot rewrite the private association.
	require.Equal(t, identity, state.precontext[0][0].id)
}

func TestTLS12PrecontextMissingSecretDoesNotInventRecordFailure(t *testing.T) {
	for _, completeContext := range []bool{true, false} {
		t.Run(fmt.Sprint(completeContext), func(t *testing.T) {
			a, _ := remainingParser(t)
			provider := precontextRecoveryTestKeys{random: bytes.Repeat([]byte{0x11}, 32), master: bytes.Repeat([]byte{0x33}, 48)}
			sender := newBinTLS()
			sender.version, sender.suite, sender.client = 0x303, 0xc02f, 0
			sender.random, sender.serverRandom = bytes.Clone(provider.random), bytes.Repeat([]byte{0x22}, 32)
			ok, err := sender.loadKey(0, provider)
			require.NoError(t, err)
			require.True(t, ok)
			receiver := newBinTLS()
			receiver.client = 0
			receiver.random = bytes.Clone(provider.random)
			receiver.ccs[0] = true
			if completeContext {
				receiver.version, receiver.suite = sender.version, sender.suite
				receiver.serverRandom = bytes.Clone(sender.serverRandom)
			}
			f := &binFlow{a: a, id: 1, protocol: "tls", tls: receiver, transportFlow: &TrafficFlow{}}
			defer func() { f.closeSession(); require.Zero(t, a.stats().BufferedBytes) }()
			previous := &ProtocolEvent{ID: 7, Raw: precontextRecoveryTestRecord(t, sender, 0, 0, 22, reliabilityHS(20, bytes.Repeat([]byte{0x44}, 12)))}
			_, err = f.consumeTLS(0, previous)
			require.NoError(t, err)
			require.False(t, previous.Session["Authentication Verified"].(bool))
			original := cloneSession(previous.Session)
			if completeContext {
				require.Empty(t, receiver.precontext[0], "missing a secret is not missing Hello context and must not start the precontext queue")
			} else {
				require.Len(t, receiver.precontext[0], 1)
			}
			current := &ProtocolEvent{ID: 9, Raw: precontextRecoveryTestRecord(t, sender, 0, 1, 23, []byte("GET /unavailable HTTP/1.1\r\n\r\n"))}
			_, err = f.consumeTLS(0, current)
			require.NoError(t, err)
			require.NotContains(t, current.Session, "Record Failure Isolated", "no record authentication or structural rejection occurred")
			require.NotEqual(t, "one record rejected; exact-boundary authentication recovery allowed", current.Session["Recovery State"])
			require.False(t, current.Session["Authentication Verified"].(bool))
			require.Nil(t, receiver.plaintext)
			require.Equal(t, original, previous.Session)
			require.Zero(t, receiver.seq[0])
			require.Zero(t, receiver.recoveryFailures[0])
		})
	}
}
