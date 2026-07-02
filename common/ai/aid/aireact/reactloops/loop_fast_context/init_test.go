package loop_fast_context

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

func TestFastContextLoopRegistered(t *testing.T) {
	_, ok := reactloops.GetLoopFactory(schema.AI_REACT_LOOP_NAME_FAST_CONTEXT)
	if !ok {
		t.Fatal("fast_context loop factory should be registered")
	}
	meta, ok := reactloops.GetLoopMetadata(schema.AI_REACT_LOOP_NAME_FAST_CONTEXT)
	if !ok || meta.VerboseNameZh == "" {
		t.Fatal("fast_context metadata should be registered")
	}
}

func TestExplorationReportFormatUserMarkdown(t *testing.T) {
	report := &ExplorationReport{
		Query:   "upload handler",
		Summary: "路由与校验逻辑分布在两个文件。",
		Locations: []LocationHit{
			{Path: "/tmp/a.go", StartLine: 10, EndLine: 15, Reason: "handler"},
		},
		SearchStats: SearchStats{Rounds: 2, ToolCalls: 4, UniqueFiles: 1},
	}
	md := report.FormatUserMarkdown()
	if md == "" || !strings.Contains(md, "/tmp/a.go") {
		t.Fatalf("unexpected markdown: %s", md)
	}
}
