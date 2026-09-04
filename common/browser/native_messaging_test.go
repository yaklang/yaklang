package browser

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNativeMessagingRoundTrip(t *testing.T) {
	buffer := bytes.NewBuffer(nil)
	require.NoError(t, WriteNativeMessagingMessage(buffer, map[string]interface{}{
		"type": "request",
		"id":   "native-1",
		"params": map[string]interface{}{
			"tabId": 42,
		},
	}))

	message, err := ReadNativeMessagingMessage(buffer)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"request","id":"native-1","params":{"tabId":42}}`, string(message))
	require.Zero(t, buffer.Len())
}

func TestNativeMessagingRejectsOversizedFrame(t *testing.T) {
	buffer := bytes.NewBuffer(nil)
	require.NoError(t, binary.Write(buffer, binary.NativeEndian, uint32(MaxNativeMessagingMessageSize+1)))
	_, err := ReadNativeMessagingMessage(buffer)
	require.ErrorContains(t, err, "exceeds")
}

func TestNativeMessagingRejectsInvalidJSON(t *testing.T) {
	buffer := bytes.NewBuffer(nil)
	payload := strings.Repeat("x", 8)
	require.NoError(t, binary.Write(buffer, binary.NativeEndian, uint32(len(payload))))
	_, _ = buffer.WriteString(payload)
	_, err := ReadNativeMessagingMessage(buffer)
	require.ErrorContains(t, err, "not valid JSON")
}
