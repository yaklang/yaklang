package yakgrpc

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaproject"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QuerySSAProject(ctx context.Context, req *ypb.QuerySSAProjectRequest) (*ypb.QuerySSAProjectResponse, error) {
	p, data, err := yakit.QuerySSAProject(consts.GetGormProfileDatabase(), req)
	if err != nil {
		return nil, err
	}
	rsp := &ypb.QuerySSAProjectResponse{
		Pagination: req.GetPagination(),
		Total:      int64(p.TotalRecord),
	}
	for _, d := range data {
		model := SSAProjectToGRPCModel(d)
		if model == nil {
			continue
		}
		rsp.Projects = append(rsp.Projects, model)
	}
	return rsp, nil
}

func (s *Server) CreateSSAProject(ctx context.Context, req *ypb.CreateSSAProjectRequest) (*ypb.CreateSSAProjectResponse, error) {
	project, err := yakit.CreateSSAProject(consts.GetGormProfileDatabase(), req)
	if err != nil {
		return &ypb.CreateSSAProjectResponse{
			Message: &ypb.DbOperateMessage{
				TableName:    "ssa_projects",
				Operation:    DbOperationCreate,
				EffectRows:   0,
				ExtraMessage: err.Error(),
			},
		}, err
	}
	return &ypb.CreateSSAProjectResponse{
		Project: SSAProjectToGRPCModel(project),
		Message: &ypb.DbOperateMessage{
			TableName:    "ssa_projects",
			Operation:    DbOperationCreate,
			EffectRows:   1,
			ExtraMessage: "create SSA project success",
		},
	}, nil
}

func (s *Server) UpdateSSAProject(ctx context.Context, req *ypb.UpdateSSAProjectRequest) (*ypb.UpdateSSAProjectResponse, error) {
	if req == nil || req.Project == nil {
		return nil, utils.Errorf("update SSA project failed: request or project is nil")
	}

	project, err := yakit.UpdateSSAProject(consts.GetGormProfileDatabase(), req.Project)
	if err != nil {
		return &ypb.UpdateSSAProjectResponse{
			Message: &ypb.DbOperateMessage{
				TableName:    "ssa_projects",
				Operation:    DbOperationUpdate,
				EffectRows:   0,
				ExtraMessage: err.Error(),
			},
		}, err
	}
	return &ypb.UpdateSSAProjectResponse{
		Project: SSAProjectToGRPCModel(project),
		Message: &ypb.DbOperateMessage{
			TableName:    "ssa_projects",
			Operation:    DbOperationUpdate,
			EffectRows:   1,
			ExtraMessage: "update SSA project success",
		},
	}, nil
}

func (s *Server) DeleteSSAProject(ctx context.Context, req *ypb.DeleteSSAProjectRequest) (*ypb.DeleteSSAProjectResponse, error) {
	count, err := yakit.DeleteSSAProject(consts.GetGormProfileDatabase(), req)
	if err != nil {
		return &ypb.DeleteSSAProjectResponse{
			Message: &ypb.DbOperateMessage{
				TableName:    "ssa_projects",
				Operation:    DbOperationDelete,
				EffectRows:   0,
				ExtraMessage: err.Error(),
			},
		}, err
	}
	return &ypb.DeleteSSAProjectResponse{
		Message: &ypb.DbOperateMessage{
			TableName:  "ssa_projects",
			Operation:  DbOperationDelete,
			EffectRows: count,
		},
	}, nil
}

func SSAProjectToGRPCModel(p *schema.SSAProject) *ypb.SSAProject {
	if p == nil {
		return nil
	}
	db := consts.GetGormSSAProjectDataBase()
	project := p.ToGRPCModelBasic()
	project.CompileTimes = yakit.QuerySSACompileTimesByProjectID(db, p.ID)
	project.RiskNumber = yakit.QuerySSARiskNumberByProjectID(db, p.ID)
	return project
}

func (s *Server) MigrateSSAProject(req *ypb.MigrateSSAProjectRequest, stream ypb.Yak_MigrateSSAProjectServer) error {
	ssaDB := consts.GetGormSSAProjectDataBase()

	sendProgress := func(percent float64, message string) {
		stream.Send(&ypb.MigrateSSAProjectResponse{
			Percent: percent,
			Message: message,
		})
	}

	oldPrograms, err := yakit.QuerySSAHasNotProjectIDProgram(ssaDB)
	if err != nil {
		return utils.Errorf("query old programs failed: %s", err)
	}

	totalCount := len(oldPrograms)
	if totalCount == 0 {
		sendProgress(1, "未找到旧数据，所有程序都已有 project_id")
		return nil
	}

	sendProgress(0, fmt.Sprintf("找到 %d 个没有 项目配置 的程序，开始创建 SSA 项目...", totalCount))

	for i, prog := range oldPrograms {
		sendProgress(float64(i+1)/float64(totalCount), fmt.Sprintf("正在为程序 '%s' 创建 SSA 项目...", prog.ProgramName))
		programName := prog.ProgramName
		language := prog.Language
		description := prog.Description

		info := prog.ConfigInput
		var codeSourceURL string
		var codeSource *ssaconfig.CodeSourceInfo
		if info != "" {
			config, err := ssaconfig.New(ssaconfig.ModeAll, ssaconfig.WithJsonRawConfig([]byte(info)))
			if err != nil {
				return utils.Errorf("new config failed: %s", err)
			}
			codeSourceURL = config.GetCodeSourceLocalFileOrURL()
			codeSource = config.GetCodeSource()
		}
		// 查询是否存在相同的项目（只有在有 URL 时才查询）
		var project *ssaproject.SSAProject
		var existingProject *ssaproject.SSAProject

		existingProject, _ = ssaproject.LoadSSAProjectByNameAndURL(programName, codeSourceURL)
		if existingProject != nil {
			// 如果项目已存在，直接复用
			project = existingProject
			sendProgress(
				float64(i+1)/float64(totalCount),
				fmt.Sprintf("[%d/%d] 程序 '%s' 的 SSA 项目已存在（ID：%d），直接复用...", i+1, totalCount, programName, project.ID),
			)
		} else {
			// 创建新项目
			project, err = ssaproject.NewSSAProject(
				ssaconfig.WithProjectName(programName),
				ssaconfig.WithProjectLanguage(language),
				ssaconfig.WithProgramDescription(description),
				ssaconfig.WithCodeSourceInfo(codeSource),
			)
			if err != nil {
				sendProgress(
					float64(i+1)/float64(totalCount),
					fmt.Sprintf("[%d/%d] 为程序 '%s' 创建 SSA 项目失败：%s", i+1, totalCount, programName, err),
				)
				continue
			}
			err = project.SaveToDB()
			if err != nil {
				sendProgress(
					float64(i+1)/float64(totalCount),
					fmt.Sprintf("[%d/%d] 保存程序 '%s' 的 SSA 项目失败：%s", i+1, totalCount, programName, err),
				)
				continue
			}
		}

		if err := yakit.UpdateIrProgramProjectID(ssaDB, prog.ID, uint64(project.ID)); err != nil {
			sendProgress(
				float64(i+1)/float64(totalCount),
				fmt.Sprintf("[%d/%d] 更新程序 '%s' 的 project_id 失败：%s", i+1, totalCount, programName, err),
			)
			continue
		}
	}
	sendProgress(1, fmt.Sprintf("迁移完成！总计：%d", totalCount))
	return nil
}
