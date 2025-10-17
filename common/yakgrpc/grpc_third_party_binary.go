package yakgrpc

import (
	"context"
	"io"
	"os"
	"sort"

	"github.com/yaklang/yaklang/common/thirdparty_bin"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
)

func (s *Server) InstallThirdPartyBinary(req *ypb.InstallThirdPartyBinaryRequest, stream ypb.Yak_InstallThirdPartyBinaryServer) error {
	progressCallback := func(progress float64, downloaded, total int64, message string) {
		stream.Send(&ypb.ExecResult{
			IsMessage: true,
			Message:   []byte(message),
			Progress:  float32(progress * 100),
		})
	}
	err := thirdparty_bin.Install(req.GetName(), &thirdparty_bin.InstallOptions{
		Proxy:    req.GetProxy(),
		Force:    req.GetForce(),
		Context:  stream.Context(),
		Progress: progressCallback,
	})
	if err != nil {
		return err
	}
	stream.Send(&ypb.ExecResult{
		Message: []byte("success"),
	})
	return nil
}

func (s *Server) UninstallThirdPartyBinary(ctx context.Context, req *ypb.UninstallThirdPartyBinaryRequest) (*ypb.GeneralResponse, error) {
	err := thirdparty_bin.Uninstall(req.GetName())
	if err != nil {
		return &ypb.GeneralResponse{
			Ok:     false,
			Reason: err.Error(),
		}, nil
	}
	return &ypb.GeneralResponse{
		Ok: true,
	}, nil
}

func (s *Server) IsThirdPartyBinaryReady(ctx context.Context, req *ypb.IsThirdPartyBinaryReadyRequest) (*ypb.IsThirdPartyBinaryReadyResponse, error) {
	status, err := thirdparty_bin.GetStatus(req.GetName())
	if err != nil {
		return &ypb.IsThirdPartyBinaryReadyResponse{
			IsReady: false,
			Error:   err.Error(),
		}, nil
	}
	return &ypb.IsThirdPartyBinaryReadyResponse{
		IsReady: status.Installed,
	}, nil
}

func (s *Server) StartThirdPartyBinary(req *ypb.StartThirdPartyBinaryRequest, stream ypb.Yak_StartThirdPartyBinaryServer) error {
	grpcStreamWriter := NewGrpcStreamWriter(stream)
	err := thirdparty_bin.Start(stream.Context(), req.GetName(), req.GetArgs(), func(reader io.Reader) {
		io.Copy(io.MultiWriter(grpcStreamWriter, os.Stdout), reader)
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) ListThirdPartyBinary(ctx context.Context, req *ypb.Empty) (*ypb.ListThirdPartyBinaryResponse, error) {
	binaries := thirdparty_bin.ListRegistered()
	response := &ypb.ListThirdPartyBinaryResponse{
		Binaries: make([]*ypb.ThirdPartyBinary, 0, len(binaries)),
	}
	// 按name 排序
	sort.Slice(binaries, func(i, j int) bool {
		return binaries[i].Name < binaries[j].Name
	})
	for _, descriptor := range binaries {
		installPath, err := thirdparty_bin.GetBinaryPath(descriptor.Name)
		if err != nil {
			installPath = ""
		}
		downloadURL := ""
		var supportCurrentPlatform bool
		downloadInfo, err := thirdparty_bin.GetDownloadInfo(descriptor.Name)
		if err != nil {
			downloadInfo = nil
		} else {
			downloadURL = downloadInfo.URL
			supportCurrentPlatform = true
		}
		response.Binaries = append(response.Binaries, &ypb.ThirdPartyBinary{
			Name:                   descriptor.Name,
			SupportCurrentPlatform: supportCurrentPlatform,
			Description:            descriptor.Description,
			InstallPath:            installPath,
			DownloadURL:            downloadURL,
		})
	}
	return response, nil
}

func NewGrpcStreamWriter(stream grpc.ServerStreamingServer[ypb.ExecResult]) *GrpcStreamWriter {
	return &GrpcStreamWriter{
		stream: stream,
	}
}

type GrpcStreamWriter struct {
	stream grpc.ServerStreamingServer[ypb.ExecResult]
}

var _ io.WriteCloser = &GrpcStreamWriter{}

func (s *GrpcStreamWriter) Write(b []byte) (int, error) {
	err := s.stream.Send(&ypb.ExecResult{
		IsMessage: true,
		Message:   b,
	})
	if err != nil {
		return 0, err
	}
	return len(b), err
}

func (s *GrpcStreamWriter) Close() (err error) {
	return nil
}
