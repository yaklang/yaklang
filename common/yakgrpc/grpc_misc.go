package yakgrpc

import (
	"context"
	"os"
	"yaklang/common/consts"
	"yaklang/common/utils"
	"yaklang/common/utils/pcapfix"
	"yaklang/common/yakgrpc/ypb"
	"runtime"
)

func (s *Server) ResetAndInvalidUserData(ctx context.Context, req *ypb.ResetAndInvalidUserDataRequest) (*ypb.Empty, error) {
	os.RemoveAll(consts.GetDefaultYakitBaseTempDir())
	for _, table := range YakitAllTables {
		s.GetProjectDatabase().Unscoped().DropTableIfExists(table)
	}
	for _, table := range YakitProfileTables {
		s.GetProfileDatabase().Unscoped().DropTableIfExists(table)
	}
	os.Exit(1)
	return &ypb.Empty{}, nil
}

func (s *Server) IsPrivilegedForNetRaw(ctx context.Context, req *ypb.Empty) (*ypb.IsPrivilegedForNetRawResponse, error) {
	if runtime.GOOS == "windows" {
		return &ypb.IsPrivilegedForNetRawResponse{
			IsPrivileged:  pcapfix.IsPrivilegedForNetRaw(),
			Advice:        "use administrator privileges for opening yak.exe or yakit",
			AdviceVerbose: "使用管理员权限打开 Yakit 或者 yak.exe",
		}, nil
	}
	return &ypb.IsPrivilegedForNetRawResponse{
		IsPrivileged:  pcapfix.IsPrivilegedForNetRaw(),
		Advice:        "use pcapfix.Fix or Yakit FixPcapPermission to fix this;",
		AdviceVerbose: "使用 pcapfix.Fix 或 Yakit 修复原始网卡权限操作",
	}, nil
}

func (s *Server) PromotePermissionForUserPcap(ctx context.Context, req *ypb.Empty) (*ypb.Empty, error) {
	err := pcapfix.Fix()
	if err != nil {
		return nil, utils.Errorf("call pcapfix.Fix error: %s", err)
	}
	return &ypb.Empty{}, nil
}
