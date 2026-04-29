package sfvm

import (
	"strings"
	"testing"
)

func TestFormatRecursiveConfigSummary(t *testing.T) {
	if got := FormatRecursiveConfigSummary(nil); got != "" {
		t.Fatalf("nil: want empty, got %q", got)
	}
	if got := FormatRecursiveConfigSummary([]*RecursiveConfigItem{}); got != "" {
		t.Fatalf("empty: want empty, got %q", got)
	}
	got := FormatRecursiveConfigSummary([]*RecursiveConfigItem{
		{Key: "depth", Value: "3"},
		nil,
		{Key: "include", Value: "foo", SyntaxFlowRule: true},
	})
	wantSub := "depth=3, <nil>, include=foo [sf]"
	if got != wantSub {
		t.Fatalf("got %q want %q", got, wantSub)
	}
	long := strings.Repeat("\u754c", 130)
	got = FormatRecursiveConfigSummary([]*RecursiveConfigItem{{Key: "until", Value: long}})
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("truncation: got %q", got)
	}
	if strings.Count(got, "\u754c") != 120 {
		t.Fatalf("rune truncation: want 120 runes in value part")
	}
}
