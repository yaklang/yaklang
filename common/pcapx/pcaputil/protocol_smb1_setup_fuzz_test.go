package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/stretchr/testify/require"
)

func FuzzSMB1SetupMessageSources(f *testing.F) {
	f.Add(smb1SetupWire(false, 2, 0, 0))
	f.Add(smb1SetupWire(true, 2, 2048, 0xc0000016))
	f.Add(smb1SetupWire(true, 3, 2048, 0))
	f.Add(smb1SetupWire(false, 2, 0, 0)[:63])
	f.Add([]byte{0, 0, 0, 80, 0xff, 'S', 'M', 'B'})
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 65599 {
			return
		}
		wire := bytes.Clone(input)
		original := bytes.Clone(wire)
		response := len(wire) > 13 && wire[13]&0x80 != 0
		fields, err := decodeSMB1SessionSetupFields(wire, DefaultParserBudget(), response)
		require.Equal(t, original, wire)
		if err != nil {
			require.Nil(t, fields)
			return
		}
		require.Equal(t, ProbeAccept, probeSMB1SessionSetupExtended(wire, 65598).Verdict)
		require.Equal(t, len(wire)-4, smb2DirectLength(wire))
		base := 63
		if response {
			base = 47
		}
		body := fields["Body"].(map[string]any)
		require.Equal(t, len(wire)-base, int(binary.LittleEndian.Uint16(wire[base-2:base])))
		for _, ownerName := range []string{"Direct TCP", "Header", "Body"} {
			owner := fields[ownerName].(map[string]any)
			for name, value := range owner["Wire Field Ranges"].(map[string]any) {
				span := value.([]any)
				lo, hi := span[0].(uint64), span[1].(uint64)
				require.LessOrEqual(t, lo, hi)
				require.LessOrEqual(t, hi, uint64(len(wire)))
				literal := wire[lo:hi]
				switch v := owner[name].(type) {
				case uint8:
					require.Len(t, literal, 1)
					require.Equal(t, literal[0], v)
				case uint16:
					require.Len(t, literal, 2)
					require.Equal(t, binary.LittleEndian.Uint16(literal), v)
				case uint32:
					if ownerName == "Direct TCP" && name == "Payload Length" {
						require.Len(t, literal, 3)
						require.Equal(t, uint32(literal[0])<<16|uint32(literal[1])<<8|uint32(literal[2]), v)
					} else {
						require.Len(t, literal, 4)
						require.Equal(t, binary.LittleEndian.Uint32(literal), v)
					}
				case []byte:
					require.Equal(t, literal, v)
				case string:
					require.Contains(t, []string{"Native OS", "Native LAN Manager"}, name)
					require.Zero(t, len(literal)%2)
					units := make([]uint16, len(literal)/2)
					for i := range units {
						units[i] = binary.LittleEndian.Uint16(literal[2*i:])
						require.NotZero(t, units[i])
					}
					require.Equal(t, string(utf16.Decode(units)), v)
					require.Equal(t, units, utf16.Encode(utf16.Decode(units)), "accepted UTF-16 must round-trip without replacement")
					raw := body[name+" Raw Bytes"].([]byte)
					require.Equal(t, append(bytes.Clone(literal), 0, 0), raw)
				default:
					t.Fatalf("untyped setup literal %s.%s: %T", ownerName, name, owner[name])
				}
			}
		}
		for _, key := range []string{"Authentication Verified", "Signature Verified", "Session Established"} {
			require.Equal(t, false, fields[key])
		}
		require.NotContains(t, fields, "Continuation Response Event ID")
		require.NotContains(t, fields, "Request Event ID")
		before, err := json.Marshal(fields)
		require.NoError(t, err)
		clear(wire)
		after, err := json.Marshal(fields)
		require.NoError(t, err)
		require.Equal(t, before, after)
	})
}

func FuzzSMB1SetupAssociationSequence(f *testing.F) {
	f.Add([]byte{0, 5, 10, 15})
	f.Add([]byte{0, 5, 0, 5, 10, 15})
	f.Add([]byte{0, 5, 8, 13, 10, 15})
	f.Fuzz(func(t *testing.T, operations []byte) {
		if len(operations) > 16 {
			return
		}
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		defer func() { s.Close("FIN"); require.Zero(t, s.Stats().BufferedBytes) }()
		byID := map[uint64]*ProtocolEvent{}
		requestCounts := map[uint16]int{}
		matched := map[uint64]bool{}
		roots := map[uint64]uint64{}
		uidOwners := map[uint16]uint64{}
		ambiguousUID := map[uint16]bool{}
		for i, operation := range operations {
			kind := operation % 4
			response := kind%2 == 1
			mid := uint16(operation/4%4 + 1)
			uid := []uint16{0, 1, 2048, 0xffff}[int(operation/16)%4]
			if kind == 0 {
				uid = 0
			}
			if response && uid == 0 {
				uid = 2048
			}
			status := uint32(0)
			if kind == 1 {
				status = 0xc0000016
			}
			dir := 0
			if response {
				dir = 1
			}
			w := smb1SetupWire(response, mid, uid, status)
			r := s.Feed(dir, time.Unix(int64(100+i), 0), w)
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			e := r.Events[0]
			require.Equal(t, "decoded", e.Status)
			if !response {
				requestCounts[mid]++
				root := e.ID
				if link, ok := e.Session["Continuation Response Event ID"].(uint64); ok {
					previous := byID[link]
					require.NotNil(t, previous)
					require.NotZero(t, previous.ResponseTo)
					require.Equal(t, uint32(0xc0000016), binary.LittleEndian.Uint32(previous.Raw[9:13]))
					require.Equal(t, uid, binary.LittleEndian.Uint16(previous.Raw[32:34]))
					require.False(t, ambiguousUID[uid], "a collided UID cannot resume an older lineage")
					root = roots[previous.ResponseTo]
					require.NotZero(t, root)
				}
				roots[e.ID] = root
			} else if e.ResponseTo != 0 {
				request := byID[e.ResponseTo]
				require.NotNil(t, request)
				require.Zero(t, request.Raw[13]&0x80)
				require.Equal(t, mid, binary.LittleEndian.Uint16(request.Raw[34:36]))
				require.Equal(t, request.Raw[16:18], e.Raw[16:18])
				require.Equal(t, request.Raw[30:32], e.Raw[30:32])
				reqUID := binary.LittleEndian.Uint16(request.Raw[32:34])
				require.True(t, reqUID == 0 || reqUID == uid)
				require.Equal(t, 1, requestCounts[mid], "a reused request key cannot receive a fresh association")
				require.False(t, matched[e.ResponseTo], "one request cannot be completed twice")
				matched[e.ResponseTo] = true
				root := roots[e.ResponseTo]
				if owner, ok := uidOwners[uid]; ok && owner != root {
					ambiguousUID[uid] = true
				} else {
					uidOwners[uid] = root
				}
			}
			byID[e.ID] = e
		}
	})
}
