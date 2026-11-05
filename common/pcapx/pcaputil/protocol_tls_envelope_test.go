package pcaputil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type envelopeSecretProbe struct{ calls int }

func (p *envelopeSecretProbe) LookupTLSSecret(string, []byte) ([]byte, bool) {
	p.calls++
	return nil, false
}

func TestTLSCiphertextRefusalOwnsNegotiationAndCannotComeFromDTO(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"))
	require.NoError(t, err)
	t.Cleanup(func() { s.Close("FIN") })
	require.Nil(t, s.Feed(0, time.Unix(1, 0), carrierTestHello()).Err)
	require.Nil(t, s.Feed(1, time.Unix(2, 0), carrierTestRecord(22, reliabilityHS(2, reliabilityHello(false, false, 29, nil)))).Err)
	r := s.Feed(0, time.Unix(3, 0), carrierTestRecord(23, []byte{1, 2, 3}))
	require.NotEmpty(t, r.Events)
	e := r.Events[0]
	observation, ok := e.NativeTLSCiphertextRefusal()
	require.True(t, ok)
	require.Equal(t, uint16(0x304), observation.Version)
	require.Equal(t, false, observation.Fields["Authentication Verified"])
	observation.Fields["Authentication Verified"] = true
	observation.Fields["Negotiated Version"] = uint16(0x303)
	_, before := e.GetFields()
	require.Error(t, before)
	wire, err := json.Marshal(e)
	require.NoError(t, err)
	var dto ProtocolEvent
	require.NoError(t, json.Unmarshal(wire, &dto))
	_, ok = dto.NativeTLSCiphertextRefusal()
	require.False(t, ok, "serialized display metadata is not native negotiation evidence")
	e.Protocol, e.Status, e.Error = "grpc", "decoded", "edited"
	e.Session = map[string]any{"Negotiated Version": uint16(0x303), "Authentication Verified": true}
	e.Fields, e.Structured = e.Session, map[string]any{"fields": e.Session}
	again, ok := e.NativeTLSCiphertextRefusal()
	require.True(t, ok)
	require.Equal(t, uint16(0x304), again.Version)
	require.Equal(t, false, again.Fields["Authentication Verified"])
	require.Equal(t, before.Error(), again.Error)
	_, after := e.GetFields()
	require.EqualError(t, after, before.Error())
}

func FuzzTLSImpossibleEnvelopeAuthenticatedRecovery(f *testing.F) {
	f.Add(byte(0), uint8(0), []byte("same sequence"), false)
	f.Add(byte(1), uint8(1), []byte("one missing record"), false)
	f.Add(byte(2), uint8(16), []byte("bounded skip"), false)
	f.Add(byte(2), uint8(4), []byte("bad tag has no plaintext"), true)
	f.Add(byte(21), uint8(0), []byte("eight impossible envelopes do not consume authentication trials"), false)
	f.Add(byte(128), uint8(1), []byte("oversized record envelope"), false)
	f.Add(byte(129), uint8(16), []byte("over-limit followed by corrupt record"), true)
	f.Add(byte(130), uint8(0), []byte{}, false)
	f.Fuzz(func(t *testing.T, which byte, gap uint8, content []byte, corrupt bool) {
		if len(content) > 4096 {
			return
		}
		suite := []uint16{0x1301, 0x1302, 0x1303}[which%3]
		a, _ := remainingParser(t)
		s := newBinTLS()
		s.version, s.suite, s.client = 0x304, suite, 0
		s.random = bytes.Repeat([]byte{3}, 32)
		s.app = [2]bool{true, true}
		size := 32
		if suite == 0x1302 {
			size = 48
		}
		for dir := 0; dir < 2; dir++ {
			require.NoError(t, s.install13(dir, bytes.Repeat([]byte{byte(dir + 1)}, size)))
		}
		flow := &binFlow{a: a, tls: s, transportFlow: &TrafficFlow{}}
		secret := bytes.Clone(s.secret[0])
		bad := carrierTestRecord(23, bytes.Repeat([]byte{which}, int(which)%17))
		if which&128 != 0 {
			bad = carrierTestRecord(23, bytes.Repeat([]byte{which}, 16384+18+int(which%7)))
		}
		for i := 0; i < 1+int(which/3)%8; i++ {
			_, err := flow.consumeTLS(0, &ProtocolEvent{Raw: bad})
			var pe *ProtocolError
			require.ErrorAs(t, err, &pe)
			require.Equal(t, ErrMalformedMessage, pe.Kind)
			require.Zero(t, s.seq[0])
			require.Zero(t, s.recoveryFailures[0], "no authentication trial was performed")
			require.Equal(t, secret, s.secret[0])
			require.Nil(t, s.plaintext)
		}
		skip := uint64(gap % 17)
		next := recoveryTLSRecord(t, s, 0, skip, content)
		if corrupt {
			next[len(next)-1] ^= 1
		}
		e := &ProtocolEvent{Raw: next}
		_, err := flow.consumeTLS(0, e)
		if corrupt {
			require.Error(t, err)
			require.Equal(t, false, e.Session["Authentication Verified"])
			require.Nil(t, s.plaintext)
		} else {
			require.NoError(t, err)
			require.Equal(t, true, e.Session["Authentication Verified"])
			require.Equal(t, true, e.Session["Record Recovery Verified"])
			require.True(t, bytes.Equal(content, s.plaintext), "authenticated content bytes must be exact, including empty application data")
			require.Equal(t, skip+1, s.seq[0])
		}
		require.Equal(t, secret, s.secret[0], "failed envelopes cannot infer a key update")
		reverse := &ProtocolEvent{Raw: recoveryTLSRecord(t, s, 1, 0, []byte("unaffected peer"))}
		_, err = flow.consumeTLS(1, reverse)
		require.NoError(t, err)
		require.Equal(t, true, reverse.Session["Authentication Verified"])
		require.False(t, flow.plaintextGap[1])
		flow.closeSession()
		require.Zero(t, a.buffered.Load())
	})
}

