package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	MITMReplacerKeyRecords     = "R1oHf8xca6CobwVg2_MITMReplacerKeyRecords"
	MITMFilterKeyRecords       = "uWokegBnCQdnxezJtMVo_MITMFilterKeyRecords"
	MITMHijackFilterKeyRecords = "XcCPLRElWMVjnCNT_MITMHijackFilterKeyRecords"
)

func (s *Server) SetMITMFilter(ctx context.Context, req *ypb.SetMITMFilterRequest) (*ypb.SetMITMFilterResponse, error) {
	projectDB, profileDB := s.GetProjectDatabase(), s.GetProfileDatabase()
	filterManager := GetMITMFilterManager(projectDB, profileDB)
	filterManager.Update(req.GetFilterData())
	// force save to project DB
	filterManager.db = projectDB
	filterManager.Save()
	return &ypb.SetMITMFilterResponse{}, nil
}

func (s *Server) GetMITMFilter(ctx context.Context, req *ypb.Empty) (*ypb.SetMITMFilterRequest, error) {
	filterManager := GetMITMFilterManager(s.GetProjectDatabase(), s.GetProfileDatabase())
	return &ypb.SetMITMFilterRequest{
		FilterData: filterManager.Data,
	}, nil
}

func (s *Server) ResetMITMFilter(ctx context.Context, req *ypb.Empty) (*ypb.SetMITMFilterRequest, error) {
	filterManager := GetMITMFilterManager(s.GetProjectDatabase(), s.GetProfileDatabase())
	filterManager.Recover()
	return &ypb.SetMITMFilterRequest{
		FilterData: filterManager.Data,
	}, nil
}

func (s *Server) SetMITMHijackFilter(ctx context.Context, req *ypb.SetMITMFilterRequest) (*ypb.SetMITMFilterResponse, error) {
	projectDB := s.GetProjectDatabase()
	filterManager := GetMITMHijackFilterManager(projectDB)
	filterManager.Update(req.GetFilterData())
	filterManager.db = projectDB
	filterManager.Save(MITMHijackFilterKeyRecords)
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
