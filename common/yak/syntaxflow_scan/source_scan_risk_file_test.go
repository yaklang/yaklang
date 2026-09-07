package syntaxflow_scan

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestStartScan_SourceMode_RiskHasFileInfoAndScanMode verifies the legion
// stream contract for source-mode (漏洞预检测) risks:
//   - every risk carries scan_mode="source"
//   - risks carry real file path / line numbers / code fragment context
//   - the payload packs the FULL file content (ir-source) so the platform can
//     store it in its irsource table and render the code workspace
func TestStartScan_SourceMode_RiskHasFileInfoAndScanMode(t *testing.T) {
	fileContent := "line-0\nAWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE\nline-2\n"
	files := map[string]string{
		"src/config/leak.env": fileContent,
	}

	ruleContent := `
desc(
	mode: "source",
	language: "general",
	title: "aws akia test",
	alert_min: 1,
)
${*}.pattern_regex(/AKIA[0-9A-Z]{16}/) as $hit
alert $hit for {
	level: "critical",
	title: "AWS key",
}
`

	var lastParts *sfreport.SSAResultParts
	err := StartScan(context.Background(),
		WithSourceFiles("src-proj", files),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content:  ruleContent,
			Language: string(ssaconfig.General),
		}),
		WithScanResultCallback(func(r *ScanResult) {
			if r == nil || r.Result == nil {
				return
			}
			// same conversion EmitSSAResult uses for the ssa-stream payload
			parts, perr := sfreport.ConvertSingleResultToSSAResultParts(
				r.Result,
				sfreport.NewStreamPartsOptions(
					sfreport.WithStreamReportType(sfreport.IRifyFullReportType),
					sfreport.WithStreamShowDataflowPath(true),
					sfreport.WithStreamShowFileContent(true),
					sfreport.WithStreamWithFile(true),
				),
			)
			require.NoError(t, perr)
			if parts != nil {
				lastParts = parts
			}
		}),
		ssaconfig.WithScanIgnoreLanguage(true),
	)
	require.NoError(t, err)
	require.NotNil(t, lastParts, "expected stream parts from source-mode scan")
	require.NotEmpty(t, lastParts.Risks, "expected risks in stream parts")

	// ---- risk assertions ----
	var risk sfreport.Risk
	found := false
	for _, rp := range lastParts.Risks {
		require.NoError(t, json.Unmarshal(rp.RiskJSON, &risk))
		if strings.Contains(risk.CodeSourceURL, "leak.env") {
			found = true
			break
		}
	}
	require.True(t, found, "expected a risk anchored to src/config/leak.env")
	require.Equal(t, "source", risk.ScanMode, "source-mode risk must be marked scan_mode=source")
	require.Equal(t, int64(2), risk.Line, "risk line must be the real line in the file")
	require.Equal(t, "/src-proj/src/config/leak.env", risk.CodeSourceURL)
	require.Contains(t, risk.CodeFragment, "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE")

	// code_range JSON carries the real start line too
	var codeRange ssaapi.CodeRange
	require.NoError(t, json.Unmarshal([]byte(risk.CodeRange), &codeRange))
	require.Equal(t, int64(2), codeRange.StartLine)
	require.Equal(t, int64(2), codeRange.EndLine)

	// ---- file assertions: full content packed for the platform irsource ----
	require.NotEmpty(t, lastParts.Files, "stream parts must pack files")
	var file *sfreport.File
	for _, f := range lastParts.Files {
		if strings.HasSuffix(f.Path, "leak.env") {
			file = f
			break
		}
	}
	require.NotNil(t, file, "expected file part for src/config/leak.env")
	require.Equal(t, fileContent, file.Content, "file part must carry the FULL file content")
	require.NotEmpty(t, file.IrSourceHash)

	// risk ↔ file linkage via file hashes
	riskPart := lastParts.Risks[0]
	require.NotEmpty(t, riskPart.FileHashes, "risk must reference its file hash")
	require.Contains(t, riskPart.FileHashes, file.IrSourceHash)
}

func TestStartScan_SourceMode_StreamsBoundedResultBatches(t *testing.T) {
	files := make(map[string]string, 1200)
	for i := 0; i < 1200; i++ {
		files[fmt.Sprintf("src/leak-%04d.env", i)] = fmt.Sprintf("AWS_ACCESS_KEY_ID=AKIA%016X\n", i)
	}

	ruleContent := `
desc(
	mode: "source",
	language: "general",
	title: "aws akia batch test",
	alert_min: 1,
)
${*}.pattern_regex(/AKIA[0-9A-Z]{16}/) as $hit
alert $hit for { level: "critical", title: "AWS key" }
`

	var mu sync.Mutex
	batchSizes := make([]int, 0, 8)
	riskCount := 0
	err := StartScan(context.Background(),
		WithSourceFiles("src-batch", files),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content:  ruleContent,
			Language: string(ssaconfig.General),
		}),
		WithScanResultCallback(func(r *ScanResult) {
			if r == nil || r.Result == nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			batchSizes = append(batchSizes, r.Result.RiskCount())
			riskCount += r.Result.RiskCount()
		}),
		ssaconfig.WithScanIgnoreLanguage(true),
	)
	require.NoError(t, err)
	require.Equal(t, 1200, riskCount)
	require.Greater(t, len(batchSizes), 1, "1200 hits must be streamed as multiple bounded batches")
	require.LessOrEqual(t, len(batchSizes), 3, "source callback batches must not be unbounded")
	for _, size := range batchSizes {
		require.LessOrEqual(t, size, 512)
	}
}
