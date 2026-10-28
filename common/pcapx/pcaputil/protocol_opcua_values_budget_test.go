package pcaputil

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOPCUAStructuredValueBudget(t *testing.T) {
	for _, field := range []string{"Read", "Write", "Call"} {
		t.Run(field, func(t *testing.T) {
			root := "opcua-" + strings.ToLower(field) + "-structured-values"
			raw, err := opcuaSelectedInput(root, "material/cases.json")
			require.NoError(t, err)
			var cases []uaReadCase
			d := json.NewDecoder(bytes.NewReader(raw))
			d.UseNumber()
			require.NoError(t, d.Decode(&cases))
			decode := func(w []byte, response bool, max int, reserve func(int64) error, opts uaValueOptions) (map[string]any, error) {
				switch field {
				case "Read":
					return decodeOPCUARead(w, response, max, reserve, opts)
				case "Write":
					return decodeOPCUAWrite(w, response, max, reserve, opts)
				default:
					return decodeOPCUACall(w, response, max, reserve, opts)
				}
			}
			for _, c := range cases {
				if c.Name != "nested-two" && c.Name != "nested-datavalue" && c.Name != "diagnostic-all-fields" && c.Name != "opaque-extension" {
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
							_, err = decode(body[:cut], step.Dir == 1, 1024, nil, uaValueOptions{structured: true, maxDepth: 16})
							require.Error(t, err, "cut%d", cut)
						}
						var cost int64
						out, err := decode(body, step.Dir == 1, 1024, func(n int64) error { cost = n; return nil }, uaValueOptions{structured: true, maxDepth: 16})
						require.NoError(t, err)
						requireExactProtocolFields(t, step.Read, out)
						for _, limit := range []int64{cost - 1, cost} {
							_, err = decode(body, step.Dir == 1, 1024, func(n int64) error {
								if n > limit {
									return protocolError(ErrResourceExceeded, "value allocation boundary")
								}
								return nil
							}, uaValueOptions{structured: true, maxDepth: 16})
							if limit < cost {
								rocTypedError(t, "ResourceExceeded", err)
							} else {
								require.NoError(t, err)
							}
						}
						if c.Name == "nested-two" {
							// Read/Write have an outer DataValue; Call arguments are Variants directly.
							depth := 3
							if field != "Call" {
								depth++
							}
							_, err = decode(body, step.Dir == 1, 1024, nil, uaValueOptions{structured: true, maxDepth: depth - 1})
							rocTypedError(t, "ResourceExceeded", err)
							_, err = decode(body, step.Dir == 1, 1024, nil, uaValueOptions{structured: true, maxDepth: depth})
							require.NoError(t, err)
							// Nested-two is seven aggregate collection/value graph entries
							// for each carrier: count operations, array slots and containers.
							_, err = decode(body, step.Dir == 1, 6, nil, uaValueOptions{structured: true, maxDepth: 16})
							rocTypedError(t, "ResourceExceeded", err)
							_, err = decode(body, step.Dir == 1, 7, nil, uaValueOptions{structured: true, maxDepth: 16})
							require.NoError(t, err)
						}
						clear(body)
						requireExactProtocolFields(t, step.Read, out)
					}
				})
			}
		})
	}
}
