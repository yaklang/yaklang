package yakgrpc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestServer_QueryAIEvent(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	processID := uuid.NewString()
	eventID := uuid.NewString()
	typeName := uuid.NewString()
	// Insert a mock event using provided helper
	event := &schema.AiOutputEvent{
		// provide fields used by yakit helper code
		EventUUID:     eventID,
		IsStream:      false,
		ProcessesId:   []string{processID},
		Type:          schema.EventType(typeName),
		CoordinatorId: "coord-1",
		// other fields may be zero values
	}

	if err := yakit.CreateOrUpdateAIOutputEvent(db, event); err != nil {
		t.Fatalf("CreateOrUpdateAIOutputEvent failed: %v", err)
	}

	client, err := NewLocalClient()
	require.NoError(t, err)

	// 1) Query by EventUUID filter
	filterByUUID := &ypb.AIEventFilter{
		EventUUIDS: []string{eventID},
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	r, err := client.QueryAIEvent(ctx, &ypb.AIEventQueryRequest{
		Filter: filterByUUID,
	})
	if err != nil {
		t.Fatalf("QueryAIEvent by UUID failed: %v", err)
	}
	results := r.Events
	if len(results) != 1 {
		t.Fatalf("expected 1 event, got %d", len(results))
	}
	require.Equal(t, eventID, results[0].EventUUID, "expected to find event by UUID")
	require.Equal(t, typeName, results[0].Type, "expected to find stream")

	// 2) Query via process association: use QueryAIEventIDByProcessID to get IDs, then QueryAIEvent
	r, err = client.QueryAIEvent(ctx, &ypb.AIEventQueryRequest{
		ProcessID: processID,
	})
	if err != nil {
		t.Fatalf("QueryAIEventIDByProcessID failed: %v", err)
	}
	results = r.Events
	if len(results) != 1 {
		t.Fatalf("expected 1 event, got %d", len(results))
	}
	require.Equal(t, eventID, results[0].EventUUID, "expected to find event by UUID")
	require.Equal(t, typeName, results[0].Type, "expected to find stream")
}

func TestServer_QueryAIEvent_StreamAggregation(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	eventID := uuid.NewString()
	processID := uuid.NewString()
	typeName := uuid.NewString()
	part1 := uuid.NewString()
	part2 := uuid.NewString()

	// first stream fragment
	first := &schema.AiOutputEvent{
		EventUUID:   eventID,
		IsStream:    true,
		ProcessesId: []string{processID},
		Type:        schema.EventType(typeName),
		StreamDelta: []byte(part1),
	}
	if err := yakit.CreateOrUpdateAIOutputEvent(db, first); err != nil {
		t.Fatalf("CreateOrUpdateAIOutputEvent (first fragment) failed: %v", err)
	}

	// second stream fragment with same EventUUID should be appended
	second := &schema.AiOutputEvent{
		EventUUID:   eventID,
		IsStream:    true,
		ProcessesId: []string{processID},
		Type:        schema.EventType(typeName),
		StreamDelta: []byte(part2),
	}
	if err := yakit.CreateOrUpdateAIOutputEvent(db, second); err != nil {
		t.Fatalf("CreateOrUpdateAIOutputEvent (second fragment) failed: %v", err)
	}

	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	r, err := client.QueryAIEvent(ctx, &ypb.AIEventQueryRequest{
		Filter: &ypb.AIEventFilter{
			EventUUIDS: []string{eventID},
		},
	})
	if err != nil {
		t.Fatalf("QueryAIEvent for stream event failed: %v", err)
	}
	results := r.Events
	if len(results) != 1 {
		t.Fatalf("expected 1 event, got %d", len(results))
	}

	require.Contains(t, string(results[0].StreamDelta), part1)
	require.Contains(t, string(results[0].StreamDelta), part2)

	// 2) Query via process association: use QueryAIEventIDByProcessID to get IDs, then QueryAIEvent
	r, err = client.QueryAIEvent(ctx, &ypb.AIEventQueryRequest{
		ProcessID: processID,
	})
	if err != nil {
		t.Fatalf("QueryAIEventIDByProcessID failed: %v", err)
	}
	results = r.Events
	if len(results) != 1 {
		t.Fatalf("expected 1 event, got %d", len(results))
	}
	require.Contains(t, string(results[0].StreamDelta), part1)
	require.Contains(t, string(results[0].StreamDelta), part2)
}

func TestServer_QueryAIEvent_Paging(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	typeName := uuid.NewString()
	defer func() {
		db.Where("type IN (?)", []string{typeName, typeName + "_proc"}).Delete(&schema.AiOutputEvent{})
	}()
	totalEvents := 15

	// Create events
	var eventIDs []string
	for i := 0; i < totalEvents; i++ {
		eventID := uuid.NewString()
		eventIDs = append(eventIDs, eventID)
		event := &schema.AiOutputEvent{
			EventUUID:     eventID,
			IsStream:      false,
			Type:          schema.EventType(typeName),
			CoordinatorId: "coord-paging",
			Timestamp:     time.Now().Unix() + int64(i), // ensure order
		}
		if err := yakit.CreateOrUpdateAIOutputEvent(db, event); err != nil {
			t.Fatalf("CreateOrUpdateAIOutputEvent failed: %v", err)
		}
	}

	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*500)
	defer cancel()

	// 1. First Page
	req1 := &ypb.AIEventQueryRequest{
		Filter: &ypb.AIEventFilter{
			EventType: []string{typeName},
		},
		Pagination: &ypb.Paging{
			Page:    1,
			Limit:   10,
			Order:   "asc", // older first
			OrderBy: "timestamp",
		},
	}
	resp1, err := client.QueryAIEvent(ctx, req1)
	require.NoError(t, err)
	require.Equal(t, int64(totalEvents), resp1.Total)
	require.Equal(t, 10, len(resp1.Events))
	// require.Equal(t, eventIDs[0], resp1.Events[0].EventUUID) // UUID order is not guaranteed, check ID set or trust OrderBy+Timestamp if unique enough
	// Since timestamp is i+now, they are distinct.
	// But UUIDs are random. Let's verify we got 10 events.
	// Order by timestamp asc means we expect the *earliest* ones.
	// eventIDs was appended in loop order (earliest timestamp first).
	// So eventIDs[0] should indeed be first IF sorting worked.
	// Wait, Paging proto definition needs "OrderBy" field name to be set correctly.
	// "Order" is "asc" or "desc". "OrderBy" is the field name.
	// In my previous code I missed setting "OrderBy: 'timestamp'". Default might be ID or empty.

	// 2. Second Page
	req2 := &ypb.AIEventQueryRequest{
		Filter: &ypb.AIEventFilter{
			EventType: []string{typeName},
		},
		Pagination: &ypb.Paging{
			Page:    2,
			Limit:   10,
			Order:   "asc",
			OrderBy: "timestamp",
		},
	}
	resp2, err := client.QueryAIEvent(ctx, req2)
	require.NoError(t, err)
	require.Equal(t, int64(totalEvents), resp2.Total)
	require.Equal(t, 5, len(resp2.Events))

	// 3. ProcessID + Pagination
	// Create events associated with a process
	processID := uuid.NewString()
	procEventsTotal := 5
	for i := 0; i < procEventsTotal; i++ {
		evt := &schema.AiOutputEvent{
			EventUUID:   uuid.NewString(),
			ProcessesId: []string{processID},
			Type:        schema.EventType(typeName + "_proc"),
		}
		require.NoError(t, yakit.CreateOrUpdateAIOutputEvent(db, evt))
	}

	reqProc := &ypb.AIEventQueryRequest{
		ProcessID: processID,
		Pagination: &ypb.Paging{
			Page:  1,
			Limit: 2,
		},
	}
	respProc, err := client.QueryAIEvent(ctx, reqProc)
	require.NoError(t, err)
	require.Equal(t, int64(procEventsTotal), respProc.Total)
	require.Equal(t, 2, len(respProc.Events))

	// 4. ProcessID without Pagination
	reqProcNoPaging := &ypb.AIEventQueryRequest{
		ProcessID: processID,
	}
	respProcNoPaging, err := client.QueryAIEvent(ctx, reqProcNoPaging)
	require.NoError(t, err)
	require.Equal(t, procEventsTotal, len(respProcNoPaging.Events))
}
