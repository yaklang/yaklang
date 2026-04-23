package reactloops

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
	_ "unsafe"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	yakScriptMatchStressPluginCount = 10000
	yakScriptMatchStressInputSize   = 1 << 20
	yakScriptMatchBatchSize         = 500
)

var yakScriptMatchBenchmarkSink int

//go:linkname constsProfileDatabase github.com/yaklang/yaklang/common/consts.profileDatabase
var constsProfileDatabase *gorm.DB

//go:linkname constsCurrentProfileDatabasePath github.com/yaklang/yaklang/common/consts.currentProfileDatabasePath
var constsCurrentProfileDatabasePath string

//go:linkname constsInitYakitDatabaseOnce github.com/yaklang/yaklang/common/consts.initYakitDatabaseOnce
var constsInitYakitDatabaseOnce *sync.Once

type yakScriptMatchStrategyMetrics struct {
	MatchedNames    []string
	Elapsed         time.Duration
	AllocDeltaBytes int64
	HeapDeltaBytes  int64
	Benchmark       testing.BenchmarkResult
}

func TestYakScriptMatchStrategies_Stress(t *testing.T) {
	if testing.Short() {
		t.Skip("skip yak script stress test in short mode")
	}

	db := createYakScriptMatchStressDatabase(t, yakScriptMatchStressPluginCount)
	installYakScriptStressProfileDatabase(t, db)

	targetNames := []string{
		"stress-plugin-00000",
		"stress-plugin-05000",
		"stress-plugin-09999",
	}
	input := buildYakScriptStressInput(targetNames, yakScriptMatchStressInputSize)
	normalizedTargets := append([]string(nil), targetNames...)
	sort.Strings(normalizedTargets)

	strategies := []struct {
		name string
		fn   func() []string
	}{
		{
			name: "db_instr_match",
			fn: func() []string {
				return matchYakScriptNamesByDatabase(db, input)
			},
		},
		{
			name: "go_strings_contains",
			fn: func() []string {
				return matchYakScriptNamesByStringsContains(db, input)
			},
		},
		{
			name: "go_index_all_substrings",
			fn: func() []string {
				return matchYakScriptNamesByIndexAllSubstrings(db, input)
			},
		},
	}

	for _, strategy := range strategies {
		metrics := measureYakScriptMatchStrategy(t, strategy.fn)
		assertYakScriptMatchTargets(t, strategy.name, metrics.MatchedNames, normalizedTargets)

		t.Logf(
			"%s: plugins=%d input_bytes=%d matched=%d query_time=%s cpu_bench=%s mem_bench=%s total_alloc_delta=%dB heap_alloc_delta=%dB",
			strategy.name,
			yakScriptMatchStressPluginCount,
			len(input),
			len(metrics.MatchedNames),
			metrics.Elapsed,
			metrics.Benchmark.String(),
			metrics.Benchmark.MemString(),
			metrics.AllocDeltaBytes,
			metrics.HeapDeltaBytes,
		)
	}
}

func measureYakScriptMatchStrategy(t *testing.T, fn func() []string) yakScriptMatchStrategyMetrics {
	t.Helper()

	runtime.GC()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	start := time.Now()
	matchedNames := fn()
	elapsed := time.Since(start)

	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	benchmark := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			matchResult := fn()
			yakScriptMatchBenchmarkSink += len(matchResult)
		}
	})

	return yakScriptMatchStrategyMetrics{
		MatchedNames:    matchedNames,
		Elapsed:         elapsed,
		AllocDeltaBytes: int64(memAfter.TotalAlloc - memBefore.TotalAlloc),
		HeapDeltaBytes:  int64(memAfter.HeapAlloc) - int64(memBefore.HeapAlloc),
		Benchmark:       benchmark,
	}
}

func assertYakScriptMatchTargets(t *testing.T, strategyName string, matchedNames []string, expectedTargets []string) {
	t.Helper()

	seen := make(map[string]bool, len(matchedNames))
	for _, name := range matchedNames {
		seen[name] = true
	}
	for _, target := range expectedTargets {
		if !seen[target] {
			t.Fatalf("%s: expected target plugin %q to be matched", strategyName, target)
		}
	}
}

func matchYakScriptNamesByDatabase(db *gorm.DB, input string) []string {
	normalizedInput := strings.ToLower(strings.TrimSpace(input))
	if normalizedInput == "" {
		return nil
	}

	var matched []*schema.YakScript
	if err := db.Model(&schema.YakScript{}).
		Select("script_name").
		Where("script_name <> ''").
		Where("enable_for_ai = ?", true).
		Where("instr(?, lower(script_name)) > 0", normalizedInput).
		Find(&matched).Error; err != nil {
		return nil
	}

	names := make([]string, 0, len(matched))
	for _, script := range matched {
		if script == nil || script.ScriptName == "" {
			continue
		}
		names = append(names, script.ScriptName)
	}
	return normalizeCapabilityStrings(names)
}

