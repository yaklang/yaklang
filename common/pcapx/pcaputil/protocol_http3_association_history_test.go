package pcaputil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func h3HistoryObservations() (*ProtocolEvent, *ProtocolEvent) {
	d := CaptureDomain{Section: 2, Interface: 7, Encapsulation: "/vlan:91"}
	response := &ProtocolEvent{ID: 11, FlowID: 7, Domain: d, Protocol: "http3", nativeQUICObservation: true, nativeQUICApplication: 1, SourceBytes: ByteSource{PacketRefs: []PacketReference{{Number: 11, Domain: d}}}, semanticSession: map[string]any{"Authentication Verified": true, "HTTP3": true}}
	link := map[string]any{"Stream ID": uint64(0), "Request PDU ID": uint64(12), "Request Headers PDU ID": uint64(12), "Response PDU ID": uint64(11), "Flow ID": uint64(7), "Observed In PDU ID": uint64(12), "Capture Domain": map[string]any{"Section": d.Section, "Interface": d.Interface, "Encapsulation": d.Encapsulation}}
	request := &ProtocolEvent{ID: 12, FlowID: 7, Domain: d, Protocol: "http3", nativeQUICObservation: true, nativeQUICApplication: 1, semanticSession: map[string]any{"Authentication Verified": true, "HTTP3": true, "HTTP3 Late Associations": []map[string]any{link}}}
	response.Session, request.Session = cloneSession(response.semanticSession), cloneSession(request.semanticSession)
	return response, request
}

func TestHTTP3AssociationHistoryRejectsDifferentOwnersAndUntrustedCaches(t *testing.T) {
	for _, mutation := range []string{"flow", "interface", "section", "vlan", "unauthenticated", "public-only", "duplicate-id", "evicted", "late-owner-mismatch", "observation-mismatch"} {
		t.Run(mutation, func(t *testing.T) {
			history, err := NewProtocolInspector(8, 1<<20)
			require.NoError(t, err)
			response, request := h3HistoryObservations()
			switch mutation {
			case "flow":
				response.FlowID++
			case "interface":
				response.Domain.Interface++
			case "section":
				response.Domain.Section++
			case "vlan":
				response.Domain.Encapsulation = "/vlan:92"
			case "unauthenticated":
				request.semanticSession["Authentication Verified"] = false
			case "public-only":
				delete(request.semanticSession, "HTTP3 Late Associations")
			case "late-owner-mismatch":
				request.FlowID++
			case "observation-mismatch":
				request.ID++
			}
			if mutation != "evicted" {
				history.OnEvent(response)
			}
			if mutation == "duplicate-id" {
				history.OnEvent(response)
			}
			history.OnEvent(request)
			if mutation == "evicted" {
				require.Len(t, history.Rows("", 0), 1)
				return
			}
			view, err := history.Details(response.ID)
			require.NoError(t, err)
			require.NotContains(t, view.Session, "HTTP3 Resolved Associations")
		})
	}
}

func TestHTTP3AssociationHistoryBudgetsContextAndDoesNotResurrectEvictions(t *testing.T) {
	response, request := h3HistoryObservations()
	history, err := NewProtocolInspector(8, 1<<20)
	require.NoError(t, err)
	history.OnEvent(response)
	baseCost := history.bytes
	// Oversized current observations can still supply bounded context to an
	// already retained response, without retaining their large wire bytes.
	request.Raw = make([]byte, 2<<20)
	history.OnEvent(request)
	require.Greater(t, history.bytes, baseCost)
	require.LessOrEqual(t, history.bytes, history.maxBytes)
	require.Len(t, history.Rows("", 0), 1)
	view, err := history.Details(11)
	require.NoError(t, err)
	require.Equal(t, []any{uint64(12)}, view.DisplayFieldValues().FieldMap()["pdu.resolved_response_to"])
	retained := history.bytes
	history.OnEvent(request)
	require.Equal(t, retained, history.bytes, "duplicate association does not allocate again")
	history.OnEvent(&ProtocolEvent{ID: 99, Raw: make([]byte, history.maxBytes-1)})
	_, err = history.Details(11)
	require.Error(t, err)
	history.OnEvent(request)
	_, err = history.Details(11)
	require.Error(t, err)
	require.LessOrEqual(t, history.bytes, history.maxBytes)
}

