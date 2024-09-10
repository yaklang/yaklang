package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) SetMITMFilter(ctx context.Context, req *ypb.SetMITMFilterRequest) (*ypb.SetMITMFilterResponse, error) {
	projectDB, profileDB := s.GetProjectDatabase(), s.GetProfileDatabase()
	filterManager := GetMITMFilterManager(projectDB, profileDB)
	filterManager.IncludeHostnames = req.GetIncludeHostname()
	filterManager.ExcludeHostnames = req.GetExcludeHostname()
	filterManager.ExcludeMethods = req.GetExcludeMethod()
	filterManager.IncludeSuffix = req.GetIncludeSuffix()
	filterManager.ExcludeSuffix = req.GetExcludeSuffix()
	filterManager.ExcludeMIME = req.GetExcludeContentTypes()
	filterManager.ExcludeUri = req.GetExcludeUri()
	filterManager.IncludeUri = req.GetIncludeUri()
	// force save to project DB
	filterManager.db = projectDB
	filterManager.Save()
	return &ypb.SetMITMFilterResponse{}, nil
}

func (s *Server) GetMITMFilter(ctx context.Context, req *ypb.Empty) (*ypb.SetMITMFilterRequest, error) {
	filterManager := GetMITMFilterManager(s.GetProjectDatabase(), s.GetProfileDatabase())
	return &ypb.SetMITMFilterRequest{
		IncludeHostname:     filterManager.IncludeHostnames,
		ExcludeHostname:     filterManager.ExcludeHostnames,
		ExcludeSuffix:       filterManager.ExcludeSuffix,
		IncludeSuffix:       filterManager.IncludeSuffix,
		ExcludeMethod:       filterManager.ExcludeMethods,
		ExcludeContentTypes: filterManager.ExcludeMIME,
		ExcludeUri:          filterManager.ExcludeUri,
		IncludeUri:          filterManager.IncludeUri,
	}, nil
}

func (s *Server) getMITMFilter() *MITMFilterManager {
	return GetMITMFilterManager(s.GetProjectDatabase(), s.GetProfileDatabase())
}
