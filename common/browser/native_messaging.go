package browser

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

const MaxNativeMessagingMessageSize = 16 << 20

func ReadNativeMessagingMessage(reader io.Reader) (json.RawMessage, error) {
	var size uint32
	if err := binary.Read(reader, binary.NativeEndian, &size); err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, fmt.Errorf("native messaging frame is empty")
	}
	if size > MaxNativeMessagingMessageSize {
		return nil, fmt.Errorf("native messaging frame exceeds %d bytes: %d", MaxNativeMessagingMessageSize, size)
	}
	payload := make([]byte, int(size))
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, fmt.Errorf("read native messaging payload: %w", err)
	}
	if !json.Valid(payload) {
		return nil, fmt.Errorf("native messaging payload is not valid JSON")
	}
	return json.RawMessage(payload), nil
}

func WriteNativeMessagingMessage(writer io.Writer, message interface{}) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal native messaging payload: %w", err)
	}
	if len(payload) == 0 || len(payload) > MaxNativeMessagingMessageSize {
		return fmt.Errorf("native messaging payload size is invalid: %d", len(payload))
	}
	if err := binary.Write(writer, binary.NativeEndian, uint32(len(payload))); err != nil {
		return fmt.Errorf("write native messaging frame header: %w", err)
	}
	if _, err := writer.Write(payload); err != nil {
		return fmt.Errorf("write native messaging payload: %w", err)
	}
	return nil
}
