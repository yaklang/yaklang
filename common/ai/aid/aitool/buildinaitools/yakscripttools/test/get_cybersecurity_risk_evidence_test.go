package test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
)

func TestGetCybersecurityRiskEvidenceTool(t *testing.T) {
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "risk-evidence.db"))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.Risk{}).Error)
	t.Cleanup(func() { _ = db.Close() })

	request := "POST /orders/1002 HTTP/1.1\r\nHost: example.test\r\nCookie: session=demo\r\nContent-Type: application/x-www-form-urlencoded\r\n\r\norderId=1002"
	response := "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{\"id\":1002,\"owner\":\"other-user\"}"
	risk := &schema.Risk{
		Hash:           "evidence-risk",
		Title:          "Order IDOR",
		RiskType:       "unauth-access",
		Severity:       "high",
		Url:            "https://example.test/orders/1002",
		Host:           "example.test",
		Parameter:      "orderId",
		Payload:        "1002",
		Description:    "A user can read another user's order.",
		Details:        strconv.Quote(`{"param":"orderId","evidence":"owner mismatch"}`),
		QuotedRequest:  strconv.Quote(request),
		QuotedResponse: strconv.Quote(response),
		Disposition:    schema.RiskDispositionActive,
	}
	require.NoError(t, db.Create(risk).Error)

	source, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/risk/get_cybersecurity_risk_evidence.yak")
	require.NoError(t, err)
	metadata := yakscripttools.LoadYakScriptToAiTools("get_cybersecurity_risk_evidence", string(source))
	require.NotNil(t, metadata)
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{metadata})
	require.Len(t, tools, 1)
	require.True(t, aicommon.IsFixedInventoryTool("get_cybersecurity_risk_evidence"))

	cfg := aitool.NewToolInvokeConfig()
	aitool.WithRuntimeConfig(&aitool.ToolRuntimeConfig{
		ProjectDatabase: db,
		RiskSelectionScope: &aitool.RiskSelectionScope{
			AllowedIDs: []int64{int64(risk.ID)},
		},
	})(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := tools[0].ExecuteToolWithCapture(ctx, map[string]any{
		"risk_ids":     strconv.FormatUint(uint64(risk.ID), 10),
		"packet_limit": 4096,
	}, cfg)
	require.NoError(t, err)
	require.Contains(t, result.Stdout, "method=POST")
	require.Contains(t, result.Stdout, "replayable=true")
	require.Contains(t, result.Stdout, "state_changing=true")
	require.Contains(t, result.Stdout, "has_auth_material=true")
	require.Contains(t, result.Stdout, "other-user")
	require.Contains(t, result.Stdout, "disposition=active")

	_, after, found := strings.Cut(result.Stdout, "request_file=")
	require.True(t, found)
	requestFile, _, _ := strings.Cut(after, "\n")
	requestFile = strings.TrimSpace(requestFile)
	require.FileExists(t, requestFile)
	t.Cleanup(func() { _ = os.Remove(requestFile) })
	stored, err := os.ReadFile(requestFile)
	require.NoError(t, err)
	require.Equal(t, request, string(stored))
	_, afterResponse, found := strings.Cut(result.Stdout, "response_file=")
	require.True(t, found)
	responseFile, _, _ := strings.Cut(afterResponse, "\n")
	responseFile = strings.TrimSpace(responseFile)
	require.FileExists(t, responseFile)
	t.Cleanup(func() { _ = os.Remove(responseFile) })

	_, err = tools[0].ExecuteToolWithCapture(ctx, map[string]any{"risk_ids": "999999"}, cfg)
	require.ErrorContains(t, err, "outside the attached risk selection")
}

func TestRiskReviewToolIsFixedInventory(t *testing.T) {
	require.True(t, aicommon.IsFixedInventoryTool("get_cybersecurity_risk_evidence"))
}
