package scannode

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	ssav1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ssa/v1"
	"google.golang.org/protobuf/proto"
)

func TestSSADebugQueryRejectsLegacyJSON(t *testing.T) {
	bridge := &legionJobBridge{}
	err := bridge.handleSSADebugQuery(context.Background(), []byte(`{"query_id":"q","job_id":"j","attempt_id":"a"}`))
	require.ErrorContains(t, err, "unmarshal ssa debug query")
}

func TestDebugRunAnalysisProtobufPreservesNestedData(t *testing.T) {
	started := time.Date(2026, 9, 24, 1, 2, 3, 456, time.UTC)
	analysis := &DebugRunAnalysis{
		RunDir: "/private/debug",
		Status: "running", StartedAt: &started,
		Phases: []PhaseAnalysis{{Phase: "compile", Source: "status_card", StartedAt: &started}},
		Samples: []SampleSummary{{
			Sequence: 1, Label: "sample-1", Timestamp: &started, DurationMS: 5000,
			HasCPU: true, CPUTop: []PprofTopFunction{{Name: "compile", CumValue: 100}},
			CPUStacks: []PprofStackPath{{Frames: []string{"root", "compile"}, Value: 75}},
			DBStats: &DBOpStatsSummary{TotalCount: 7, Ops: map[string]DBOpBucketSummary{
				"insert": {Count: 3, TotalMs: 12},
			}},
			Runtime: &RuntimeStatsSummary{ProcessRSSBytes: 1 << 34, HostCPUPercent: 42.5},
		}},
		Summary: &RunSummary{TotalDurationMS: 5000, HasSSADB: true},
		Errors:  []string{"partial profile"}, Partial: true,
	}
	result := &ssav1.DebugQueryResult{QueryId: "query-1", Found: true, Analysis: debugRunAnalysisToProto(analysis)}
	raw, err := proto.Marshal(result)
	require.NoError(t, err)
	var decoded ssav1.DebugQueryResult
	require.NoError(t, proto.Unmarshal(raw, &decoded))
	require.Equal(t, "query-1", decoded.QueryId)
	require.Equal(t, started, decoded.Analysis.StartedAt.AsTime())
	require.Equal(t, "compile", decoded.Analysis.Phases[0].Phase)
	require.Equal(t, int64(100), decoded.Analysis.Samples[0].CpuTop[0].CumValue)
	require.Equal(t, []string{"root", "compile"}, decoded.Analysis.Samples[0].CpuStacks[0].Frames)
	require.Equal(t, int64(3), decoded.Analysis.Samples[0].DbStats.Ops["insert"].Count)
	require.Equal(t, uint64(1<<34), decoded.Analysis.Samples[0].Runtime.ProcessRssBytes)
	require.True(t, decoded.Analysis.Summary.HasSsadb)
	require.Equal(t, []string{"partial profile"}, decoded.Analysis.Errors)
	require.NotContains(t, decoded.String(), "/private/debug")
}

func TestBoundDebugQueryResultUsesProtobufSize(t *testing.T) {
	result := &ssav1.DebugQueryResult{
		QueryId: "query-1", Found: true,
		Analysis: &ssav1.DebugRunAnalysis{Samples: []*ssav1.DebugSampleSummary{{
			Label: "oversized", LogExcerpt: strings.Repeat("x", 2*1024*1024),
		}}},
	}
	require.Greater(t, proto.Size(result), 1024*1024)
	require.NoError(t, boundDebugQueryResult(result))
	require.LessOrEqual(t, proto.Size(result), 900*1024)
	require.Empty(t, result.Analysis.Samples[0].LogExcerpt)
}
