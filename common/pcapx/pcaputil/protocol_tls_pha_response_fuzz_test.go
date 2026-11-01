package pcaputil

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzTLS13PHAResponseAssociation(f *testing.F) {
	for _, seed := range [][]byte{{0, 3, 4, 1, 3, 4}, {8, 4, 9, 4}, {1, 3, 4, 8, 4}, {0, 5}, {8, 3}, {2}, {7, 3, 4}, {15, 10, 4}} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, program []byte) {
		if len(program) > 64 {
			program = program[:64]
		}
		flow, state := phaResponseTestFlow(t, 0x1301, 1)
		for _, context := range [][]byte{{0xa1}, {0xb2}} {
			_, err := phaResponseTestSend(t, flow, state, 1, phaContextTestRequest(context), 22)
			require.NoError(t, err)
		}
		// This model describes the response block, independently of private
		// parser state. Public constants supply valid AEAD envelopes, not valid
		// CertificateVerify signatures or Finished transcript MACs.
		seen := [2]bool{}
		active, needVerify, empty := false, false, false
		clientGap, serverGap := false, false
		for _, instruction := range program {
			op := instruction & 7
			if op == 7 {
				if instruction&8 != 0 {
					flow.markTLSInnerGap(1)
					serverGap = true
				} else {
					flow.markTLSInnerGap(0)
					clientGap, active = true, false
				}
				continue
			}
			var encoded []byte
			kind, refuse := byte(22), false
			switch op {
			case 0, 1, 2:
				context := []byte{0xff}
				if op < 2 {
					context[0] = []byte{0xa1, 0xb2}[op]
				}
				chosenEmpty := instruction&8 != 0
				encoded = phaResponseTestCertificate(context, chosenEmpty)
				refuse = active || op < 2 && seen[op] || op == 2 && !serverGap
				if !refuse {
					if op < 2 {
						seen[op] = true
					}
					active, needVerify, empty = true, !chosenEmpty, chosenEmpty
				}
			case 3:
				encoded = certificateVerifyTestMessage(0x304, 0x0804, []byte{1})
				refuse = active && !needVerify || !active && !clientGap
				if active && !refuse {
					needVerify = false
				}
			case 4:
				encoded = finishedTestMessage(32)
				refuse = active && needVerify || !active && !clientGap
				if !refuse {
					active = false
				}
			case 5:
				encoded, kind, refuse = []byte("GET /fuzz HTTP/1.1\r\n\r\n"), 23, active
			case 6:
				encoded, refuse = []byte{24, 0, 0, 1, 0}, active
			}
			secret, iv, key, sequence := bytes.Clone(state.secret[0]), bytes.Clone(state.iv[0]), state.keys[0], state.seq[0]
			event, err := phaResponseTestSend(t, flow, state, 0, encoded, kind)
			if refuse {
				var typed *ProtocolError
				require.ErrorAs(t, err, &typed)
				require.Equal(t, ErrMalformedMessage, typed.Kind)
				require.NotContains(t, event.Session, "Handshake Messages")
				require.Equal(t, secret, state.secret[0])
				require.Equal(t, iv, state.iv[0])
				require.Same(t, key, state.keys[0])
				require.Equal(t, sequence+1, state.seq[0])
				return // a production decoder also stops at a normative refusal
			}
			require.NoError(t, err)
			if op < 5 {
				message := event.Session["Handshake Messages"].([]map[string]any)[0]
				association := phaResponseTestAssociation(t, message)
				if op < 2 {
					require.Equal(t, uint64(op+1), association["Request Ordinal"])
				}
				if empty && active {
					require.Equal(t, "empty certificate list (declined)", association["Certificate Selection"])
				}
			}
		}
	})
}
