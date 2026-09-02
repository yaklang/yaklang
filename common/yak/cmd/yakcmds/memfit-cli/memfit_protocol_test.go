package memfitcli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestMemfitProtocolRoundTrip(t *testing.T) {
	var output bytes.Buffer
	writer := newMemfitProtocolWriter(&output)
	require.NoError(t, writer.send("input", "request-1", memfitInput{Text: "第一行\nsecond"}))

	envelope, err := decodeMemfitEnvelope(output.Bytes())
	require.NoError(t, err)
	require.Equal(t, memfitProtocolVersion, envelope.Version)
	require.Equal(t, "input", envelope.Type)
	require.Equal(t, "request-1", envelope.ID)

	payload, err := decodeMemfitPayload[memfitInput](envelope)
	require.NoError(t, err)
	require.Equal(t, "第一行\nsecond", payload.Text)
}

func TestMemfitProtocolRejectsNoiseAndVersionMismatch(t *testing.T) {
	_, err := decodeMemfitEnvelope([]byte("dependency wrote to stdout"))
	require.ErrorIs(t, err, errMemfitNotProtocolFrame)

	raw, err := json.Marshal(memfitEnvelope{Version: 99, Type: "ready"})
	require.NoError(t, err)
	_, err = decodeMemfitEnvelope(append([]byte(memfitFramePrefix), raw...))
	require.ErrorContains(t, err, "unsupported memfit protocol version")
}

func TestMemfitWorkerRejectsInvalidStartWithoutStartingAI(t *testing.T) {
	var input bytes.Buffer
	require.NoError(t, newMemfitProtocolWriter(&input).send("start", "start-1", memfitStartConfig{ReviewPolicy: "unsafe"}))

	var output bytes.Buffer
	err := runMemfitWorker(&input, &output)
	require.ErrorContains(t, err, "invalid review policy")

	scanner := newMemfitFrameScanner(&output)
	require.True(t, scanner.Scan())
	envelope, decodeErr := decodeMemfitEnvelope(scanner.Bytes())
	require.NoError(t, decodeErr)
	require.Equal(t, "error", envelope.Type)
	status, decodeErr := decodeMemfitPayload[memfitStatus](envelope)
	require.NoError(t, decodeErr)
	require.Contains(t, status.Message, "invalid review policy")
}

func TestMemfitWorkerEventRedactsSecretAndSignalsTerminalState(t *testing.T) {
	const secret = "sk-memfit-secret"
	var output bytes.Buffer
	protocol := newMemfitProtocolWriter(&output)
	event := &schema.AiOutputEvent{
		Type:        schema.EVENT_TYPE_STRUCTURED,
		NodeId:      "react_task_status_changed",
		Content:     []byte(`{"react_task_id":"task-1","react_task_now_status":"completed","debug":"sk-memfit-secret"}`),
		StreamDelta: []byte("never print sk-memfit-secret"),
	}
	forwardMemfitWorkerEvent(protocol, secret, event)

	require.NotContains(t, output.String(), secret)
	scanner := newMemfitFrameScanner(&output)
	require.True(t, scanner.Scan())
	first, err := decodeMemfitEnvelope(scanner.Bytes())
	require.NoError(t, err)
	require.Equal(t, "event", first.Type)
	wireEvent, err := decodeMemfitPayload[memfitWorkerEvent](first)
	require.NoError(t, err)
	require.Contains(t, wireEvent.Content, "[REDACTED]")
	require.Contains(t, wireEvent.StreamDelta, "[REDACTED]")

	require.True(t, scanner.Scan())
	second, err := decodeMemfitEnvelope(scanner.Bytes())
	require.NoError(t, err)
	require.Equal(t, "turn_done", second.Type)
	status, err := decodeMemfitPayload[memfitStatus](second)
	require.NoError(t, err)
	require.Equal(t, "task-1", status.TaskID)
	require.Equal(t, "completed", status.Status)
}

func TestMemfitStartConfigDefaultsToYOLOContract(t *testing.T) {
	require.NoError(t, validateMemfitStartConfig(memfitStartConfig{ReviewPolicy: "yolo"}))
	require.Error(t, validateMemfitStartConfig(memfitStartConfig{ReviewPolicy: ""}))
	require.Error(t, validateMemfitStartConfig(memfitStartConfig{ReviewPolicy: "yolo", MaxIterations: -1}))
}

func TestMemfitReviewHotpatchUpdatesPolicyAndInteraction(t *testing.T) {
	manual := memfitReviewHotpatchEvents("manual")
	require.Len(t, manual, 2)
	require.Equal(t, "AgreePolicy", manual[0].GetHotpatchType())
	require.Equal(t, "manual", manual[0].GetParams().GetReviewPolicy())
	require.Equal(t, "AllowRequireForUserInteract", manual[1].GetHotpatchType())
	require.False(t, manual[1].GetParams().GetDisallowRequireForUserPrompt())

	yolo := memfitReviewHotpatchEvents("yolo")
	require.True(t, yolo[1].GetParams().GetDisallowRequireForUserPrompt())
}

func TestMemfitChildEnvironmentKeepsAPIKeyOutOfWorkerEnvironment(t *testing.T) {
	child := memfitChildEnvironment([]string{
		"PATH=/bin",
		"YAK_AI_API_KEY=secret",
		memfitWorkerEnvironment + "=stale",
	})
	require.Contains(t, child, "PATH=/bin")
	require.Contains(t, child, memfitWorkerEnvironment+"=1")
	for _, entry := range child {
		require.False(t, strings.HasPrefix(entry, "YAK_AI_API_KEY="))
	}
}
