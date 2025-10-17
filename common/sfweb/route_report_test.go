package sfweb_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/sfweb"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func TestReportFalsePositive(t *testing.T) {
	t.Run("missing parameter", func(t *testing.T) {
		checkMissingParameter := func(t *testing.T, req *sfweb.ReportFalsePositiveRequest, missing string) {
			t.Helper()

			var rsp sfweb.ErrorResponse
			body, err := json.Marshal(req)
			require.NoError(t, err)

			rawRsp, err := DoResponse(http.MethodPost, "/report/false_positive", &rsp, poc.WithReplaceHttpPacketBody(body, false))
			require.NoError(t, err)
			require.Equal(t, http.StatusInternalServerError, rawRsp.GetStatusCode())
			require.Equal(t, sfweb.NewReportMissingParameterError(missing).Error(), rsp.Message)
		}
		t.Run("missing content", func(t *testing.T) {
			t.Parallel()

			checkMissingParameter(t, &sfweb.ReportFalsePositiveRequest{
				Lang:     "yak",
				RiskHash: "123",
			}, "content")
		})

		t.Run("missing lang", func(t *testing.T) {
			t.Parallel()

			checkMissingParameter(t, &sfweb.ReportFalsePositiveRequest{
				Content:  "content",
				RiskHash: "123",
			}, "lang")
		})

		t.Run("missing risk_hash", func(t *testing.T) {
			t.Parallel()

			checkMissingParameter(t, &sfweb.ReportFalsePositiveRequest{
				Content: "content",
				Lang:    "yak",
			}, "risk_hash")
		})
	})

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		var risks []*sfweb.SyntaxFlowScanRisk
		progress := 0.0

		wc, err := lowhttp.NewWebsocketClient(
			GetScanHTTPRequest(),
			lowhttp.WithWebsocketFromServerHandlerEx(func(wc *lowhttp.WebsocketClient, b []byte, f []*lowhttp.Frame) {
				var rsp sfweb.SyntaxFlowScanResponse
				err := json.Unmarshal(b, &rsp)
				require.NoError(t, err)
				if len(rsp.Risk) > 0 {
					risks = append(risks, rsp.Risk...)
				}
				if rsp.Progress > 0 {
					progress = rsp.Progress
				}
			}),
		)
		require.NoError(t, err)

		err = writeJSON(wc, &sfweb.SyntaxFlowScanRequest{
			Content:        scanFileContent,
			Lang:           `java`,
			ControlMessage: `start`,
			TimeoutSecond:  15, // 将超时从默认的180秒减少到15秒
		})
		require.NoError(t, err)

		wc.Start()
		wc.Wait()

		require.GreaterOrEqual(t, len(risks), 1)
		require.Equal(t, 1.0, progress)

		t.Cleanup(func() {
			if len(risks) > 0 {
				ssadb.DeleteProgram(ssadb.GetDB(), risks[0].ProgramName)
			}
		})

		var rsp sfweb.ReportResponse
		firstRisk := risks[0]
		riskHash := firstRisk.RiskHash

		body, err := json.Marshal(&sfweb.ReportFalsePositiveRequest{
			Content:  scanFileContent,
			Lang:     `java`,
			RiskHash: riskHash,
		})
		require.NoError(t, err)

		rawRsp, err := DoResponse(http.MethodPost, "/report/false_positive", &rsp, poc.WithReplaceHttpPacketBody(body, false))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, rawRsp.GetStatusCode(), string(rawRsp.GetBody()))
		require.NotEmpty(t, rsp.Link)
		require.NotEmpty(t, rsp.Body)
		t.Log(rsp.Link)
	})
}

func TestReportFalseNegative(t *testing.T) {
	t.Run("missing parameter", func(t *testing.T) {
		checkMissingParameter := func(t *testing.T, req *sfweb.ReportFalseNegativeRequest, missing string) {
			t.Helper()

			var rsp sfweb.ErrorResponse
			body, err := json.Marshal(req)
			require.NoError(t, err)

			rawRsp, err := DoResponse(http.MethodPost, "/report/false_negative", &rsp, poc.WithReplaceHttpPacketBody(body, false))
			require.NoError(t, err)
			require.Equal(t, http.StatusInternalServerError, rawRsp.GetStatusCode())
			require.Equal(t, sfweb.NewReportMissingParameterError(missing).Error(), rsp.Message)
		}
		t.Run("missing content", func(t *testing.T) {
			t.Parallel()

			checkMissingParameter(t, &sfweb.ReportFalseNegativeRequest{
				Lang:     "yak",
				RuleName: "rule",
			}, "content")
		})

		t.Run("missing lang", func(t *testing.T) {
			t.Parallel()

			checkMissingParameter(t, &sfweb.ReportFalseNegativeRequest{
				Content:  "content",
				RuleName: "rule",
			}, "lang")
		})

		t.Run("missing rule_name", func(t *testing.T) {
			t.Parallel()

			checkMissingParameter(t, &sfweb.ReportFalseNegativeRequest{
				Content: "content",
				Lang:    "yak",
			}, "rule_name")
		})
	})
	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		var rsp sfweb.ReportResponse
		body, err := json.Marshal(&sfweb.ReportFalseNegativeRequest{
			Content:  scanFileContent,
			Lang:     `java`,
			RuleName: "rule",
		})
		require.NoError(t, err)

		rawRsp, err := DoResponse(http.MethodPost, "/report/false_negative", &rsp, poc.WithReplaceHttpPacketBody(body, false))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, rawRsp.GetStatusCode(), string(rawRsp.GetBody()))
		require.NotEmpty(t, rsp.Link)
		require.NotEmpty(t, rsp.Body)
		t.Log(rsp.Link)
	})
}
