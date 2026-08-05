package yakgrpc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Soft-compat Package RPCs: backed by SyntaxFlowGroup catalog + Rule.RuleGroup.

func (s *Server) QuerySyntaxFlowPackages(ctx context.Context, req *ypb.QuerySyntaxFlowPackagesRequest) (*ypb.QuerySyntaxFlowPackagesResponse, error) {
	db := s.GetProfileDatabase().Model(&schema.SyntaxFlowGroup{})
	filter := req.GetFilter()
	if filter == nil {
		filter = &ypb.SyntaxFlowPackageFilter{}
	}
	db = sfdb.FilterSyntaxFlowPackages(db, filter.GetNames(), filter.GetSources(), filter.GetKeyword(), filter.GetFilterBuiltinKind())

	paging := req.GetPagination()
	if paging == nil {
		paging = &ypb.Paging{Page: 1, Limit: 100}
	}
	var data []*schema.SyntaxFlowGroup
	p, db := bizhelper.Paging(db, int(paging.Page), int(paging.Limit), &data)
	if db.Error != nil {
		return nil, db.Error
	}
	resp := &ypb.QuerySyntaxFlowPackagesResponse{
		Pagination: paging,
		Total:      int64(p.TotalRecord),
	}
	for _, pkg := range data {
		count := int32(sfdb.CountRulesInPackage(s.GetProfileDatabase(), pkg.GroupName))
		resp.Packages = append(resp.Packages, pkg.ToPackageGRPCModel(count))
	}
	return resp, nil
}

func (s *Server) CreateSyntaxFlowPackage(ctx context.Context, req *ypb.CreateSyntaxFlowPackageRequest) (*ypb.DbOperateMessage, error) {
	name := strings.TrimSpace(req.GetName())
	if name == "" {
		return nil, utils.Error("package name is required")
	}
	version := req.GetVersion()
	if version == "" {
		version = "0.1.0"
	}
	_, err := sfdb.GetOrCreatePackage(s.GetProfileDatabase(), name, version, req.GetDescription(), schema.SyntaxFlowPackageSourceUser, false)
	if err != nil {
		return nil, err
	}
	return &ypb.DbOperateMessage{Operation: "create", ExtraMessage: "create syntaxflow package ok", EffectRows: 1}, nil
}

func (s *Server) UpdateSyntaxFlowPackage(ctx context.Context, req *ypb.UpdateSyntaxFlowPackageRequest) (*ypb.DbOperateMessage, error) {
	db := s.GetProfileDatabase()
	pkg, err := sfdb.QueryPackageByName(db, req.GetName())
	if err != nil {
		return nil, err
	}
	if pkg.IsBuildIn {
		return nil, utils.Errorf("cannot rename/update builtin package: %s", pkg.GroupName)
	}
	newName := strings.TrimSpace(req.GetNewName())
	if newName != "" && newName != pkg.GroupName {
		if err := sfdb.RenameGroup(db, pkg.GroupName, newName); err != nil {
			return nil, err
		}
		pkg, err = sfdb.QueryPackageByName(db, newName)
		if err != nil {
			return nil, err
		}
	}
	if req.GetDescription() != "" {
		pkg.Description = req.GetDescription()
	}
	if req.GetVersion() != "" {
		pkg.Version = req.GetVersion()
	}
	if err := db.Save(pkg).Error; err != nil {
		return nil, err
	}
	return &ypb.DbOperateMessage{Operation: "update", ExtraMessage: "update syntaxflow package ok", EffectRows: 1}, nil
}

func (s *Server) DeleteSyntaxFlowPackage(ctx context.Context, req *ypb.DeleteSyntaxFlowPackageRequest) (*ypb.DbOperateMessage, error) {
	var rows int64
	for _, name := range req.GetNames() {
		if err := sfdb.DeletePackage(s.GetProfileDatabase(), name, req.GetDeleteRules()); err != nil {
			return nil, err
		}
		rows++
	}
	return &ypb.DbOperateMessage{Operation: "delete", ExtraMessage: "delete syntaxflow package ok", EffectRows: rows}, nil
}

func (s *Server) ExportSyntaxFlowPackage(req *ypb.ExportSyntaxFlowPackageRequest, stream ypb.Yak_ExportSyntaxFlowPackageServer) error {
	db := s.GetProfileDatabase()
	name := strings.TrimSpace(req.GetName())
	if name == "" {
		return utils.Error("package name is required")
	}
	target := req.GetTargetPath()
	if target == "" {
		return utils.Error("target path is required")
	}
	_ = stream.Send(&ypb.SyntaxFlowPackageProgress{Progress: 0.1, Message: "building package.yaml", MessageType: "info", PackageName: name})
	meta, err := sfdb.BuildPackageYAMLFromDB(db, name)
	if err != nil {
		return err
	}
	_ = stream.Send(&ypb.SyntaxFlowPackageProgress{Progress: 0.4, Message: "exporting rules", MessageType: "info", PackageName: name, PackageVersion: meta.Version})

	ruleDB := db.Model(&schema.SyntaxFlowRule{}).Where("rule_group = ?", name)
	opts := []sfdb.RuleExportOption{}
	if req.GetPassword() != "" {
		opts = append(opts, sfdb.WithExportPassword(req.GetPassword()))
	}
	result, err := sfdb.ExportRulesToZip(stream.Context(), ruleDB, target, opts...)
	if err != nil {
		return err
	}
	_ = sfdb.WritePackageYAML(filepath.Dir(target), meta)
	_ = stream.Send(&ypb.SyntaxFlowPackageProgress{
		Progress: 1, Message: fmt.Sprintf("exported %d rules", result.Count),
		MessageType: "success", PackageName: name, PackageVersion: meta.Version,
	})
	return nil
}

