package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

// Compare native display aliases with the captured record bytes, including
// refused records. A literal outer header never authenticates its plaintext.
func TestTLSNativeHeaderFieldRanges(t *testing.T) {
	capture, err := trafficfixture.ReadFile("testdata/protocol-sessions/first-batch-m1/tls-h2-bidi.pcap")
	require.NoError(t, err)
	keylog, err := trafficfixture.ReadFile("testdata/protocol-sessions/first-batch-m1/tls-h2-bidi.keys")
	require.NoError(t, err)
	keys, err := ParseTLSKeyLog(string(keylog))
	require.NoError(t, err)
	wrong := &TLSKeyLog{secrets: map[string][]byte{}}
	for key, secret := range keys.secrets {
		wrong.secrets[key] = bytes.Repeat([]byte{0xff}, len(secret))
	}
	for _, mode := range []string{"missing-key", "valid-key", "wrong-key"} {
		for _, deferred := range []bool{false, true} {
			for _, workers := range []int{1, 2, 4} {
				t.Run(fmt.Sprintf("%s/deferred=%t/workers=%d", mode, deferred, workers), func(t *testing.T) {
					options := []CaptureOption{WithProtocolDeferred(deferred)}
					if mode == "valid-key" {
						options = append(options, WithTLSSecrets(keys))
					} else if mode == "wrong-key" {
						options = append(options, WithTLSSecrets(wrong))
					}
					events, stats, err := binReplay(t, capture, workers, options...)
					require.NoError(t, err)
					require.Zero(t, stats.BufferedBytes)
					records, refused, opaque := 0, 0, 0
					for _, e := range events {
						if e.Protocol != "tls" || len(e.Raw) < 5 {
							continue
						}
						records++
						want := map[string]struct {
							value uint64
							span  DisplayFieldRange
						}{
							"tls.record.wire_type": {uint64(e.Raw[0]), DisplayFieldRange{Offset: 0, Length: 1}},
							"tls.record.version":   {uint64(binary.BigEndian.Uint16(e.Raw[1:3])), DisplayFieldRange{Offset: 1, Length: 2}},
							"tls.record.length":    {uint64(binary.BigEndian.Uint16(e.Raw[3:5])), DisplayFieldRange{Offset: 3, Length: 2}},
						}
						check := func(snapshot DisplayFieldSnapshot) {
							seen := map[string]int{}
							for _, value := range snapshot.Values {
								if expected, ok := want[value.Name]; ok {
									seen[value.Name]++
									require.Equal(t, expected.value, value.Value, value.Name)
									require.Equal(t, "pdu", value.Origin, value.Name)
									require.Equal(t, &expected.span, value.Range, value.Name)
								}
								if value.Name == "tls.record.opaque_type" {
									opaque++
									require.Equal(t, uint64(23), value.Value)
									require.Equal(t, &DisplayFieldRange{Offset: 0, Length: 1}, value.Range)
								}
								if value.Name == "tls.record.content_type" || value.Name == "tls.record.authenticated" || value.Name == "tls.version" || value.Name == "tls.handshake.ciphersuite" {
									require.Nil(t, value.Range, "derived facts and context have no literal record-header span")
								}
							}
							for name := range want {
								require.Equal(t, 1, seen[name], name)
							}
						}
						check(e.DisplayFieldValues())
						fields, fieldErr := e.GetFields()
						if fieldErr != nil {
							refused++
							require.Nil(t, fields)
						} else {
							// Edited field-query results cannot become range authority.
							fields["Record Type"] = byte(1)
							fields["Authentication Verified"] = true
							fields["Wire Field Ranges"] = map[string]any{"Record Type": []any{999, 1000}}
						}
						e.Protocol, e.Status, e.Profile = "dns", "decoded", "edited"
						e.Session = map[string]any{"Record Type": byte(1), "Record Version": uint16(1), "Record Length": 1, "Authentication Verified": true, "Wire Field Ranges": map[string]any{"Record Type": []any{999, 1000}}}
						e.Fields = map[string]any{"Record Type": byte(1)}
						check(e.DisplayFieldValues())
						_, laterErr := e.GetFields()
						if fieldErr != nil {
							require.EqualError(t, laterErr, fieldErr.Error(), "public edits cannot admit refused ciphertext")
						} else {
							require.NoError(t, laterErr)
						}
						original := e.Raw
						typeEdit, versionEdit, lengthEdit := bytes.Clone(original), bytes.Clone(original), bytes.Clone(original)
						typeEdit[0] ^= 1
						versionEdit[1] ^= 1
						lengthEdit[4] ^= 1
						for _, edited := range [][]byte{original[:4], typeEdit, versionEdit, lengthEdit, original[:len(original)-1]} {
							e.Raw = edited
							for _, value := range e.DisplayFieldValues().Values {
								if _, header := want[value.Name]; header || value.Name == "tls.record.opaque_type" {
									require.Nil(t, value.Range, "edited/truncated raw cannot retain an admitted header span")
								}
							}
						}
						e.Raw = original
						check(e.DisplayFieldValues())
					}
					require.Greater(t, records, 20)
					require.Positive(t, opaque)
					if mode == "wrong-key" {
						require.Positive(t, refused)
					}
				})
			}
		}
	}
}

