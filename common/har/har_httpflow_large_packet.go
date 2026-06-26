package har

import (
	"io"
	"os"
	"strconv"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func loadHTTPFlowRequestPacket(flow *schema.HTTPFlow) ([]byte, error) {
	if flow == nil {
		return nil, utils.Error("flow is nil")
	}
	if flow.Request != "" {
		reqRaw, err := strconv.Unquote(flow.Request)
		if err != nil {
			return nil, err
		}
		if len(reqRaw) > 0 {
			return []byte(reqRaw), nil
		}
	}
	if flow.IsTooLargeRequest && flow.TooLargeRequestHeaderFile != "" && flow.TooLargeRequestBodyFile != "" {
		return readHTTPFlowSpillPacket(flow.TooLargeRequestHeaderFile, flow.TooLargeRequestBodyFile)
	}
	return nil, nil
}

func loadHTTPFlowResponsePacket(flow *schema.HTTPFlow) ([]byte, error) {
	if flow == nil {
		return nil, utils.Error("flow is nil")
	}
	if flow.Response != "" {
		respRaw, err := strconv.Unquote(flow.Response)
		if err != nil {
			return nil, err
		}
		if len(respRaw) > 0 {
			return []byte(respRaw), nil
		}
	}
	if flow.IsTooLargeResponse && flow.TooLargeResponseHeaderFile != "" && flow.TooLargeResponseBodyFile != "" {
		return readHTTPFlowSpillPacket(flow.TooLargeResponseHeaderFile, flow.TooLargeResponseBodyFile)
	}
	return nil, nil
}

func readHTTPFlowSpillPacket(headerFile, bodyFile string) ([]byte, error) {
	headerFP, err := os.Open(headerFile)
	if err != nil {
		return nil, err
	}
	defer headerFP.Close()
	bodyFP, err := os.Open(bodyFile)
	if err != nil {
		return nil, err
	}
	defer bodyFP.Close()
	return io.ReadAll(io.MultiReader(headerFP, bodyFP))
}

func applyImportedLargeHTTPFlowFlags(flow *schema.HTTPFlow, reqBodySize, rspBodySize int) {
	yakit.SyncLargeHTTPFlowFlagsFromStoredPacket(flow, int64(reqBodySize), int64(rspBodySize))
}
