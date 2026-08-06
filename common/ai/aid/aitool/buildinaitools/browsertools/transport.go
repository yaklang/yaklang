package browsertools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/browser"
)

const (
	ReadTimeout    = 20 * time.Second
	ReplayTimeout  = 60 * time.Second
	maxResultBytes = 2 << 20
)

type Caller interface {
	CallDevice(context.Context, string, string, interface{}) (json.RawMessage, error)
}

type Bridge interface {
	Caller
	Available() bool
	CapabilityCatalog(deviceID string) (*browser.ExtensionBridgeCapabilityCatalog, bool)
}

type Target struct {
	TabID      int
	FrameID    int
	DocumentID string
}

func (t Target) Params() map[string]interface{} {
	result := map[string]interface{}{
		"tabId":   t.TabID,
		"frameId": t.FrameID,
	}
	if t.DocumentID != "" {
		result["documentId"] = t.DocumentID
	}
	return result
}

func cloneParams(params aitool.InvokeParams) map[string]interface{} {
	result := make(map[string]interface{}, len(params)+3)
	for key, value := range params {
		result[key] = value
	}
	return result
}

func decodeResult(raw json.RawMessage) (interface{}, error) {
	if len(raw) > maxResultBytes {
		return nil, fmt.Errorf(
			"browser capability result exceeds %d bytes; narrow the requested target or evidence",
			maxResultBytes,
		)
	}
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var result interface{}
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("decode browser capability result: %w", err)
	}
	return result, nil
}

func CallCapability(
	ctx context.Context,
	caller Caller,
	deviceID string,
	target Target,
	method string,
	params aitool.InvokeParams,
	timeout time.Duration,
	withTarget bool,
) (interface{}, error) {
	if caller == nil {
		return nil, errors.New("browser extension bridge is not running")
	}

	payload := cloneParams(params)
	if withTarget {
		for key, value := range target.Params() {
			payload[key] = value
		}
	}

	callContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	raw, err := caller.CallDevice(callContext, deviceID, method, payload)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", method, err)
	}
	return decodeResult(raw)
}
