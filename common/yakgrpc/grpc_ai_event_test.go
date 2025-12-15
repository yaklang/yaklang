package yakgrpc

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"testing"
	"time"

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
