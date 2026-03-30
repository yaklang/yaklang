package aireact

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestVerifyUserSatisfaction_EmitsRequestAndResponseReferenceMaterials(t *testing.T) {
	var (
		events   []*schema.AiOutputEvent
		eventsMu sync.Mutex
	)

	queryToken := "verify-query-" + utils.RandStringBytes(8)
	payloadToken := "verify-payload-" + utils.RandStringBytes(8)
	rawResponse := `{"@action":"verify-satisfaction","user_satisfied":true,"reasoning":"verified","human_readable_result":"验证完成"}`

	ins, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			eventsMu.Lock()
			defer eventsMu.Unlock()
			events = append(events, e)
		}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()
			require.Contains(t, prompt, queryToken)
			require.Contains(t, prompt, payloadToken)

			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(rawResponse))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	result, err := ins.VerifyUserSatisfaction(context.Background(), queryToken, false, payloadToken)
	require.NoError(t, err)
	require.True(t, result.Satisfied)

	ins.WaitForStream()

	eventsMu.Lock()
	defer eventsMu.Unlock()

	streamStartIDs := make(map[string]bool)
	var requestPayload string
	var responsePayload string
	var requestEventID string
	var responseEventID string

	for _, event := range events {
		if event.Type == schema.EVENT_TYPE_STREAM_START {
			streamStartIDs[event.GetStreamEventWriterId()] = true
		}
		if event.Type != schema.EVENT_TYPE_REFERENCE_MATERIAL {
			continue
		}

		var payload map[string]any
		require.NoError(t, json.Unmarshal(event.Content, &payload))

		payloadStr, _ := payload["payload"].(string)
		eventID, _ := payload["event_uuid"].(string)

		switch {
		case strings.Contains(payloadStr, "AI 请求原文"):
			requestPayload = payloadStr
			requestEventID = eventID
		case strings.Contains(payloadStr, "AI 响应原文"):
			responsePayload = payloadStr
			responseEventID = eventID
		}
	}

	require.NotEmpty(t, requestPayload)
	require.NotEmpty(t, responsePayload)
	require.Contains(t, requestPayload, queryToken)
	require.Contains(t, requestPayload, payloadToken)
	require.Contains(t, responsePayload, rawResponse)
	require.True(t, streamStartIDs[requestEventID], "request reference should attach to a valid stream event")
	require.True(t, streamStartIDs[responseEventID], "response reference should attach to a valid stream event")
}
