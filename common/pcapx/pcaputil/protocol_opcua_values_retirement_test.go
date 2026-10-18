package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOPCUAStructuredAssociationRetirement(t *testing.T) {
	for _, field := range []string{"Read", "Write", "Call"} {
		t.Run(field, func(t *testing.T) {
			root := "opcua-" + strings.ToLower(field) + "-structured-values"
			raw, err := opcuaSelectedInput(root, "material/cases.json")
			require.NoError(t, err)
			var cases []uaReadCase
			d := json.NewDecoder(bytes.NewReader(raw))
			d.UseNumber()
			require.NoError(t, d.Decode(&cases))
			var normal, bad uaReadCase
			for _, c := range cases {
				switch c.Name {
				case "nested-two":
					normal = c
				case "diagnostic-null-table-entry":
					bad = c
				}
			}
			for _, kind := range []string{"ResourceExceeded", "ContextRequired"} {
				t.Run(kind, func(t *testing.T) {
					s := &binOPCUA{read: field == "Read", write: field == "Write", call: field == "Call", structured: true, arrays: true, maxDepth: 64}
					c := normal
					if kind == "ContextRequired" {
						c = bad
					}
					for _, step := range c.Steps[:2] {
						w, err := hex.DecodeString(step.Hex)
						require.NoError(t, err)
						out, err := s.consumeEvent(step.Dir, &ProtocolEvent{Raw: w}, 1024, 1<<20)
						require.NoError(t, err)
						requireExactProtocolFields(t, step.Session, out)
					}
					require.Len(t, s.appPending, 1)
					w, err := hex.DecodeString(c.Steps[2].Hex)
					require.NoError(t, err)
					if kind == "ResourceExceeded" {
						s.appReserve = func(int64) error { return protocolError(ErrResourceExceeded, "structured value admission") }
					}
					_, err = s.consumeEvent(1, &ProtocolEvent{Raw: w}, 1024, 1<<20)
					rocTypedError(t, kind, err)
					require.Empty(t, s.appPending)
					require.Empty(t, s.chunks)
					require.Equal(t, map[uaKey]uint32{{0, 6, 1}: 446}, s.pending)
					s.appReserve = nil
					later, err := hex.DecodeString(normal.Steps[2].Hex)
					require.NoError(t, err)
					binary.LittleEndian.PutUint32(later[16:20], 2)
					out, err := s.consumeEvent(1, &ProtocolEvent{Raw: later}, 1024, 1<<20)
					require.NoError(t, err)
					require.Equal(t, false, out["Matched"])
					requireExactProtocolFields(t, normal.Steps[2].Read, out[field].(map[string]any))
					require.Empty(t, s.appPending)
				})
			}
		})
	}
}
