package test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestCoordinator_ReportGenerationAfterPlanExecution(t *testing.T) {
	inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
	outputChan := make(chan *schema.AiOutputEvent, 100)

	var aiCallCount int64
	var mu sync.Mutex

	ins, err := aid.NewCoordinator(
		"generate a report after plan execution test",
		aicommon.WithEventInputChanx(inputChan),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			select {
			case outputChan <- event:
			default:
			}
		}),
		aid.WithPlanMocker(func(coordinator *aid.Coordinator) *aid.PlanResponse {
			return &aid.PlanResponse{
				RootTask: &aid.AiTask{
					Name: "test-report-root",
					Goal: "test report generation after plan execution",
					Subtasks: []*aid.AiTask{
						{
							Name: "simple-subtask",
							Goal: "a simple subtask that completes immediately",
						},
					},
				},
			}
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			callIdx := atomic.AddInt64(&aiCallCount, 1)
			prompt := request.GetPrompt()
			rsp := config.NewAIResponse()

			mu.Lock()
			defer mu.Unlock()

			switch {
			case utils.MatchAllOfSubString(prompt, "summary", "status_summary"):
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "summary", "status_summary": "task completed successfully", "task_short_summary": "subtask finished", "task_long_summary": "the simple subtask has been completed successfully with all objectives met"}`))
				rsp.Close()
				return rsp, nil

			case utils.MatchAllOfSubString(prompt, "analyze-report-intent", "is_modify"):
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "analyze-report-intent", "is_modify": false, "target_file": "", "analysis_reason": "creating new report"}`))
				rsp.Close()
				return rsp, nil

			case strings.Contains(prompt, "GEN_REPORT"):
				nonce := extractNonceFromPrompt(prompt, "GEN_REPORT")
				if nonce == "" {
					nonce = "TESTNONCE"
				}
				reportContent := fmt.Sprintf(`{"@action": "write_section", "human_readable_thought": "writing the execution report"}

<|GEN_REPORT_%s|>
# Task Execution Report

## Overview

This report summarizes the execution results of the plan.

## Task Details

### Task: simple-subtask

- Status: Completed
- Goal: a simple subtask that completes immediately
- Result: The subtask was completed successfully.

## Conclusion

All planned tasks have been executed successfully.
<|GEN_REPORT_END_%s|>
`, nonce, nonce)
				rsp.EmitOutputStream(strings.NewReader(reportContent))
				rsp.Close()
				return rsp, nil

			default:
				_ = callIdx
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "finish", "human_readable_thought": "task completed"}`))
				rsp.Close()
				return rsp, nil
			}
		}),
	)
	require.NoError(t, err)

	coordinatorDone := make(chan error, 1)
	go func() {
		coordinatorDone <- ins.Run()
	}()

	planReviewed := false
	taskReviewed := false
	reportFileEmitted := false
	var reportFilePath string

	timeout := time.After(120 * time.Second)

LOOP:
	for {
		select {
		case result := <-outputChan:
			eventStr := result.String()

			if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE && !planReviewed {
				planReviewed = true
				t.Logf("plan review received, sending continue")
				inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
				continue
			}

			if result.Type == schema.EVENT_TYPE_TASK_REVIEW_REQUIRE && !taskReviewed {
				taskReviewed = true
				t.Logf("task review received, sending continue")
				inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
				continue
			}

			if result.Type == schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME {
				contentStr := string(result.Content)
				if strings.Contains(contentStr, ".md") {
					reportFileEmitted = true
					var pinData struct {
						Path string `json:"path"`
					}
					if err := json.Unmarshal(result.Content, &pinData); err == nil && pinData.Path != "" {
						reportFilePath = pinData.Path
					} else {
						reportFilePath = strings.TrimSpace(contentStr)
					}
					t.Logf("report file emitted: %s", reportFilePath)
				}
			}

			_ = eventStr

		case err := <-coordinatorDone:
			if err != nil {
				t.Logf("coordinator finished with error: %v", err)
			} else {
				t.Logf("coordinator finished successfully")
			}
			break LOOP

		case <-timeout:
			t.Fatal("test timeout: coordinator did not complete within 120 seconds")
		}
	}

	require.True(t, planReviewed, "plan review event should have been received and processed")
	require.True(t, reportFileEmitted, "report .md file should have been emitted via EVENT_TYPE_FILESYSTEM_PIN_FILENAME")

	if reportFilePath != "" {
		content, err := os.ReadFile(reportFilePath)
		if err == nil {
			require.NotEmpty(t, content, "report file should not be empty")
			t.Logf("report content length: %d bytes", len(content))
		} else {
			t.Logf("could not read report file (may have been cleaned up): %v", err)
		}
	}

	totalCalls := atomic.LoadInt64(&aiCallCount)
	t.Logf("total AI callback invocations: %d", totalCalls)
	require.Greater(t, totalCalls, int64(1), "AI callback should have been invoked multiple times (plan tasks + report generation)")
}

func TestCoordinator_DefaultReportGenerationEnabled(t *testing.T) {
	t.Run("ConfigDefaultValue", func(t *testing.T) {
		config := aicommon.NewConfig(context.Background())
		require.True(t, config.GenerateReport,
			"GenerateReport should default to true in a fresh Config")
	})

	t.Run("CoordinatorDefaultBehavior", func(t *testing.T) {
		inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
		outputChan := make(chan *schema.AiOutputEvent, 100)

		var mu sync.Mutex
		reportLoopEntered := false

		ins, err := aid.NewCoordinator(
			"verify default report generation is enabled",
			aicommon.WithEventInputChanx(inputChan),
			aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
				select {
				case outputChan <- event:
				default:
				}
			}),
			aid.WithPlanMocker(func(coordinator *aid.Coordinator) *aid.PlanResponse {
				return &aid.PlanResponse{
					RootTask: &aid.AiTask{
						Name: "default-report-test-root",
						Goal: "verify report generation triggers by default",
						Subtasks: []*aid.AiTask{
							{
								Name: "trivial-task",
								Goal: "a trivial task for testing default report flow",
							},
						},
					},
				}
			}),
			aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				prompt := request.GetPrompt()
				rsp := config.NewAIResponse()

				mu.Lock()
				defer mu.Unlock()

				switch {
				case utils.MatchAllOfSubString(prompt, "summary", "status_summary"):
					rsp.EmitOutputStream(strings.NewReader(`{"@action": "summary", "status_summary": "done", "task_short_summary": "done", "task_long_summary": "done"}`))
					rsp.Close()
					return rsp, nil

				case strings.Contains(prompt, "GEN_REPORT"):
					reportLoopEntered = true
					nonce := extractNonceFromPrompt(prompt, "GEN_REPORT")
					if nonce == "" {
						nonce = "DEFNONCE"
					}
					content := fmt.Sprintf(`{"@action": "write_section", "human_readable_thought": "writing default report"}

