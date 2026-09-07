package aiengine

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

func TestStreamFinishedOnlyReadsContentForRegisteredCallbacks(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	writerID := uuid.NewString()
	stream := &schema.AiOutputEvent{
		EventUUID: writerID, Type: schema.EVENT_TYPE_STREAM, NodeId: "re-act-loop-answer-payload",
		IsStream: true, StreamDelta: []byte("hello"),
	}
	require.NoError(t, db.Create(stream).Error)
	t.Cleanup(func() { db.Unscoped().Delete(stream) })
	var queries atomic.Int32
	callbackName := "test:stream-finished:" + writerID
	db.Callback().Query().After("gorm:query").Register(callbackName, func(scope *gorm.Scope) {
		for _, value := range scope.SQLVars {
			if value == writerID {
				queries.Add(1)
				break
			}
		}
	})
	t.Cleanup(func() { db.Callback().Query().Remove(callbackName) })

	for _, tc := range []struct {
		name       string
		end, total bool
		system     bool
	}{
		{name: "raw events only"},
		{name: "end callback", end: true},
		{name: "content callback", total: true},
		{name: "both callbacks", end: true, total: true},
		{name: "system stream", end: true, total: true, system: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := NewAIEngineConfig()
			endCalls, totalCalls := 0, 0
			if tc.end {
				WithOnStreamEnd(func(_ aicommon.AIEngineOperator, event *schema.AiOutputEvent, node string) {
					endCalls++
					require.Equal(t, writerID, event.EventUUID)
					require.Equal(t, stream.NodeId, node)
				})(cfg)
			}
			if tc.total {
				WithOnStreamContent(func(_ aicommon.AIEngineOperator, _ *schema.AiOutputEvent, node string, content []byte) {
					totalCalls++
					require.Equal(t, stream.NodeId, node)
					require.Equal(t, "hello", string(content))
				})(cfg)
			}
			engine := &AIEngine{config: cfg}
			queries.Store(0)
			engine.handleStreamFinishedEvent(&schema.AiOutputEvent{
				Type: schema.EVENT_TYPE_STRUCTURED, NodeId: "stream-finished", IsSystem: tc.system,
				Content: []byte(fmt.Sprintf(`{"event_writer_id":%q}`, writerID)),
			})
			if (tc.end || tc.total) && !tc.system {
				require.EqualValues(t, 1, queries.Load())
			} else {
				require.Zero(t, queries.Load(), "unused content must not query the database")
			}
			require.Equal(t, tc.end && !tc.system, endCalls == 1)
			require.Equal(t, tc.total && !tc.system, totalCalls == 1)
		})
	}
}
