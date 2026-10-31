package stream_parser

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func sshEdwardsPacket(format string, key, signature, hostTail, signatureTail []byte, profile string) []byte {
	s := sshPlaintextTestString
	host := bytes.Join([][]byte{s([]byte(format)), s(key), hostTail}, nil)
	sig := bytes.Join([][]byte{s([]byte(format)), s(signature), signatureTail}, nil)
	code, exchange := byte(31), s([]byte{2})
	switch profile {
	case "dh-gex":
		code = 33
	case "ecdh-curve25519":
		exchange = s(bytes.Repeat([]byte{0x81}, 32))
	case "ecdh-nistp256":
		exchange = s(append([]byte{4}, bytes.Repeat([]byte{0x82}, 64)...))
	}
	return sshPlaintextTestPacket([]byte{code}, s(host), exchange, s(sig))
}

func TestSSHPlaintextEdwardsInnerBoundaries(t *testing.T) {
	for _, tc := range []struct {
		format, name string
		key, sig     int
	}{{"ssh-ed25519", "Ed25519", 32, 64}, {"ssh-ed448", "Ed448", 57, 114}} {
		for _, profile := range []string{"dh", "dh-gex", "ecdh-curve25519", "ecdh-nistp256"} {
			t.Run(tc.format+"/"+profile, func(t *testing.T) {
				key, sig := bytes.Repeat([]byte{0xff}, tc.key), bytes.Repeat([]byte{0x81}, tc.sig)
				wire := sshEdwardsPacket(tc.format, key, sig, nil, nil, profile)
				fields, info, err := decodeSSHPlaintextPacket(wire, profile)
				require.NoError(t, err)
				tlsCertificateTestCoverage(t, fields, len(wire))
				require.Equal(t, true, info["Host Key Fields Decoded"])
				require.Equal(t, true, info["Signature Fields Decoded"])
				for _, fact := range []string{"Signature Validated", "Peer Identity Validated", "Key Mathematics Validated", "Handshake Completion Validated"} {
					require.Equal(t, false, info[fact])
				}
				value, err := DecodeSSHInitialPacket(wire, profile)
				require.NoError(t, err)
				payload := value["Wire Fields"].(map[string]any)["Payload"].(map[string]any)
				host := payload["Host Key"].(map[string]any)
				signature := payload["Signature"].(map[string]any)
				require.Equal(t, key, host[tc.name+" Public Key"])
				require.Equal(t, uint32(tc.key), host[tc.name+" Public Key Length"])
				require.Equal(t, sig, signature[tc.name+" Signature Bytes"])
				require.Equal(t, uint32(tc.sig), signature[tc.name+" Signature Bytes Length"])
				require.NotContains(t, host, "Opaque Host Key Data")
				require.NotContains(t, signature, "Opaque Signature Data")
				for cut := range wire {
					_, _, err := decodeSSHPlaintextPacket(wire[:cut], profile)
					require.Error(t, err, "truncated prefix %d", cut)
				}
				for _, n := range []int{0, tc.key - 1, tc.key + 1} {
					_, _, err := decodeSSHPlaintextPacket(sshEdwardsPacket(tc.format, make([]byte, n), sig, nil, nil, profile), profile)
					require.Error(t, err, "key size %d", n)
				}
				for _, n := range []int{0, tc.sig - 1, tc.sig + 1} {
					_, _, err := decodeSSHPlaintextPacket(sshEdwardsPacket(tc.format, key, make([]byte, n), nil, nil, profile), profile)
					require.Error(t, err, "signature size %d", n)
				}
				for _, tail := range []struct{ host, sig []byte }{{[]byte{0}, nil}, {nil, []byte{0}}} {
					_, _, err := decodeSSHPlaintextPacket(sshEdwardsPacket(tc.format, key, sig, tail.host, tail.sig, profile), profile)
					require.Error(t, err, "nested trailing bytes")
				}
				// No selected exchange means the method remains opaque, even for
				// an invalid inner key. A format name is not session context.
				generic, err := DecodeSSHInitialPacket(sshEdwardsPacket(tc.format, key[:1], sig[:1], nil, nil, profile), "transport")
				require.NoError(t, err)
				require.Equal(t, false, generic["Payload Layout Decoded"])
			})
		}
	}
	for _, name := range []string{"ssh-ed25519-cert-v01@openssh.com", "ssh-ed448@example.test"} {
		wire := sshEdwardsPacket(name, []byte{1}, []byte{2}, nil, nil, "dh")
		_, info, err := decodeSSHPlaintextPacket(wire, "dh")
		require.NoError(t, err)
		require.Equal(t, false, info["Host Key Fields Decoded"], fmt.Sprint(info))
		require.Equal(t, false, info["Signature Fields Decoded"])
	}
}

func FuzzSSHPlaintextEdwardsBoundaries(f *testing.F) {
	for _, format := range []string{"ssh-ed25519", "ssh-ed448"} {
		for _, profile := range []string{"dh", "dh-gex", "ecdh-curve25519", "ecdh-nistp256"} {
			k, s := 32, 64
			if format == "ssh-ed448" {
				k, s = 57, 114
			}
			f.Add(sshEdwardsPacket(format, bytes.Repeat([]byte{0x81}, k), bytes.Repeat([]byte{0xff}, s), nil, nil, profile), profile)
		}
	}
	f.Fuzz(func(t *testing.T, wire []byte, profile string) {
		if len(wire) > 35008 {
			t.Skip()
		}
		fields, _, err := decodeSSHPlaintextPacket(wire, profile)
		if err == nil {
			tlsCertificateTestCoverage(t, fields, len(wire))
			_, err := DecodeSSHInitialPacket(wire, profile)
			require.NoError(t, err)
		}
	})
}
