package pcaputil

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOPCUAExtendedVariantFields(t *testing.T) {
	for _, field := range []string{"Read", "Write", "Call"} {
		t.Run(field, func(t *testing.T) {
			root := "opcua-" + strings.ToLower(field) + "-extended-variants"
			t.Run("Codec", func(t *testing.T) { opcuaSelectedCodecCases(t, root, field, false) })
			t.Run("Sealed", func(t *testing.T) { opcuaSelectedSealed(t, root, field, 51, false) })
			t.Run("Resources", func(t *testing.T) { opcuaExtendedVariantResources(t, root, field) })
		})
	}
}

func opcuaExtendedVariantResources(t *testing.T, root, field string) {
	raw, err := opcuaSelectedInput(root, "material/cases.json")
	require.NoError(t, err)
	var cases []uaReadCase
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	require.NoError(t, d.Decode(&cases))
	decode := func(w []byte, response bool, reserve func(int64) error) (map[string]any, error) {
		switch field {
		case "Read":
			return decodeOPCUARead(w, response, 1024, reserve, uaValueOptions{arrays: true})
		case "Write":
			return decodeOPCUAWrite(w, response, 1024, reserve, uaValueOptions{arrays: true})
		default:
			return decodeOPCUACall(w, response, 1024, reserve, uaValueOptions{arrays: true})
		}
	}
	for _, c := range cases {
		switch c.Name {
		case "qualified-name", "localized-both", "expanded-uri-server", "future26-bytes", "xml-text", "qualified-array", "future26-array":
		default:
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			for _, step := range c.Steps[1:] {
				if field == "Read" && step.Dir != 1 || field == "Write" && step.Dir != 0 {
					continue
				}
				w, err := hex.DecodeString(step.Hex)
				require.NoError(t, err)
				_, n, err := uaService(w[24:])
				require.NoError(t, err)
				body := bytes.Clone(w[24+n:])
				for cut := 0; cut < len(body); cut++ {
					_, err = decode(body[:cut], step.Dir == 1, nil)
					require.Error(t, err, "cut %d", cut)
				}
				var cost int64
				out, err := decode(body, step.Dir == 1, func(n int64) error { cost = n; return nil })
				require.NoError(t, err)
				requireExactProtocolFields(t, step.Read, out)
				for _, limit := range []int64{cost - 1, cost} {
					_, err = decode(body, step.Dir == 1, func(n int64) error {
						if n > limit {
							return protocolError(ErrResourceExceeded, "extended Variant allocation")
						}
						return nil
					})
					if limit < cost {
						rocTypedError(t, "ResourceExceeded", err)
					} else {
						require.NoError(t, err)
					}
				}
				clear(body)
				requireExactProtocolFields(t, step.Read, out)
			}
		})
	}
}
