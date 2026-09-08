package yakit

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	ScreenshotSubscribe = "yakit_screenshot_subscribe"
	ScreenshotRequest   = "yakit_screenshot_request"
	ScreenshotResponse  = "yakit_screenshot_response"
)

// YakitScreenshot contains a PNG watermarked using the desktop's local time.
type YakitScreenshot struct {
	RequestID  string `json:"requestId"`
	Data       string `json:"data,omitempty"`
	CapturedAt string `json:"capturedAt,omitempty"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	Error      string `json:"error,omitempty"`
}

type pendingScreenshot struct {
	clientID string
	result   chan *YakitScreenshot
}

var screenshotRequests sync.Map

// DeliverYakitScreenshot accepts replies only from the selected duplex client.
func DeliverYakitScreenshot(clientID string, data []byte) error {
	if len(data) > 32*1024*1024 {
		return fmt.Errorf("screenshot response exceeds 32 MiB")
	}
	var result YakitScreenshot
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("invalid screenshot response: %w", err)
	}
	value, ok := screenshotRequests.Load(result.RequestID)
	if !ok {
		return nil // A canceled or timed-out request may still receive a reply.
	}
	pending := value.(*pendingScreenshot)
	if pending.clientID != clientID {
		return nil
	}
	select {
	case pending.result <- &result:
	default:
	}
	return nil
}

// RequestYakitScreenshot uses the existing engine/desktop duplex transport.
func RequestYakitScreenshot(ctx context.Context) (*YakitScreenshot, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	clients := snapshotServerPushSubscribers(ScreenshotRequest)
	if len(clients) == 0 {
		return nil, fmt.Errorf("no screenshot-capable Yakit frontend is connected to this engine; open an updated Yakit frontend first")
	}
	if len(clients) > 1 {
		return nil, fmt.Errorf("multiple Yakit frontends are connected; keep one frontend connected before taking a screenshot")
	}
	client := clients[0]
	id := uuid.NewString()
	pending := &pendingScreenshot{clientID: client.Name, result: make(chan *YakitScreenshot, 1)}
	screenshotRequests.Store(id, pending)
	defer screenshotRequests.Delete(id)
	data, _ := json.Marshal(map[string]string{"requestId": id})
	if !client.enqueue(&ypb.DuplexConnectionResponse{
		MessageType: ScreenshotRequest, Data: data, Timestamp: time.Now().UnixNano(),
	}) {
		return nil, fmt.Errorf("Yakit screenshot request could not be delivered")
	}
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("waiting for Yakit screenshot: %w", ctx.Err())
	case <-client.done:
		return nil, fmt.Errorf("Yakit frontend disconnected while taking a screenshot")
	case result := <-pending.result:
		if result.Error != "" {
			return nil, fmt.Errorf("Yakit screenshot failed: %s", result.Error)
		}
		return result, nil
	}
}
