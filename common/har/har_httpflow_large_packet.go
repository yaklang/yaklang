package har

import (
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func LoadHTTPFlowRequestPacket(flow *schema.HTTPFlow) ([]byte, error) {
	return yakit.LoadHTTPFlowRequestPacket(flow)
}

func LoadHTTPFlowResponsePacket(flow *schema.HTTPFlow) ([]byte, error) {
	return yakit.LoadHTTPFlowResponsePacket(flow)
}

func loadHTTPFlowRequestPacket(flow *schema.HTTPFlow) ([]byte, error) {
	return LoadHTTPFlowRequestPacket(flow)
}

func loadHTTPFlowResponsePacket(flow *schema.HTTPFlow) ([]byte, error) {
	return LoadHTTPFlowResponsePacket(flow)
}

func applyImportedLargeHTTPFlowFlags(flow *schema.HTTPFlow, reqBodySize, rspBodySize int) {
	yakit.SyncLargeHTTPFlowFlagsFromStoredPacket(flow, int64(reqBodySize), int64(rspBodySize))
}
