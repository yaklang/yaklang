package main

import (
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestAppendPipelineTimingSamplesCorrelatesUniqueFlows(t *testing.T) {
	response := &ypb.QueryHTTPFlowResponse{
		SystemTiming: &ypb.QueryHTTPFlowSystemTiming{
			QueryDurationUs:      2_500,
			CountDurationUs:      1_000,
			DataQueryDurationUs:  1_250,
			CountExecuted:        true,
			ConversionDurationUs: 500,
			AsyncWriteQueueDepth: 3,
			FlowTimings: []*ypb.HTTPFlowSystemTiming{
				{
					Id:                             7,
					RequestHijackAtUnixMs:          100,
					ResponseMirrorAtUnixMs:         150,
					FlowBuiltAtUnixMs:              160,
					PersistEnqueuedAtUnixMs:        170,
					PersistStartedAtUnixMs:         180,
					PersistedAtUnixMs:              190,
					DatabaseChangeDetectedAtUnixMs: 200,
				},
			},
		},
	}
	var samples pipelineTimingSamples
	seen := make(map[uint64]struct{})
	receivedAt := time.UnixMilli(250)
	if !appendPipelineTimingSamples(&samples, response, receivedAt, seen) {
		t.Fatal("expected system timing to be accepted")
	}
	if !appendPipelineTimingSamples(&samples, response, receivedAt, seen) {
		t.Fatal("query timing should still be accepted when its flow was already sampled")
	}

	assertSingle := func(name string, values []float64, expected float64) {
		t.Helper()
		if len(values) != 1 || values[0] != expected {
			t.Fatalf("%s: expected [%v], got %v", name, expected, values)
		}
	}
	if len(samples.backendQueryMS) != 2 || samples.backendQueryMS[0] != 2.5 {
		t.Fatalf("unexpected backend query samples: %v", samples.backendQueryMS)
	}
	if len(samples.backendCountMS) != 2 || samples.backendCountMS[0] != 1 {
		t.Fatalf("unexpected backend COUNT samples: %v", samples.backendCountMS)
	}
	if len(samples.backendDataQueryMS) != 2 || samples.backendDataQueryMS[0] != 1.25 {
		t.Fatalf("unexpected backend data-query samples: %v", samples.backendDataQueryMS)
	}
	if samples.countExecuted != 2 {
		t.Fatalf("expected two COUNT executions, got %d", samples.countExecuted)
	}
	if samples.observedFlowCount != 1 {
		t.Fatalf("expected one unique flow, got %d", samples.observedFlowCount)
	}
	assertSingle("persist queue", samples.persistQueueWaitMS, 10)
	assertSingle("persist write", samples.persistWriteMS, 10)
	assertSingle("database detection", samples.databaseChangeDetectionMS, 10)
	assertSingle("request to built", samples.requestToFlowBuiltMS, 60)
	assertSingle("response to built", samples.responseToFlowBuiltMS, 10)
	assertSingle("request to receive", samples.requestToProbeReceiveMS, 150)
	assertSingle("response to receive", samples.responseToProbeReceiveMS, 100)
	assertSingle("persist to receive", samples.persistToProbeReceiveMS, 60)
}

func TestAppendPipelineTimingSamplesRejectsMissingDiagnosticPayload(t *testing.T) {
	var samples pipelineTimingSamples
	if appendPipelineTimingSamples(&samples, nil, time.Now(), map[uint64]struct{}{}) {
		t.Fatal("nil response must not be accepted")
	}
	if appendPipelineTimingSamples(&samples, &ypb.QueryHTTPFlowResponse{}, time.Now(), map[uint64]struct{}{}) {
		t.Fatal("response without SystemTiming must not be accepted")
	}
}

func TestRealtimeProbeSkipsUnusedExactTotal(t *testing.T) {
	query := newRealtimeQueryForToken("scenario-token")
	if !query.GetSkipTotal() {
		t.Fatal("realtime probe must skip the exact total it does not consume")
	}
	if query.GetSearchURL() != "scenario-token" {
		t.Fatalf("unexpected scenario filter: %q", query.GetSearchURL())
	}

	staticQuery := newRealtimeQuery()
	if staticQuery.GetSkipTotal() {
		t.Fatal("static database-read query must retain the exact COUNT baseline")
	}
}
