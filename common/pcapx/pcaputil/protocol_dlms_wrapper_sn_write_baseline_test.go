package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// Existing public API witnesses: no new symbol is required for the old code to
// compile. Exact fixed-source wire fields distinguish this from name detection.
func TestDLMSWrapperSNWriteExistingAPI(t *testing.T) {
	raw := finsReadBatch(t, "dlms-wrapper-sn-write", "DLMS_WRAPPER_SN_WRITE_TEST_ROOT", "candidate-vectors.json")
	require.Equal(t, "1d8e4fb0014a2a78a5f07eb4ebc9501b95ef29ed61c53b0811e42219c2dbbb4a", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var gold struct {
		Cases []struct {
			Name   string         `json:"name"`
			Wire   string         `json:"input_hex"`
			Fields map[string]any `json:"proposed_fields"`
		} `json:"positive_syntax_candidates"`
	}
	require.NoError(t, json.Unmarshal(raw, &gold))
	require.Len(t, gold.Cases, 5)
	type testCase struct {
		name, wire string
		dir        int
		fields     map[string]any
	}
	cases := make([]testCase, 0, 6)
	for _, c := range gold.Cases {
		dir := 0
		if c.Fields["command"] == float64(13) {
			dir = 1
		}
		cases = append(cases, testCase{c.Name, c.Wire, dir, c.Fields})
	}
	cases = append(cases, testCase{"adjacent-LN-owner", "", 0, nil})
	for _, tc := range cases {
		for _, tr := range []string{"tcp", "udp"} {
			for _, deferred := range []bool{false, true} {
				for _, chunk := range []int{1, 7, 65543} {
					if tr == "udp" && chunk != 65543 {
						continue
					}
					t.Run(fmt.Sprintf("%s/%s/%v/%d", tc.name, tr, deferred, chunk), func(t *testing.T) {
						s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(tr), WithSessionPorts(40000, 4059))
						require.NoError(t, err)
						defer s.Close("defer")
						s.(*captureSession).f.a.config.Deferred = deferred
						feed := func(dir int, w string) []*ProtocolEvent {
							wire := wrapperWire(t, w)
							var es []*ProtocolEvent
							for at := 0; at < len(wire); {
								end := min(at+chunk, len(wire))
								in := append([]byte(nil), wire[at:end]...)
								out := s.Feed(dir, time.Unix(100, 0), in)
								require.Equal(t, end-at, out.Consumed)
								for j := range in {
									in[j] ^= 255
								}
								if out.Err != nil {
									require.Equal(t, ErrNeedMore, out.Err.Kind)
								}
								es = append(es, out.Events...)
								at = end
							}
							return es
						}
						if tc.wire == "" {
							qs := feed(0, "000100100001000dc001c1000100002a0000ff0200")
							require.Len(t, qs, 1)
							_, err = qs[0].GetFields()
							require.NoError(t, err)
							rs := feed(1, "0001000100100006c401c1001107")
							require.Len(t, rs, 1)
							e := rs[0]
							f, err := e.GetFields()
							require.NoError(t, err)
							require.Equal(t, "dlms-wrapper-v1-get-normal", e.Profile)
							require.Equal(t, qs[0].ID, e.TransactionID)
							require.Equal(t, qs[0].ID, e.ResponseTo)
							require.Equal(t, "observed-response", e.Session["Association"])
							require.Equal(t, 0, e.Session["Outstanding"])
							rocEqualFields(t, map[string]any{"type": float64(17), "raw_hex": "1107", "value": float64(7)}, f["data"].(map[string]any))
						} else {
							es := feed(tc.dir, tc.wire)
							require.Len(t, es, 1)
							e := es[0]
							f, err := e.GetFields()
							require.NoError(t, err)
							require.Equal(t, "dlms-wrapper", e.Protocol)
							require.Equal(t, "dlms-wrapper-v1-sn-write", e.Profile)
							require.Equal(t, wrapperWire(t, tc.wire), e.Raw)
							require.Zero(t, e.TransactionID)
							require.Zero(t, e.ResponseTo)
							rocEqualFields(t, tc.fields, f)
							expected := cloneSession(tc.fields)
							expected["Association"], expected["Outstanding"] = "unverified-sn-write-observation", 0
							rocEqualFields(t, expected, e.Session)
							require.IsType(t, uint16(0), f["wrapper_version"])
							require.IsType(t, byte(0), f["command"])
							require.IsType(t, uint64(0), f["list_count"])
							for _, key := range []string{"invoke_id", "invoke_id_and_priority", "confirmed_service", "high_priority", "service_choice"} {
								require.NotContains(t, f, key)
							}
							poisonWrapperStructured(e.Session)
							poisonWrapperStructured(f)
							again, err := e.GetFields()
							require.NoError(t, err)
							rocEqualFields(t, tc.fields, again)
						}
						require.Empty(t, s.Close("baseline"))
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
					})
				}
			}
		}
	}
}
