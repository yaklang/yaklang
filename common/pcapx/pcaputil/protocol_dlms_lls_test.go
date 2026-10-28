package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDLMSLLSExistingAPI(t *testing.T) {
	t.Run("wrapper", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
		require.NoError(t, err)
		defer s.Close("baseline")
		o := s.Feed(0, time.Unix(1, 0), wrapperWire(t, "00010010000100486046a1090607608574050801018a0207808b0760857405080201ac1a80187075626c69632d73796e7468657469632d66697874757265be10040e01000000065f1f04000000100400"))
		require.Nil(t, o.Err)
		require.Len(t, o.Events, 1)
		f, err := o.Events[0].GetFields()
		require.NoError(t, err)
		require.Equal(t, true, f["authentication_requested"])
		require.Equal(t, false, f["authentication_verified"])
		require.Equal(t, "2.16.756.5.8.2.1", f["authentication_mechanism_oid"])
		require.Equal(t, "7075626c69632d73796e7468657469632d66697874757265", f["calling_authentication_value_hex"])
	})
	t.Run("hdlc", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
		require.NoError(t, err)
		defer s.Close("baseline")
		o := s.Feed(0, time.Unix(1, 0), wrapperWire(t, "7ea0540321104747e6e6006046a1090607608574050801018a0207808b0760857405080201ac1a80187075626c69632d73796e7468657469632d66697874757265be10040e01000000065f1f04000000100400303a7e"))
		require.Nil(t, o.Err)
		require.Len(t, o.Events, 1)
		f, err := o.Events[0].GetFields()
		require.NoError(t, err)
		f = f["ACSE"].(map[string]any)
		require.Equal(t, true, f["authentication_requested"])
		require.Equal(t, false, f["authentication_verified"])
		require.Equal(t, "2.16.756.5.8.2.1", f["authentication_mechanism_oid"])
		require.Equal(t, "7075626c69632d73796e7468657469632d66697874757265", f["calling_authentication_value_hex"])
	})
}
func TestDLMSWrapperOrphanAARECannotOwnFreshAARQ(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
	require.NoError(t, err)
	defer s.Close("orphan")
	q := wrapperWire(t, "000100100001001f601da109060760857405080101be10040e01000000065f1f04000000100400")
	r := wrapperWire(t, "000100010010002b6129a109060760857405080101a203020100a305a103020100be10040e0800065f1f040000001004000007")
	orphan := s.Feed(1, time.Unix(1, 0), r)
	require.Nil(t, orphan.Err)
	require.Len(t, orphan.Events, 1)
	require.Zero(t, orphan.Events[0].ResponseTo)
	fresh := s.Feed(0, time.Unix(2, 0), q)
	require.Nil(t, fresh.Err)
	require.Len(t, fresh.Events, 1)
	late := s.Feed(1, time.Unix(3, 0), r)
	require.Len(t, late.Events, 1)
	require.Zero(t, late.Events[0].ResponseTo, "unobserved older AARE must not acquire the fresh AARQ ID")
	require.Zero(t, late.Events[0].TransactionID)
}
func llsWrapperControls(t *testing.T) []wrapperBlockControl {
	t.Helper()
	var b []byte
	var e error
	if root := os.Getenv("DLMS_LLS_TEST_ROOT"); root != "" {
		b, e = os.ReadFile(filepath.Join(root, "dlms-lls-wrapper/controls.json"))
	} else {
		b, e = trafficfixture.ReadFile("dlms-lls-wrapper/controls.json")
	}
	require.NoError(t, e)
	var m struct{ Cases []wrapperBlockControl }
	require.NoError(t, json.Unmarshal(b, &m))
	require.Len(t, m.Cases, 40)
	if os.Getenv("DLMS_LLS_TEST_ROOT") == "" {
		all, err := trafficfixture.AllExpectations()
		require.NoError(t, err)
		for _, c := range m.Cases {
			bound := 0
			for _, batch := range all {
				for _, v := range batch.Cases {
					if v.ID != "dlms-lls-wrapper/"+c.Name {
						continue
					}
					require.Equal(t, c.SHA, v.Input.SHA256)
					require.Equal(t, c.Alias, v.Input.OriginalPath)
					for _, expect := range v.Expectations {
						var x struct {
							File string `json:"answer_file"`
							SHA  string `json:"answer_sha256"`
						}
						require.NoError(t, json.Unmarshal(expect.PayloadConstraints, &x))
						if x.File == "controls.json" {
							require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(b)), x.SHA)
							bound++
						}
					}
				}
			}
			require.Equal(t, 1, bound, c.Name)
		}
	}
	return m.Cases
}

func llsHDLCControls(t *testing.T) []dlmsListControl {
	t.Helper()
	if root := os.Getenv("DLMS_LLS_TEST_ROOT"); root != "" {
		b, e := os.ReadFile(filepath.Join(root, "dlms-lls-hdlc/controls.json"))
		require.NoError(t, e)
		var m struct {
			Schema string
			Cases  []dlmsListControl
		}
		require.NoError(t, json.Unmarshal(b, &m))
		require.Equal(t, "owned-dlms-lls-hdlc/v1", m.Schema)
		require.Len(t, m.Cases, 40)
		return m.Cases
	}
	return dlmsNamedBlockControls(t, "dlms-lls-hdlc", "owned-dlms-lls-hdlc/v1", 40)
}
func TestDLMSLLSWrapperSealedMatrix(t *testing.T) { wrapperBlocksReplay(t, llsWrapperControls(t)) }
func TestDLMSLLSWrapperSealedOwnership(t *testing.T) {
	wrapperBlocksOwnership(t, llsWrapperControls(t))
}
func TestDLMSLLSHDLCSealedMatrix(t *testing.T)    { dlmsSealedMatrix(t, llsHDLCControls(t)) }
func TestDLMSLLSHDLCSealedOwnership(t *testing.T) { dlmsOwnershipAndChunks(t, llsHDLCControls(t)) }

