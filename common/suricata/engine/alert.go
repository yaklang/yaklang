package engine

import (
	"time"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/suricata/rule"
)

type Alert struct {
	Timestamp     time.Time
	Rule          *rule.Rule
	Flow          FlowSnapshot
	Direction     int
	View          InspectionView
	Protocol      string
	Profile       string
	Target        string
	Tagged        bool
	EventID       uint64
	FlowID        uint64
	TCPFlowID     uint64
	TransactionID uint64
	ResponseTo    uint64
	References    []pcaputil.PacketReference
	Summary       string
}

type Diagnostic struct {
	Timestamp time.Time
	Flow      FlowSnapshot
	Kind      string
	Message   string
	Rule      *rule.Rule
}

type Result struct {
	Alerts      []Alert
	Diagnostics []Diagnostic
}