func (s *Server) ImportSyntaxFlowPackage(req *ypb.ImportSyntaxFlowPackageRequest, stream ypb.Yak_ImportSyntaxFlowPackageServer) error {
	db := s.GetProfileDatabase()
	input := req.GetInputPath()
	if input == "" {
		return utils.Error("input path is required")
	}
	_ = stream.Send(&ypb.SyntaxFlowPackageProgress{Progress: 0.05, Message: "loading package", MessageType: "info"})

	pkgName := strings.TrimSpace(req.GetName())
	meta, err := tryLoadPackageYAML(input)
	if err != nil {
		if pkgName == "" {
			pkgName = fmt.Sprintf("imported-%d", time.Now().Unix())
		}
		meta = &sfdb.PackageYAML{
			Name:    pkgName,
			Version: "0.1.0",
			Source:  schema.SyntaxFlowPackageSourceLocal,
		}
		_ = stream.Send(&ypb.SyntaxFlowPackageProgress{
			Progress: 0.1, Message: "legacy import without package.yaml → " + pkgName,
			MessageType: "info", PackageName: pkgName,
		})
	}
	if pkgName != "" {
		meta.Name = pkgName
	}
	if meta.Source == "" {
		meta.Source = schema.SyntaxFlowPackageSourceLocal
	}
	if _, err := sfdb.GetOrCreatePackage(db, meta.Name, meta.Version, meta.Description, meta.Source, false); err != nil {
		return err
	}

	force := req.GetForceOverwriteConflicts()
	st, err := os.Stat(input)
	if err != nil {
		return err
	}
	if st.IsDir() {
		_ = stream.Send(&ypb.SyntaxFlowPackageProgress{Progress: 0.3, Message: "syncing directory", MessageType: "info", PackageName: meta.Name})
		localFS := filesys.NewLocalFs()
		if err := sfbuildin.SyncPackageFromFileSystemToDB(db, localFS, input, meta.Name, false, func(p float64, msg string) {
			_ = stream.Send(&ypb.SyntaxFlowPackageProgress{Progress: 0.3 + 0.6*p, Message: msg, MessageType: "info", PackageName: meta.Name})
		}); err != nil {
			return err
		}
		_ = stream.Send(&ypb.SyntaxFlowPackageProgress{Progress: 1, Message: "import ok", MessageType: "success", PackageName: meta.Name, PackageVersion: meta.Version})
		return nil
	}

	opts := []sfdb.RuleImportOption{}
	if req.GetPassword() != "" {
		opts = append(opts, sfdb.WithImportPassword(req.GetPassword()))
	}
	_ = stream.Send(&ypb.SyntaxFlowPackageProgress{Progress: 0.3, Message: "importing zip", MessageType: "info", PackageName: meta.Name})
	if _, err := sfdb.ImportRulesFromZip(stream.Context(), db, input, opts...); err != nil {
		return err
	}
	for _, r := range meta.Rules {
		if c := sfdb.CheckRulePackageIdentityConflict(db, meta.Name, r.RuleID, r.RuleName, r.Version); c != nil {
			_ = stream.Send(&ypb.SyntaxFlowPackageProgress{
				Progress: 0.8, Message: "conflict", MessageType: "conflict", PackageName: meta.Name,
				Conflict: &ypb.SyntaxFlowPackageConflict{
					RuleId: c.RuleID, RuleName: c.RuleName, LocalVersion: c.LocalVersion,
					RemoteVersion: c.RemoteVersion, Reason: c.Reason, PackageName: c.PackageName,
				},
			})
			if !force {
				continue
			}
		}
		_ = db.Model(&schema.SyntaxFlowRule{}).Where("rule_id = ?", r.RuleID).
			Updates(map[string]any{"rule_group": meta.Name, "version": r.Version}).Error
	}
	_ = stream.Send(&ypb.SyntaxFlowPackageProgress{Progress: 1, Message: "import finished", MessageType: "success", PackageName: meta.Name, PackageVersion: meta.Version})
	return nil
}

func tryLoadPackageYAML(input string) (*sfdb.PackageYAML, error) {
	if meta, err := sfdb.LoadPackageYAML(input); err == nil {
		return meta, nil
	}
	dir := input
	if st, e := os.Stat(input); e == nil && !st.IsDir() {
		dir = filepath.Dir(input)
	}
	return sfdb.LoadPackageYAML(dir)
}

func (s *Server) SyncSyntaxFlowPackage(req *ypb.SyncSyntaxFlowPackageRequest, stream ypb.Yak_SyncSyntaxFlowPackageServer) error {
	_ = stream.Send(&ypb.SyntaxFlowPackageProgress{Progress: 0, Message: "sync embed packages", MessageType: "info"})
	notify := func(p float64, name string) {
		_ = stream.Send(&ypb.SyntaxFlowPackageProgress{Progress: p, Message: name, MessageType: "info"})
	}
	var err error
	if req.GetForce() {
		err = sfbuildin.ForceSyncEmbedRule(notify)
	} else {
		err = sfbuildin.SyncEmbedRule(notify)
	}
	if err != nil {
		_ = stream.Send(&ypb.SyntaxFlowPackageProgress{Progress: 1, Message: err.Error(), MessageType: "error"})
		return err
	}
	_ = stream.Send(&ypb.SyntaxFlowPackageProgress{Progress: 1, Message: "sync ok", MessageType: "success"})
	return nil
}
