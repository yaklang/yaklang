package yakgrpc

import (
	"context"
	"mime"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) FixUploadPacket(ctx context.Context, req *ypb.FixUploadPacketRequest) (*ypb.FixUploadPacketResponse, error) {
	var request []byte = req.GetRequest()

	request = lowhttp.FixHTTPRequestOut(request)
	return &ypb.FixUploadPacketResponse{Request: request}, nil
}

func (s *Server) IsMultipartFormDataRequest(ctx context.Context, req *ypb.FixUploadPacketRequest) (*ypb.IsMultipartFormDataRequestResult, error) {
	var request []byte = req.GetRequest()

	request = lowhttp.FixHTTPRequestOut(request)
	reqIns, err := lowhttp.ParseBytesToHttpRequest(request)
	if err != nil {
		return nil, utils.Errorf("parse bytes to request failed: %s", err)
	}
	_, err = reqIns.MultipartReader()
	if err != nil {
		return nil, utils.Errorf("multipart reader error: %s", err)
	}

	contentType := reqIns.Header.Get("content-type")
	mimeType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, utils.Errorf("not invalid mimetype(%s): %s", contentType, err)
	}
	if !utils.MatchAnyOfRegexp(mimeType, `(?i)multipart/form-data`) {
		return nil, utils.Error("not multipart/form-data request")
	}
	boundary, ok := params["boundary"]
	if !ok {
		return nil, utils.Errorf("no boundary found for: %s", contentType)
	}
	_ = boundary
	return &ypb.IsMultipartFormDataRequestResult{IsMultipartFormData: true}, nil
}
