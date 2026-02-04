package yakgrpc

import (
	"context"
	"os"
	"runtime"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/pcapfix"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) ResetAndInvalidUserData(ctx context.Context, req *ypb.ResetAndInvalidUserDataRequest) (*ypb.Empty, error) {
	if req != nil && req.GetOnlyClearCache() {
		// 仅清理缓存：持久化 KV/缓存 存在于两处——
		// 1. Profile 库 GeneralStorage：yakit.Get/Set、embed 同步 hash、全局配置、MITM 替换规则等
		// 2. Project 库 ProjectGeneralStorage：项目内 KV，如 fuzzer-list-cache、BARE_REQUEST 等
		profileDB := s.GetProfileDatabase()
		if profileDB != nil {
			profileDB.Unscoped().DropTableIfExists(&schema.GeneralStorage{})
		}
		projectDB := s.GetProjectDatabase()
		if projectDB != nil {
			projectDB.Unscoped().DropTableIfExists(&schema.ProjectGeneralStorage{})
		}
		os.RemoveAll(consts.GetDefaultYakitBaseTempDir())
		return &ypb.Empty{}, nil
	}
	// 原有行为：清理临时目录、删除所有表并退出进程
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
