package pcaputil

import (
	"bytes"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func droneCANInfoFrames(t *testing.T, name string) ([][]byte, droneCANStatsAnswer) {
	t.Helper()
	raw, err := trafficfixture.ReadFile("dronecan-node-info/answers/" + name + ".json")
	require.NoError(t, err)
	var answer droneCANStatsAnswer
	require.NoError(t, decodeExactProtocolAnswer(raw, &answer))
	var frames [][]byte
	for _, event := range answer.Events {
		w, err := hex.DecodeString(event.Wire)
		require.NoError(t, err)
		frames = append(frames, w)
	}
	return frames, answer
}

func droneCANInfoIsolationAndResources(t *testing.T) {
	frames, answer := droneCANInfoFrames(t, "ordinary")
	now := time.Unix(1700002000, 0)
	parser := func(events *[]*ProtocolEvent, budget ParserBudget) *binParser {
		a := droneCANStatsParser(events, budget)
		a.canDecodeAs = map[int]string{0: "dronecan-services", 1: "dronecan-services"}
		return a
	}
	t.Run("CompleteDomainFieldsAndOwnership", func(t *testing.T) {
		var events []*ProtocolEvent
		a := parser(&events, DefaultParserBudget())
		domains := []CaptureDomain{{}, {Interface: 1}, {Section: 1}, {Encapsulation: "vpn/node-info"}}
		for i, w := range frames {
			for j, domain := range domains {
				input := bytes.Clone(w)
				droneCANStatsDeliver(a, input, uint64(i*4+j+1), domain, now.Add(time.Duration(i)*time.Millisecond))
				clear(input)
			}
		}
		require.Len(t, events, 4*len(frames))
		for j, domain := range domains {
			e := events[4*(len(frames)-1)+j]
			require.Equal(t, domain, e.Domain)
			require.Zero(t, e.FlowID)
			require.Zero(t, e.TransactionID)
			require.Zero(t, e.ResponseTo)
			require.Len(t, e.SourceBytes.PacketRefs, len(frames))
			for i, ref := range e.SourceBytes.PacketRefs {
				require.EqualValues(t, 4*i+j+1, ref.Number)
				require.Equal(t, domain, ref.Domain)
			}
			for _, query := range []func() (map[string]any, error){e.GetFields, e.Decode} {
				f, err := query()
				require.NoError(t, err)
				if _, wrapped := f["fields"]; wrapped {
					requireExactProtocolFields(t, answer.Events[len(frames)-1].Fields, protocolFields(f))
				} else {
					requireExactProtocolFields(t, answer.Events[len(frames)-1].Fields, f)
				}
				droneCANStatsMutateFields(f)
			}
		}
		a.closeDroneCANStats()
		a.closeDroneCANStats()
		require.Len(t, events, 4*len(frames))
		require.Zero(t, a.stats().BufferedBytes)
	})
	t.Run("ServiceTypesCannotMix", func(t *testing.T) {
		stats := droneCANStatsFrames(t)
		var events []*ProtocolEvent
		a := parser(&events, DefaultParserBudget())
		for i, w := range frames {
			droneCANStatsDeliver(a, w, uint64(2*i+1), CaptureDomain{}, now)
			if i < len(stats) {
				droneCANStatsDeliver(a, stats[i], uint64(2*i+2), CaptureDomain{}, now)
			}
		}
		var completed []*ProtocolEvent
		for _, e := range events {
			if e.sessionError == nil {
				completed = append(completed, e)
			}
		}
		require.Len(t, completed, 2)
		require.Equal(t, "dronecan-v0-transport-stats", completed[0].Profile)
		require.Equal(t, "dronecan-v0-node-info", completed[1].Profile)
		requireExactProtocolFields(t, answer.Events[len(frames)-1].Fields, completed[1].Session)
		for i, ref := range completed[1].SourceBytes.PacketRefs {
			require.EqualValues(t, 2*i+1, ref.Number)
		}
		a.closeDroneCANStats()
		require.Zero(t, a.stats().BufferedBytes)
	})
	t.Run("ArrayAndPayloadBudgets", func(t *testing.T) {
		for _, elements := range []int{15, 16} {
			var events []*ProtocolEvent
			budget := DefaultParserBudget()
			budget.MaxCollectionElements = elements
			a := parser(&events, budget)
			for i, w := range frames {
				droneCANStatsDeliver(a, w, uint64(i+1), CaptureDomain{}, now)
			}
			if elements == 15 {
				rocTypedError(t, "ResourceExceeded", events[len(frames)-1].sessionError)
				require.Nil(t, events[len(frames)-1].Session)
			} else {
				requireExactProtocolFields(t, answer.Events[len(frames)-1].Fields, events[len(frames)-1].Session)
			}
			a.closeDroneCANStats()
			require.Len(t, events, len(frames))
			require.Zero(t, a.stats().BufferedBytes)
		}
		for _, name := range []string{"minimum-name", "ordinary"} {
			f, want := droneCANInfoFrames(t, name)
			var events []*ProtocolEvent
			budget := DefaultParserBudget()
			budget.MaxMessageBytes = 42
			a := parser(&events, budget)
			for i, w := range f {
				droneCANStatsDeliver(a, w, uint64(i+1), CaptureDomain{}, now)
			}
			if name == "minimum-name" {
				requireExactProtocolFields(t, want.Events[len(f)-1].Fields, events[len(f)-1].Session)
			} else {
				require.Nil(t, events[len(f)-1].Session)
				found := false
				for _, e := range events {
					var pe *ProtocolError
					found = found || errors.As(e.sessionError, &pe) && pe.Kind == ErrResourceExceeded
				}
				require.True(t, found)
			}
			a.closeDroneCANStats()
			require.Zero(t, a.stats().BufferedBytes)
		}
	})
	t.Run("CertificateBudgetExactBoundary", func(t *testing.T) {
		f, want := droneCANInfoFrames(t, "maximum-certificate-name")
		for _, limit := range []int{254, 255} {
			var events []*ProtocolEvent
			b := DefaultParserBudget()
			b.MaxCollectionElements = limit
			a := parser(&events, b)
			for i, w := range f {
				droneCANStatsDeliver(a, w, uint64(i+1), CaptureDomain{}, now)
			}
			if limit == 254 {
				rocTypedError(t, "ResourceExceeded", events[len(f)-1].sessionError)
				require.Nil(t, events[len(f)-1].Session)
			} else {
				requireExactProtocolFields(t, want.Events[len(f)-1].Fields, events[len(f)-1].Session)
			}
			a.closeDroneCANStats()
			require.Len(t, events, len(f))
			require.Zero(t, a.stats().BufferedBytes)
		}
	})
	t.Run("BudgetRefusalRetiresTransferID", func(t *testing.T) {
		var events []*ProtocolEvent
		b := DefaultParserBudget()
		b.MaxCollectionElements = 15
		a := parser(&events, b)
		for i, w := range frames {
			droneCANStatsDeliver(a, w, uint64(i+1), CaptureDomain{}, now)
		}
		rocTypedError(t, "ResourceExceeded", events[len(frames)-1].sessionError)
		a.budget.MaxCollectionElements = 16
		for i, w := range frames {
			droneCANStatsDeliver(a, w, uint64(len(frames)+i+1), CaptureDomain{}, now.Add(time.Second))
			rocTypedError(t, "ContextRequired", events[len(frames)+i].sessionError)
			require.Nil(t, events[len(frames)+i].Session)
		}
		var next [][]byte
		for _, w := range frames {
			v := bytes.Clone(w)
			v[7+int(v[4])] &= 0xe0
			next = append(next, v)
		}
		for i, w := range next {
			droneCANStatsDeliver(a, w, uint64(2*len(frames)+i+1), CaptureDomain{}, now.Add(time.Second))
		}
		e := events[len(events)-1]
		want := cloneSession(answer.Events[len(frames)-1].Fields)
		want["DroneCAN"].(map[string]any)["Transfer ID"] = uint8(0)
		w := next[len(next)-1]
		want["Data"] = bytes.Clone(w[8 : 8+int(w[4])])
		requireExactProtocolFields(t, want, e.Session)
		require.Zero(t, e.ResponseTo)
		require.Zero(t, e.TransactionID)
		require.Zero(t, e.FlowID)
		for i, ref := range e.SourceBytes.PacketRefs {
			require.EqualValues(t, 2*len(frames)+i+1, ref.Number)
		}
		a.closeDroneCANStats()
		require.Len(t, events, 3*len(frames))
		require.Zero(t, a.stats().BufferedBytes)
	})
	t.Run("OuterRefusalCannotCompleteLater", func(t *testing.T) {
		var events []*ProtocolEvent
		a := parser(&events, DefaultParserBudget())
		droneCANStatsDeliver(a, frames[0], 1, CaptureDomain{}, now)
		a.decodeCANRecord(frames[1][:15], captureEvidence{Ref: PacketReference{Number: 2}}, gopacket.CaptureInfo{CaptureLength: 15, Length: 16, Timestamp: now})
		for i, w := range frames[2:] {
			droneCANStatsDeliver(a, w, uint64(i+3), CaptureDomain{}, now)
			rocTypedError(t, "ResourceExceeded", events[i+2].sessionError)
			require.Nil(t, events[i+2].Session)
		}
		a.closeDroneCANStats()
		require.Len(t, events, len(frames))
		require.Zero(t, a.stats().BufferedBytes)
	})
	t.Run("ReserveBeforeAllocationAndFailClosed", func(t *testing.T) {
		var events []*ProtocolEvent
		a := parser(&events, DefaultParserBudget())
		droneCANStatsDeliver(a, frames[0], 1, CaptureDomain{}, now)
		exact := a.stats().BufferedBytes
		require.Greater(t, exact, int64(0))
		a.closeDroneCANStats()
		require.Zero(t, a.stats().BufferedBytes)
		for _, limit := range []int64{exact - 1, exact} {
			events = nil
			b := DefaultParserBudget()
			b.MaxBufferedBytes = int(limit)
			a := parser(&events, b)
			for i, w := range frames {
				droneCANStatsDeliver(a, w, uint64(i+1), CaptureDomain{}, now)
			}
			if limit < exact {
				for _, e := range events {
					rocTypedError(t, "ResourceExceeded", e.sessionError)
					require.Nil(t, e.Session)
				}
			} else {
				requireExactProtocolFields(t, answer.Events[len(frames)-1].Fields, events[len(frames)-1].Session)
			}
			a.closeDroneCANStats()
			a.closeDroneCANStats()
			require.Zero(t, a.stats().BufferedBytes)
			require.LessOrEqual(t, a.stats().PeakBufferedBytes, limit)
		}
	})
	t.Run("ConcurrentCloseLargeTransfer", func(t *testing.T) {
		f, _ := droneCANInfoFrames(t, "maximum-certificate-name")
		a := parser(new([]*ProtocolEvent), DefaultParserBudget())
		a.config.OnEvent = nil
		var wg sync.WaitGroup
		for j := 0; j < 4; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for _, w := range f {
					droneCANStatsDeliver(a, w, 1, CaptureDomain{}, now)
				}
			}()
		}
		wg.Add(1)
		go func() { defer wg.Done(); a.closeDroneCANStats(); a.closeDroneCANStats() }()
		wg.Wait()
		a.closeDroneCANStats()
		require.Zero(t, a.stats().BufferedBytes)
		require.Nil(t, a.droneCANStats)
	})
	t.Run("PartialCloseKeepsCorrectProfile", func(t *testing.T) {
		var events []*ProtocolEvent
		a := parser(&events, DefaultParserBudget())
		droneCANStatsDeliver(a, frames[0], 9, CaptureDomain{}, now)
		a.closeDroneCANStats()
		a.closeDroneCANStats()
		require.Len(t, events, 2)
		require.Equal(t, "dronecan-v0-node-info", events[1].Profile)
		rocTypedError(t, "NeedMore", events[1].sessionError)
		require.Equal(t, []PacketReference{{Number: 9}}, events[1].SourceBytes.PacketRefs)
		require.Nil(t, events[1].Session)
		require.Zero(t, a.stats().BufferedBytes)
	})
}
