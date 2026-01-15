package yakgrpc

import (
	"context"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools/metadata"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QueryAIForge(ctx context.Context, req *ypb.QueryAIForgeRequest) (*ypb.QueryAIForgeResponse, error) {
	paging, data, err := yakit.QueryAIForge(s.GetProfileDatabase(), req.GetFilter(), req.GetPagination())
	if err != nil {
		return nil, err
	}

	var res []*ypb.AIForge
	for _, r := range data {
		m := r.ToGRPC()
		if m == nil {
			log.Errorf("failed to convert schema to ypb grpc: %v", r)
		} else {
			res = append(res, m)
		}
	}

	return &ypb.QueryAIForgeResponse{
		Pagination: &ypb.Paging{
			Page:    int64(paging.Page),
			Limit:   int64(paging.Limit),
			OrderBy: req.GetPagination().GetOrderBy(),
			Order:   req.GetPagination().GetOrder(),
		},
		Total: int64(paging.TotalRecord),
		Data:  res,
	}, nil
}

func (s *Server) DeleteAIForge(ctx context.Context, req *ypb.AIForgeFilter) (*ypb.DbOperateMessage, error) {
	count, err := yakit.DeleteAIForge(s.GetProfileDatabase(), req)
	if err != nil {
		return nil, err
	}
	return &ypb.DbOperateMessage{
		TableName:  "ai_forge",
		Operation:  "delete",
		EffectRows: count,
	}, nil
}

func (s *Server) UpdateAIForge(ctx context.Context, req *ypb.AIForge) (*ypb.DbOperateMessage, error) {
	forge := schema.GRPC2AIForge(req)
	applyForgeMetadata(s.GetProfileDatabase(), forge)
	applyForgeRequestOverrides(req, forge)
	err := yakit.UpdateAIForge(s.GetProfileDatabase(), forge)
	if err != nil {
		return nil, err
	}
	updateMessage := &ypb.DbOperateMessage{
		TableName:  "ai_forge",
		Operation:  "update",
		EffectRows: int64(1),
	}
	return updateMessage, nil
}

func (s *Server) CreateAIForge(ctx context.Context, req *ypb.AIForge) (*ypb.DbOperateMessage, error) {
	forgeIns := schema.GRPC2AIForge(req)
	applyForgeMetadata(s.GetProfileDatabase(), forgeIns)
	applyForgeRequestOverrides(req, forgeIns)
	err := yakit.CreateAIForge(s.GetProfileDatabase(), forgeIns)
	if err != nil {
		return nil, err
	}
	return &ypb.DbOperateMessage{
		TableName:  "ai_forge",
		Operation:  "create",
		EffectRows: 1,
		CreateID:   int64(forgeIns.ID),
	}, nil
}

func (s *Server) GetAIForge(ctx context.Context, req *ypb.GetAIForgeRequest) (*ypb.AIForge, error) {
	var forge *schema.AIForge
	var err error
	if req.GetID() > 0 {
		forge, err = yakit.GetAIForgeByID(s.GetProfileDatabase(), req.GetID())
		if err != nil {
			return nil, err
		}
	} else {
		forge, err = yakit.GetAIForgeByName(s.GetProfileDatabase(), req.GetForgeName())
		if err != nil {
			return nil, err
		}
	}

	return forge.ToGRPC(), nil
}

func (s *Server) ExportAIForge(req *ypb.ExportAIForgeRequest, stream ypb.Yak_ExportAIForgeServer) error {
	names := req.GetForgeNames()
	if len(names) == 0 {
		return utils.Error("forge names are required")
	}
	progress := func(percent float64, msg string) {
		_ = stream.Send(&ypb.GeneralProgress{
			Percent: percent,
			Message: msg,
		})
	}
	_, err := aiforge.ExportAIForgesToZip(
		s.GetProfileDatabase(),
		names,
		"",
		aiforge.WithExportProgress(progress),
		aiforge.WithExportPassword(req.GetPassword()),
		aiforge.WithExportOutputName(req.GetOutputName()),
	)
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) ImportAIForge(req *ypb.ImportAIForgeRequest, stream ypb.Yak_ImportAIForgeServer) error {
	progress := func(percent float64, msg string) {
		_ = stream.Send(&ypb.GeneralProgress{
			Percent: percent,
			Message: msg,
		})
	}
	_, err := aiforge.ImportAIForgesFromZip(
		s.GetProfileDatabase(),
		req.GetInputPath(),
		aiforge.WithImportProgress(progress),
		aiforge.WithImportOverwrite(req.GetOverwrite()),
		aiforge.WithImportNewName(req.GetNewForgeName()),
		aiforge.WithImportPassword(req.GetPassword()),
	)
	if err != nil {
		return err
	}
	return nil
}