func FuzzTLSNativeHeaderFieldRanges(f *testing.F) {
	f.Add(byte(0), []byte{1}, uint16(2))
	f.Add(byte(1), []byte{2, 0}, uint16(5))
	f.Add(byte(2), reliabilityHS(1, reliabilityHello(true, false, 29, nil)), uint16(7))
	f.Add(byte(3), []byte{1, 2, 3}, uint16(1))
	f.Fuzz(func(t *testing.T, which byte, payload []byte, cut uint16) {
		if len(payload) > 4096 {
			return
		}
		a := carrierTestParser(t)
		a.budget.MaxMessageBytes = 4096
		carrier := newCarrierTLS(a, 1, [2]string{"a", "b"}, CaptureDomain{}, "PEAP")
		wire := carrierTestRecord(20+which%4, payload)
		at := min(int(cut), len(wire))
		events := carrier.feed(0, wire[:at], carrierTestParent(1, false))
		events = append(events, carrier.feed(0, wire[at:], carrierTestParent(2, false))...)
		events = append(events, carrier.close("fuzz")...)
		require.Zero(t, a.buffered.Load())
		for _, e := range events {
			if !e.nativeTLSObservation {
				continue
			}
			before := e.DisplayFieldValues()
			header := map[string]DisplayFieldRange{
				"tls.record.wire_type": {Offset: 0, Length: 1},
				"tls.record.version":   {Offset: 1, Length: 2},
				"tls.record.length":    {Offset: 3, Length: 2},
			}
			seen := map[string]int{}
			for _, value := range before.Values {
				if span, ok := header[value.Name]; ok {
					seen[value.Name]++
					require.GreaterOrEqual(t, len(e.Raw), 5)
					require.Equal(t, int(binary.BigEndian.Uint16(e.Raw[3:5]))+5, len(e.Raw))
					require.Equal(t, &span, value.Range)
					want := uint64(e.Raw[0])
					if span.Length == 2 {
						want = uint64(binary.BigEndian.Uint16(e.Raw[span.Offset : span.Offset+2]))
					}
					require.Equal(t, want, value.Value)
				}
				if value.Origin == "derived" || value.Origin == "context" {
					require.Nil(t, value.Range)
				}
			}
			for name := range header {
				if e.semanticFields != nil {
					require.Equal(t, 1, seen[name], name)
				} else {
					require.Zero(t, seen[name], "framing refusal has no admitted literal record words")
				}
			}
			_, beforeErr := e.GetFields()
			e.Session = map[string]any{"Record Type": byte(23), "Record Version": uint16(0x304), "Record Length": 0, "Authentication Verified": true, "Wire Field Ranges": map[string]any{"Record Type": []any{0, 1}}}
			e.Fields = e.Session
			require.Equal(t, before, e.DisplayFieldValues(), "public maps cannot authorize wire ranges or authentication")
			_, afterErr := e.GetFields()
			if beforeErr == nil {
				require.NoError(t, afterErr)
			} else {
				require.EqualError(t, afterErr, beforeErr.Error())
			}
		}
	})
}
