package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func releaseSegmentControls(t *testing.T) []dlmsListControl {
	t.Helper()
	if root := os.Getenv("DLMS_RELEASE_SEG_TEST_ROOT"); root != "" {
		b, err := os.ReadFile(filepath.Join(root, "dlms-release-segmented/controls.json"))
		require.NoError(t, err)
		var d struct {
			Schema string
			Cases  []dlmsListControl
		}
		require.NoError(t, json.Unmarshal(b, &d))
		require.Equal(t, "owned-dlms-release-segmented/v1", d.Schema)
		require.Len(t, d.Cases, 54)
		return d.Cases
	}
	// This batch binds the complete control document, which includes every
	// per-case answer digest. Check both links rather than assuming the older
	// batches' direct per-answer manifest layout or accepting an unbound answer.
	b, err := trafficfixture.ReadFile("dlms-release-segmented/controls.json")
	require.NoError(t, err)
	var d struct {
		Schema string
		Cases  []dlmsListControl
	}
	require.NoError(t, json.Unmarshal(b, &d))
	require.Equal(t, "owned-dlms-release-segmented/v1", d.Schema)
	require.Len(t, d.Cases, 54)
	controlSHA := fmt.Sprintf("%x", sha256.Sum256(b))
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for _, c := range d.Cases {
		answer, err := trafficfixture.ReadFile("dlms-release-segmented/" + c.Answer)
		require.NoError(t, err)
		require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(answer)))
		var a dlmsListControl
		require.NoError(t, json.Unmarshal(answer, &a))
		require.Equal(t, c.Events, a.Events)
		bound := 0
		for _, batch := range all {
			for _, input := range batch.Cases {
				if input.ID != "dlms-release-segmented/"+c.ID {
					continue
				}
				require.Equal(t, c.SHA256, input.Input.SHA256)
				require.Equal(t, c.InputAlias, input.Input.OriginalPath)
				require.Equal(t, c.Packets, input.Facts.PacketCount)
				for _, expectation := range input.Expectations {
					var link struct {
						File string `json:"answer_file"`
						SHA  string `json:"answer_sha256"`
					}
					require.NoError(t, json.Unmarshal(expectation.PayloadConstraints, &link))
					if link.File == "controls.json" {
						require.Equal(t, controlSHA, link.SHA)
						bound++
					}
				}
			}
		}
		require.Equal(t, 1, bound, c.ID)
	}
	return d.Cases
}

func TestDLMSReleaseSegmentedSealedMatrix(t *testing.T) {
	dlmsSealedMatrix(t, releaseSegmentControls(t))
}

func TestDLMSReleaseSegmentedSealedOwnership(t *testing.T) {
	dlmsOwnershipAndChunks(t, releaseSegmentControls(t))
}

func releaseSegmentNormal(t *testing.T, transport string) dlmsListControl {
	t.Helper()
	for _, c := range releaseSegmentControls(t) {
		if c.ID == "normal-"+transport {
			return c
		}
	}
	t.Fatal("missing independent normal exchange")
	return dlmsListControl{}
}

