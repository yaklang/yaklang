package pcaputil

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOPCUAVariantArrays(t *testing.T) {
	for _, field := range []string{"Read", "Write", "Call"} {
		t.Run(field, func(t *testing.T) {
			root := "opcua-" + strings.ToLower(field) + "-variants"
			t.Run("Codec", func(t *testing.T) { opcuaSelectedCodecCases(t, root, field, false) })
			t.Run("Sealed", func(t *testing.T) {
				profiles := 38
				if field == "Call" {
					profiles = 40
				}
				opcuaSelectedSealed(t, root, field, profiles, false)
			})
			t.Run("Resources", func(t *testing.T) { opcuaVariantArrayResources(t, root, field) })
		})
	}
}

func opcuaVariantArrayResources(t *testing.T, root, field string) {
	raw, err := opcuaSelectedInput(root, "material/cases.json")
	require.NoError(t, err)
	var cases []uaReadCase
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	require.NoError(t, d.Decode(&cases))
	decode := func(w []byte, response bool, max int, reserve func(int64) error, enabled bool) (map[string]any, error) {
		switch field {
		case "Read":
			return decodeOPCUARead(w, response, max, reserve, uaValueOptions{arrays: enabled})
		case "Write":
			return decodeOPCUAWrite(w, response, max, reserve, uaValueOptions{arrays: enabled})
		default:
			return decodeOPCUACall(w, response, max, reserve, uaValueOptions{arrays: enabled})
		}
	}
	for _, c := range cases {
		if c.Name != "uint64" && !strings.HasPrefix(c.Name, "matrix-") && c.Name != "null-array" && c.Name != "empty-array" {
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
					_, err = decode(body[:cut], step.Dir == 1, 1024, nil, true)
					require.Error(t, err, "truncation %d", cut)
				}
				var cost int64
				out, err := decode(body, step.Dir == 1, 1024, func(n int64) error { cost = n; return nil }, true)
				require.NoError(t, err)
				requireExactProtocolFields(t, step.Read, out)
				for _, limit := range []int64{cost - 1, cost} {
					_, err = decode(body, step.Dir == 1, 1024, func(n int64) error {
						if n > limit {
							return protocolError(ErrResourceExceeded, "Variant array allocation")
						}
						return nil
					}, true)
					if limit < cost {
						rocTypedError(t, "ResourceExceeded", err)
					} else {
						require.NoError(t, err)
					}
				}
				// The collection budget counts outer operations plus flattened
				// elements and dimensions, not byte size or Method argument arity.
				count := 1
				if field == "Call" {
					count++
				}
				switch c.Name {
				case "uint64":
					count += 2
				case "matrix-two-dimensions":
					count += 4 + 2
				case "matrix-three-dimensions":
					count += 8 + 3
				}
				_, err = decode(body, step.Dir == 1, count-1, nil, true)
				rocTypedError(t, "ResourceExceeded", err)
				_, err = decode(body, step.Dir == 1, count, nil, true)
				require.NoError(t, err)
				_, err = decode(body, step.Dir == 1, 1024, nil, false)
				rocTypedError(t, "UnsupportedFeature", err)
				clear(body)
				requireExactProtocolFields(t, step.Read, out)
			}
		})
	}
}
