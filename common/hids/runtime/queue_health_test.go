//go:build hids && linux

package runtime

import (
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/hids/model"
	"github.com/yaklang/yaklang/common/hids/rule"
)

func assertQueueHealth(t *testing.T, detail map[string]any, enqueued, dropped uint64, depth, capacity int) {
	t.Helper()
	if detail["enqueued"] != enqueued || detail["dropped"] != dropped || detail["queue_depth"] != depth || detail["capacity"] != capacity {
		t.Fatalf("incorrect queue admission/overflow counters: %#v", detail)
	}
}

func TestManagerQueueOverflowIsReportedAcrossRuntimeReplacement(t *testing.T) {
	t.Parallel()
	manager := NewManager()
	manager.alerts = make(chan model.Alert, 1)
	manager.observations = make(chan model.Event, 1)
	alerts := make(chan model.Alert, 3)
	observations := make(chan model.Event, 3)
	for index := 0; index < 3; index++ {
		alerts <- model.Alert{}
		observations <- model.Event{}
	}
	close(alerts)
	close(observations)
	manager.forwardAlerts(alerts)
	manager.forwardObservations(observations)
	for attempt := 0; attempt < 2; attempt++ {
		state := manager.State()
		delivery := state.Detail["delivery"].(map[string]any)
		assertQueueHealth(t, delivery["alerts"].(map[string]any), 1, 2, 1, 1)
		assertQueueHealth(t, delivery["observations"].(map[string]any), 1, 2, 1, 1)
		// Pipeline health belongs to each runtime, but manager drops must survive
		// spec changes so a reconfiguration cannot hide downstream pressure.
		manager.instance = &Instance{pipeline: newPipeline(nil)}
	}
}

func TestPipelineQueueOverflowDoesNotBlockRuleEvaluation(t *testing.T) {
	t.Parallel()
	engine, err := rule.NewEngine(model.DesiredSpec{CustomRules: []model.CustomRule{{
		RuleID: "queue-pressure", Enabled: true, MatchEventType: model.EventTypeFileChange,
		Condition: "true", Severity: "high",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	p := newPipeline(engine)
	p.alerts = make(chan model.Alert, 1)
	p.observations = make(chan model.Event, 1)
	events := make(chan model.Event, 3)
	for index := 0; index < 3; index++ {
		events <- model.Event{Type: model.EventTypeFileChange, File: &model.File{Path: "/tmp/queue-pressure"}}
	}
	close(events)
	p.Run(context.Background(), events)
	detail := (&Instance{pipeline: p}).runtimeStatusDetail()["pipeline"].(map[string]any)
	assertQueueHealth(t, detail["observations"].(map[string]any), 1, 2, 1, 1)
	assertQueueHealth(t, detail["alerts"].(map[string]any), 1, 2, 1, 1)
	if got := detail["observations"].(map[string]any)["snapshot_suppressed"]; got != uint64(0) {
		t.Fatalf("overflow was incorrectly counted as snapshot policy suppression: %v", got)
	}
}

func TestPipelineIntentionalSnapshotSuppressionIsSeparateFromOverflow(t *testing.T) {
	t.Parallel()
	p := newPipeline(nil)
	p.emitSnapshots = false
	p.observations = make(chan model.Event, 1)
	events := make(chan model.Event, 3)
	events <- model.Event{Type: model.EventTypeProcessExec}
	events <- model.Event{Type: model.EventTypeProcessExec}
	events <- model.Event{Type: model.EventTypeProcessState, Source: "inventory.process"}
	close(events)
	p.Run(context.Background(), events)
	detail := p.queueStatusDetail()["observations"].(map[string]any)
	assertQueueHealth(t, detail, 1, 0, 1, 1)
	if detail["snapshot_suppressed"] != uint64(2) {
		t.Fatalf("reporting policy suppression was not observable: %#v", detail)
	}
}
