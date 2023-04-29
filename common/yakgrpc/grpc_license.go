package yakgrpc

import (
	"context"
	"github.com/pkg/errors"
	"yaklang/common/utils"
	"yaklang/common/xlic"
	"yaklang/common/yakgrpc/ypb"
)

func (s *Server) GetLicense(ctx context.Context, _ *ypb.Empty) (_ *ypb.GetLicenseResponse, unexpectedError error) {
	defer func() {
		if err := recover(); err != nil {
			unexpectedError = errors.Errorf("fetch license error: %v", err)
		}
	}()
	req, err := xlic.GetLicenseRequest()
	if err != nil {
		return nil, err
	}
	return &ypb.GetLicenseResponse{License: req}, nil
}

func (s *Server) CheckLicense(ctx context.Context, r *ypb.CheckLicenseRequest) (_ *ypb.Empty, unexpectedError error) {
	defer func() {
		if err := recover(); err != nil {
			unexpectedError = errors.Errorf("CheckLicense error: %v", err)
		}
	}()

	if len(r.GetLicenseActivation()) == 0 {
		return nil, utils.Errorf("license is empty")
	}

	lic := r.GetLicenseActivation()
	_, err := xlic.Machine.VerifyLicense(lic)
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}
