package spec

type NodeType string

var (
	NodeType_HIDSAgent NodeType = "hids-agent"
	NodeType_Scanner   NodeType = "scanner-agent"
	NodeType_LogAgent  NodeType = "log-agent"
	NodeType_MITMAgent NodeType = "mitm-agent"
)

type NodeRegisterRequest struct {
	NodeId    string   `json:"node_id"`
	NodeType  NodeType `json:"node_type"`
	Token     string   `json:"token"`
	Timestamp int64    `json:"timestamp"`
}

type NodeUnregisterRequest struct {
	NodeId string `json:"node_id"`
	Token  string `json:"token"`
}

type NodeRegisterResponse struct {
	OriginNodeId  string `json:"origin_node_id"`
	Token         string `json:"token"`
	Ok            bool   `json:"ok"`
	Reason        string `json:"reason,omitempty"`
	WebServerPort string `json:"web_server_port"`
}

type NodeUnregisterResponse struct {
	Ok     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
}