func TestDLMSLLSLiveOwnershipBudgetIdleClose(t *testing.T) {
	for _, carrier := range []string{"wrapper", "hdlc"} {
		for _, tr := range []string{"tcp", "udp"} {
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("owned-close/%s/%s/%t", carrier, tr, deferred), func(t *testing.T) {
					var q, r string
					var want map[string]any
					if carrier == "wrapper" {
						for _, c := range llsWrapperControls(t) {
							if c.Name == "accepted-"+tr {
								q, r = c.Answers[0].Wire, c.Answers[1].Wire
								want = cloneSession(c.Answers[1].Fields)
							}
						}
					} else {
						for _, c := range llsHDLCControls(t) {
							if c.ID == "accepted-"+tr {
								q, r = c.Events[0].Raw, c.Events[1].Raw
								want = cloneSession(c.Events[1].Fields)
							}
						}
					}
					require.NotEmpty(t, q)
					for _, finish := range []bool{false, true} {
						s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(tr), WithSessionPorts(40000, 4059))
						require.NoError(t, err)
						s.(*captureSession).f.a.config.Deferred = deferred
						input := wrapperWire(t, q)
						out := s.Feed(0, time.Unix(1, 0), input)
						clear(input)
						require.Nil(t, out.Err)
						require.Len(t, out.Events, 1)
						e := out.Events[0]
						id, flow, src, dst := e.ID, e.FlowID, e.Source, e.Destination
						f, err := e.GetFields()
						require.NoError(t, err)
						a := f
						if carrier == "hdlc" {
							a = f["ACSE"].(map[string]any)
						}
						require.Equal(t, true, a["authentication_requested"])
						require.Equal(t, false, a["authentication_verified"])
						a["authentication_mechanism_oid"] = "mutated"
						a["calling_authentication_value_hex"] = "mutated"
						e.Session["Observation"] = "mutated"
						e.ID = 999999
						clear(e.Raw)
						if finish {
							out = s.Feed(1, time.Unix(2, 0), bytes.Clone(wrapperWire(t, r)))
							require.Nil(t, out.Err)
							require.Len(t, out.Events, 1)
							resp := out.Events[0]
							require.Equal(t, id, resp.ResponseTo)
							require.Equal(t, flow, resp.FlowID)
							fields, err := resp.GetFields()
							require.NoError(t, err)
							expected := cloneSession(want)
							auth := expected
							if carrier == "hdlc" {
								auth = expected["ACSE"].(map[string]any)
							}
							auth["association_request_event"] = id
							rocEqualFields(t, expected, fields)
							require.Equal(t, false, auth["authentication_verified"])
							require.Empty(t, s.Close("complete"))
						} else {
							closed := s.Close("unmatched")
							require.Len(t, closed, 1)
							c := closed[0]
							require.Equal(t, src, c.Source)
							require.Equal(t, dst, c.Destination)
							require.Equal(t, flow, c.FlowID)
							profile := "dlms-wrapper-v1-acse"
							session := map[string]any{"Outstanding": 1, "source_wport": 16, "destination_wport": 1}
							if carrier == "hdlc" {
								profile = "dlms-hdlc-acse"
								session = map[string]any{"Outstanding": 1}
							}
							require.Equal(t, profile, c.Profile)
							require.Equal(t, "incomplete", c.Completeness)
							require.Zero(t, c.ResponseTo)
							require.Zero(t, c.TransactionID)
							_, err = c.GetFields()
							rocTypedError(t, "NeedMore", err)
							rocEqualFields(t, session, c.Session)
						}
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
					}
				})
			}
		}
	}
	for _, carrier := range []string{"wrapper", "hdlc"} {
		var q, r string
		if carrier == "wrapper" {
			c := llsWrapperControls(t)[0]
			q, r = c.Answers[0].Wire, c.Answers[1].Wire
		} else {
			c := llsHDLCControls(t)[0]
			q, r = c.Events[0].Raw, c.Events[1].Raw
		}
		t.Run("budget/"+carrier, func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxFrameBytes, b.MaxMessageBytes, b.MaxBufferedBytes = 2049, 2049, 8192
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("test")
			out := s.Feed(0, time.Unix(1, 0), wrapperWire(t, q))
			require.Len(t, out.Events, 1)
			fields, err := out.Events[0].GetFields()
			rocTypedError(t, "ResourceExceeded", err)
			require.Nil(t, fields)
			require.Nil(t, out.Events[0].Session)
			s.(*captureSession).f.a.budget.MaxBufferedBytes = 8 << 20
			for _, v := range []struct {
				d int
				w string
			}{{0, q}, {1, r}} {
				o := s.Feed(v.d, time.Unix(2, 0), wrapperWire(t, v.w))
				require.Len(t, o.Events, 1)
				require.Zero(t, o.Events[0].ResponseTo)
				require.Zero(t, o.Events[0].TransactionID)
			}
			require.Empty(t, s.Close("refusal"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
		t.Run("idle/"+carrier, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("test")
			o := s.Feed(0, time.Unix(1, 0), wrapperWire(t, q))
			require.Nil(t, o.Err)
			require.Len(t, o.Events, 1)
			late := s.Feed(1, time.Unix(32, 0), wrapperWire(t, r))
			require.Len(t, late.Events, 1)
			require.Zero(t, late.Events[0].ResponseTo)
			require.Zero(t, late.Events[0].TransactionID)
			require.Empty(t, s.Close("idle"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
