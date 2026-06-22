package minirehs

import (
	"sort"
	"testing"
)

func collectMatches(t *testing.T, db Database, data []byte) []Match {
	t.Helper()
	sc, err := db.NewScratch()
	if err != nil {
		t.Fatalf("new scratch: %v", err)
	}
	defer sc.Close()
	var out []Match
	if err := db.Scan(data, sc, func(m Match) bool {
		out = append(out, m)
		return true
	}); err != nil {
		t.Fatalf("scan: %v", err)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out
}

func TestSmokeEngineVsStdlib(t *testing.T) {
	patterns := []Pattern{
		{ID: 1, Expr: `password\s*=\s*\S+`},
		{ID: 2, Expr: `AKIA[0-9A-Z]{16}`},
		{ID: 3, Expr: `\bDruid\b`, Flags: FlagCaseless},
		{ID: 4, Expr: `[0-9]{3,}`}, // always-on (无必需字面量)
	}
	data := []byte("user password = hunter2; key AKIAABCDEFGHIJKLMNOP; using DRUID pool id 12345")

	engine, err := Compile(patterns, WithBackend(BackendEngine))
	if err != nil {
		t.Fatalf("compile engine: %v", err)
	}
	defer engine.Close()
	oracle, err := Compile(patterns, WithBackend(BackendStdlib))
	if err != nil {
		t.Fatalf("compile oracle: %v", err)
	}
	defer oracle.Close()

	got := collectMatches(t, engine, data)
	want := collectMatches(t, oracle, data)

	if len(got) != len(want) {
		t.Fatalf("match count mismatch: engine=%d oracle=%d\nengine=%v\noracle=%v", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("match[%d] mismatch: engine=%v oracle=%v", i, got[i], want[i])
		}
	}
	t.Logf("engine info: %+v", engine.Info())
	t.Logf("matches: %v", got)
}

func TestRegexp2Fallback(t *testing.T) {
	// 负向先行 (?!...) 是 RE2 不支持、regexp2 支持的构造, 应作为 always-on 被承载.
	patterns := []Pattern{
		{ID: 10, Expr: `foo(?!bar)`},
	}
	db, err := Compile(patterns, WithBackend(BackendEngine))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	defer db.Close()

	info := db.Info()
	if info.NumAlwaysOn != 1 {
		t.Fatalf("expected 1 always-on (regexp2), got %d", info.NumAlwaysOn)
	}

	matched := false
	sc, _ := db.NewScratch()
	defer sc.Close()
	_ = db.Scan([]byte("foobaz foobar"), sc, func(m Match) bool {
		if m.ID == 10 {
			matched = true
		}
		return true
	})
	if !matched {
		t.Fatalf("regexp2 negative-lookahead pattern should match 'foobaz'")
	}
}
