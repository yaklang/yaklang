package pcaputil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func dlmsNextRequestControls(t *testing.T) []dlmsListControl {
	return dlmsNamedBlockControls(t, "dlms-next-request-segments", "owned-dlms-next-request-segments/v1", 22)
}

func dlmsReservedNextFlagControls(t *testing.T) []dlmsListControl {
	return dlmsNamedBlockControls(t, "dlms-reserved-next-flags", "owned-dlms-reserved-next-flags/v1", 4)
}

// Original answers remain immutable and fully hash validated by their loader.
// Only complete, decoded service identity is corrected; no fields/errors/IDs
// or partial observations may be changed by an upgrade record.
func dlmsCompleteProfileUpgrade(t *testing.T, namespace string, controls []dlmsListControl) []dlmsListControl {
	t.Helper()
	raw, err := trafficfixture.ReadFile("dlms-next-request-segments/historical-profile-upgrade.json")
	require.NoError(t, err)
	var doc struct {
		Schema  string
		Changes []struct {
			Namespace, ID string
			InputSHA      string `json:"input_sha256"`
			AnswerSHA     string `json:"original_answer_sha256"`
			Index         int    `json:"event_index"`
			Old           string `json:"old_profile"`
			New           string `json:"new_profile"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Equal(t, "owned-dlms-complete-profile-upgrade/v1", doc.Schema)
	require.Len(t, doc.Changes, 6)
	for _, patch := range doc.Changes {
		if patch.Namespace != namespace {
			continue
		}
		matches := 0
		for ci := range controls {
			c := &controls[ci]
			if c.ID != patch.ID {
				continue
			}
			matches++
			require.Equal(t, patch.InputSHA, c.SHA256)
			require.Equal(t, patch.AnswerSHA, c.AnswerSHA)
			require.GreaterOrEqual(t, patch.Index, 0)
			require.Less(t, patch.Index, len(c.Events))
			e := &c.Events[patch.Index]
			require.Equal(t, patch.Old, e.Profile)
			require.Empty(t, e.Error)
			meta, ok := e.Fields["HDLC Segmentation"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, true, meta["complete"])
			key, expected := "Get List", "dlms-hdlc-get-list"
			if e.Fields["Get Block"] != nil {
				key, expected = "Get Block", "dlms-hdlc-get-block"
			}
			require.NotNil(t, e.Fields[key])
			require.Equal(t, expected, patch.New)
			e.Profile = patch.New
		}
		require.Equal(t, 1, matches)
	}
	return controls
}

func testDLMSNextRequestExistingAPI(t *testing.T) {
	for _, c := range dlmsNextRequestControls(t) {
		t.Run(c.ID, func(t *testing.T) { dlmsOwnershipAndChunks(t, []dlmsListControl{c}) })
	}
}

func testDLMSNextRequestCloseAndBudget(t *testing.T) {
	var normal dlmsListControl
	for _, c := range dlmsNextRequestControls(t) {
		if c.ID == "next-two-udp" {
			normal = c
		}
	}
	require.NotEmpty(t, normal.ID)
	for _, transport := range []string{"tcp", "udp"} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("close/%s/deferred%t", transport, deferred), func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(40000, 4059))
				require.NoError(t, err)
				defer s.Close("cleanup")
				a := s.(*captureSession).f.a
				a.config.Deferred = deferred
				var events []*ProtocolEvent
				for i, w := range normal.Events[:3] {
					wire := bytes.Clone(wrapperWire(t, w.Raw))
					out := s.Feed(w.Direction, time.Unix(int64(i+1), 0), wire)
					clear(wire)
					require.Len(t, out.Events, 1)
					events = append(events, out.Events...)
				}
				prefix := normal
				prefix.Transport = transport
				prefix.Events = prefix.Events[:3]
				dlmsListAssert(t, prefix, events)
				closed := s.Close("incomplete-next")
				require.Len(t, closed, 1)
				require.Equal(t, "incomplete", closed[0].Completeness)
				profile := "dlms"
				if transport == "udp" {
					profile = "dlms-hdlc-segmented-request"
				}
				require.Equal(t, profile, closed[0].Profile)
				require.Equal(t, events[0].FlowID, closed[0].FlowID)
				require.Zero(t, closed[0].Direction)
				require.Zero(t, closed[0].ResponseTo)
				require.Zero(t, closed[0].TransactionID)
				rocEqualFields(t, map[string]any{"Outstanding": float64(1), "ObservedFrames": float64(1), "InformationBytes": float64(6), "ObservedBlocks": float64(1), "EncodedBytes": float64(1)}, closed[0].Session)
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
			t.Run(fmt.Sprintf("collection/%s/deferred%t", transport, deferred), func(t *testing.T) {
				b := DefaultParserBudget()
				b.MaxCollectionElements = 9
				s, err := NewProtocolSessionWithOptions(b, WithSessionTransport(transport), WithSessionPorts(40000, 4059))
				require.NoError(t, err)
				defer s.Close("cleanup")
				a := s.(*captureSession).f.a
				a.config.Deferred = deferred
				for i, w := range normal.Events[:3] {
					out := s.Feed(w.Direction, time.Unix(int64(i+1), 0), wrapperWire(t, w.Raw))
					require.Len(t, out.Events, 1)
					_, err = out.Events[0].GetFields()
					require.NoError(t, err)
				}
				// Ten assembled information bytes exceed this fixed nine-byte pool.
				// Refusal must retire both fragments and the observed block transfer.
				w := normal.Events[3]
				out := s.Feed(w.Direction, time.Unix(3, 0), wrapperWire(t, w.Raw))
				require.Len(t, out.Events, 1)
				_, err = out.Events[0].GetFields()
				rocTypedError(t, "ResourceExceeded", err)
				require.Nil(t, out.Events[0].Session)
				require.Zero(t, out.Events[0].ResponseTo)
				require.Zero(t, out.Events[0].TransactionID)
				require.Empty(t, s.Close("refused"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}

func testDLMSRequestRetryOwnership(t *testing.T) {
	for _, c := range dlmsNextRequestControls(t) {
		if c.ID != "next-final-retry-tcp" && c.ID != "next-final-retry-udp" {
			continue
		}
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/deferred%t", c.ID, deferred), func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(c.Transport), WithSessionPorts(40000, 4059))
				require.NoError(t, err)
				defer s.Close("cleanup")
				s.(*captureSession).f.a.config.Deferred = deferred
				var firstComplete uint64
				var ids []uint64
				for i, w := range c.Events {
					wire := bytes.Clone(wrapperWire(t, w.Raw))
					out := s.Feed(w.Direction, time.Unix(int64(i+1), 0), wire)
					clear(wire)
					require.Len(t, out.Events, 1)
					e := out.Events[0]
					ids = append(ids, e.ID)
					fields, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, w.Fields, fields)
					require.Equal(t, w.Profile, e.Profile)
					require.Zero(t, e.TransactionID)
					if i == 3 {
						firstComplete = e.ID
						// Returned carrier bytes/maps cannot change the privately retained APDU,
						// final wire retry recognition or first complete request identity.
						clear(e.Raw)
						fields["HDLC Segmentation"].(map[string]any)["assembled_information_hex"] = "caller-mutated"
						e.Session["HDLC Segmentation"].(map[string]any)["assembled_information_hex"] = "public-mutated"
						e.Session["Get Block"].(map[string]any)["block_number"] = 999999
					}
					if w.Reply == nil {
						require.Zero(t, e.ResponseTo)
					} else {
						require.Equal(t, ids[*w.Reply], e.ResponseTo)
						if i == 5 {
							require.Equal(t, firstComplete, e.ResponseTo)
						}
					}
				}
				require.NotZero(t, firstComplete)
				require.Empty(t, s.Close("complete"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
	// A nine-byte information pool rejects the ten-byte assembled next request;
	// the exact ten-byte adjacent budget must preserve the complete chain.
	for _, c := range dlmsNextRequestControls(t) {
		if c.ID != "next-two-tcp" && c.ID != "next-two-udp" {
			continue
		}
		b := DefaultParserBudget()
		b.MaxCollectionElements = 10
		s, err := NewProtocolSessionWithOptions(b, WithSessionTransport(c.Transport), WithSessionPorts(40000, 4059))
		require.NoError(t, err)
		var events []*ProtocolEvent
		for i, w := range c.Events {
			events = append(events, s.Feed(w.Direction, time.Unix(int64(i+1), 0), wrapperWire(t, w.Raw)).Events...)
		}
		dlmsListAssert(t, c, events)
		require.Empty(t, s.Close("complete"))
		require.Zero(t, s.Stats().BufferedBytes)
	}
}