func applyForgeMetadata(db *gorm.DB, forge *schema.AIForge) {
	if forge == nil {
		return
	}
	if forge.ForgeType == schema.FORGE_TYPE_Config {
		applyForgeDefaultsFromDB(db, forge)
	}
	if forge.ForgeType != schema.FORGE_TYPE_YAK {
		return
	}
	if forge.ForgeContent == "" {
		return
	}
	prog, err := static_analyzer.SSAParse(forge.ForgeContent, "yak")
	if err != nil {
		log.Warnf("parse forge metadata failed: %v", err)
		return
	}
	scriptMetadata, err := metadata.ParseYakScriptMetadataProg(forge.ForgeName, prog)
	if err != nil {
		log.Warnf("parse forge metadata failed: %v", err)
		return
	}
	if forge.ForgeVerboseName == "" && scriptMetadata.VerboseName != "" {
		forge.ForgeVerboseName = scriptMetadata.VerboseName
	}
	if forge.Description == "" && scriptMetadata.Description != "" {
		forge.Description = scriptMetadata.Description
	}
	if forge.Tags == "" && len(scriptMetadata.Keywords) > 0 {
		forge.Tags = strings.Join(scriptMetadata.Keywords, ",")
	}
	if forge.ToolKeywords == "" && len(scriptMetadata.Keywords) > 0 {
		forge.ToolKeywords = strings.Join(scriptMetadata.Keywords, ",")
	}
}

func applyForgeDefaultsFromDB(db *gorm.DB, forge *schema.AIForge) {
	if db == nil {
		return
	}
	var (
		dbForge *schema.AIForge
		err     error
	)
	if forge.ForgeName != "" {
		dbForge, err = yakit.GetAIForgeByName(db, forge.ForgeName)
	} else {
		return
	}
	if err != nil || dbForge == nil {
		return
	}
	if forge.ForgeName == "" {
		forge.ForgeName = dbForge.ForgeName
	}
	if forge.ForgeType == "" {
		forge.ForgeType = dbForge.ForgeType
	}
	if forge.ForgeVerboseName == "" {
		forge.ForgeVerboseName = dbForge.ForgeVerboseName
	}
	if forge.ForgeContent == "" {
		forge.ForgeContent = dbForge.ForgeContent
	}
	if forge.ParamsUIConfig == "" {
		forge.ParamsUIConfig = dbForge.ParamsUIConfig
	}
	if forge.Params == "" {
		forge.Params = dbForge.Params
	}
	if forge.UserPersistentData == "" {
		forge.UserPersistentData = dbForge.UserPersistentData
	}
	if forge.Description == "" {
		forge.Description = dbForge.Description
	}
	if forge.Tools == "" {
		forge.Tools = dbForge.Tools
	}
	if forge.ToolKeywords == "" {
		forge.ToolKeywords = dbForge.ToolKeywords
	}
	if forge.Actions == "" {
		forge.Actions = dbForge.Actions
	}
	if forge.Tags == "" {
		forge.Tags = dbForge.Tags
	}
	if forge.InitPrompt == "" {
		forge.InitPrompt = dbForge.InitPrompt
	}
	if forge.PersistentPrompt == "" {
		forge.PersistentPrompt = dbForge.PersistentPrompt
	}
	if forge.PlanPrompt == "" {
		forge.PlanPrompt = dbForge.PlanPrompt
	}
	if forge.ResultPrompt == "" {
		forge.ResultPrompt = dbForge.ResultPrompt
	}
}

func applyForgeRequestOverrides(req *ypb.AIForge, forge *schema.AIForge) {
	if req == nil || forge == nil {
		return
	}
	forge.ID = uint(req.GetId())
	forge.ForgeName = req.GetForgeName()
	forge.ForgeVerboseName = req.GetForgeVerboseName()
	forge.ForgeContent = req.GetForgeContent()
	forge.ForgeType = req.GetForgeType()
	forge.ParamsUIConfig = req.GetParamsUIConfig()
	forge.Params = req.GetParams()
	forge.UserPersistentData = req.GetUserPersistentData()
	forge.Description = req.GetDescription()
	forge.Tools = strings.Join(req.GetToolNames(), ",")
	forge.ToolKeywords = strings.Join(req.GetToolKeywords(), ",")
	forge.Actions = req.GetAction()
	forge.Tags = strings.Join(req.GetTag(), ",")
	forge.InitPrompt = req.GetInitPrompt()
	forge.PersistentPrompt = req.GetPersistentPrompt()
	forge.PlanPrompt = req.GetPlanPrompt()
	forge.ResultPrompt = req.GetResultPrompt()
}
