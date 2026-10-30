package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Independent MS-CIFS 2.2.3.1/2.2.4.52.1 construction. This request is the
// bounded native profile; response-header controls below do not extend it.
func smb1AliasContractRequest(status uint32, flags2, high, low uint16) []byte {
	w := make([]byte, 39)
	copy(w[4:], []byte{0xff, 'S', 'M', 'B', 0x72})
	binary.LittleEndian.PutUint32(w[9:13], status)
	w[13] = 0x18
	binary.LittleEndian.PutUint16(w[14:16], flags2)
	binary.LittleEndian.PutUint16(w[16:18], high)
	binary.LittleEndian.PutUint16(w[30:32], low)
	w = append(w, 2)
	w = append(w, "SMB 2.002"...)
	w = append(w, 0)
	binary.LittleEndian.PutUint16(w[37:39], uint16(len(w)-39))
	binary.BigEndian.PutUint32(w[:4], uint32(len(w)-4))
	return w
}

func smb1AliasContractEvent(t *testing.T, flags2 uint16) *ProtocolEvent {
	t.Helper()
	w := smb1AliasContractRequest(0xc0000022, flags2, 0x1122, 0x7788)
	fields, err := decodeSMB1NegotiateFields(w, DefaultParserBudget())
	require.NoError(t, err)
	return &ProtocolEvent{ID: 1, Protocol: "smb", Status: "decoded", Raw: w, Length: len(w), Fields: fields, Session: fields}
}

func smb1AliasContractValue(t *testing.T, e *ProtocolEvent, name string) DisplayFieldValue {
	t.Helper()
	var matches []DisplayFieldValue
	for _, value := range e.DisplayFieldValues().Values {
		if value.Name == name {
			matches = append(matches, value)
		}
	}
	require.Len(t, matches, 1, name)
	return matches[0]
}

func TestSMB1AliasContractRequestStatusIsRawNotNTStatus(t *testing.T) {
	for _, flags2 := range []uint16{0, 0x4000, 0xc853} {
		t.Run(fmt.Sprintf("flags2-%04x", flags2), func(t *testing.T) {
			e := smb1AliasContractEvent(t, flags2)
			before := bytes.Clone(e.Raw)
			values := e.DisplayFieldValues()
			require.False(t, values.Incomplete)
			require.NotContains(t, values.FieldMap(), "smb.nt_status", "request flag requests future response formatting, not a server error")
			require.Equal(t, []any{flags2&0x4000 != 0}, values.FieldMap()["smb.nt_status_requested"])
			raw := smb1AliasContractValue(t, e, "smb.status_raw")
			require.Equal(t, uint64(0xc0000022), raw.Value)
			require.Equal(t, "pdu", raw.Origin)
			require.Equal(t, &DisplayFieldRange{Offset: 9, Length: 4}, raw.Range)
			require.Equal(t, uint32(0xc0000022), binary.LittleEndian.Uint32(e.Raw[raw.Range.Offset:raw.Range.Offset+raw.Range.Length]))
			require.Equal(t, uint32(0xc0000022), e.Session["Header"].(map[string]any)["Status"], "raw canonical value is unchanged")
			require.Equal(t, before, e.Raw)
		})
	}
}

func TestSMB1AliasContractResponseStatusRequiresBothFlags(t *testing.T) {
	// These are explicit header-projection controls, not native response parses.
	for _, reply := range []bool{false, true} {
		for _, nt := range []bool{false, true} {
			t.Run(fmt.Sprintf("reply-%v-nt-%v", reply, nt), func(t *testing.T) {
				flags2 := uint16(0)
				if nt {
					flags2 = 0x4000
				}
				e := smb1AliasContractEvent(t, flags2)
				if reply {
					e.Raw[13] |= 0x80
					e.Session["Header"].(map[string]any)["Flags"] = uint8(e.Raw[13])
					_, err := decodeSMB1NegotiateFields(e.Raw, DefaultParserBudget())
					require.Error(t, err, "response admission remains outside the native request profile")
				}
				for _, throughJSON := range []bool{false, true} {
					projected := e
					if throughJSON {
						encoded, err := json.Marshal(e)
						require.NoError(t, err)
						var restored ProtocolEvent
						require.NoError(t, json.Unmarshal(encoded, &restored))
						projected = &restored
					}
					values := projected.DisplayFields()
					if reply && nt {
						status := smb1AliasContractValue(t, projected, "smb.nt_status")
						require.Equal(t, uint64(0xc0000022), status.Value)
						require.Equal(t, &DisplayFieldRange{Offset: 9, Length: 4}, status.Range)
					} else {
						require.NotContains(t, values, "smb.nt_status")
					}
					if reply {
						require.NotContains(t, values, "smb.nt_status_requested")
					} else {
						require.Equal(t, []any{nt}, values["smb.nt_status_requested"])
					}
					require.Equal(t, []any{uint64(0xc0000022)}, values["smb.status_raw"])
				}
			})
		}
	}
}

