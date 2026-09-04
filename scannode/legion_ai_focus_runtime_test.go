package scannode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/aiengine"
	"github.com/yaklang/yaklang/common/schema"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

func newTestAttachmentFocusRuntime(t *testing.T) (*legionServerFocusRuntime, *legionAIFocusResultSink, *recordingAIFocusRiskPublisher) {
	t.Helper()
	publisher := &recordingAIFocusRiskPublisher{}
	sink, err := newLegionAIFocusResultSink(publisher, "bind-attachment", testAttachmentTaskResultContext())
	if err != nil {
		t.Fatal(err)
	}
	value, err := newLegionServerFocusRuntime(context.Background(), testAttachmentTaskTarget, sink)
	if err != nil {
		t.Fatal(err)
	}
	runtime := value.(*legionServerFocusRuntime)
	runtime.authorizedFocusReleaseID = testAttachmentTaskFocusRelease().ReleaseId
	return runtime, sink.(*legionAIFocusResultSink), publisher
}

func testAttachmentTaskExecutionContract(t *testing.T) *legionFocusExecutionContract {
	t.Helper()
	contract, err := parseLegionFocusExecutionContract(testAttachmentTaskContractJSON)
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func TestLegionAttachmentFocusRuntimeStageAndReport(t *testing.T) {
	t.Run("stage", func(t *testing.T) {
		runtime, _, _ := newTestAttachmentFocusRuntime(t)
		var emitted map[string]any
		runtime.emitEvent = func(kind string, payload []byte) {
			if kind != "task.stage" {
				t.Errorf("unexpected event kind: %s", kind)
			}
			if err := json.Unmarshal(payload, &emitted); err != nil {
				t.Error(err)
			}
		}
		if err := runtime.activateFocusTurn(runtime.authorizedFocusReleaseID, testAttachmentTaskExecutionContract(t)); err != nil {
			t.Fatal(err)
		}
		defer runtime.deactivateFocusTurn(runtime.authorizedFocusReleaseID)
		result, err := runtime.Execute(serverFocusCapabilityTaskStage, map[string]any{
			"phase": "anomaly_detection", "status": "progress", "progress": 0.5,
			"resource_id": "forged", "workspace_id": "forged",
		})
		if err != nil {
			t.Fatalf("publish attachment stage: %v", err)
		}
		for _, payload := range []map[string]any{result, emitted} {
			if payload["resource_id"] != testLegionCodeWorkspaceID || payload["phase"] != "anomaly_detection" || payload["progress"] != 0.5 {
				t.Fatalf("unexpected attachment stage: %#v", payload)
			}
			if _, exists := payload["workspace_id"]; exists {
				t.Fatal("attachment stage fabricated a source workspace")
			}
		}
		if _, err := runtime.Execute(serverFocusCapabilityTaskStage, map[string]any{"phase": "undeclared", "status": "started"}); err == nil {
			t.Fatal("undeclared stage accepted")
		}
	})
	t.Run("required_report", func(t *testing.T) {
		runtime, sink, publisher := newTestAttachmentFocusRuntime(t)
		if err := runtime.activateFocusTurn(runtime.authorizedFocusReleaseID, testAttachmentTaskExecutionContract(t)); err != nil {
			t.Fatal(err)
		}
		defer runtime.deactivateFocusTurn(runtime.authorizedFocusReleaseID)
		if err := sink.Succeed(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "required results") {
			t.Errorf("completion must require immutable attachment report: %v", err)
		}
		receipt, err := runtime.Execute(serverFocusCapabilitySubmitReportV1, map[string]any{
			"markdown":           "# Synthetic log analysis\nOne retry.",
			"structured_summary": map[string]any{"retry_count": 1},
			"resource_id":        "forged", "workspace_id": "forged", "source_sha256": "forged",
		})
		if err != nil {
			t.Fatalf("publish attachment report: %v", err)
		}
		if receipt["result_id"] == "" || len(publisher.reports) != 1 || publisher.reportKinds[0] != "ai_log_analysis_v1" {
			t.Fatalf("report was not published through the bound sink: receipt=%#v kinds=%v", receipt, publisher.reportKinds)
		}
		var report map[string]any
		if err := json.Unmarshal(publisher.reports[0], &report); err != nil {
			t.Fatal(err)
		}
		if report["resource_id"] != testLegionCodeWorkspaceID || report["title"] == "代码安全审计报告" {
			t.Fatalf("report lost attachment identity: %#v", report)
		}
		for _, key := range []string{"workspace_id", "source_sha256", "locked_revision", "file", "evidence"} {
			if _, exists := report[key]; exists {
				t.Fatalf("attachment report fabricated source evidence: %s", key)
			}
		}
		if err := sink.Succeed(context.Background(), nil); err != nil {
			t.Fatalf("complete reported attachment task: %v", err)
		}
		if publisher.succeeded != 1 {
			t.Fatalf("success not published: %d", publisher.succeeded)
		}
	})
}

func TestLegionAttachmentFocusRuntimeLifecycleGate(t *testing.T) {
	runtime, _, publisher := newTestAttachmentFocusRuntime(t)
	emitted := 0
	runtime.emitEvent = func(string, []byte) { emitted++ }
	checkDormant := func() {
		t.Helper()
		for _, capability := range []string{serverFocusCapabilityTaskStage, serverFocusCapabilitySubmitReportV1} {
			if _, err := runtime.Execute(capability, map[string]any{"phase": "anomaly_detection", "status": "started"}); err == nil || !strings.Contains(err.Error(), "authorized Focus Turn") {
				t.Errorf("%s must be dormant outside its Focus Turn: %v", capability, err)
			}
		}
	}
	checkDormant()
	contract := testAttachmentTaskExecutionContract(t)
	if err := runtime.activateFocusTurn("wrong-release", contract); err == nil {
		t.Error("mismatched release activated attachment capabilities")
	}
	if err := runtime.activateFocusTurn(runtime.authorizedFocusReleaseID); err == nil {
		t.Error("attachment turn activated without immutable contract")
	}
	if err := runtime.activateFocusTurn(runtime.authorizedFocusReleaseID, contract); err != nil {
		t.Fatal(err)
	}
	if err := runtime.activateFocusTurn(runtime.authorizedFocusReleaseID, contract); err == nil {
		t.Error("overlapping attachment turn activated")
	}
	runtime.deactivateFocusTurn(runtime.authorizedFocusReleaseID)
	checkDormant()
	if emitted != 0 || len(publisher.reports) != 0 {
		t.Fatal("dormant runtime published attachment results")
	}
}

func TestLegionAttachmentFocusRuntimeRejectsUnsafeCapabilities(t *testing.T) {
	for _, capability := range []string{
		serverFocusCapabilityHTTPRequest, serverFocusCapabilityExtractReferences,
		serverFocusCapabilitySubmitAsset, serverFocusCapabilitySubmitRisk,
		serverFocusCapabilitySourceWorkspaceInfo, serverFocusCapabilitySourceList,
		serverFocusCapabilitySourceRead, serverFocusCapabilitySourceSearch,
		serverFocusCapabilitySubmitFindingV1, serverFocusCapabilitySubmitRiskJudgementV1,
	} {
		t.Run(capability, func(t *testing.T) {
			runtime, _, publisher := newTestAttachmentFocusRuntime(t)
			contract := testAttachmentTaskExecutionContract(t)
			contract.Capabilities = append(contract.Capabilities, capability)
			contract.capabilitySet[capability] = struct{}{}
			if err := runtime.activateFocusTurn(runtime.authorizedFocusReleaseID, contract); err == nil {
				t.Fatal("attachment task activated a contract with capabilities outside stage/report")
			}
			if len(publisher.assets) != 0 || len(publisher.risks) != 0 || len(publisher.reports) != 0 {
				t.Fatal("unsafe contract published results")
			}
		})
	}
	t.Run("undeclared_stage", func(t *testing.T) {
		runtime, _, _ := newTestAttachmentFocusRuntime(t)
		contract := testAttachmentTaskExecutionContract(t)
		contract.Capabilities = []string{serverFocusCapabilitySubmitReportV1}
		delete(contract.capabilitySet, serverFocusCapabilityTaskStage)
		if err := runtime.activateFocusTurn(runtime.authorizedFocusReleaseID, contract); err != nil {
			t.Fatal(err)
		}
		defer runtime.deactivateFocusTurn(runtime.authorizedFocusReleaseID)
		if _, err := runtime.Execute(serverFocusCapabilityTaskStage, nil); err == nil || !strings.Contains(err.Error(), "immutable Focus execution contract") {
			t.Fatalf("undeclared stage capability was not fenced: %v", err)
		}
	})
}

func TestLegionAttachmentFocusStatelessTurnLifecycle(t *testing.T) {
	for _, terminal := range []string{"complete", "construction_error", "cancel", "close"} {
		t.Run(terminal, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			binding := testAttachmentTaskBinding(t, testAttachmentTaskContent)
			binding.ProviderPolicySnapshotJSON = []byte(`{"disable_tool_use":false,"enable_system_file_system_operator":true,"enable_ai_search_tool":true,"enable_ai_search_internet":true,"enabled_capabilities":[{"name":"forbidden","type":"tool"}]}`)
			runtime := binding.LegionResultRuntime.(*legionServerFocusRuntime)
			stageCount := 0
			runtime.emitEvent = func(string, []byte) { stageCount++ }
			emitter := recordingSingleRunEmitter{done: make(chan singleRunCompletion, 1)}
			handle, err := newStatelessAIEngineRuntimeDriver().Bind(ctx, binding, emitter)
			if err != nil {
				t.Fatalf("bind attachment stateless driver: %v", err)
			}
			defer handle.Close("test cleanup")
			state := handle.(*statelessAIEngineRuntimeHandle)
			engine := newFakeStatelessTurnEngine()
			state.newEngine = func(options ...aiengine.AIEngineConfigOption) (statelessTurnEngine, error) {
				config := aiengine.NewAIEngineConfig(options...)
				if config.Focus != testAttachmentTaskFocusRelease().RuntimeName || !config.DisableToolUse || len(config.ExtraMCPServers) != 0 {
					t.Error("attachment engine did not retain the pinned Focus and closed tool surface")
				}
				common := aicommon.NewConfig(ctx, config.ExtOptions...)
				if !common.DisallowMCPServers || !common.DisableWebSearch || config.EnableAISearchTool || len(common.GetEnabledCapabilities()) != 0 {
					t.Error("provider/context options expanded attachment tool permissions")
				}
				if _, err := runtime.Execute(serverFocusCapabilityTaskStage, map[string]any{"phase": "log_collect", "status": "started"}); err != nil {
					t.Errorf("Focus capabilities were not activated before engine construction: %v", err)
				}
				if terminal == "construction_error" {
					return nil, fmt.Errorf("synthetic engine construction failure")
				}
				return engine, nil
			}
			if _, err := runtime.Execute(serverFocusCapabilityTaskStage, nil); err == nil {
				t.Fatal("attachment capability available before the first Turn")
			}
			input := aiSessionInput{Ref: aiSessionCommandRef{CommandID: "attachment-turn"}, InputType: "message", ContextPackage: &aiv1.ContextPackage{
				UserInput: "Analyze the pinned logs", FocusRelease: testAttachmentTaskFocusRelease(),
				RuntimeOptionSnapshotJson: []byte(`{"session_mcp_servers":[{"name":"forbidden","url":"https://mcp.invalid/sse"}],"disable_tool_use":false,"enable_ai_search_internet":true}`),
			}}
			err = state.SendInput(context.Background(), input)
			if terminal == "construction_error" {
				if err == nil || !strings.Contains(err.Error(), "synthetic engine") {
					t.Fatalf("unexpected construction result: %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("first attachment Turn rejected: %v", err)
				}
				select {
				case <-engine.started:
				case <-time.After(time.Second):
					t.Fatal("attachment Turn did not start")
				}
				switch terminal {
				case "complete":
					if _, err := runtime.Execute(serverFocusCapabilitySubmitReportV1, map[string]any{"markdown": "# Log report", "structured_summary": map[string]any{"status": "complete"}}); err != nil {
						t.Fatal(err)
					}
					close(engine.release)
					select {
					case <-emitter.done:
					case <-time.After(time.Second):
						t.Fatal("attachment Turn did not finish")
					}
				case "cancel":
					handle.Cancel("test cancel")
				case "close":
					handle.Close("test close")
				}
			}
			if stageCount != 1 {
				t.Fatalf("expected one stage inside the Turn, got %d", stageCount)
			}
			for _, capability := range []string{serverFocusCapabilityTaskStage, serverFocusCapabilitySubmitReportV1} {
				if _, err := runtime.Execute(capability, nil); err == nil || !strings.Contains(err.Error(), "authorized Focus Turn") {
					t.Fatalf("%s remained usable after %s: %v", capability, terminal, err)
				}
			}
		})
	}
}

type recordingServerFocusSink struct {
	assets []aiFocusAssetResult
	risks  []*schema.Risk
}

func (s *recordingServerFocusSink) SubmitAsset(
	_ context.Context,
	asset aiFocusAssetResult,
) (aiFocusResultReceipt, error) {
	s.assets = append(s.assets, asset)
	return aiFocusResultReceipt{ResultID: fmt.Sprintf("asset-%d", len(s.assets))}, nil
}

func (s *recordingServerFocusSink) SubmitRisk(
	_ context.Context,
	risk *schema.Risk,
) (aiFocusResultReceipt, error) {
	s.risks = append(s.risks, risk)
	return aiFocusResultReceipt{ResultID: fmt.Sprintf("risk-%d", len(s.risks))}, nil
}

func TestLegionServerFocusRuntimeExecutesBoundedCapabilities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/start":
			response.Header().Set("Content-Type", "text/html")
			_, _ = response.Write([]byte(`<a href="/api/users">users</a><script src="/app.js"></script><a href="https://outside.test/api">outside</a>`))
		case "/app.js":
			response.Header().Set("Content-Type", "application/javascript")
			_, _ = response.Write([]byte(`const endpoint = "/api/orders";`))
		default:
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"ok":true}`))
		}
	}))
	defer server.Close()

	sink := &recordingServerFocusSink{}
	runtimeValue, err := newLegionServerFocusRuntime(context.Background(), server.URL+"/start", sink)
	if err != nil {
		t.Fatalf("new focus runtime: %v", err)
	}
	runtime := runtimeValue.(*legionServerFocusRuntime)

	seed, err := runtime.Execute(serverFocusCapabilityHTTPRequest, map[string]any{
		"headers": map[string]any{"Accept-Language": "en-US"},
	})
	if err != nil {
		t.Fatalf("request seed: %v", err)
	}
	if seed["status_code"] != http.StatusOK || !strings.Contains(seed["body"].(string), "/api/users") {
		t.Fatalf("unexpected seed response: %#v", seed)
	}

	refs, err := runtime.Execute(serverFocusCapabilityExtractReferences, map[string]any{
		"base_url": seed["url"],
		"document": seed["body"],
	})
	if err != nil {
		t.Fatalf("extract references: %v", err)
	}
	pages := refs["pages"].([]string)
	scripts := refs["scripts"].([]string)
	if len(pages) != 1 || pages[0] != server.URL+"/api/users" {
		t.Fatalf("unexpected pages: %#v", pages)
	}
	if len(scripts) != 1 || scripts[0] != server.URL+"/app.js" {
		t.Fatalf("unexpected scripts: %#v", scripts)
	}

	receipt, err := runtime.Execute(serverFocusCapabilitySubmitAsset, map[string]any{
		"kind":         "http_endpoint",
		"title":        "Discovered endpoint",
		"target":       server.URL + "/api/users",
		"identity_key": "http_endpoint:" + server.URL + "/api/users",
		"payload":      map[string]any{"status": "responded"},
	})
	if err != nil {
		t.Fatalf("submit asset: %v", err)
	}
	if receipt["result_id"] != "asset-1" || len(sink.assets) != 1 {
		t.Fatalf("unexpected asset receipt: %#v", receipt)
	}

	riskReceipt, err := runtime.Execute(serverFocusCapabilitySubmitRisk, map[string]any{
		"verified":    true,
		"target":      server.URL + "/start?q=payload",
		"title":       "Reflected XSS",
		"risk_type":   "xss",
		"severity":    "high",
		"description": "q is reflected into HTML without encoding",
		"solution":    "apply contextual output encoding",
		"details":     `{"summary":"confirmed reflection"}`,
	})
	if err != nil {
		t.Fatalf("submit structured risk: %v", err)
	}
	if riskReceipt["result_id"] != "risk-1" || len(sink.risks) != 1 {
		t.Fatalf("unexpected risk receipt: %#v", riskReceipt)
	}
	if sink.risks[0].Description == "" || sink.risks[0].Solution == "" {
		t.Fatalf("structured risk fields were dropped: %#v", sink.risks[0])
	}
}

func TestLegionServerFocusRuntimeRejectsBoundaryViolations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte("ok"))
	}))
	defer server.Close()

	sink := &recordingServerFocusSink{}
	runtimeValue, err := newLegionServerFocusRuntime(context.Background(), server.URL+"/", sink)
	if err != nil {
		t.Fatalf("new focus runtime: %v", err)
	}
	runtime := runtimeValue.(*legionServerFocusRuntime)

	if _, err := runtime.Execute(serverFocusCapabilityHTTPRequest, map[string]any{"url": "https://outside.test/"}); err == nil || !strings.Contains(err.Error(), "outside the authorized origin") {
		t.Fatalf("expected same-origin rejection, got %v", err)
	}
	if _, err := runtime.Execute(serverFocusCapabilityHTTPRequest, map[string]any{"headers": map[string]any{"Authorization": "secret"}}); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("expected header rejection, got %v", err)
	}
	if _, err := runtime.Execute(serverFocusCapabilitySubmitRisk, map[string]any{
		"verified":          false,
		"target":            server.URL,
		"title":             "unverified",
		"risk_type":         "test",
		"request_evidence":  "GET / HTTP/1.1",
		"response_evidence": "HTTP/1.1 200 OK",
	}); err == nil || !strings.Contains(err.Error(), "verified evidence") {
		t.Fatalf("expected evidence gate, got %v", err)
	}
	if _, err := runtime.Execute(serverFocusCapabilitySubmitRisk, map[string]any{
		"verified":  true,
		"target":    server.URL,
		"title":     "missing evidence",
		"risk_type": "test",
	}); err == nil || !strings.Contains(err.Error(), "structured evidence") {
		t.Fatalf("expected structured evidence rejection, got %v", err)
	}

	runtime.mu.Lock()
	runtime.requestCount = maxServerFocusRequests
	runtime.mu.Unlock()
	if _, err := runtime.Execute(serverFocusCapabilityHTTPRequest, nil); err == nil || !strings.Contains(err.Error(), "budget exhausted") {
		t.Fatalf("expected budget rejection, got %v", err)
	}
}