func matchYakScriptNamesByStringsContains(db *gorm.DB, input string) []string {
	normalizedInput := strings.ToLower(strings.TrimSpace(input))
	if normalizedInput == "" {
		return nil
	}

	var allNames []string
	if err := db.Model(&schema.YakScript{}).
		Where("script_name <> ''").
		Where("enable_for_ai = ?", true).
		Pluck("script_name", &allNames).Error; err != nil {
		return nil
	}

	var matched []string
	for _, name := range allNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if strings.Contains(normalizedInput, strings.ToLower(name)) {
			matched = append(matched, name)
		}
	}
	return normalizeCapabilityStrings(matched)
}

func matchYakScriptNamesByIndexAllSubstrings(db *gorm.DB, input string) []string {
	normalizedInput := strings.ToLower(strings.TrimSpace(input))
	if normalizedInput == "" {
		return nil
	}

	var allNames []string
	if err := db.Model(&schema.YakScript{}).
		Where("script_name <> ''").
		Where("enable_for_ai = ?", true).
		Pluck("script_name", &allNames).Error; err != nil {
		return nil
	}

	loweredNames := make([]string, 0, len(allNames))
	originalNames := make([]string, 0, len(allNames))
	for _, name := range allNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		originalNames = append(originalNames, name)
		loweredNames = append(loweredNames, strings.ToLower(name))
	}

	matches := utils.IndexAllSubstrings(normalizedInput, loweredNames...)
	seen := make(map[int]bool)
	var matched []string
	for _, match := range matches {
		if len(match) != 2 {
			continue
		}
		patternIdx := match[0]
		if patternIdx < 0 || patternIdx >= len(originalNames) || seen[patternIdx] {
			continue
		}
		seen[patternIdx] = true
		matched = append(matched, originalNames[patternIdx])
	}
	return normalizeCapabilityStrings(matched)
}

func createYakScriptMatchStressDatabase(t *testing.T, pluginCount int) *gorm.DB {
	t.Helper()

	db, err := utils.CreateTempTestDatabaseInMemory()
	if err != nil {
		t.Fatalf("create temp profile db failed: %v", err)
	}

	schema.AutoMigrate(db, schema.KEY_SCHEMA_PROFILE_DATABASE)
	schema.ApplyPatches(db, schema.KEY_SCHEMA_PROFILE_DATABASE)

	types := []string{"yak", "mitm", "port-scan"}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction failed: %v", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback().Error
			panic(r)
		}
	}()

	flush := func(start, end int) {
		if start >= end {
			return
		}
		now := time.Now()
		var query strings.Builder
		var args []interface{}
		query.WriteString(`INSERT INTO yak_scripts (script_name, type, help, enable_for_ai, ai_desc, ai_keywords, ai_usage, created_at, updated_at) VALUES `)
		for i := start; i < end; i++ {
			if i > start {
				query.WriteString(",")
			}
			query.WriteString("(?, ?, ?, ?, ?, ?, ?, ?, ?)")
			args = append(args,
				fmt.Sprintf("stress-plugin-%05d", i),
				types[i%len(types)],
				fmt.Sprintf("stress test plugin #%d", i),
				true,
				fmt.Sprintf("AI description for stress plugin #%d", i),
				"stress,plugin,match",
				"stress test usage",
				now,
				now,
			)
		}
		if err := tx.Exec(query.String(), args...).Error; err != nil {
			t.Fatalf("seed yak scripts failed: %v", err)
		}
	}

	for start := 0; start < pluginCount; start += yakScriptMatchBatchSize {
		end := start + yakScriptMatchBatchSize
		if end > pluginCount {
			end = pluginCount
		}
		flush(start, end)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit yak script seed transaction failed: %v", err)
	}

	return db
}

func installYakScriptStressProfileDatabase(t *testing.T, db *gorm.DB) {
	t.Helper()

	oldProfileDB := constsProfileDatabase
	oldProfilePath := constsCurrentProfileDatabasePath
	oldInitOnce := constsInitYakitDatabaseOnce

	constsProfileDatabase = db
	constsCurrentProfileDatabasePath = filepath.Join(t.TempDir(), "yak-script-match-stress-profile.db")
	constsInitYakitDatabaseOnce = completedSyncOnce()
	schema.SetGormProfileDatabase(db)

	t.Cleanup(func() {
		schema.SetGormProfileDatabase(oldProfileDB)
		constsProfileDatabase = oldProfileDB
		constsCurrentProfileDatabasePath = oldProfilePath
		constsInitYakitDatabaseOnce = oldInitOnce
		_ = db.Close()
	})
}

func completedSyncOnce() *sync.Once {
	var once sync.Once
	once.Do(func() {})
	return &once
}

func buildYakScriptStressInput(targetNames []string, targetSize int) string {
	if targetSize <= 0 {
		targetSize = yakScriptMatchStressInputSize
	}

	var builder strings.Builder
	noiseChunk := "0123456789abcdefghijklmnopqrstuvwxyz_stress_noise_"
	for builder.Len() < targetSize-len(strings.Join(targetNames, " "))-128 {
		builder.WriteString(noiseChunk)
	}

	builder.WriteString(" begin_target_plugins ")
	for _, name := range targetNames {
		builder.WriteString(name)
		builder.WriteString(" ")
	}
	builder.WriteString(" end_target_plugins ")

	for builder.Len() < targetSize {
		builder.WriteString("x")
	}

	return builder.String()
}
