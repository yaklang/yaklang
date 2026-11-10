//go:build !yakit_exclude

package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) SetMITMFilter(ctx context.Context, req *ypb.SetMITMFilterRequest) (*ypb.SetMITMFilterResponse, error) {
	if _, err := saveMITMFilter(s.GetProjectDatabase(), req.GetFilterData(), nil); err != nil {
		return nil, err
	}
	return &ypb.SetMITMFilterResponse{}, nil
}

func (s *Server) GetMITMFilter(ctx context.Context, req *ypb.Empty) (*ypb.SetMITMFilterRequest, error) {
	filterManager := GetMITMFilterManager(s.GetProjectDatabase(), s.GetProfileDatabase())
	return &ypb.SetMITMFilterRequest{
		FilterData: filterManager.Data,
	}, nil
}

func (s *Server) ResetMITMFilter(ctx context.Context, req *ypb.Empty) (*ypb.SetMITMFilterRequest, error) {
	data, err := saveMITMFilter(s.GetProjectDatabase(), defaultMITMFilterData, nil)
	if err != nil {
		return nil, err
	}
	return &ypb.SetMITMFilterRequest{FilterData: data}, nil
}

func (s *Server) SetMITMHijackFilter(ctx context.Context, req *ypb.SetMITMFilterRequest) (*ypb.SetMITMFilterResponse, error) {
	projectDB := s.GetProjectDatabase()
	filterManager := GetMITMHijackFilterManager(projectDB)
	filterManager.Update(req.GetFilterData())
	filterManager.db = projectDB
	filterManager.Save(yakit.MITMHijackFilterKeyRecords)
	return &ypb.SetMITMFilterResponse{}, nil
}

func (s *Server) GetMITMHijackFilter(ctx context.Context, req *ypb.Empty) (*ypb.SetMITMFilterRequest, error) {
	filterManager := GetMITMHijackFilterManager(s.GetProjectDatabase())
	return &ypb.SetMITMFilterRequest{
		FilterData: filterManager.Data,
	}, nil
}

func (s *Server) ResetMITMHijackFilter(ctx context.Context, req *ypb.Empty) (*ypb.SetMITMFilterRequest, error) {
	filterManager := GetMITMHijackFilterManager(s.GetProjectDatabase())
	filterManager.RecoverHijackFIlter()
	return &ypb.SetMITMFilterRequest{
		FilterData: filterManager.Data,
	}, nil
}
