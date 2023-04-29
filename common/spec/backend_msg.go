package spec

import (
	"encoding/json"
	"github.com/pkg/errors"
	"time"
)

type MessageType string

var (
	MessageType_HIDS          MessageType = "hids"
	MessageType_Scanner       MessageType = "scanner"
	MessageType_SystemMatrix  MessageType = "system—matrix"
	MessageType_ScriptRuntime MessageType = "script-runtime"
	MessageType_AuditLog      MessageType = "audit-log"
	MessageType_MITM          MessageType = "mitm"
	MessageType_NodeLog       MessageType = "node-log"
)

type Message struct {
	NodeId    string      `json:"node_id"`
	Token     string      `json:"token"`
	Type      MessageType `json:"type"`
	Timestamp int64       `json:"timestamp"`

	Content json.RawMessage `json:"content"`
}

func NewScanNodeMessage(id, token string, r *ScanResult) (*Message, error) {
	raw, err := json.Marshal(r)
	if err != nil {
		return nil, errors.Errorf("marshal scan port result failed: %v", err)
	}

	return &Message{
		NodeId:    id,
		Token:     token,
		Type:      MessageType_Scanner,
		Timestamp: time.Now().Unix(),
		Content:   raw,
	}, nil
}
