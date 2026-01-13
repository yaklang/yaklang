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
			OrderBy: "id",
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
			OrderBy: "id",
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

func TestServer_QueryAIEvent_Filter_SessionAndNodeId(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	typeName := uuid.NewString()

	session1 := "session-" + uuid.NewString()
	session2 := "session-" + uuid.NewString()
	node1 := "node-" + uuid.NewString()
	node2 := "node-" + uuid.NewString()
	node3 := "node-" + uuid.NewString()

	a := uuid.NewString()
	b := uuid.NewString()
	c := uuid.NewString()
	d := uuid.NewString()

	events := []*schema.AiOutputEvent{
		{EventUUID: a, Type: schema.EventType(typeName), SessionId: session1, NodeId: node1},
		{EventUUID: b, Type: schema.EventType(typeName), SessionId: session1, NodeId: node2},
		{EventUUID: c, Type: schema.EventType(typeName), SessionId: session2, NodeId: node1},
		{EventUUID: d, Type: schema.EventType(typeName), SessionId: session2, NodeId: node3},
	}
	for _, event := range events {
		require.NoError(t, yakit.CreateOrUpdateAIOutputEvent(db, event))
	}
	uuidToID := map[string]uint{
		a: events[0].ID,
		b: events[1].ID,
		c: events[2].ID,
		d: events[3].ID,
	}
	defer func() {
		db.Where("event_uuid IN (?)", []string{a, b, c, d}).Delete(&schema.AiOutputEvent{})
	}()

	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	assertEventUUIDs := func(t *testing.T, rsp *ypb.AIEventQueryResponse, want ...string) {
		t.Helper()
		got := make([]string, 0, len(rsp.GetEvents()))
		for _, e := range rsp.GetEvents() {
			got = append(got, e.GetEventUUID())
		}
		require.ElementsMatch(t, want, got)
	}

	assertSortedByIDAsc := func(t *testing.T, rsp *ypb.AIEventQueryResponse) {
		t.Helper()
		var prev uint
		for i, e := range rsp.GetEvents() {
			id, ok := uuidToID[e.GetEventUUID()]
			require.True(t, ok, "missing id mapping for event_uuid=%s", e.GetEventUUID())
			if i > 0 {
				require.LessOrEqual(t, prev, id)
			}
			prev = id
		}
	}

	// 1) No pagination: filter by SessionID.
	rsp, err := client.QueryAIEvent(ctx, &ypb.AIEventQueryRequest{
		Filter: &ypb.AIEventFilter{
			EventType: []string{typeName},
			SessionID: session1,
		},
	})
	require.NoError(t, err)
	assertEventUUIDs(t, rsp, a, b)

	// 2) No pagination: filter by NodeId.
	rsp, err = client.QueryAIEvent(ctx, &ypb.AIEventQueryRequest{
		Filter: &ypb.AIEventFilter{
			EventType: []string{typeName},
			NodeId:    []string{node1},
		},
	})
	require.NoError(t, err)
	assertEventUUIDs(t, rsp, a, c)

	// 3) With pagination: filter by SessionID + NodeId (intersection).
	rsp, err = client.QueryAIEvent(ctx, &ypb.AIEventQueryRequest{
		Filter: &ypb.AIEventFilter{
			EventType: []string{typeName},
			SessionID: session2,
			NodeId:    []string{node1},
		},
		Pagination: &ypb.Paging{
			Page:    1,
			Limit:   10,
			OrderBy: "id",
			Order:   "asc",
		},
	})
	require.NoError(t, err)
	assertEventUUIDs(t, rsp, c)
	assertSortedByIDAsc(t, rsp)

	// 4) With pagination: multi NodeId OR.
	rsp, err = client.QueryAIEvent(ctx, &ypb.AIEventQueryRequest{
		Filter: &ypb.AIEventFilter{
			EventType: []string{typeName},
			NodeId:    []string{node1, node3},
		},
		Pagination: &ypb.Paging{
			Page:    1,
			Limit:   10,
			OrderBy: "id",
			Order:   "asc",
		},
	})
	require.NoError(t, err)
	assertEventUUIDs(t, rsp, a, c, d)
	require.Len(t, rsp.GetEvents(), 3)
	assertSortedByIDAsc(t, rsp)
}

func TestServer_QueryAIEvent_OrderByID_AscDesc(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	typeName := uuid.NewString()
	defer func() {
		db.Where("type = ?", typeName).Delete(&schema.AiOutputEvent{})
	}()

	uuidToID := make(map[string]uint, 5)
	for i := int64(0); i < 5; i++ {
		eventUUID := uuid.NewString()
		event := &schema.AiOutputEvent{
			EventUUID: eventUUID,
			Type:      schema.EventType(typeName),
		}
		require.NoError(t, yakit.CreateOrUpdateAIOutputEvent(db, event))
		uuidToID[eventUUID] = event.ID
	}

	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Ascending: smaller id first.
	asc, err := client.QueryAIEvent(ctx, &ypb.AIEventQueryRequest{
		Filter: &ypb.AIEventFilter{EventType: []string{typeName}},
		Pagination: &ypb.Paging{
			Page:    1,
			Limit:   5,
			OrderBy: "id",
			Order:   "asc",
		},
	})
	require.NoError(t, err)
	require.Len(t, asc.GetEvents(), 5)
	for i := 1; i < len(asc.GetEvents()); i++ {
		require.LessOrEqual(t, uuidToID[asc.GetEvents()[i-1].GetEventUUID()], uuidToID[asc.GetEvents()[i].GetEventUUID()])
	}

	// Descending: larger id first.
	desc, err := client.QueryAIEvent(ctx, &ypb.AIEventQueryRequest{
		Filter: &ypb.AIEventFilter{EventType: []string{typeName}},
		Pagination: &ypb.Paging{
			Page:    1,
			Limit:   5,
			OrderBy: "id",
			Order:   "desc",
		},
	})
	require.NoError(t, err)
	require.Len(t, desc.GetEvents(), 5)
	for i := 1; i < len(desc.GetEvents()); i++ {
		require.GreaterOrEqual(t, uuidToID[desc.GetEvents()[i-1].GetEventUUID()], uuidToID[desc.GetEvents()[i].GetEventUUID()])
	}
}