func TestSMB1AliasContractPIDCombinesSeparatedLiteralHalves(t *testing.T) {
	for _, halves := range [][2]uint16{{0, 0}, {0x1122, 0x7788}, {0xffff, 0xffff}} {
		t.Run(fmt.Sprintf("%04x-%04x", halves[0], halves[1]), func(t *testing.T) {
			w := smb1AliasContractRequest(0, 0x4000, halves[0], halves[1])
			fields, err := decodeSMB1NegotiateFields(w, DefaultParserBudget())
			require.NoError(t, err)
			e := &ProtocolEvent{Protocol: "smb", Raw: w, Length: len(w), Session: fields}
			for _, throughJSON := range []bool{false, true} {
				projected := e
				if throughJSON {
					encoded, err := json.Marshal(e)
					require.NoError(t, err)
					var restored ProtocolEvent
					require.NoError(t, json.Unmarshal(encoded, &restored))
					projected = &restored
				}
				high := smb1AliasContractValue(t, projected, "smb.pid.high")
				low := smb1AliasContractValue(t, projected, "smb.pid.low")
				pid := smb1AliasContractValue(t, projected, "smb.pid")
				require.Equal(t, uint64(halves[0]), high.Value)
				require.Equal(t, uint64(halves[1]), low.Value)
				require.Equal(t, &DisplayFieldRange{Offset: 16, Length: 2}, high.Range)
				require.Equal(t, &DisplayFieldRange{Offset: 30, Length: 2}, low.Range)
				require.Equal(t, uint64(halves[0])<<16|uint64(halves[1]), pid.Value)
				require.Equal(t, "derived", pid.Origin)
				require.Nil(t, pid.Range, "noncontiguous halves must not invent a single PDU span")
				descriptor, ok := LookupDisplayField("smb.pid")
				require.True(t, ok)
				require.Equal(t, 32, descriptor.Bits)
				filter, err := CompileDisplayFilter(fmt.Sprintf("smb.pid == %d", uint64(halves[0])<<16|uint64(halves[1])))
				require.NoError(t, err)
				require.True(t, filter.Match(projected))
			}
		})
	}
}

func TestSMB1AliasContractMissingAndInvalidHalvesNeverDefaultToZero(t *testing.T) {
	for _, key := range []string{"PID High", "Process ID Low"} {
		for _, value := range []any{nil, uint32(0x10000), -1, 0.5, "not-an-integer"} {
			e := smb1AliasContractEvent(t, 0x4000)
			header := e.Session["Header"].(map[string]any)
			if value == nil {
				delete(header, key)
			} else {
				header[key] = value
			}
			require.NotContains(t, e.DisplayFields(), "smb.pid", "%s=%v", key, value)
		}
	}
	for _, key := range []string{"Flags", "Flags 2"} {
		e := smb1AliasContractEvent(t, 0x4000)
		delete(e.Session["Header"].(map[string]any), key)
		require.NotContains(t, e.DisplayFields(), "smb.nt_status")
		require.NotContains(t, e.DisplayFields(), "smb.nt_status_requested")
	}
}

func TestSMB1AliasContractNativeSessionAndJSONKeepOwnedHeaderValues(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprintf("deferred-%v", deferred), func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"))
			require.NoError(t, err)
			t.Cleanup(func() { s.Close("alias contract complete") })
			s.(*captureSession).f.a.config.Deferred = deferred
			wire := smb1AliasContractRequest(0xc0000022, 0x4000, 0x1122, 0x7788)
			first := s.Feed(0, time.Unix(1, 0), wire[:32])
			require.Equal(t, 32, first.Consumed)
			require.NotNil(t, first.Err)
			require.Equal(t, ErrNeedMore, first.Err.Kind)
			require.True(t, first.NeedMore)
			require.Empty(t, first.Events)
			result := s.Feed(0, time.Unix(2, 0), wire[32:])
			require.Equal(t, len(wire)-32, result.Consumed)
			require.Nil(t, result.Err)
			require.Len(t, result.Events, 1)
			e := result.Events[0]
			require.Equal(t, "smb", e.Protocol)
			require.Equal(t, "cifs-negotiate-request", e.Profile)
			before := e.DisplayFields()
			require.NotContains(t, before, "smb.nt_status")
			require.Equal(t, []any{uint64(0x11227788)}, before["smb.pid"])
			require.Equal(t, []any{uint64(0xc0000022)}, before["smb.status_raw"])
			encoded, err := json.Marshal(e)
			require.NoError(t, err)
			var restored ProtocolEvent
			require.NoError(t, json.Unmarshal(encoded, &restored))
			for _, projected := range []*ProtocolEvent{e, &restored} {
				require.Equal(t, before, projected.DisplayFields())
				require.Nil(t, smb1AliasContractValue(t, projected, "smb.pid").Range)
				require.Equal(t, &DisplayFieldRange{Offset: 16, Length: 2}, smb1AliasContractValue(t, projected, "smb.pid.high").Range)
				require.Equal(t, &DisplayFieldRange{Offset: 30, Length: 2}, smb1AliasContractValue(t, projected, "smb.pid.low").Range)
			}
			fields, err := e.GetFields()
			require.NoError(t, err)
			fields["Header"].(map[string]any)["PID High"] = uint16(0)
			fields["Header"].(map[string]any)["Status"] = uint32(0)
			e.Session["Header"].(map[string]any)["Flags"] = uint8(0x80)
			e.Session["Header"].(map[string]any)["Process ID Low"] = uint16(0)
			require.Equal(t, before, e.DisplayFields(), "mutable public DTOs cannot change accepted alias semantics")
			fresh, err := e.GetFields()
			require.NoError(t, err)
			require.Equal(t, uint16(0x1122), fresh["Header"].(map[string]any)["PID High"])
			require.Equal(t, uint16(0x7788), fresh["Header"].(map[string]any)["Process ID Low"])
		})
	}
}
