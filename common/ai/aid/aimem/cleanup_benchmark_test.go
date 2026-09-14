package aimem

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

func seedCleanupLoad(t testing.TB, db *gorm.DB, rows, bodyBytes int) {
	t.Helper()
	// Build a large database without constructing a large Go object graph.
	// Empty/malformed legacy vectors and questions must never be decoded by
	// the candidate scan; only scores and IDs are needed.
	err := db.Exec(`WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n < ?)
		INSERT INTO ai_memory_entities_v1 (memory_id, session_id, content, created_at,
			r_score,a_score,p_score,c_score,o_score,e_score,t_score,core_pact_vector,potential_questions)
		SELECT printf('memory-%09d',n),'default',replace(hex(zeroblob(?)), '00', 'x'),
			'2020-01-01', (n % 100)*0.01, 0.2, 0.1, 0.3, 0.4, 0.1, 0.9, '[0,0,0,0,0,0,0]', '[]' FROM seq`, rows, bodyBytes).Error
	require.NoError(t, err)
}

func BenchmarkCleanupMemoryScan(b *testing.B) {
	for _, rows := range []int{1000, 10000} {
		b.Run(fmt.Sprintf("rows=%d/body=16KiB", rows), func(b *testing.B) {
			db := cleanupRegressionDB(b)
			seedCleanupLoad(b, db, rows, 16*1024)
			config := DefaultCleanupConfig()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ids, err := ScanOverCountMemories(db, "ai_memory_entities_v1", "default", config)
				if err != nil {
					b.Fatal(err)
				}
				if len(ids) == 0 {
					b.Fatal("no candidates")
				}
			}
		})
	}
}

func TestMUSTPASS_CleanupLargeDirtyDatabase(t *testing.T) {
	rows := 2000
	bodyBytes := 16 * 1024
	if raw := os.Getenv("YAK_AIMEM_STRESS_BODY_BYTES"); raw != "" {
		var err error
		bodyBytes, err = strconv.Atoi(raw)
		require.NoError(t, err)
		require.Positive(t, bodyBytes)
		require.LessOrEqual(t, bodyBytes, 1024*1024)
	}
	if raw := os.Getenv("YAK_AIMEM_STRESS_ROWS"); raw != "" {
		var err error
		rows, err = strconv.Atoi(raw)
		require.NoError(t, err)
		require.Positive(t, rows)
		require.LessOrEqual(t, rows, 1000000)
	}
	db := cleanupRegressionDB(t)
	seedCleanupLoad(t, db, rows, bodyBytes)
	require.NoError(t, db.Exec("UPDATE ai_memory_entities_v1 SET core_pact_vector = ?, potential_questions = ?", "not-json", "not-json").Error)
	config := DefaultCleanupConfig()
	ids, err := ScanAllCleanupMemories(db, "ai_memory_entities_v1", "default", config)
	require.NoError(t, err)
	require.Len(t, ids, 100)
	t.Logf("selected %d candidates from %d rows with %d MiB of payload", len(ids), rows, int64(rows)*int64(bodyBytes)/(1024*1024))
}
