package vulinboxagentproto

import (
	"encoding/base64"
	"math/rand"
	"net/netip"
	"time"
)

const (
	ActionUDP         = "udp"
	ActionAck         = "ack"
	ActionDataback    = "databack"
	ActionSubscribe   = "subscribe"
	ActionUnsubscribe = "unsubscribe"
	ActionPing        = "ping"
)

type AgentProtocol struct {
	ActionId uint32 `json:"id"`
	Action   string `json:"action"`
}

func newAgentProtocol(action string) AgentProtocol {
	return AgentProtocol{
		ActionId: rand.Uint32(),
		Action:   action,
	}
}

type DatabackAction struct {
	AgentProtocol
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

func NewDataBackAction(tp string, data any) *DatabackAction {
	return &DatabackAction{
		AgentProtocol: newAgentProtocol(ActionDataback),
		Type:          tp,
		Data:          data,
	}
}

type UDPAction struct {
	AgentProtocol
	// base64 encoded content
	Content     string         `json:"content"`
	Target      netip.AddrPort `json:"target"`
	WaitTimeout time.Duration  `json:"wait_timeout"`
}

func NewUDPAction(content []byte, target netip.AddrPort) *UDPAction {
	return &UDPAction{
		AgentProtocol: newAgentProtocol(ActionUDP),
		Content:       base64.StdEncoding.EncodeToString(content),
		Target:        target,
	}
}

type AckAction struct {
	AgentProtocol
	Status string `json:"status"`
	Data   any    `json:"data,omitempty"`
}

func NewAckAction(id uint32, status string, data any) *AckAction {
	return &AckAction{
		AgentProtocol: AgentProtocol{
			ActionId: id,
			Action:   ActionAck,
		},
		Status: status,
		Data:   data,
	}
}

type PingAction struct {
	AgentProtocol
}

func NewPingAction() *PingAction {
	return &PingAction{
		AgentProtocol: newAgentProtocol(ActionPing),
	}
}

type SubscribeAction struct {
	AgentProtocol
	Type  string   `json:"type"`
	Rules []string `json:"rules"`
}

func NewSubscribeAction(tp string, rules []string) *SubscribeAction {
	return &SubscribeAction{
		AgentProtocol: newAgentProtocol(ActionSubscribe),
		Type:          tp,
		Rules:         rules,
	}
}

type UnsubscribeAction struct {
	AgentProtocol
	Type  string   `json:"type"`
	Rules []string `json:"rules"`
}

func NewUnsubscribeAction(tp string, rules []string) *UnsubscribeAction {
	return &UnsubscribeAction{
		AgentProtocol: newAgentProtocol(ActionUnsubscribe),
		Type:          tp,
		Rules:         rules,
	}
}
