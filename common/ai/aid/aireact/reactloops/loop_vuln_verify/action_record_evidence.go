package loop_vuln_verify

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// evidenceEntry holds one piece of verification evidence collected during the loop.
type evidenceEntry struct {
	Seq          int    `json:"seq"`
	Type         string `json:"type"`
	Significance string `json:"significance"`
	Observation  string `json:"observation"`
	RawData      string `json:"raw_data,omitempty"`
	Timestamp    string `json:"timestamp"`
}

func buildRecordEvidenceAction(invoker aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"record_evidence",
		"记录验证步骤中观察到的一条证据。在每次针对目标发出的验证工具调用（HTTP 请求等）返回结果后立即调用。"+
			"SSA 信息收集工具（ssa-risk、ssa-read-file、ssa-grep）的结果不需要通过本动作记录。",
		[]aitool.ToolOption{
			aitool.WithStringParam("evidence_type",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description(
					"证据类型：http_response | error_message | behavioral_difference | "+
						"data_leak | code_execution | timing_difference | other"),
			),
			aitool.WithStringParam("observation",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description(
					"描述观察到的内容，要具体——包括状态码、报错文本片段、使用的 payload 或响应差异。"),
			),
			aitool.WithStringParam("significance",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("证据重要程度：high | medium | low"),
			),
			aitool.WithStringParam("raw_data",
				aitool.WithParam_Description(
					"可选：原始 HTTP 响应、命令输出或其他可观测数据的简短摘录（最多约 500 字符）"),
			),
		},
		[]*reactloops.LoopStreamField{
			{FieldName: "observation", AINodeId: "re-act-loop-thought", ContentType: aicommon.TypeTextMarkdown},
		},
		verifyRecordEvidence,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			handleRecordEvidence(loop, action, op, invoker)
		},
	)
}

func verifyRecordEvidence(_ *reactloops.ReActLoop, action *aicommon.Action) error {
	if action.GetString("evidence_type") == "" {
		return utils.Error("evidence_type is required")
	}
	if action.GetString("observation") == "" {
		return utils.Error("observation is required")
	}
	sig := action.GetString("significance")
	switch sig {
	case "high", "medium", "low":
		// valid
	default:
		return fmt.Errorf("significance must be high | medium | low, got %q", sig)
	}
	return nil
}

func handleRecordEvidence(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator, invoker aicommon.AIInvokeRuntime) {
	countStr := loop.Get(keyEvidenceCount)
	count, _ := strconv.Atoi(countStr)
	count++

	entry := evidenceEntry{
		Seq:          count,
		Type:         action.GetString("evidence_type"),
		Significance: action.GetString("significance"),
		Observation:  action.GetString("observation"),
		RawData:      action.GetString("raw_data"),
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	}

	// Persist to loop state.
	loop.Set(keyEvidenceCount, strconv.Itoa(count))

	entryJSON, _ := json.Marshal(entry)

	// Append to the collected evidence list.
	collected := loop.Get(keyEvidenceJSON)
	var entries []evidenceEntry
	if collected != "" && collected != "[]" {
		_ = json.Unmarshal([]byte(collected), &entries)
	}
	entries = append(entries, entry)
	if newBs, err := json.Marshal(entries); err == nil {
		loop.Set(keyEvidenceJSON, string(newBs))
	}

	// Keep the most recent evidence visible in ReactiveData.
	loop.Set(keyRecentEvidence, fmt.Sprintf("[%s / %s] %s", entry.Type, entry.Significance, entry.Observation))

	invoker.AddToTimeline("record_evidence", fmt.Sprintf(
		"evidence #%d type=%s significance=%s: %s",
		count, entry.Type, entry.Significance,
		utils.ShrinkTextBlock(entry.Observation, 120)))

	feedback := fmt.Sprintf(
		"证据 #%d 已记录（类型=%s，重要程度=%s）。\n"+
			"观察内容：%s\n"+
			"当前已收集证据共 %d 条。",
		count, entry.Type, entry.Significance, entry.Observation, count)

	if entry.Significance == "high" {
		feedback += "\n发现高重要程度证据。请判断是否已足够确认漏洞，若是则立即调用 directly_answer(CONFIRMED)。"
	}

	_ = entryJSON
	op.Feedback(feedback)
	op.Continue()
}