func TestTLSNegotiatedEnvelopeShapePrecedesSecretLookup(t *testing.T) {
	for _, c := range []struct {
		version, suite uint16
		minimum        int
	}{
		{0x303, 0xc02f, 24}, {0x303, 0xc02b, 24}, {0x303, 0xc030, 24}, {0x303, 0xc02c, 24},
		{0x303, 0x009c, 24}, {0x303, 0x009d, 24}, {0x303, 0xcca8, 16}, {0x303, 0xcca9, 16},
		{0x304, 0x1301, 17}, {0x304, 0x1302, 17}, {0x304, 0x1303, 17},
	} {
		for _, n := range []int{0, c.minimum - 1, c.minimum, c.minimum + 1} {
			t.Run(fmt.Sprintf("%04x-%04x/body%d", c.version, c.suite, n), func(t *testing.T) {
				a, _ := remainingParser(t)
				provider := &envelopeSecretProbe{}
				a.tlsSecrets = provider
				state := newBinTLS()
				state.client, state.version, state.suite = 0, c.version, c.suite
				state.random = bytes.Repeat([]byte{1}, 32)
				state.seq = [2]uint64{9, 19}
				f := &binFlow{a: a, tls: state}
				e := &ProtocolEvent{Raw: carrierTestRecord(23, bytes.Repeat([]byte{1}, n))}
				_, err := f.consumeTLS(0, e)
				if n < c.minimum {
					var pe *ProtocolError
					require.ErrorAs(t, err, &pe)
					require.Equal(t, ErrMalformedMessage, pe.Kind)
					require.Zero(t, provider.calls)
					require.NotContains(t, e.Session, "Secret Lookup Status")
				} else {
					require.NoError(t, err)
					require.Equal(t, 1, provider.calls)
					require.Equal(t, "missing-key", e.Session["Secret Lookup Status"])
				}
				require.Equal(t, [2]uint64{9, 19}, state.seq)
				require.Nil(t, state.keys[0])
				require.Nil(t, state.plaintext)
				require.False(t, e.Session["Authentication Verified"].(bool))
				require.Equal(t, "encrypted", e.Session["Content Visibility"])
				f.closeSession()
				require.Zero(t, a.buffered.Load())
			})
		}
	}
}

func TestTLSUnknownNegotiationDoesNotInventEnvelopeMinimum(t *testing.T) {
	for _, c := range []struct{ version, suite uint16 }{{0, 0}, {0x303, 0}, {0x303, 0x002f}, {0x304, 0x1304}, {0x302, 0xc02f}} {
		t.Run(fmt.Sprintf("%04x-%04x", c.version, c.suite), func(t *testing.T) {
			a, _ := remainingParser(t)
			s := newBinTLS()
			s.client, s.version, s.suite = 0, c.version, c.suite
			s.random = bytes.Repeat([]byte{1}, 32)
			f := &binFlow{a: a, tls: s}
			e := &ProtocolEvent{Raw: carrierTestRecord(23, []byte{1, 2, 3})}
			_, err := f.consumeTLS(0, e)
			require.NoError(t, err)
			require.False(t, e.Session["Authentication Verified"].(bool))
			require.Contains(t, []string{"missing-context", "unsupported-suite"}, e.Session["Secret Lookup Status"])
			f.closeSession()
			require.Zero(t, a.buffered.Load())
		})
	}
}

func TestTLS13ProtectedHeaderRequiresObservedLegacyVersionWithoutKey(t *testing.T) {
	for _, version := range []byte{0, 1, 2, 4} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			a, _ := remainingParser(t)
			s := newBinTLS()
			s.client, s.version, s.suite = 0, 0x304, 0x1301
			s.random = bytes.Repeat([]byte{1}, 32)
			f := &binFlow{a: a, tls: s}
			w := carrierTestRecord(23, bytes.Repeat([]byte{1}, 17))
			w[2] = version
			e := &ProtocolEvent{Raw: w}
			_, err := f.consumeTLS(0, e)
			var pe *ProtocolError
			require.ErrorAs(t, err, &pe)
			require.Equal(t, ErrMalformedMessage, pe.Kind)
			require.False(t, e.Session["Authentication Verified"].(bool))
			require.Zero(t, s.seq[0])
			f.closeSession()
			require.Zero(t, a.buffered.Load())
		})
	}
}