func TestHTTP3AssociationHistoryMakesInsufficientBudgetAndAmbiguityExplicit(t *testing.T) {
	for _, limited := range []bool{false, true} {
		t.Run(map[bool]string{false: "ambiguous", true: "budget"}[limited], func(t *testing.T) {
			history, err := NewProtocolInspector(8, 1<<20)
			require.NoError(t, err)
			response, request := h3HistoryObservations()
			history.OnEvent(response)
			request.Raw = make([]byte, 2<<20)
			if limited {
				history.maxBytes = history.bytes + 1
			}
			history.OnEvent(request)
			if !limited {
				request.semanticSession["HTTP3 Late Associations"].([]map[string]any)[0]["Request PDU ID"] = uint64(13)
				history.OnEvent(request)
			}
			view, err := history.Details(11)
			require.NoError(t, err)
			require.True(t, view.DisplayFieldValues().Incomplete)
			require.Empty(t, view.DisplayFieldValues().FieldMap()["pdu.resolved_response_to"])
			require.Equal(t, false, view.Session["HTTP3 Association View"].(map[string]any)["Complete"])
			require.LessOrEqual(t, history.bytes, history.maxBytes)
		})
	}
}

func TestHTTP3AssociationHistoryKeepsMultipleStreamsAndExactLargeIDs(t *testing.T) {
	history, err := NewProtocolInspector(8, 1<<20)
	require.NoError(t, err)
	response, request := h3HistoryObservations()
	history.OnEvent(response)
	link := request.semanticSession["HTTP3 Late Associations"].([]map[string]any)[0]
	link["Request PDU ID"] = uint64(1<<60) + 7
	history.OnEvent(request)
	link["Stream ID"], link["Request PDU ID"] = uint64(4), uint64(1<<60)+8
	history.OnEvent(request)
	view, err := history.Details(11)
	require.NoError(t, err)
	require.Equal(t, []any{uint64(1<<60) + 7, uint64(1<<60) + 8}, view.DisplayFieldValues().FieldMap()["pdu.resolved_response_to"])
	require.Zero(t, view.ResponseTo, "several streams must not invent one wire response_to")
	for _, value := range view.DisplayFieldValues().Values {
		if value.Name == "pdu.resolved_response_to" {
			require.Nil(t, value.Range)
			require.Equal(t, "context", value.Origin)
			require.NotEmpty(t, value.ExactValue)
		}
	}
}

func TestHTTP3AssociationHistoryCapsPerResponseAndOwnsPublishedRelations(t *testing.T) {
	history, err := NewProtocolInspector(8, 1<<20)
	require.NoError(t, err)
	response, request := h3HistoryObservations()
	history.OnEvent(response)
	request.Raw = make([]byte, 2<<20)
	link := request.semanticSession["HTTP3 Late Associations"].([]map[string]any)[0]
	for i := uint64(0); i < 65; i++ {
		link["Stream ID"] = i * 4
		link["Request PDU ID"] = i + 100
		history.OnEvent(request)
	}
	view, err := history.Details(11)
	require.NoError(t, err)
	links := view.Session["HTTP3 Resolved Associations"].([]map[string]any)
	require.Len(t, links, 64)
	require.True(t, view.DisplayFieldValues().Incomplete)
	require.LessOrEqual(t, history.bytes, history.maxBytes)
	links[0]["Request PDU ID"] = uint64(999)
	again, err := history.Details(11)
	require.NoError(t, err)
	require.Equal(t, uint64(100), again.Session["HTTP3 Resolved Associations"].([]map[string]any)[0]["Request PDU ID"])
}
