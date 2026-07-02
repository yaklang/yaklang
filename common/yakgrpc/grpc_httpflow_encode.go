package yakgrpc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) EncodeHTTPPacketContent(ctx context.Context, req *ypb.EncodeHTTPPacketContentRequest) (*ypb.EncodeHTTPPacketContentResponse, error) {
	rsp := &ypb.EncodeHTTPPacketContentResponse{}

	input, flow, err := resolveEncodeHTTPPacketInput(s, req)
	if err != nil {
		rsp.Error = err.Error()
		return rsp, nil
	}

	encoded, err := s.encodeHTTPPacketContent(ctx, req, input)
	if err != nil {
		rsp.Error = err.Error()
		return rsp, nil
	}

	if req.GetSaveToFile() {
		savedPath, savedDir, err := saveEncodedHTTPPacketContent(encoded, req.GetFilePath(), req.GetEncodingType(), flow)
		if err != nil {
			rsp.Error = err.Error()
			return rsp, nil
		}
		rsp.SavedPath = savedPath
		rsp.SavedDir = savedDir
		return rsp, nil
	}

	rsp.EncodedText = encoded
	return rsp, nil
}

func resolveEncodeHTTPPacketInput(s *Server, req *ypb.EncodeHTTPPacketContentRequest) ([]byte, *schema.HTTPFlow, error) {
	if req.GetHTTPFlowId() > 0 {
		flow, err := yakit.GetHTTPFlow(s.GetProjectDatabase(), req.GetHTTPFlowId())
		if err != nil {
			return nil, nil, err
		}
		part, err := yakit.ExtractHTTPFlowPacketPart(flow, req.GetIsRequest(), req.GetPosition())
		if err != nil {
			return nil, nil, err
		}
		return part, flow, nil
	}

	var raw []byte
	switch {
	case len(req.GetInputBytes()) > 0:
		raw = req.GetInputBytes()
	case req.GetText() != "":
		raw = []byte(req.GetText())
	default:
		return nil, nil, utils.Error("either Text, InputBytes, or HTTPFlowId is required")
	}

	position := strings.TrimSpace(req.GetPosition())
	if position == "" {
		return raw, nil, nil
	}

	part, err := yakit.ExtractHTTPPacketPart(raw, nil, req.GetIsRequest(), position)
	if err != nil {
		return nil, nil, err
	}
	return part, nil, nil
}

func (s *Server) encodeHTTPPacketContent(ctx context.Context, req *ypb.EncodeHTTPPacketContentRequest, input []byte) (string, error) {
	encodingType := strings.TrimSpace(req.GetEncodingType())
	if encodingType == "" {
		return string(input), nil
	}

	codecResp, err := s.Codec(ctx, &ypb.CodecRequest{
		Text:       string(input),
		InputBytes: input,
		Type:       encodingType,
		Params:     req.GetParams(),
	})
	if err != nil {
		return "", err
	}
	if codecResp == nil || codecResp.GetResult() == "" {
		return "", utils.Errorf("codec[%s] returned empty result", encodingType)
	}
	return codecResp.GetResult(), nil
}

func saveEncodedHTTPPacketContent(content string, filePath, encodingType string, flow *schema.HTTPFlow) (savedPath, savedDir string, err error) {
	targetPath, err := resolveEncodedHTTPPacketSavePath(filePath, encodingType, flow)
	if err != nil {
		return "", "", err
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", "", utils.Wrap(err, "create save directory failed")
	}
	if err := os.WriteFile(targetPath, []byte(content), 0o644); err != nil {
		return "", "", utils.Wrap(err, "write encoded content failed")
	}
	return targetPath, filepath.Dir(targetPath), nil
}

func resolveEncodedHTTPPacketSavePath(filePath, encodingType string, flow *schema.HTTPFlow) (string, error) {
	filePath = strings.TrimSpace(filePath)
	ext := encodedHTTPPacketFileExt(encodingType)

	if filePath == "" {
		name := fmt.Sprintf("encoded-http-packet-%d%s", time.Now().UnixNano(), ext)
		return filepath.Join(consts.GetDefaultYakitBaseTempDir(), name), nil
	}

	info, err := os.Stat(filePath)
	if err == nil && info.IsDir() {
		name := defaultEncodedHTTPPacketFilename(flow, ext)
		return filepath.Join(filePath, name), nil
	}
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}

	if strings.HasSuffix(filePath, string(os.PathSeparator)) || strings.HasSuffix(filePath, "/") {
		name := defaultEncodedHTTPPacketFilename(flow, ext)
		return filepath.Join(strings.TrimRight(filePath, `/\`), name), nil
	}

	if filepath.Ext(filePath) == "" {
		return filePath + ext, nil
	}
	return filePath, nil
}

func encodedHTTPPacketFileExt(encodingType string) string {
	switch strings.ToLower(strings.TrimSpace(encodingType)) {
	case "base64", "base64-decode", "url-base64", "url-base64-decode":
		return ".b64.txt"
	case "hex-encode", "hex-decode":
		return ".hex.txt"
	default:
		return ".txt"
	}
}

func defaultEncodedHTTPPacketFilename(flow *schema.HTTPFlow, ext string) string {
	if flow != nil && flow.Url != "" {
		if u := utils.ParseStringToUrl(flow.Url); u != nil {
			base := filepath.Base(u.Path)
			if base != "" && base != "." && base != "/" {
				return fmt.Sprintf("encoded-%s-%d%s", base, time.Now().UnixNano(), ext)
			}
		}
	}
	return fmt.Sprintf("encoded-http-packet-%d%s", time.Now().UnixNano(), ext)
}
