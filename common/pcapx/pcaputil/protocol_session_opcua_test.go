package pcaputil

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOPCUASessionValuesDepthAndOwnership(t *testing.T) {
	for _, field := range []string{"Read", "Write", "Call"} {
		t.Run(field, func(t *testing.T) {
			root := "opcua-" + strings.ToLower(field) + "-structured-values"
			raw, err := opcuaSelectedInput(root, "material/cases.json")
			require.NoError(t, err)
			var cases []uaReadCase
			d := json.NewDecoder(bytes.NewReader(raw))
			d.UseNumber()
			require.NoError(t, d.Decode(&cases))
			var c uaReadCase
			for _, v := range cases {
				if v.Name == "nested-two" {
					c = v
				}
			}
			require.NotEmpty(t, c.Name)
			depth := 3
			if field != "Call" {
				depth++
			}
			for _, limit := range []int{depth - 1, depth} {
				t.Run(string(rune('0'+limit)), func(t *testing.T) {
					b := DefaultParserBudget()
					b.MaxRecursionDepth = limit
					o := OPCUAObservationConfig{StructuredValues: true}
					switch field {
					case "Read":
						o.Read = true
					case "Write":
						o.Write = true
					case "Call":
						o.Call = true
					}
					s, err := NewProtocolSessionWithOptions(b, WithSessionOPCUAObservation(o))
					require.NoError(t, err)
					o = OPCUAObservationConfig{} // caller changes cannot alter the selected profile
					for i, step := range c.Steps {
						w, err := hex.DecodeString(step.Hex)
						require.NoError(t, err)
						copyWire := bytes.Clone(w)
						r := s.Feed(step.Dir, time.Unix(int64(i+1), 0), w)
						clear(w)
						isValue := step.Dir == 1 && field == "Read" || step.Dir == 0 && field != "Read" && step.Service != 446
						if limit < depth && isValue {
							require.Len(t, r.Events, 1)
							rocTypedError(t, "ResourceExceeded", r.Err)
							require.Nil(t, r.Events[0].Session)
							require.Equal(t, copyWire, r.Events[0].Raw)
							break
						}
						require.Nil(t, r.Err)
						require.Len(t, r.Events, 1)
						e := r.Events[0]
						require.Equal(t, copyWire, e.Raw)
						require.Zero(t, e.ResponseTo)
						require.Zero(t, e.TransactionID)
						require.Equal(t, step.Dir, e.Direction)
						requireExactProtocolFields(t, step.Session, e.Session)
						if step.Read != nil {
							fields, err := e.GetFields()
							require.NoError(t, err)
							requireExactProtocolFields(t, step.Read, fields[field].(map[string]any))
							fields[field] = nil
							again, err := e.GetFields()
							require.NoError(t, err)
							requireExactProtocolFields(t, step.Read, again[field].(map[string]any))
						}
					}
					_ = s.Close("FIN")
					require.Empty(t, s.Close("FIN"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		})
	}
}

func TestOPCUASessionObservationOptionBoundaries(t *testing.T) {
	_, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionOPCUAObservation(OPCUAObservationConfig{StructuredValues: true}))
	require.Error(t, err)
	for _, reverse := range []bool{false, true} {
		options := []ProtocolSessionOption{WithSessionOPCUAObservation(OPCUAObservationConfig{Read: true}), WithSessionTransport("udp")}
		if reverse {
			options[0], options[1] = options[1], options[0]
		}
		_, err = NewProtocolSessionWithOptions(DefaultParserBudget(), options...)
		require.Error(t, err)
	}
}
