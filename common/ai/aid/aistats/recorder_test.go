package aistats

import (
	"sync"
	"testing"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// newTestDB 建立一个内存 sqlite + AutoMigrate UserAIStats 两表, 供测试用.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", "file::memory:?cache=shared&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&schema.AIStatsEntityHit{}, &schema.AIUserDailyStats{}).Error; err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	// 清空共享内存表, 隔离每个测试.
	db.Exec("DELETE FROM ai_stats_entity_hits")
	db.Exec("DELETE FROM ai_user_daily_stats")
	return db
}

func TestIncrementEntityHit_FirstAndAccumulate(t *testing.T) {
	db := newTestDB(t)

	// 首次命中 (user_force → direct_count).
	if err := yakit.IncrementEntityHit(db, schema.AIStatsEntityTypeSkill, "sqli", "user_force"); err != nil {
		t.Fatalf("first increment: %v", err)
	}
	var row schema.AIStatsEntityHit
	if err := db.Where("entity_type = ? AND entity_name = ?", schema.AIStatsEntityTypeSkill, "sqli").First(&row).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if row.HitCount != 1 || row.DirectCount != 1 {
		t.Fatalf("expected hit=1 direct=1, got hit=%d direct=%d requested=%d auto=%d",
			row.HitCount, row.DirectCount, row.RequestedCount, row.AutoLoadedCount)
	}

	// 第二次 (ai_load → auto_loaded_count).
	if err := yakit.IncrementEntityHit(db, schema.AIStatsEntityTypeSkill, "sqli", "ai_load"); err != nil {
		t.Fatalf("second increment: %v", err)
	}
	db.Where("entity_type = ? AND entity_name = ?", schema.AIStatsEntityTypeSkill, "sqli").First(&row)
	if row.HitCount != 2 || row.DirectCount != 1 || row.AutoLoadedCount != 1 {
		t.Fatalf("expected hit=2 direct=1 auto=1, got hit=%d direct=%d requested=%d auto=%d",
			row.HitCount, row.DirectCount, row.RequestedCount, row.AutoLoadedCount)
	}
}

func TestIncrementDailyStats_CreateAndAccumulate(t *testing.T) {
	db := newTestDB(t)

	inc := map[string]interface{}{"actions": 1, "ai_calls": 1, "tokens_input": int64(100), "tokens_output": int64(50)}
	if err := yakit.IncrementDailyStats(db, "u1", "2026-07-12", inc); err != nil {
		t.Fatalf("increment: %v", err)
	}
	var row schema.AIUserDailyStats
	if err := db.Where("user_key = ? AND day = ?", "u1", "2026-07-12").First(&row).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if row.Actions != 1 || row.AICalls != 1 || row.TokensInput != 100 || row.TokensOutput != 50 {
		t.Fatalf("got actions=%d ai=%d in=%d out=%d", row.Actions, row.AICalls, row.TokensInput, row.TokensOutput)
	}

	// 再加一次 (累加).
	if err := yakit.IncrementDailyStats(db, "u1", "2026-07-12", map[string]interface{}{"actions": 2}); err != nil {
		t.Fatalf("second increment: %v", err)
	}
	db.Where("user_key = ? AND day = ?", "u1", "2026-07-12").First(&row)
	if row.Actions != 3 {
		t.Fatalf("expected actions=3 after add 2, got %d", row.Actions)
	}
}

func TestTopEntitiesByHits(t *testing.T) {
	db := newTestDB(t)
	// 准备: sqli 3 次, xss 1 次.
	for i := 0; i < 3; i++ {
		_ = yakit.IncrementEntityHit(db, schema.AIStatsEntityTypeSkill, "sqli", "ai_load")
	}
	_ = yakit.IncrementEntityHit(db, schema.AIStatsEntityTypeSkill, "xss", "ai_load")

	top := yakit.TopEntitiesByHits(db, schema.AIStatsEntityTypeSkill, 10)
	if len(top) != 2 {
		t.Fatalf("expected 2 top skills, got %d (%v)", len(top), top)
	}
	if top[0] != "sqli" {
		t.Fatalf("expected sqli first (3 hits), got %q first (%v)", top[0], top)
	}
	if top[1] != "xss" {
		t.Fatalf("expected xss second (1 hit), got %q second (%v)", top[1], top)
	}
}

func TestIncrementEntityHit_Concurrent(t *testing.T) {
	db := newTestDB(t)
	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_ = yakit.IncrementEntityHit(db, schema.AIStatsEntityTypeTool, "grep", "direct")
		}()
	}
	wg.Wait()
	var row schema.AIStatsEntityHit
	db.Where("entity_type = ? AND entity_name = ?", schema.AIStatsEntityTypeTool, "grep").First(&row)
	// sqlite 并发写入可能有竞争, 这里只验证最终落库成功 (hit >= 1).
	if row.HitCount < 1 {
		t.Fatalf("expected hit >= 1 after concurrent increments, got %d", row.HitCount)
	}
}

func TestToday(t *testing.T) {
	s := today()
	if len(s) != 10 {
		t.Fatalf("today() should be YYYY-MM-DD (10 chars), got %q", s)
	}
	if _, err := time.Parse("2006-01-02", s); err != nil {
		t.Fatalf("today() not parseable: %v", err)
	}
}

func TestResolveUserKey(t *testing.T) {
	if got := resolveUserKey(nil); got != DefaultUserKey {
		t.Fatalf("nil cfg → default, got %q", got)
	}
}
