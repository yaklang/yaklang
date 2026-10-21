package pcaputil

import (
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

type finsClockControl struct {
	ID, Capture, SHA256 string
	Events              []struct {
		Raw                                                string
		Fields                                             map[string]any
		Reply                                              *int
		Direction, Interface, OtherPort, FlowOwner, Packet int
		TimestampUS                                        int64
	}
	Close []struct {
		FlowOwner, Count, Interface, OtherPort int
		TimestampUS                            int64
	}
}

func finsClockRead(t *testing.T, n string) []byte {
	t.Helper()
	if root := os.Getenv("FINS_CLOCK_TEST_ROOT"); root != "" {
		b, err := os.ReadFile(filepath.Join(root, "fins-clock", n))
		require.NoError(t, err)
		return b
	}
	b, err := trafficfixture.ReadFile("fins-clock/" + n)
	require.NoError(t, err)
	return b
}
func finsClockControls(t *testing.T) []finsClockControl {
	t.Helper()
	var v struct {
		Schema string
		Cases  []finsClockControl
	}
	require.NoError(t, json.Unmarshal(finsClockRead(t, "controls.json"), &v))
	require.Equal(t, "owned-fins-conversation-clock/v1", v.Schema)
	require.Len(t, v.Cases, 12)
	return v.Cases
}
func TestFINSConversationClockIsolation(t *testing.T) {
	for _, c := range finsClockControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw := finsClockRead(t, c.Capture)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				events, stats := discoveryReplay(t, raw, workers, deferred, observe, WithProtocolDecodeAs("udp", 9600, "fins"))
				require.Len(t, events, len(c.Events)+len(c.Close))
				ids := map[uint64]bool{}
				flows := map[int]uint64{}
				for i, w := range c.Events {
					e := events[i]
					require.NotZero(t, e.ID)
					require.False(t, ids[e.ID])
					ids[e.ID] = true
					require.NotZero(t, e.FlowID)
					if old, ok := flows[w.FlowOwner]; ok {
						require.Equal(t, old, e.FlowID)
					} else {
						for _, f := range flows {
							require.NotEqual(t, f, e.FlowID)
						}
						flows[w.FlowOwner] = e.FlowID
					}
					require.Equal(t, events[w.FlowOwner].FlowID, e.FlowID)
					require.Equal(t, "fins", e.Protocol)
					require.Equal(t, "udp", e.Transport)
					require.Equal(t, finsProfile, e.Profile)
					require.Equal(t, "message", e.Completeness)
					require.Equal(t, CaptureDomain{Interface: w.Interface}, e.Domain)
					require.True(t, time.UnixMicro(w.TimestampUS).Equal(e.Timestamp))
					want := finsResolve(w.Fields, events).(map[string]any)
					f, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, want, f)
					rocEqualFields(t, want, e.Session)
					require.Equal(t, wrapperWire(t, w.Raw), e.Raw)
					require.Equal(t, len(w.Raw)/2, e.Length)
					require.Equal(t, w.Direction, e.Direction)
					reply := uint64(0)
					if w.Reply != nil {
						reply = events[*w.Reply].ID
					}
					require.Equal(t, reply, e.ResponseTo)
					owner, _ := want["observed_request_id"].(uint64)
					require.Equal(t, owner, e.TransactionID)
					src, dst := fmt.Sprintf("192.0.2.1:%d", 40000+w.OtherPort), "192.0.2.2:9600"
					if w.Direction == 1 {
						src, dst = dst, src
					}
					require.Equal(t, src, e.Source)
					require.Equal(t, dst, e.Destination)
					require.Len(t, e.SourceBytes.PacketRefs, 1)
					require.EqualValues(t, w.Packet, e.SourceBytes.PacketRefs[0].Number)
					require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
					status := "decoded"
					if deferred {
						status = "deferred"
					}
					require.Equal(t, status, e.Status)
					clear(f)
					clear(e.Session)
					clear(e.Raw)
					e.Fields = map[string]any{"fake": true}
					e.Structured = map[string]any{"fields": e.Fields}
					e.Protocol = "edited"
					again, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, want, again)
					out, err := e.Decode()
					require.NoError(t, err)
					rocEqualFields(t, want, out["session"].(map[string]any))
				}
				closes := map[uint64]*ProtocolEvent{}
				for _, e := range events[len(c.Events):] {
					require.NotZero(t, e.ID)
					require.False(t, ids[e.ID])
					ids[e.ID] = true
					require.Equal(t, "fins", e.Protocol)
					require.Equal(t, "udp", e.Transport)
					require.Equal(t, "incomplete", e.Status)
					require.Empty(t, e.Raw)
					require.Zero(t, e.Length)
					require.Zero(t, e.TransactionID)
					require.Zero(t, e.ResponseTo)
					f, err := e.GetFields()
					rocTypedError(t, "NeedMore", err)
					require.Nil(t, f)
					require.Nil(t, closes[e.FlowID])
					closes[e.FlowID] = e
				}
				for _, w := range c.Close {
					e := closes[events[w.FlowOwner].FlowID]
					require.NotNil(t, e)
					rocEqualFields(t, map[string]any{"Outstanding": w.Count}, e.Session)
					require.Equal(t, CaptureDomain{Interface: w.Interface}, e.Domain)
					require.True(t, time.UnixMicro(w.TimestampUS).Equal(e.Timestamp))
					require.Equal(t, fmt.Sprintf("192.0.2.1:%d", 40000+w.OtherPort), e.Source)
					require.Equal(t, "192.0.2.2:9600", e.Destination)
				}
				require.Zero(t, stats.BufferedBytes)
			})
		})
	}
}
