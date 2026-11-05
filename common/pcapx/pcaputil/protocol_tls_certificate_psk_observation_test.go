package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func certificatePSKObservationRecord(client bool, sid, data []byte) []byte {
	ch, sh := certificatePSKTestHellos(0x1302)
	ext := sh
	kind := byte(2)
	if client {
		ext, kind = ch, 1
	}
	ext = certificatePSKTestChange(ext, 33, data, false)
	body := tlsResumptionHello(client, sid, 0x1302, ext...)
	hs := reliabilityHS(kind, body)
	return append(binary.BigEndian.AppendUint16([]byte{22, 3, 3}, uint16(len(hs))), hs...)
}

func TestTLSCertificatePSKHelloRecordObservation(t *testing.T) {
	for _, client := range []bool{false, true} {
		for _, sid := range []int{0, 16, 32} {
			for _, data := range []int{0, 1, 17} {
				t.Run(fmt.Sprintf("client%v/sid%d/data%d", client, sid, data), func(t *testing.T) {
					raw := certificatePSKObservationRecord(client, bytes.Repeat([]byte{7}, sid), bytes.Repeat([]byte{0xff}, data))
					before := bytes.Clone(raw)
					v, err := ObserveTLSCertificatePSKHelloRecord(raw, 64)
					require.NoError(t, err)
					require.Equal(t, raw[5], v.HandshakeType)
					require.Equal(t, uint16(0x303), v.LegacyVersion)
					require.Equal(t, []byte{0, 33}, raw[v.ExtensionOffset:v.ExtensionOffset+2])
					require.Equal(t, uint16(data), binary.BigEndian.Uint16(raw[v.ExtensionOffset+2:]))
					require.Equal(t, bytes.Repeat([]byte{0xff}, data), raw[v.ExtensionOffset+4:v.ExtensionOffset+4+v.ExtensionDataLength])
					require.Equal(t, before, raw)
				})
			}
		}
	}
}

func TestTLSCertificatePSKHelloRecordObservationFraming(t *testing.T) {
	for _, client := range []bool{false, true} {
		for _, mode := range []string{"short", "extra-record", "incomplete-hello", "session-too-long", "duplicate-extension", "late-truncation", "wrong-record-type", "valid-zero-budget"} {
			t.Run(fmt.Sprintf("client%v/%s", client, mode), func(t *testing.T) {
				raw := certificatePSKObservationRecord(client, nil, []byte{0xff})
				switch mode {
				case "short":
					raw = raw[:20]
				case "extra-record":
					raw = append(raw, raw...)
				case "incomplete-hello":
					raw[8]++
				case "session-too-long":
					raw[43] = 33
				case "duplicate-extension":
					v, err := ObserveTLSCertificatePSKHelloRecord(raw, 64)
					require.NoError(t, err)
					// Replace a different complete entry's type; its body stays opaque.
					at := 9 + 38 + 2
					if client {
						at = 9 + 41 + 2
					}
					require.NotEqual(t, v.ExtensionOffset, at)
					binary.BigEndian.PutUint16(raw[at:], 33)
				case "late-truncation":
					raw = raw[:len(raw)-1]
					binary.BigEndian.PutUint16(raw[3:], uint16(len(raw)-5))
					n := len(raw) - 9
					raw[6], raw[7], raw[8] = byte(n>>16), byte(n>>8), byte(n)
				case "wrong-record-type":
					raw[0] = 23
				}
				v, err := ObserveTLSCertificatePSKHelloRecord(raw, 0)
				var typed *ProtocolError
				require.ErrorAs(t, err, &typed)
				want := ErrMalformedMessage
				if mode == "valid-zero-budget" {
					want = ErrResourceExceeded
				}
				require.Equal(t, want, typed.Kind)
				require.Nil(t, v)
			})
		}
	}
}

func FuzzTLSCertificatePSKHelloRecordObservation(f *testing.F) {
	for _, raw := range [][]byte{certificatePSKObservationRecord(false, nil, nil), certificatePSKObservationRecord(true, nil, []byte{1}), nil, {22, 3, 3, 0, 1, 2}, {22, 3, 3, 0, 0}, certificatePSKObservationRecord(false, []byte{1}, []byte{0xff})} {
		f.Add(raw, byte(64))
	}
	f.Fuzz(func(t *testing.T, raw []byte, limit byte) {
		before := bytes.Clone(raw)
		v, err := ObserveTLSCertificatePSKHelloRecord(raw, int(limit))
		require.Equal(t, before, raw)
		if err != nil {
			require.Nil(t, v)
			return
		}
		require.LessOrEqual(t, v.Extensions, int(limit))
		require.GreaterOrEqual(t, v.ExtensionOffset, 9)
		require.LessOrEqual(t, v.ExtensionOffset+4+v.ExtensionDataLength, len(raw))
		require.Equal(t, []byte{0, 33}, raw[v.ExtensionOffset:v.ExtensionOffset+2])
		require.Equal(t, len(raw)-5, int(binary.BigEndian.Uint16(raw[3:])))
	})
}
