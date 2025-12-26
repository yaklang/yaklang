package loop_httpflow_analyst

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// EvidenceItem represents a single piece of evidence
type EvidenceItem struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"` // observation/hypothesis/conclusion
	Claim       string   `json:"claim"`
	Evidence    string   `json:"evidence"`
	FlowIDs     []uint   `json:"flow_ids"`
	QueryRef    string   `json:"query_ref"`
	Confidence  string   `json:"confidence"` // high/medium/low
	Limitations []string `json:"limitations"`
}

// EvidencePack is the structured evidence container
type EvidencePack struct {
	Scope       string         `json:"scope"`
	Queries     []string       `json:"queries"`
	Items       []EvidenceItem `json:"items"`
	Provenance  string         `json:"provenance"`
	GeneratedAt string         `json:"generated_at"`
}

// aggregateEvidenceAction creates the action for building evidence pack from query results
var aggregateEvidenceAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"aggregate_evidence",
		`Aggregate Evidence - 从查询结果构建证据包

【功能说明】
将之前查询的结果聚合成结构化的证据包，为写作报告做准备。
每个证据必须分类为：
- observation（观察）：仅描述命中事实与统计
- hypothesis（假设）：可解释但明确待验证
- conclusion（结论）：有强证据链支持

【参数说明】
- evidence_type (必需): observation/hypothesis/conclusion
- claim (必需): 证据声明/结论文本
- supporting_evidence (必需): 支持该声明的证据描述
- flow_ids (可选): 相关的 HTTPFlow ID 列表（用于可复核）
- query_reference (可选): 引用的查询 ID
- confidence (必需): 置信度 high/medium/low
- limitations (可选): 该证据的局限性说明
- reason (必需): 为什么得出这个结论

【使用时机】
- 完成查询后，需要提取关键发现时
- 需要将统计数据转化为可写作的声明时
- 在写报告之前整理证据时`,
		[]aitool.ToolOption{
			aitool.WithStringParam("evidence_type",
				aitool.WithParam_Required(true),
				aitool.WithParam_Enum("observation", "hypothesis", "conclusion"),
				aitool.WithParam_Description("Type of evidence: observation/hypothesis/conclusion")),
			aitool.WithStringParam("claim",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("The claim or finding statement")),
			aitool.WithStringParam("supporting_evidence",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Evidence supporting this claim")),
			aitool.WithNumberArrayParam("flow_ids",
				aitool.WithParam_Description("Related HTTPFlow IDs for verification")),
			aitool.WithStringParam("query_reference",
				aitool.WithParam_Description("Reference to the query that produced this evidence")),
			aitool.WithStringParam("confidence",
				aitool.WithParam_Required(true),
				aitool.WithParam_Enum("high", "medium", "low"),
				aitool.WithParam_Description("Confidence level")),
			aitool.WithStringArrayParam("limitations",
				aitool.WithParam_Description("Limitations or caveats for this evidence")),
			aitool.WithStringParam("reason",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Reasoning for this evidence")),
		},
		[]*reactloops.LoopStreamField{
			{
				FieldName: "reason",
				AINodeId:  "evidence-reasoning",
			},
		},
		// Validator
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			evidenceType := action.GetString("evidence_type")
			claim := action.GetString("claim")
			confidence := action.GetString("confidence")

			if evidenceType == "" {
				return utils.Error("aggregate_evidence requires 'evidence_type' parameter")
			}
			if claim == "" {
				return utils.Error("aggregate_evidence requires 'claim' parameter")
			}
			if confidence == "" {
				return utils.Error("aggregate_evidence requires 'confidence' parameter")
			}

			// For conclusions, require high confidence and flow_ids
			if evidenceType == "conclusion" {
				params := action.GetParams()
				flowIDsRaw, ok := params["flow_ids"]
				if !ok || flowIDsRaw == nil {
					return utils.Error("conclusions require 'flow_ids' for verification")
				}
				if arr, ok := flowIDsRaw.([]interface{}); ok && len(arr) == 0 {
					return utils.Error("conclusions require 'flow_ids' for verification")
				}
			}

			return nil
		},
		// Handler
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			evidenceType := action.GetString("evidence_type")
			claim := action.GetString("claim")
			supportingEvidence := action.GetString("supporting_evidence")
			queryRef := action.GetString("query_reference")
			confidence := action.GetString("confidence")
			limitations := action.GetStringSlice("limitations")
			reason := action.GetString("reason")

			invoker := loop.GetInvoker()
			emitter := loop.GetEmitter()

			// Extract flow_ids from raw params
			var uintFlowIDs []uint
			params := action.GetParams()
			if flowIDsRaw, ok := params["flow_ids"]; ok && flowIDsRaw != nil {
				if arr, ok := flowIDsRaw.([]interface{}); ok {
					for _, v := range arr {
						switch id := v.(type) {
						case float64:
							uintFlowIDs = append(uintFlowIDs, uint(id))
						case int:
							uintFlowIDs = append(uintFlowIDs, uint(id))
						case int64:
							uintFlowIDs = append(uintFlowIDs, uint(id))
						}
					}
				}
			}

			// Create evidence item
			evidenceID := fmt.Sprintf("E_%d", time.Now().UnixNano()%100000)
			item := EvidenceItem{
				ID:          evidenceID,
				Type:        evidenceType,
				Claim:       claim,
				Evidence:    supportingEvidence,
				FlowIDs:     uintFlowIDs,
				QueryRef:    queryRef,
				Confidence:  confidence,
				Limitations: limitations,
			}

			// Update claims index
			claimsIndex := loop.Get("claims_index")
			typeLabel := map[string]string{
				"observation": "【观察】",
				"hypothesis":  "【假设】",
				"conclusion":  "【结论】",
			}[evidenceType]

			newClaim := fmt.Sprintf("\n[%s] %s %s\n  - 证据: %s\n  - 置信度: %s\n  - FlowIDs: %v",
				evidenceID, typeLabel, claim, supportingEvidence, confidence, uintFlowIDs)
			loop.Set("claims_index", claimsIndex+newClaim)

			// Save evidence to file
			outputDir := loop.Get("output_directory")
			if outputDir == "" {
				outputDir = os.TempDir()
			}

			evidenceFile := filepath.Join(outputDir, "evidence_pack.json")

			// Load existing evidence pack or create new one
			var pack EvidencePack
			if data, err := os.ReadFile(evidenceFile); err == nil {
				json.Unmarshal(data, &pack)
			} else {
				pack = EvidencePack{
					Scope:      loop.Get("query_scope"),
					Queries:    []string{},
					Items:      []EvidenceItem{},
					Provenance: "HTTPFlow Database Analysis",
				}
			}

			pack.Items = append(pack.Items, item)
			pack.GeneratedAt = time.Now().Format("2006-01-02 15:04:05")

			// Save updated pack
			packJSON, _ := json.MarshalIndent(pack, "", "  ")
			if err := os.WriteFile(evidenceFile, packJSON, 0644); err != nil {
				log.Errorf("failed to save evidence pack: %v", err)
			}

			log.Infof("evidence aggregated: [%s] %s - %s", evidenceID, evidenceType, truncateString(claim, 50))

			// Build summary for AI context
			summaryText := fmt.Sprintf("**证据 [%s]** (%s, 置信度: %s)\n%s\n推理: %s",
				evidenceID, typeLabel, confidence, claim, reason)

			// Emit summary
			emitter.EmitThoughtStream("evidence_aggregation", summaryText)
			invoker.AddToTimeline("evidence_aggregation", summaryText)

			// Update evidence pack summary
			evidencePack := loop.Get("evidence_pack")
			loop.Set("evidence_pack", evidencePack+"\n"+summaryText)
		},
	)
}
