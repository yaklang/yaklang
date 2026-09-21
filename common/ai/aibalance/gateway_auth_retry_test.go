package aibalance

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/aibalanceclient"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestChatTOTPRecoveryDoesNotPublishIntermediateFailure(t *testing.T) {
	testChatAuthRecovery(t, true, true)
}

func TestChatTOTPRecoveryPublishesFinalFailure(t *testing.T) {
	testChatAuthRecovery(t, true, false)
}

func TestChatUnrelatedUnauthorizedIsNotHidden(t *testing.T) {
	testChatAuthRecovery(t, false, false)
}

func testChatAuthRecovery(t *testing.T, totpFailure, recoverySucceeds bool) {
	t.Helper()
	previous := aibalanceclient.GetCachedSecret()
	aibalanceclient.SetCachedSecret("test-secret")
	t.Cleanup(func() { aibalanceclient.SetCachedSecret(previous) })
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/memfit-totp-uuid" {
			fmt.Fprint(w, `{"uuid":"MEMFIT-AItest-secretMEMFIT-AI"}`)
			return
		}
		requests++
		if requests == 1 || !recoverySucceeds {
			w.WriteHeader(http.StatusUnauthorized)
			if totpFailure {
				fmt.Fprint(w, `{"error":{"type":"memfit_totp_auth_failed","message":"Memfit TOTP authentication failed"}}`)
			} else {
				fmt.Fprint(w, `{"error":{"type":"invalid_api_key","message":"Invalid key"}}`)
			}
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"recovered\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	var statuses []int
	var responseStatuses []int
	var requestStatuses []int
	client := &GatewayClient{}
	client.LoadOption(aispec.WithAPIKey("test-key"), aispec.WithModel("memfit-test"),
		aispec.WithRawHTTPResponseHeaderCallback(func(header []byte) {
			statuses = append(statuses, lowhttp.GetStatusCodeFromResponse(header))
		}), aispec.WithRawHTTPResponseCallback(func(header, body []byte) {
			responseStatuses = append(responseStatuses, lowhttp.GetStatusCodeFromResponse(header))
		}), aispec.WithRawHTTPRequestResponseCallback(func(request, header, body []byte, usage *aispec.ChatUsage) {
			requestStatuses = append(requestStatuses, lowhttp.GetStatusCodeFromResponse(header))
		}))
	client.targetUrl = server.URL + "/v1/chat/completions"
	result, err := client.Chat("synthetic request")
	expectedStatus := http.StatusUnauthorized
	if recoverySucceeds {
		require.NoError(t, err)
		require.Contains(t, result, "recovered")
		expectedStatus = http.StatusOK
	} else {
		require.Error(t, err)
	}
	expectedRequests := 1
	if totpFailure {
		expectedRequests = 2
	}
	require.Equal(t, expectedRequests, requests)
	require.Equal(t, []int{expectedStatus}, statuses)
	require.Equal(t, []int{expectedStatus}, responseStatuses)
	require.Equal(t, []int{expectedStatus}, requestStatuses)
}