<|GEN_REPORT_%s|>
# Default Report
Generated by default behavior.
<|GEN_REPORT_END_%s|>
`, nonce, nonce)
					rsp.EmitOutputStream(strings.NewReader(content))
					rsp.Close()
					return rsp, nil

				default:
					rsp.EmitOutputStream(strings.NewReader(`{"@action": "finish", "human_readable_thought": "done"}`))
					rsp.Close()
					return rsp, nil
				}
			}),
		)
		require.NoError(t, err)

		coordinatorDone := make(chan error, 1)
		go func() {
			coordinatorDone <- ins.Run()
		}()

		planReviewed := false
		reportFileEmitted := false
		timeout := time.After(120 * time.Second)

	LOOP:
		for {
			select {
			case result := <-outputChan:
				if result.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE && !planReviewed {
					planReviewed = true
					inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
					continue
				}
				if result.Type == schema.EVENT_TYPE_TASK_REVIEW_REQUIRE {
					inputChan.SafeFeed(ContinueSuggestionInputEvent(result.GetInteractiveId()))
					continue
				}
				if result.Type == schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME {
					contentStr := string(result.Content)
					if strings.Contains(contentStr, ".md") {
						reportFileEmitted = true
						t.Logf("default behavior: report file emitted")
					}
				}
			case err := <-coordinatorDone:
				if err != nil {
					t.Logf("coordinator finished with error: %v", err)
				}
				break LOOP
			case <-timeout:
				t.Fatal("test timeout")
			}
		}

		require.True(t, planReviewed, "plan review should have been triggered")

		mu.Lock()
		entered := reportLoopEntered
		mu.Unlock()
		require.True(t, entered,
			"report_generating loop should have been entered by default (without explicit WithGenerateReport(true))")
		require.True(t, reportFileEmitted,
			"report .md file should have been emitted by default behavior")
	})

	t.Run("ExplicitDisableSkipsReport", func(t *testing.T) {
		config := aicommon.NewConfig(context.Background(), aicommon.WithGenerateReport(false))
		require.False(t, config.GenerateReport,
			"GenerateReport should be false when explicitly disabled")
	})
}

func extractNonceFromPrompt(prompt, prefix string) string {
	marker := prefix + "_"
	idx := strings.Index(prompt, "<|"+marker)
	if idx < 0 {
		return ""
	}
	start := idx + 2 + len(marker)
	end := strings.Index(prompt[start:], "|>")
	if end < 0 {
		return ""
	}
	nonce := prompt[start : start+end]
	if strings.Contains(nonce, "_END") {
		return ""
	}
	if len(nonce) > 20 {
		return ""
	}
	return nonce
}
