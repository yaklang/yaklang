package yakit

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestYieldAIEvent(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)

	err = db.AutoMigrate(&schema.AiOutputEvent{}).Error
	require.NoError(t, err)

	// Prepare data
	totalEvents := 100
	eventUUIDs := make([]string, totalEvents)
	types := []schema.EventType{"type_a", "type_b", "type_c"}

	tx := db.Begin()
	for i := 0; i < totalEvents; i++ {
		uuidStr := uuid.NewString()
		eventUUIDs[i] = uuidStr
		event := &schema.AiOutputEvent{
			EventUUID:     uuidStr,
			CoordinatorId: fmt.Sprintf("coord-%d", i%5), // 5 coordinators
			Type:          types[i%3],                   // 3 types
			TaskIndex:     fmt.Sprintf("task-%d", i%10), // 10 tasks
			Content:       []byte(fmt.Sprintf("content-%d", i)),
			Timestamp:     time.Now().Unix(),
		}
		err := tx.Create(event).Error
		require.NoError(t, err)
	}
	tx.Commit()

	t.Run("Basic_Yield_All", func(t *testing.T) {
		ctx := context.Background()
		filter := &ypb.AIEventFilter{} // Empty filter
		ch := YieldAIEvent(ctx, db, filter)

		count := 0
		for range ch {
			count++
		}
		assert.Equal(t, totalEvents, count)
	})

	t.Run("Filter_Single_Large_Array", func(t *testing.T) {
		// Test chunking logic with > 10 items (batch size is 10)
		targetUUIDs := eventUUIDs[:25] // 25 items -> 3 chunks
		ctx := context.Background()
		filter := &ypb.AIEventFilter{
			EventUUIDS: targetUUIDs,
		}

		ch := YieldAIEvent(ctx, db, filter)
		var results []*schema.AiOutputEvent
		for item := range ch {
			results = append(results, item)
		}

		assert.Equal(t, 25, len(results))
		// Verify IDs
		foundIDs := make(map[string]bool)
		for _, item := range results {
			foundIDs[item.EventUUID] = true
		}
		for _, id := range targetUUIDs {
			assert.True(t, foundIDs[id], "UUID %s should be found", id)
		}
	})

	t.Run("Filter_Multiple_Arrays_Cartesian", func(t *testing.T) {
		targetTypes := []string{"type_a", "type_b"}
		targetCoords := []string{"coord-0", "coord-1"}

		filter := &ypb.AIEventFilter{
			EventType:     targetTypes,
			CoordinatorId: targetCoords,
		}

		ch := YieldAIEvent(context.Background(), db, filter)
		var results []*schema.AiOutputEvent
		for item := range ch {
			results = append(results, item)
		}

		expectedCount := 0
		for i := 0; i < totalEvents; i++ {
			tStr := string(types[i%3])
			cStr := fmt.Sprintf("coord-%d", i%5)
			matchType := false
			for _, t := range targetTypes {
				if t == tStr {
					matchType = true
					break
				}
			}
			matchCoord := false
			for _, c := range targetCoords {
				if c == cStr {
					matchCoord = true
					break
				}
			}

			if matchType && matchCoord {
				expectedCount++
			}
		}

		assert.Equal(t, expectedCount, len(results))
	})

	t.Run("Filter_Huge_Multiple_Arrays", func(t *testing.T) {

		hugeList1 := make([]string, 25) // 3 chunks
		for i := 0; i < 25; i++ {
			hugeList1[i] = fmt.Sprintf("coord-%d", i)
		}

		hugeList2 := make([]string, 25) // 3 chunks
		for i := 0; i < 25; i++ {
			hugeList2[i] = fmt.Sprintf("task-%d", i)
		}

		// Total combinations: 3 * 3 = 9 chunks queries
		filter := &ypb.AIEventFilter{
			CoordinatorId: hugeList1,
			TaskIndex:     hugeList2,
		}

		ch := YieldAIEvent(context.Background(), db, filter)
		count := 0
		for range ch {
			count++
		}
		t.Logf("Huge filter query returned %d items", count)
	})

	t.Run("Context_Cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		filter := &ypb.AIEventFilter{} // All items
		ch := YieldAIEvent(ctx, db, filter)

		<-ch
		cancel()

		count := 0
		for range ch {
			count++
		}
		assert.True(t, count < totalEvents-1)
	})
}
