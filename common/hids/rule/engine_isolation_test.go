//go:build hids

package rule

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/hids/model"
)

func isolationRule(condition, action string) model.CustomRule {
	return model.CustomRule{RuleID: "isolated", Enabled: true, MatchEventType: model.EventTypeProcessExec, Severity: "low", Condition: condition, Action: action}
}

func TestEngineDoesNotMergePreviousEventFields(t *testing.T) {
	engine, err := NewEngine(model.DesiredSpec{CustomRules: []model.CustomRule{isolationRule(`matched = false; for key, value = range data { if key == "history_marker" && value == "present" { matched = true } }; matched`, "")}})
	if err != nil {
		t.Fatal(err)
	}
	if alerts := engine.Evaluate(model.Event{Type: model.EventTypeProcessExec, Data: map[string]any{"history_marker": "present"}}); len(alerts) != 1 {
		t.Fatalf("expected first event match, got %d", len(alerts))
	}
	if alerts := engine.Evaluate(model.Event{Type: model.EventTypeProcessExec, Data: map[string]any{}}); len(alerts) != 0 {
		t.Fatalf("previous event leaked into current data: %#v", alerts)
	}
}

func TestEngineConditionStateRemainsAvailableToAction(t *testing.T) {
	engine, err := NewEngine(model.DesiredSpec{CustomRules: []model.CustomRule{isolationRule(`condition_image = process.image; true`, `{"detail":{"captured_image":condition_image}}`)}})
	if err != nil {
		t.Fatal(err)
	}
	for _, image := range []string{"/bin/first", "/bin/second"} {
		alerts := engine.Evaluate(model.Event{Type: model.EventTypeProcessExec, Process: &model.Process{Image: image}})
		if len(alerts) != 1 || alerts[0].Detail["captured_image"] != image || alerts[0].Detail["action_error"] != nil {
			t.Fatalf("condition/action state changed: %#v", alerts)
		}
	}
}

func TestEngineStaticHelpersCannotLeakActionMutation(t *testing.T) {
	engine, err := NewEngine(model.DesiredSpec{CustomRules: []model.CustomRule{isolationRule(`process.image == "/trigger" || str.Contains("abc", "missing")`, `str.Contains = (left, right) => true; {"title":"helper mutation"}`)}})
	if err != nil {
		t.Fatal(err)
	}
	if alerts := engine.Evaluate(model.Event{Type: model.EventTypeProcessExec, Process: &model.Process{Image: "/trigger"}}); len(alerts) != 1 {
		t.Fatalf("expected first match, got %#v", alerts)
	}
	if alerts := engine.Evaluate(model.Event{Type: model.EventTypeProcessExec}); len(alerts) != 0 {
		t.Fatalf("action mutated static helpers across events: %#v", alerts)
	}
}

func TestEngineIsolationKeepsSandboxWhitelist(t *testing.T) {
	for _, condition := range []string{`os.Getpid() > 0`, `exec.Command("true") != nil`, `http.Get("http://127.0.0.1") != nil`} {
		if _, err := NewEngine(model.DesiredSpec{CustomRules: []model.CustomRule{isolationRule(condition, "")}}); err == nil {
			t.Fatalf("forbidden library became accessible: %s", condition)
		}
	}
}

type retainedRuleEvent struct{ payload [8192]byte }

func TestEngineContinuousEventsReleaseOldPayloads(t *testing.T) {
	engine, err := NewEngine(model.DesiredSpec{CustomRules: []model.CustomRule{isolationRule(`process.image == "/never-match"`, "")}})
	if err != nil {
		t.Fatal(err)
	}
	const events = 2000
	var released atomic.Int64
	for index := 0; index < events; index++ {
		payload := new(retainedRuleEvent)
		runtime.SetFinalizer(payload, func(*retainedRuleEvent) { released.Add(1) })
		if alerts := engine.Evaluate(model.Event{Type: model.EventTypeProcessExec, Process: &model.Process{Image: "/bin/worker"}, Data: map[string]any{"old_payload": payload}}); len(alerts) != 0 {
			t.Fatal("unexpected match")
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for released.Load() < events*9/10 && time.Now().Before(deadline) {
		runtime.GC()
		runtime.Gosched()
		time.Sleep(5 * time.Millisecond)
	}
	if released.Load() < events*9/10 {
		t.Fatalf("live engine retained historical event payloads: released=%d/%d", released.Load(), events)
	}
	runtime.KeepAlive(engine)
}

func TestEngineCustomIndexKeepsRuleOrder(t *testing.T) {
	var rules []model.CustomRule
	for index, eventType := range []string{model.EventTypeProcessExec, model.EventTypeFileChange, model.EventTypeProcessExec} {
		rule := isolationRule("true", "")
		rule.RuleID = fmt.Sprintf("rule-%d", index)
		rule.MatchEventType = eventType
		rules = append(rules, rule)
	}
	engine, err := NewEngine(model.DesiredSpec{CustomRules: rules})
	if err != nil {
		t.Fatal(err)
	}
	alerts := engine.Evaluate(model.Event{Type: model.EventTypeProcessExec})
	if len(alerts) != 2 || alerts[0].RuleID != "rule-0" || alerts[1].RuleID != "rule-2" {
		t.Fatalf("event index changed rule selection/order: %#v", alerts)
	}
	if !engine.HasRulesForEventType(model.EventTypeFileChange) || engine.HasRulesForEventType(model.EventTypeNetworkConnect) {
		t.Fatal("event index returned incorrect capability")
	}
}

func TestEngineConcurrentEventsHaveIndependentSessions(t *testing.T) {
	engine, err := NewEngine(model.DesiredSpec{CustomRules: []model.CustomRule{isolationRule(`condition_image = process.image; true`, `{"detail":{"captured_image":condition_image}}`)}})
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	failures := make(chan string, 8)
	for worker := 0; worker < 8; worker++ {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			image := fmt.Sprintf("/bin/worker-%d", worker)
			for event := 0; event < 50; event++ {
				alerts := engine.Evaluate(model.Event{Type: model.EventTypeProcessExec, Process: &model.Process{Image: image}})
				if len(alerts) != 1 || alerts[0].Detail["captured_image"] != image {
					failures <- image
					return
				}
			}
		}(worker)
	}
	workers.Wait()
	close(failures)
	for image := range failures {
		t.Errorf("condition/action state crossed an event boundary for %s", image)
	}
}

func BenchmarkEngineContinuousEventHistory(b *testing.B) {
	for _, history := range []int{0, 1000, 10000} {
		b.Run(fmt.Sprintf("history_%d", history), func(b *testing.B) {
			engine, err := NewEngine(model.DesiredSpec{CustomRules: []model.CustomRule{isolationRule(`process.image == "/never-match"`, "")}})
			if err != nil {
				b.Fatal(err)
			}
			event := model.Event{Type: model.EventTypeProcessExec, Process: &model.Process{Image: "/bin/worker"}}
			for index := 0; index < history; index++ {
				engine.Evaluate(event)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				engine.Evaluate(event)
			}
		})
	}
}
