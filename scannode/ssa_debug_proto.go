package scannode

import (
	"time"

	ssav1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ssa/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// debugRunAnalysisToProto keeps the NATS result fully typed. The JSON cache
// remains a local debug artifact; it is never placed inside the wire message.
func debugRunAnalysisToProto(analysis *DebugRunAnalysis) *ssav1.DebugRunAnalysis {
	if analysis == nil {
		return nil
	}
	result := &ssav1.DebugRunAnalysis{
		Status:     analysis.Status,
		StartedAt:  debugTimestamp(analysis.StartedAt),
		FinishedAt: debugTimestamp(analysis.FinishedAt),
		Duration:   analysis.Duration,
		Errors:     append([]string(nil), analysis.Errors...),
		Partial:    analysis.Partial,
		Summary:    debugRunSummaryToProto(analysis.Summary),
	}
	for _, phase := range analysis.Phases {
		result.Phases = append(result.Phases, &ssav1.DebugPhaseAnalysis{
			Phase:      phase.Phase,
			Source:     phase.Source,
			StartedAt:  debugTimestamp(phase.StartedAt),
			FinishedAt: debugTimestamp(phase.FinishedAt),
			Duration:   phase.Duration,
			Status:     phase.Status,
		})
	}
	for _, sample := range analysis.Samples {
		result.Samples = append(result.Samples, debugSampleToProto(sample))
	}
	return result
}

func debugTimestamp(value *time.Time) *timestamppb.Timestamp {
	if value == nil {
		return nil
	}
	return timestamppb.New(value.UTC())
}

func debugSampleToProto(sample SampleSummary) *ssav1.DebugSampleSummary {
	result := &ssav1.DebugSampleSummary{
		Sequence:          int32(sample.Sequence),
		Label:             sample.Label,
		Timestamp:         debugTimestamp(sample.Timestamp),
		EndedAt:           debugTimestamp(sample.EndedAt),
		DurationMs:        sample.DurationMS,
		Phase:             sample.Phase,
		PhaseSource:       sample.PhaseSource,
		HasCpu:            sample.HasCPU,
		HasHeap:           sample.HasHeap,
		HasGoroutine:      sample.HasGoroutine,
		CpuTop:            debugTopFunctionsToProto(sample.CPUTop),
		CpuTopYaklang:     debugTopFunctionsToProto(sample.CPUTopYaklang),
		CpuStacks:         debugStackPathsToProto(sample.CPUStacks),
		CpuStacksYaklang:  debugStackPathsToProto(sample.CPUStacksYaklang),
		HeapTop:           debugTopFunctionsToProto(sample.HeapTop),
		HeapTopYaklang:    debugTopFunctionsToProto(sample.HeapTopYaklang),
		HeapStacks:        debugStackPathsToProto(sample.HeapStacks),
		HeapStacksYaklang: debugStackPathsToProto(sample.HeapStacksYaklang),
		Goroutines:        int32(sample.Goroutines),
		LogExcerpt:        sample.LogExcerpt,
		Status:            sample.Status,
		DbStats:           debugDBStatsToProto(sample.DBStats),
		Runtime:           debugRuntimeToProto(sample.Runtime),
	}
	return result
}

func debugTopFunctionsToProto(items []PprofTopFunction) []*ssav1.DebugPprofTopFunction {
	if len(items) == 0 {
		return nil
	}
	result := make([]*ssav1.DebugPprofTopFunction, 0, len(items))
	for _, item := range items {
		result = append(result, &ssav1.DebugPprofTopFunction{
			Name: item.Name, CumValue: item.CumValue, CumPct: item.CumPct,
			FlatValue: item.FlatValue, FlatPct: item.FlatPct,
		})
	}
	return result
}

func debugStackPathsToProto(items []PprofStackPath) []*ssav1.DebugPprofStackPath {
	if len(items) == 0 {
		return nil
	}
	result := make([]*ssav1.DebugPprofStackPath, 0, len(items))
	for _, item := range items {
		result = append(result, &ssav1.DebugPprofStackPath{
			Frames: append([]string(nil), item.Frames...), Value: item.Value, Pct: item.Pct,
		})
	}
	return result
}

func debugRuntimeToProto(runtime *RuntimeStatsSummary) *ssav1.DebugRuntimeStatsSummary {
	if runtime == nil {
		return nil
	}
	return &ssav1.DebugRuntimeStatsSummary{
		Timestamp:             runtime.Timestamp,
		NumCpu:                int32(runtime.NumCPU),
		Load1:                 runtime.Load1,
		HostCpuPercent:        runtime.HostCPUPercent,
		ProcessCpuPercent:     runtime.ProcessCPUPercent,
		HostMemTotalBytes:     runtime.HostMemTotalBytes,
		HostMemUsedBytes:      runtime.HostMemUsedBytes,
		HostMemAvailableBytes: runtime.HostMemAvailableBytes,
		ProcessRssBytes:       runtime.ProcessRSSBytes,
		ProcessHeapAllocBytes: runtime.ProcessHeapAllocBytes,
		ProcessHeapSysBytes:   runtime.ProcessHeapSysBytes,
		Goroutines:            int32(runtime.Goroutines),
	}
}

func debugDBStatsToProto(stats *DBOpStatsSummary) *ssav1.DebugDBOpStatsSummary {
	if stats == nil {
		return nil
	}
	result := &ssav1.DebugDBOpStatsSummary{
		Dialect:    stats.Dialect,
		TotalCount: stats.TotalCount,
		TotalMs:    stats.TotalMs,
		WindowMs:   stats.WindowMs,
		ErrorCount: stats.ErrorCount,
	}
	if len(stats.Ops) != 0 {
		result.Ops = make(map[string]*ssav1.DebugDBOpBucketSummary, len(stats.Ops))
		for name, bucket := range stats.Ops {
			result.Ops[name] = &ssav1.DebugDBOpBucketSummary{
				Count: bucket.Count, TotalMs: bucket.TotalMs, MinMs: bucket.MinMs,
				MaxMs: bucket.MaxMs, AvgMs: bucket.AvgMs, ErrorCount: bucket.ErrorCount,
			}
		}
	}
	return result
}

func debugRunSummaryToProto(summary *RunSummary) *ssav1.DebugRunSummary {
	if summary == nil {
		return nil
	}
	return &ssav1.DebugRunSummary{
		TotalDurationMs:   summary.TotalDurationMS,
		CpuProfileFiles:   int32(summary.CPUProfileFiles),
		HeapProfileFiles:  int32(summary.HeapProfileFiles),
		GoroutineFiles:    int32(summary.GoroutineFiles),
		HasLog:            summary.HasLog,
		HasReport:         summary.HasReport,
		HasSsadb:          summary.HasSSADB,
		HasCmd:            summary.HasCmd,
		CompilePhaseFound: summary.CompilePhaseFound,
		ScanPhaseFound:    summary.ScanPhaseFound,
		DbStatsTotal:      debugDBStatsToProto(summary.DBStatsTotal),
	}
}
