package yakit

import (
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestLoadAIReActThinkingAggregatedForSession(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AIReActThinkingChunk{}).Error)

	sid := "sess-1"
	loop := "main"
	require.NoError(t, db.Create(&schema.AIReActThinkingChunk{
		PersistentSessionId: sid,
		LoopName:            loop,
		Content:             "a",
	}).Error)
	require.NoError(t, db.Create(&schema.AIReActThinkingChunk{
		PersistentSessionId: sid,
		LoopName:            loop,
		Content:             "b",
	}).Error)
	require.NoError(t, db.Create(&schema.AIReActThinkingChunk{
		PersistentSessionId: "other",
		LoopName:              loop,
		Content:               "x",
	}).Error)

	merged, err := LoadAIReActThinkingAggregatedForSession(db, sid, loop)
	require.NoError(t, err)
	require.Equal(t, "ab", merged)

	empty, err := LoadAIReActThinkingAggregatedForSession(db, "missing", loop)
	require.NoError(t, err)
	require.Equal(t, "", empty)
}

func TestLoadAIReActThinkingAggregated_OrderByCreatedAt(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AIReActThinkingChunk{}).Error)

	sid := "sess-order"
	loop := "main"
	tNewer := time.Now().UTC()
	tOlder := tNewer.Add(-time.Hour)

	// Insert newer row first (larger id); older timestamp should still sort first.
	require.NoError(t, db.Create(&schema.AIReActThinkingChunk{
		Model:               gorm.Model{CreatedAt: tNewer},
		PersistentSessionId: sid,
		LoopName:            loop,
		Content:             "B",
	}).Error)
	require.NoError(t, db.Create(&schema.AIReActThinkingChunk{
		Model:               gorm.Model{CreatedAt: tOlder},
		PersistentSessionId: sid,
		LoopName:            loop,
		Content:             "A",
	}).Error)

	merged, err := LoadAIReActThinkingAggregatedForSession(db, sid, loop)
	require.NoError(t, err)
	require.Equal(t, "AB", merged)
}

func TestLoadAIReActThinkingAggregatedForRuntimeScope(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AIReActThinkingChunk{}).Error)

	rt := "run-1"
	loop := "main"
	require.NoError(t, db.Create(&schema.AIReActThinkingChunk{
		RuntimeId: rt,
		LoopName:  loop,
		Content:   "x",
	}).Error)
	require.NoError(t, db.Create(&schema.AIReActThinkingChunk{
		PersistentSessionId: "ps1",
		RuntimeId:           rt,
		LoopName:            loop,
		Content:             "y",
	}).Error)

	merged, err := LoadAIReActThinkingAggregated(db, loop, "", rt)
	require.NoError(t, err)
	require.Equal(t, "x", merged)
}
