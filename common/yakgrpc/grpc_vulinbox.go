package yakgrpc

import (
	"context"
	_ "embed"
	"os"
	"os/exec"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//go:embed grpc_vulinbox_script.yak
var genQualityInspectionReport []byte

func (s *Server) StartVulinbox(req *ypb.StartVulinboxRequest, stream ypb.Yak_StartVulinboxServer) error {
	p := consts.GetVulinboxPath()
	if p == "" {
		return utils.Error("vulinbox is not installed")
	}

	log.Infof("start vulinbox in path: %v", p)

	var opts []string
	if req.GetNoHttps() {
		opts = append(opts, "--nohttps")
	}
	if req.GetSafeMode() {
		opts = append(opts, "--safe")
	}
	if req.GetHost() != "" {
		opts = append(opts, "--host", req.GetHost())
	}
	if req.GetPort() != "" {
		opts = append(opts, "--port", req.GetPort())
	}
	log.Infof("start to run vulinbox %v", opts)
	cmd := exec.CommandContext(stream.Context(), p, opts...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (s *Server) IsVulinboxReady(ctx context.Context, req *ypb.IsVulinboxReadyRequest) (*ypb.IsVulinboxReadyResponse, error) {
	rsp, err := s.IsThirdPartyBinaryReady(ctx, &ypb.IsThirdPartyBinaryReadyRequest{
		Name: "vulinbox",
	})
	if err != nil {
		return &ypb.IsVulinboxReadyResponse{Ok: false, Reason: err.Error()}, nil
	}
	if rsp.GetError() != "" {
		return &ypb.IsVulinboxReadyResponse{
			Ok:     rsp.GetIsReady(),
			Reason: rsp.GetError(),
		}, nil
	}
	if !rsp.GetIsReady() {
		return &ypb.IsVulinboxReadyResponse{
			Ok:     false,
			Reason: "vulinbox is not installed",
		}, nil
	}
	return &ypb.IsVulinboxReadyResponse{Ok: true}, nil
}

func (s *Server) InstallVulinbox(req *ypb.InstallVulinboxRequest, stream ypb.Yak_InstallVulinboxServer) error {
	err := s.InstallThirdPartyBinary(&ypb.InstallThirdPartyBinaryRequest{
		Name:  "vulinbox",
		Proxy: req.GetProxy(),
		Force: true,
	}, stream)
	if err != nil {
		return err
	}

	rsp, err := s.IsVulinboxReady(
		stream.Context(),
		&ypb.IsVulinboxReadyRequest{},
	)
	if err != nil {
		return utils.Errorf("download finished, checking available error: %v", err)
	}

	if rsp.GetOk() {
		return nil
	}

	return utils.Errorf("download finished, but vulinbox is not available: %v", rsp.GetReason())
}

func (s *Server) GenQualityInspectionReport(req *ypb.GenQualityInspectionReportRequest, stream ypb.Yak_GenQualityInspectionReportServer) error {
	reqParams := &ypb.ExecRequest{
		Script: string(genQualityInspectionReport),
	}

	reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
		Key:   "script-name",
		Value: strings.Join(req.GetScriptNames(), ","),
	})

	reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
		Key:   "task-name",
		Value: req.GetTaskName(),
	})
	return s.Exec(reqParams, stream)
}
