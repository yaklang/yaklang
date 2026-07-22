package yakit

import (
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func LoadHTTPFlowRequestPacket(flow *schema.HTTPFlow) ([]byte, error) {
	if flow == nil {
		return nil, utils.Error("flow is nil")
	}
	// Multipart-spilled request: in-DB skeleton carries placeholders; rebuild
	// the complete body on demand from the sidecar part files. This loads the
	// full body into memory and is only used by caller-driven paths (HAR
	// export, encode-to-file), never by list queries.
	if FlowIsMultipartSpill(flow) {
		return loadMultipartSpillRequestPacket(flow)
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

// loadMultipartSpillRequestPacket reconstructs the full request packet for a
// multipart-spilled flow: header file + rebuilt complete multipart body.
func loadMultipartSpillRequestPacket(flow *schema.HTTPFlow) ([]byte, error) {
	header, err := os.ReadFile(flow.TooLargeRequestHeaderFile)
	if err != nil {
		return nil, utils.Wrap(err, "read multipart spill header file failed")
	}
	rr, err := RebuildFlowMultipartBody(flow)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(rr)
	if err != nil {
		return nil, utils.Wrap(err, "rebuild multipart body failed")
	}
	return append(header, body...), nil
}

func LoadHTTPFlowResponsePacket(flow *schema.HTTPFlow) ([]byte, error) {
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

// ExtractHTTPFlowPacketPart loads request/response packet from flow (including spilled large packets)
// and returns the slice indicated by position: header, body, or all.
func ExtractHTTPFlowPacketPart(flow *schema.HTTPFlow, isRequest bool, position string) ([]byte, error) {
	if flow == nil {
		return nil, utils.Error("http flow is nil")
	}

	var (
		packet []byte
		err    error
	)
	if isRequest {
		packet, err = LoadHTTPFlowRequestPacket(flow)
	} else {
		packet, err = LoadHTTPFlowResponsePacket(flow)
	}
	if err != nil {
		return nil, err
	}

	return ExtractHTTPPacketPart(packet, flow, isRequest, position)
}

// ExtractHTTPPacketPart extracts header/body/all from a raw HTTP packet bytes.
func ExtractHTTPPacketPart(packet []byte, flow *schema.HTTPFlow, isRequest bool, position string) ([]byte, error) {
	pos := normalizeHTTPPacketPartPosition(position)
	if pos == "" {
		return nil, utils.Errorf("unsupported position %q, use header, body, or all", position)
	}

	switch pos {
	case "header":
		if flow != nil {
			headerFile := flow.TooLargeRequestHeaderFile
			if !isRequest {
				headerFile = flow.TooLargeResponseHeaderFile
			}
			if headerFile != "" {
				return os.ReadFile(headerFile)
			}
		}
		header, _ := lowhttp.SplitHTTPHeadersAndBodyFromPacket(packet)
		if header == "" {
			return nil, utils.Error("empty header from packet")
		}
		return []byte(header), nil
	case "body":
		if flow != nil {
			// Multipart-spilled request: body file is a 0-byte placeholder;
			// rebuild the complete multipart body from the sidecar parts.
			if isRequest && FlowIsMultipartSpill(flow) {
				rr, rerr := RebuildFlowMultipartBody(flow)
				if rerr != nil {
					return nil, rerr
				}
				body, berr := io.ReadAll(rr)
				if berr != nil {
					return nil, berr
				}
				if len(body) == 0 {
					return nil, utils.Error("empty rebuilt multipart body")
				}
				return body, nil
			}
			bodyFile := flow.TooLargeRequestBodyFile
			if !isRequest {
				bodyFile = flow.TooLargeResponseBodyFile
			}
			if bodyFile != "" {
				return os.ReadFile(bodyFile)
			}
		}
		_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(packet)
		if len(body) == 0 {
			return nil, utils.Error("empty body from packet")
		}
		return body, nil
	default: // all
		if len(packet) > 0 {
			return packet, nil
		}
		if flow == nil {
			return nil, utils.Error("empty packet")
		}
		return loadSpillFullPacket(flow, isRequest)
	}
}

func normalizeHTTPPacketPartPosition(position string) string {
	switch strings.ToLower(strings.TrimSpace(position)) {
	case "", "all", "packet", "full", "complete":
		return "all"
	case "header", "headers":
		return "header"
	case "body":
		return "body"
	default:
		return ""
	}
}

func loadSpillFullPacket(flow *schema.HTTPFlow, isRequest bool) ([]byte, error) {
	var headerFile, bodyFile string
	if isRequest {
		headerFile = flow.TooLargeRequestHeaderFile
		bodyFile = flow.TooLargeRequestBodyFile
	} else {
		headerFile = flow.TooLargeResponseHeaderFile
		bodyFile = flow.TooLargeResponseBodyFile
	}
	if headerFile == "" || bodyFile == "" {
		return nil, utils.Error("empty packet")
	}
	return readHTTPFlowSpillPacket(headerFile, bodyFile)
}