func TestDLMSReleaseSegmentedLiveOwnership(t *testing.T) {
	for _, transport := range []string{"tcp", "udp"} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", transport, deferred), func(t *testing.T) {
				c := releaseSegmentNormal(t, transport)
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(40000, 4059))
				require.NoError(t, err)
				defer s.Close("test")
				s.(*captureSession).f.a.config.Deferred = deferred
				var requestID uint64
				for i, w := range c.Events {
					input := bytes.Clone(wrapperWire(t, w.Raw))
					out := s.Feed(w.Direction, time.Unix(int64(i+1), 0), input)
					clear(input)
					require.Nil(t, out.Err)
					require.Len(t, out.Events, 1)
					e := out.Events[0]
					want := cloneSession(w.Fields)
					if i == 1 {
						requestID = e.ID
					}
					if w.Reply != nil {
						require.Equal(t, requestID, e.ResponseTo)
						want["ACSE"].(map[string]any)["release_request_event"] = requestID
					} else {
						require.Zero(t, e.ResponseTo)
					}
					require.Zero(t, e.TransactionID)
					fields, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, want, fields)
					rocEqualFields(t, want, e.Session)
					fields["ACSE Segmentation"].(map[string]any)["assembled_information_hex"] = "mutated"
					e.ID, e.ResponseTo, e.TransactionID = 999999, 999999, 999999
					e.Session, e.Fields, e.Structured = map[string]any{"fake": true}, map[string]any{"fake": true}, map[string]any{"fake": true}
					clear(e.Raw)
					again, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, want, again)
				}
				require.Empty(t, s.Close("done"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}

func TestDLMSReleaseSegmentedCloseAndBudgets(t *testing.T) {
	for _, transport := range []string{"tcp", "udp"} {
		for _, deferred := range []bool{false, true} {
			for _, count := range []int{1, 3} {
				t.Run(fmt.Sprintf("close/%s/%t/%d", transport, deferred, count), func(t *testing.T) {
					c := releaseSegmentNormal(t, transport)
					s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(40000, 4059))
					require.NoError(t, err)
					defer s.Close("test")
					s.(*captureSession).f.a.config.Deferred = deferred
					var first *ProtocolEvent
					for i, w := range c.Events[:count] {
						out := s.Feed(w.Direction, time.Unix(int64(i+1), 0), wrapperWire(t, w.Raw))
						require.Nil(t, out.Err)
						require.Len(t, out.Events, 1)
						if first == nil {
							first = out.Events[0]
						}
					}
					out := s.Close("partial-lifecycle")
					require.Len(t, out, 1)
					e := out[0]
					require.Equal(t, "dlms-hdlc-acse-segmented", e.Profile)
					require.Equal(t, "incomplete", e.Status)
					require.Equal(t, "incomplete", e.Completeness)
					// Preserve the existing exchange-requester Close orientation,
					// including a response whose partial information is outstanding.
					require.Equal(t, first.Direction, e.Direction)
					require.Equal(t, first.Source, e.Source)
					require.Equal(t, first.Destination, e.Destination)
					require.Equal(t, first.FlowID, e.FlowID)
					require.Equal(t, first.Domain, e.Domain)
					require.Empty(t, e.Raw)
					require.Zero(t, e.ResponseTo)
					require.Zero(t, e.TransactionID)
					_, err = e.GetFields()
					rocTypedError(t, "NeedMore", err)
					rocEqualFields(t, map[string]any{"Outstanding": 1, "ObservedFrames": 1, "InformationBytes": 5}, e.Session)
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
	for _, collection := range []bool{false, true} {
		t.Run(fmt.Sprintf("budget/collection-%t", collection), func(t *testing.T) {
			b := DefaultParserBudget()
			if collection {
				b.MaxCollectionElements = 4
			} else {
				b.MaxFrameBytes, b.MaxMessageBytes = 2049, 2049
				b.MaxBufferedBytes = 8192
			}
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("test")
			c := releaseSegmentNormal(t, "udp")
			for _, index := range []int{0, 1, 2, 3} {
				w := c.Events[index]
				out := s.Feed(w.Direction, time.Unix(int64(index+1), 0), wrapperWire(t, w.Raw))
				require.Len(t, out.Events, 1)
				e := out.Events[0]
				fields, err := e.GetFields()
				require.Error(t, err)
				require.Nil(t, fields)
				require.Nil(t, e.Session)
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
				if index == 0 {
					rocTypedError(t, "ResourceExceeded", err)
				}
			}
			require.Empty(t, s.Close("refused"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDLMSHDLCReverseCloseDirection(t *testing.T) {
	for _, transport := range []string{"tcp", "udp"} {
		t.Run(transport, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("test")
			out := s.Feed(1, time.Unix(1, 0), wrapperWire(t, "7ea81d032110b3f2e6e600601da109060760857405080101be10040e47017e"))
			require.Nil(t, out.Err)
			require.Len(t, out.Events, 1)
			first := out.Events[0]
			require.Equal(t, 1, first.Direction)
			closed := s.Close("incomplete")
			require.Len(t, closed, 1)
			e := closed[0]
			require.Equal(t, first.Source, e.Source)
			require.Equal(t, first.Destination, e.Destination)
			require.Equal(t, first.Direction, e.Direction)
			require.Equal(t, first.FlowID, e.FlowID)
			require.Zero(t, e.ResponseTo)
			require.Zero(t, e.TransactionID)
			_, err = e.GetFields()
			rocTypedError(t, "NeedMore", err)
			rocEqualFields(t, map[string]any{"Outstanding": 1, "ObservedFrames": 1, "InformationBytes": 20}, e.Session)
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDLMSReleaseSegmentedServerFirstAbort(t *testing.T) {
	for _, transport := range []string{"tcp", "udp"} {
		for _, count := range []int{1, 2} {
			t.Run(fmt.Sprintf("%s/%d", transport, count), func(t *testing.T) {
				var c dlmsListControl
				for _, x := range releaseSegmentControls(t) {
					if x.ID == "abort-server-"+transport {
						c = x
					}
				}
				require.Len(t, c.Events, 3)
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(40000, 4059))
				require.NoError(t, err)
				defer s.Close("test")
				var first *ProtocolEvent
				for i, w := range c.Events[1 : count+1] {
					out := s.Feed(1, time.Unix(int64(i+1), 0), wrapperWire(t, w.Raw))
					require.Nil(t, out.Err)
					require.Len(t, out.Events, 1)
					e := out.Events[0]
					if i == 0 {
						first = e
					}
					require.Equal(t, 1, e.Direction)
					require.Equal(t, "192.0.2.2:4059", e.Source)
					require.Equal(t, "192.0.2.1:40000", e.Destination)
					fields, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, w.Fields, fields)
					rocEqualFields(t, w.Fields, e.Session)
					require.Zero(t, e.ResponseTo)
					require.Zero(t, e.TransactionID)
				}
				closed := s.Close("abort")
				if count == 1 {
					require.Len(t, closed, 1)
					e := closed[0]
					require.Equal(t, first.Direction, e.Direction)
					require.Equal(t, first.Source, e.Source)
					require.Equal(t, first.Destination, e.Destination)
					require.Zero(t, e.ResponseTo)
					require.Zero(t, e.TransactionID)
					_, err = e.GetFields()
					rocTypedError(t, "NeedMore", err)
					rocEqualFields(t, map[string]any{"Outstanding": 1, "ObservedFrames": 1, "InformationBytes": 5}, e.Session)
				} else {
					require.Empty(t, closed)
				}
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}
