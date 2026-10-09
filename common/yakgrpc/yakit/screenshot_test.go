package yakit

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func screenshotTestClient(t *testing.T, send func(*ypb.DuplexConnectionResponse) error) string {
	t.Helper()
	id := uuid.NewString()
	// Duplex connections also receive general broadcasts. This fixture models
	// the frontend screenshot dispatcher without changing production routing.
	registerServerPushCallback(id, context.Background(), 8, func(message *ypb.DuplexConnectionResponse) error {
		if message.MessageType != ScreenshotRequest {
			return nil
		}
		return send(message)
	})
	require.True(t, SetServerPushSubscription(id, ScreenshotRequest, true))
	t.Cleanup(func() { UnRegisterServerPushCallback(id) })
	return id
}

func TestYakitScreenshotRoundTrip(t *testing.T) {
	var clientID string
	clientID = screenshotTestClient(t, func(message *ypb.DuplexConnectionResponse) error {
		require.Equal(t, ScreenshotRequest, message.MessageType)
		var request YakitScreenshot
		require.NoError(t, json.Unmarshal(message.Data, &request))
		wrong, _ := json.Marshal(YakitScreenshot{RequestID: request.RequestID, Error: "wrong client"})
		require.NoError(t, DeliverYakitScreenshot("other-client", wrong))
		data, _ := json.Marshal(YakitScreenshot{RequestID: request.RequestID, Data: "png", CapturedAt: "2026-09-08 12:34:56 +08:00"})
		return DeliverYakitScreenshot(clientID, data)
	})
	// The general consumer must still receive its broadcast while the screenshot
	// fixture dispatches only its own message type.
	generalID := uuid.NewString()
	noise, err := json.Marshal(map[string]string{"testID": generalID})
	require.NoError(t, err)
	general := make(chan *ypb.DuplexConnectionResponse, 1)
	registerServerPushCallback(generalID, context.Background(), 8, func(message *ypb.DuplexConnectionResponse) error {
		if message.MessageType == ServerPushType_HttpFlow && string(message.Data) == string(noise) {
			select {
			case general <- message:
			default:
			}
		}
		return nil
	})
	t.Cleanup(func() { UnRegisterServerPushCallback(generalID) })
	broadcastRaw(&ypb.DuplexConnectionResponse{MessageType: ServerPushType_HttpFlow, Data: noise})
	select {
	case message := <-general:
		require.Equal(t, ServerPushType_HttpFlow, message.MessageType)
		require.Equal(t, noise, message.Data)
	case <-time.After(time.Second):
		t.Fatal("general duplex broadcast was lost")
	}
	result, err := RequestYakitScreenshot(context.Background())
	require.NoError(t, err)
	require.Equal(t, "png", result.Data)
	_, pending := screenshotRequests.Load(result.RequestID)
	require.False(t, pending)
}

func TestYakitScreenshotUnavailable(t *testing.T) {
	_, err := RequestYakitScreenshot(context.Background())
	require.ErrorContains(t, err, "no screenshot-capable")
	screenshotTestClient(t, func(*ypb.DuplexConnectionResponse) error { return nil })
	screenshotTestClient(t, func(*ypb.DuplexConnectionResponse) error { return nil })
	_, err = RequestYakitScreenshot(context.Background())
	require.ErrorContains(t, err, "multiple Yakit")
}

func TestYakitScreenshotCancellationAndLateReply(t *testing.T) {
	requests := make(chan *ypb.DuplexConnectionResponse, 1)
	id := screenshotTestClient(t, func(message *ypb.DuplexConnectionResponse) error {
		requests <- message
		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := RequestYakitScreenshot(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	message := <-requests
	var request YakitScreenshot
	require.NoError(t, json.Unmarshal(message.Data, &request))
	_, pending := screenshotRequests.Load(request.RequestID)
	require.False(t, pending)
	require.NoError(t, DeliverYakitScreenshot(id, message.Data))
}

func TestYakitScreenshotFrontendError(t *testing.T) {
	var id string
	id = screenshotTestClient(t, func(message *ypb.DuplexConnectionResponse) error {
		var result YakitScreenshot
		if err := json.Unmarshal(message.Data, &result); err != nil {
			return err
		}
		result.Error = "Yakit main window is unavailable"
		data, _ := json.Marshal(result)
		return DeliverYakitScreenshot(id, data)
	})
	_, err := RequestYakitScreenshot(context.Background())
	require.ErrorContains(t, err, "window is unavailable")
}