func TestTLSImpossibleEnvelopeRecoversOnlyAtAuthenticatedBoundary(t *testing.T) {
	for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
		for _, skip := range []uint64{0, 1, 16} {
			t.Run(fmt.Sprintf("%04x/skip%d", suite, skip), func(t *testing.T) {
				a, _ := remainingParser(t)
				s := newBinTLS()
				s.version, s.suite, s.client = 0x304, suite, 0
				s.app = [2]bool{true, true}
				s.random = bytes.Repeat([]byte{3}, 32)
				size := 32
				if suite == 0x1302 {
					size = 48
				}
				for dir := 0; dir < 2; dir++ {
					require.NoError(t, s.install13(dir, bytes.Repeat([]byte{byte(dir + 1)}, size)))
				}
				old := bytes.Clone(s.secret[0])
				f := &binFlow{a: a, tls: s, transportFlow: &TrafficFlow{}}
				e := &ProtocolEvent{Raw: carrierTestRecord(23, []byte{1, 2, 3})}
				_, err := f.consumeTLS(0, e)
				var pe *ProtocolError
				require.ErrorAs(t, err, &pe)
				require.Equal(t, ErrMalformedMessage, pe.Kind)
				require.Equal(t, true, e.Session["Record Failure Isolated"])
				require.Zero(t, s.seq[0], "impossible ciphertext does not consume an authenticated sequence")
				require.Equal(t, old, s.secret[0])
				require.True(t, s.recordGap[0])
				require.True(t, s.handshakeGap[0])
				require.False(t, s.recordGap[1])
				e = &ProtocolEvent{Raw: recoveryTLSRecord(t, s, 0, skip, []byte("later authentic content"))}
				_, err = f.consumeTLS(0, e)
				require.NoError(t, err)
				require.Equal(t, true, e.Session["Authentication Verified"])
				require.Equal(t, true, e.Session["Record Recovery Verified"])
				require.Equal(t, skip+1, s.seq[0])
				require.True(t, f.plaintextGap[0])
				e = &ProtocolEvent{Raw: recoveryTLSRecord(t, s, 1, 0, []byte("reverse unaffected"))}
				_, err = f.consumeTLS(1, e)
				require.NoError(t, err)
				require.Equal(t, true, e.Session["Authentication Verified"])
				require.False(t, f.plaintextGap[1])
				f.closeSession()
				require.Zero(t, a.buffered.Load())
			})
		}
	}
}

func TestTLSImpossibleEnvelopeReplayKeepsRecordSourceAndIdentity(t *testing.T) {
	ch := carrierTestHello()
	sh := carrierTestRecord(22, reliabilityHS(2, reliabilityHello(false, false, 29, nil)))
	bad := carrierTestRecord(23, []byte{1, 2, 3})
	good := carrierTestRecord(23, bytes.Repeat([]byte{7}, 17))
	steps := []tcpStep{{seq: 99, syn: true}, {seq: 499, ack: 100, syn: true, synack: true, reverse: true}, {seq: 100, ack: 500, data: string(ch)}, {seq: 500, ack: 100 + uint32(len(ch)), reverse: true, data: string(sh)}, {seq: 100 + uint32(len(ch)), data: string(append(bytes.Clone(bad), good...))}}
	for _, workers := range []int{1, 4} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("workers%d/deferred%t", workers, deferred), func(t *testing.T) {
				events, stats, err := binReplay(t, binTestPcap(t, steps, 443, false, false), workers, WithProtocolDeferred(deferred))
				require.NoError(t, err)
				require.Zero(t, stats.BufferedBytes)
				var records []*ProtocolEvent
				for _, e := range events {
					if e.Protocol == "tls" && len(e.Raw) > 0 && e.Raw[0] == 23 {
						records = append(records, e)
					}
				}
				require.Len(t, records, 2)
				require.Equal(t, "malformed", records[0].Status)
				require.Equal(t, bad, records[0].Raw)
				require.Equal(t, good, records[1].Raw)
				require.Equal(t, records[0].FlowID, records[1].FlowID)
				require.Equal(t, []PacketReference{{Number: 5}}, records[0].SourceBytes.PacketRefs)
				require.Equal(t, records[0].SourceBytes.PacketRefs, records[1].SourceBytes.PacketRefs)
				_, err = records[0].GetFields()
				var pe *ProtocolError
				require.ErrorAs(t, err, &pe)
				require.Equal(t, ErrMalformedMessage, pe.Kind)
				for _, e := range records {
					require.False(t, e.Session["Authentication Verified"].(bool))
					require.Equal(t, uint16(0x304), e.Session["Negotiated Version"])
					seen := 0
					for _, v := range e.DisplayFieldValues().Values {
						if v.Name == "tls.record.length" {
							seen++
							require.Equal(t, uint64(len(e.Raw)-5), v.Value)
							require.Equal(t, &DisplayFieldRange{Offset: 3, Length: 2}, v.Range)
						}
					}
					require.Equal(t, 1, seen)
				}
			})
		}
	}
}
