package scannode

import (
	"testing"

	"github.com/yaklang/yaklang/common/spec"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
)

func TestEmitSSAFileDedup(t *testing.T) {
	e := &StreamEmitter{enabled: true, chunkSize: 256 * 1024, inlineMax: 16 * 1024}
	file := &sfreport.File{
		Path: "src/main/java/A.java", Length: 5, LineCount: 1,
		IrSourceHash: "file-h1", Content: "hello",
	}

	tests := []struct {
		name          string
		taskId        string
		wantFileCount int
		wantStarted   bool
	}{
		{"first_emit", "task-file-1", 1, true},
		{"second_emit_deduped", "task-file-1", 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := e.EmitSSAFile(tt.taskId, "rt-1", "sub-1", file); err != nil {
				t.Fatalf("emit file failed: %v", err)
			}
			state := e.getTaskState(tt.taskId)
			state.mu.Lock()
			started := state.started
			fileCount := len(state.sentFiles)
			state.mu.Unlock()
			if started != tt.wantStarted {
				t.Fatalf("started mismatch: got=%v want=%v", started, tt.wantStarted)
			}
			if fileCount != tt.wantFileCount {
				t.Fatalf("file count mismatch: got=%d want=%d", fileCount, tt.wantFileCount)
			}
		})
	}
}

func TestEmitSSADataflowDedup(t *testing.T) {
	e := &StreamEmitter{enabled: true, chunkSize: 256 * 1024, inlineMax: 16 * 1024}
	payload := []byte(`{"nodes":[1],"edges":[]}`)

	tests := []struct {
		name          string
		taskId        string
		wantFlowCount int
	}{
		{"first_emit", "task-flow-1", 1},
		{"second_emit_deduped", "task-flow-1", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := e.EmitSSADataflow(tt.taskId, "rt-1", "sub-1", "flow-h1", payload); err != nil {
				t.Fatalf("emit dataflow failed: %v", err)
			}
			state := e.getTaskState(tt.taskId)
			state.mu.Lock()
			flowCount := len(state.sentFlows)
			state.mu.Unlock()
			if flowCount != tt.wantFlowCount {
				t.Fatalf("flow count mismatch: got=%d want=%d", flowCount, tt.wantFlowCount)
			}
		})
	}
}

func TestEmitSSARisk(t *testing.T) {
	tests := []struct {
		name        string
		dropInfo    bool
		riskJSON    []byte
		wantStarted bool
	}{
		{
			name:        "drop_info_severity",
			dropInfo:    true,
			riskJSON:    []byte(`{"severity":"info","title":"x","program_name":"demo","risk_type":"demo","code_source_url":"a.java","code_range":"1:1"}`),
			wantStarted: false,
		},
		{
			name:        "high_severity_emitted",
			dropInfo:    false,
			riskJSON:    []byte(`{"severity":"high","title":"Possible SQL Injection","program_name":"demo","risk_type":"sql","code_source_url":"src/A.java","code_range":"10:20"}`),
			wantStarted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &StreamEmitter{enabled: true, dropInfo: tt.dropInfo}
			taskId := "task-risk-" + tt.name
			err := e.EmitSSARisk(taskId, "rt-1", "sub-1", &spec.SSAStreamRiskEvent{RiskJSON: tt.riskJSON})
			if err != nil {
				t.Fatalf("emit risk failed: %v", err)
			}
			v, ok := e.perTask.Load(taskId)
			if !tt.wantStarted {
				if ok {
					t.Fatal("should not create task state when risk is dropped")
				}
				return
			}
			if !ok {
				t.Fatal("task state should exist after emitting risk")
			}
			state := v.(*taskStreamState)
			state.mu.Lock()
			started := state.started
			state.mu.Unlock()
			if !started {
				t.Fatal("task_start should be auto-emitted")
			}
		})
	}
}

func TestCalcFallbackRiskHash(t *testing.T) {
	h1 := calcFallbackRiskHashFromFields("t", "u", "1:1", "p", "rt")
	h2 := calcFallbackRiskHashFromFields("t", "u", "1:1", "p", "rt")
	if h1 == "" || h1 != h2 {
		t.Fatalf("fallback hash should be deterministic and non-empty: h1=%q h2=%q", h1, h2)
	}
	h3 := calcFallbackRiskHashFromFields("t2", "u", "1:1", "p", "rt")
	if h1 == h3 {
		t.Fatal("different inputs should produce different hashes")
	}
}
