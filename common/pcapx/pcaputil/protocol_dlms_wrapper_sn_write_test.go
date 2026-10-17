package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

type snWriteControl struct {
	snReadControl
	UDPHash, TCPHash string
	Folder           string
}

func snWriteControls(t *testing.T) []snWriteControl {
	return snWriteCases(t, "controls.json", "2ae6c6da52d6c30fd1a9c4fb5e86935d89794f16e53a665aacd85a00a0b024c3", 15)
}
func snWriteGBTControls(t *testing.T) []snWriteControl {
	return snWriteCases(t, "gbt-supplement.json", "fd4308c0004ab0f653174b5b5f7ebc09db879ca354ea663273820f1939a03b94", 2)
}
func snWriteCases(t *testing.T, name, pin string, count int) []snWriteControl {
	t.Helper()
	raw := finsReadBatch(t, "dlms-wrapper-sn-write", "DLMS_WRAPPER_SN_WRITE_TEST_ROOT", name)
	require.Equal(t, pin, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []snReadControl }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, count)
	var h struct {
		Cases []struct {
			UDP string `json:"sha256"`
			TCP string `json:"tcp_sha256"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &h))
	require.Len(t, h.Cases, len(d.Cases))
	out := make([]snWriteControl, len(d.Cases))
	for i, c := range d.Cases {
		out[i] = snWriteControl{c, h.Cases[i].UDP, h.Cases[i].TCP, map[bool]string{true: "dlms-wrapper-sn-write-gbt-supplement", false: "dlms-wrapper-sn-write"}[name == "gbt-supplement.json"]}
	}
	return out
}
func snWriteTypes(t *testing.T, v any) {
	t.Helper()
	snReadTypes(t, v)
	var check func(any)
	check = func(value any) {
		switch x := value.(type) {
		case map[string]any:
			for k, q := range x {
				switch k {
				case "values":
					require.IsType(t, []map[string]any{}, q)
				case "data_count":
					require.IsType(t, uint64(0), q)
				case "result_success":
					require.IsType(t, false, q)
				}
				check(q)
			}
		case []map[string]any:
			for _, q := range x {
				check(q)
			}
		case []any:
			for _, q := range x {
				check(q)
			}
		}
	}
	check(v)
}
func snWriteQueries(t *testing.T, e *ProtocolEvent, w map[string]any, ids []uint64) {
	t.Helper()
	snReadCheckSaved(t, e, w, ids)
	if w["error"] != nil {
		return
	}
	for n := 0; n < 2; n++ {
		f, err := e.GetFields()
		require.NoError(t, err)
		snWriteTypes(t, f)
		d, err := e.Decode()
		require.NoError(t, err)
		for _, k := range []string{"fields", "session"} {
			v, ok := d[k].(map[string]any)
			require.True(t, ok, "Decode.%s must retain its public map type", k)
			snWriteTypes(t, v)
		}
		poisonWrapperStructured(f)
		poisonWrapperStructured(d)
	}
}
func TestDLMSWrapperSNWriteNative(t *testing.T)    { snWriteNative(t, snWriteControls(t)) }
func TestDLMSWrapperSNWriteGBTNative(t *testing.T) { snWriteNative(t, snWriteGBTControls(t)) }
func snWriteNative(t *testing.T, cases []snWriteControl) {
	for _, c := range cases {
		for _, tr := range []string{"tcp", "udp"} {
			for _, d := range []bool{false, true} {
				chunks := []int{0, 1, 7}
				if tr == "udp" {
					chunks = []int{0}
				}
				for _, chunk := range chunks {
					t.Run(fmt.Sprintf("%s/%s/d%t/chunk%d", c.Name, tr, d, chunk), func(t *testing.T) {
						b := DefaultParserBudget()
						if c.Budget.Collection > 0 {
							b.MaxCollectionElements = c.Budget.Collection
						}
						if c.Budget.Depth > 0 {
							b.MaxRecursionDepth = c.Budget.Depth
						}
						s, err := NewProtocolSessionWithOptions(b, WithSessionTransport(tr), WithSessionPorts(40000, 4059))
						require.NoError(t, err)
						defer s.Close("cleanup")
						s.(*captureSession).f.a.config.Deferred = d
						ids := make([]uint64, len(c.Steps))
						saved := make([]*ProtocolEvent, len(c.Steps))
						var flow uint64
						for i, given := range c.Steps {
							w := snReadStep(t, given, tr)
							if tr == "udp" {
								w["direction"] = w["native_udp_direction"]
							}
							wire := wrapperWire(t, w["input_hex"].(string))
							require.Equal(t, w["input_sha256"], fmt.Sprintf("%x", sha256.Sum256(wire)))
							var es []*ProtocolEvent
							size := chunk
							if size == 0 {
								size = len(wire)
							}
							fatal := false
							if tr == "tcp" {
								for _, o := range c.Overrides {
									if o.Step == i+1 {
										require.Equal(t, "FatalSessionError", o.Error)
										require.Zero(t, o.Events)
										require.Equal(t, len(wire), o.Consumed)
										require.False(t, o.NeedMore)
										fatal = true
									}
								}
							}
							var final *ProtocolError
							for at := 0; at < len(wire); at += size {
								end := min(at+size, len(wire))
								part := append([]byte(nil), wire[at:end]...)
								r := s.Feed(int(w["direction"].(float64)), time.Unix(int64(w["timestamp_unix"].(float64)), 0), part)
								clear(part)
								require.Equal(t, end-at, r.Consumed)
								if fatal {
									require.Empty(t, r.Events)
									require.False(t, r.NeedMore)
									rocTypedError(t, "FatalSessionError", r.Err)
									require.Equal(t, "session stopped", r.Err.Message)
								} else if len(r.Events) == 0 {
									require.Equal(t, "tcp", tr)
									rocTypedError(t, "NeedMore", r.Err)
									require.True(t, r.NeedMore)
								} else {
									require.False(t, r.NeedMore)
									final = r.Err
									es = append(es, r.Events...)
								}
							}
							if fatal {
								continue
							}
							require.Len(t, es, 1)
							if w["error"] == nil {
								require.Nil(t, final)
							} else {
								rocTypedError(t, w["query_error"].(string), final)
							}
							e := es[0]
							expectedFlow := snReadFlow(t, e, w, tr, false, &flow)
							ids[i] = e.ID
							require.NotZero(t, e.ID)
							require.Equal(t, tr, e.Transport)
							require.Equal(t, CaptureDomain{}, e.Domain)
							for j := 0; j < i; j++ {
								if ids[j] != 0 {
									require.NotEqual(t, ids[j], e.ID)
								}
							}
							snReadCheckEvent(t, e, w, ids, d, expectedFlow)
							if w["error"] == nil {
								snWriteTypes(t, e.Session)
							}
							snWriteQueries(t, e, w, ids)
							wrapperClosePoison(e)
							clear(wire)
							snWriteQueries(t, e, w, ids)
							saved[i] = e
						}
						closed := s.Close(c.Close.Reason)
						snWriteCloseIdentity(t, closed, ids)
						snReadCheckClose(t, c.snReadControl, closed, tr, flow, false)
						require.Empty(t, s.Close("repeat"))
						require.Zero(t, s.Stats().BufferedBytes)
						require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
						want := c.Close
						if tr == "tcp" {
							want = c.TCPClose
						}
						require.EqualValues(t, len(want.Events), s.Stats().Incomplete)
						for i, e := range saved {
							if e != nil {
								snWriteQueries(t, e, c.Steps[i], ids)
							}
						}
					})
				}
			}
		}
	}
}
func TestDLMSWrapperSNWriteMatrix(t *testing.T)    { snWriteMatrix(t, snWriteControls(t)) }
func TestDLMSWrapperSNWriteGBTMatrix(t *testing.T) { snWriteMatrix(t, snWriteGBTControls(t)) }
func snWriteMatrix(t *testing.T, cases []snWriteControl) {
	for _, c := range cases {
		for _, tr := range []string{"tcp", "udp"} {
			t.Run(c.Name+"/"+tr, func(t *testing.T) {
				reuseFolder, reuseEnv := "dlms-wrapper-sn-write", "DLMS_WRAPPER_SN_WRITE_TEST_ROOT"
				if c.Folder == "dlms-hdlc-sn-write-resources" {
					reuseFolder, reuseEnv = c.Folder, "DLMS_HDLC_SN_WRITE_TEST_ROOT"
				}
				if c.Folder == "dlms-sn-upload-captures" {
					reuseFolder, reuseEnv = c.Folder, "DLMS_SN_UPLOAD_TEST_ROOT"
				}
				if c.Folder == "dlms-sn-read-block-captures" {
					reuseFolder, reuseEnv = c.Folder, "DLMS_SN_READ_BLOCK_TEST_ROOT"
				}
				if c.Folder == "dlms-sn-parameterized-wrapper" {
					reuseFolder, reuseEnv = c.Folder, "DLMS_SN_PARAMETERIZED_TEST_ROOT"
				}
				var reuse map[string]string
				require.NoError(t, json.Unmarshal(finsReadBatch(t, reuseFolder, reuseEnv, "reuse-inputs.json"), &reuse))
				name := c.Name + "." + tr + ".pcap"
				var raw []byte
				if p, ok := reuse[name]; ok {
					require.True(t, strings.HasPrefix(p, "common/pcapx/pcaputil/"))
					const prefix = "common/pcapx/pcaputil/"
					if (c.Folder == "dlms-sn-parameterized-wrapper" || c.Folder == "dlms-sn-read-block-captures" || c.Folder == "dlms-sn-upload-captures") && strings.HasPrefix(p, prefix+c.Folder+"/") {
						name := strings.TrimPrefix(p, prefix+c.Folder+"/")
						require.NotContains(t, name, "/")
						raw = finsReadBatch(t, c.Folder, reuseEnv, name)
					} else {
						raw = wrapperStructuredInput(t, p)
					}
				} else {
					raw = finsReadBatch(t, c.Folder, reuseEnv, name)
				}
				want := c.UDPHash
				if tr == "tcp" {
					want = c.TCPHash
				}
				require.NotEmpty(t, want)
				require.Equal(t, want, fmt.Sprintf("%x", sha256.Sum256(raw)))
				discoveryMatrix(t, func(t *testing.T, workers int, d, o bool) {
					cfg := hdlcOpCase{}
					cfg.Steps = make([]hdlcOpStep, len(c.Steps))
					cfg.Budget.Collection = c.Budget.Collection
					cfg.Budget.Depth = c.Budget.Depth
					es, stats := selectedReplay(t, cfg, raw, workers, d, o, tr, "dlms-wrapper")
					indices := make([]int, 0, len(c.Steps))
					for i := range c.Steps {
						skip := false
						if tr == "tcp" {
							for _, v := range c.Overrides {
								if v.Step == i+1 {
									skip = true
								}
							}
						}
						if !skip {
							indices = append(indices, i)
						}
					}
					close := c.Close
					if tr == "tcp" {
						close = c.TCPClose
					}
					require.Len(t, es, len(indices)+len(close.Events))
					ids := make([]uint64, len(c.Steps))
					for j, i := range indices {
						ids[i] = es[j].ID
					}
					var flow uint64
					for j, i := range indices {
						e := es[j]
						w := snReadStep(t, c.Steps[i], tr)
						if tr == "udp" {
							w["direction"] = w["capture_udp_direction"]
						}
						expectedFlow := snReadFlow(t, e, w, tr, true, &flow)
						require.NotZero(t, e.ID)
						require.Equal(t, tr, e.Transport)
						require.Equal(t, CaptureDomain{}, e.Domain)
						for j := 0; j < i; j++ {
							if ids[j] != 0 {
								require.NotEqual(t, ids[j], e.ID)
							}
						}
						snReadCheckEvent(t, e, w, ids, d, expectedFlow)
						if w["error"] == nil {
							snWriteTypes(t, e.Session)
						}
						snWriteQueries(t, e, w, ids)
						require.Equal(t, CaptureDomain{}, e.Domain)
						require.Len(t, e.SourceBytes.PacketRefs, 1)
						shift := 1
						if tr == "tcp" {
							shift = 4
						}
						require.EqualValues(t, i+shift, e.SourceBytes.PacketRefs[0].Number)
						require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
						wrapperClosePoison(e)
						snWriteQueries(t, e, w, ids)
					}
					snWriteCloseIdentity(t, es[len(indices):], ids)
					snReadCheckClose(t, c.snReadControl, es[len(indices):], tr, flow, true)
					require.EqualValues(t, len(indices), stats.Messages)
					require.EqualValues(t, len(close.Events), stats.Incomplete)
				})
			})
		}
	}
}

// Zero is a concrete SN-only UDP expectation, not a wildcard for any flow.
// Retained LN/association state and every TCP byte flow instead bind one actual
// nonzero ID, which all later events in that context must preserve.

func snWriteCloseIdentity(t *testing.T, events []*ProtocolEvent, ids []uint64) {
	t.Helper()
	seen := map[uint64]bool{}
	for _, id := range ids {
		if id != 0 {
			seen[id] = true
		}
	}
	for _, e := range events {
		require.NotZero(t, e.ID)
		require.False(t, seen[e.ID])
		seen[e.ID] = true
	}
}
