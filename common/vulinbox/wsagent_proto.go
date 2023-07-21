package vulinbox

import (
	"encoding/base64"
	"github.com/yaklang/yaklang/common/utils"
	"math/rand"
	"net"
	"net/netip"
	"time"
)

const (
	ActionUDP       = "udp"
	ActionAck       = "ack"
	ActionDataback  = "databack"
	ActionSubscribe = "subscribe"
	ActionPing      = "ping"
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
	Data any    `json:"data"`
}

func newDataBackAction(tp string, data any) *DatabackAction {
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

func newUDPAction(content []byte, target netip.AddrPort) *UDPAction {
	return &UDPAction{
		AgentProtocol: newAgentProtocol(ActionUDP),
		Content:       base64.StdEncoding.EncodeToString(content),
		Target:        target,
	}
}

type AckAction struct {
	AgentProtocol
	Status string `json:"status"`
	Data   any
}

func newAckAction(id uint32, status string, data any) *AckAction {
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

func newPingAction() *PingAction {
	return &PingAction{
		AgentProtocol: newAgentProtocol(ActionPing),
	}
}

type SubscribeAction struct {
	AgentProtocol
	Subscribe []string `json:"subscribe"`
}

func MessageMux(data []byte, ack func(ack *AckAction)) {
	ap := utils.MustUnmarshalJson[AgentProtocol](data)
	var err error
	var rec any
	switch ap.Action {
	case ActionUDP:
		rec, err = handleUDP(utils.MustUnmarshalJson[UDPAction](data))
	}
	if err != nil {
		ack(newAckAction(ap.ActionId, "error", err))
		return
	}
	ack(newAckAction(ap.ActionId, "ok", rec))
}

func handleUDP(udp *UDPAction) ([]byte, error) {
	if udp == nil {
		return nil, nil
	}
	conn, err := net.DialUDP("udp", nil, net.UDPAddrFromAddrPort(udp.Target))
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	bytes, err := base64.StdEncoding.DecodeString(udp.Content)
	if err != nil {
		return nil, err
	}

	if _, err = conn.Write(bytes); err != nil {
		return nil, err
	}

	if udp.WaitTimeout == 0 {
		return nil, nil
	}

	if err := conn.SetDeadline(time.Now().Add(udp.WaitTimeout)); err != nil {
		return nil, err
	}

	buf := make([]byte, 1024)
	_, _, err = conn.ReadFromUDP(buf)
	if err != nil {
		return nil, err
	}
	return buf, nil
}
