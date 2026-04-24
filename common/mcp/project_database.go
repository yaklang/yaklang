package mcp

import (
	"context"

	"github.com/go-viper/mapstructure/v2"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type databaseContext struct {
	YakitHome            string                  `json:"yakit_home"`
	DefaultProjectDBPath string                  `json:"default_project_db_path"`
	CurrentProjectDBPath string                  `json:"current_project_db_path"`
	CurrentProfileDBPath string                  `json:"current_profile_db_path"`
	ProjectType          string                  `json:"project_type"`
	CurrentProject       *ypb.ProjectDescription `json:"current_project,omitempty"`
}

type projectDatabaseItem struct {
	ID              int64  `json:"id"`
	ProjectName     string `json:"project_name"`
	Description     string `json:"description,omitempty"`
	Type            string `json:"type"`
	DatabasePath    string `json:"database_path"`
	IsCurrent       bool   `json:"is_current"`
	UpdateAt        int64  `json:"updated_at"`
	FolderName      string `json:"folder_name,omitempty"`
	ChildFolderName string `json:"child_folder_name,omitempty"`
}

func init() {
	AddGlobalToolSet("project_database",
		WithTool(mcp.NewTool("get_current_database_context",
			mcp.WithDescription("Get current MCP database context, including yakit home, current project database path, default project database path, and current project metadata"),
			mcp.WithString("projectType",
				mcp.Description("Project type to inspect"),
				mcp.Enum(yakit.TypeProject, yakit.TypeSSAProject),
			),
		), handleGetCurrentDatabaseContext),
		WithTool(mcp.NewTool("list_project_databases",
			mcp.WithDescription("List available project databases from Yakit profile database, including project id, path, current status and basic project metadata"),
			mcp.WithString("projectType",
				mcp.Description("Project type to list"),
				mcp.Enum(yakit.TypeProject, yakit.TypeSSAProject),
			),
			mcp.WithString("keyword",
				mcp.Description("Keyword to filter project name or description"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Maximum number of projects to return"),
				mcp.Default(20),
				mcp.Min(1),
				mcp.Max(200),
			),
		), handleListProjectDatabases),
		WithTool(mcp.NewTool("switch_current_project_database",
			mcp.WithDescription("Switch the current project database by project id. Warning: this changes the current project in the current MCP/Yak process, not just one query"),
			mcp.WithNumber("id",
				mcp.Description("Project id returned by list_project_databases"),
				mcp.Required(),
			),
			mcp.WithString("projectType",
				mcp.Description("Project type to switch"),
				mcp.Enum(yakit.TypeProject, yakit.TypeSSAProject),
			),
		), handleSwitchCurrentProjectDatabase),
		WithTool(mcp.NewTool("create_project_database",
			mcp.WithDescription("Create a new project database in Yakit and optionally switch current project to the newly created project"),
			mcp.WithString("projectName",
				mcp.Description("Project name"),
				mcp.Required(),
			),
			mcp.WithString("description",
				mcp.Description("Project description"),
			),
			mcp.WithString("projectType",
				mcp.Description("Project type to create"),
				mcp.Enum(yakit.TypeProject, yakit.TypeSSAProject),
			),
			mcp.WithBool("switchToCurrent",
				mcp.Description("Whether to switch current project to the newly created project after creation"),
				mcp.Default(true),
			),
			mcp.WithString("databasePath",
				mcp.Description("Optional absolute database path. If empty, backend auto-creates a project database file"),
			),
		), handleCreateProjectDatabase),
	)
}

func normalizeProjectType(raw string) string {
	switch raw {
	case "", yakit.TypeProject:
		return yakit.TypeProject
	case yakit.TypeSSAProject:
		return yakit.TypeSSAProject
	default:
		return yakit.TypeProject
	}
}

func buildCurrentDatabaseContext(ctx context.Context, s *MCPServer, projectType string) *databaseContext {
	projectType = normalizeProjectType(projectType)
	currentProject, err := s.grpcClient.GetCurrentProjectEx(ctx, &ypb.GetCurrentProjectExRequest{Type: projectType})
	if err != nil {
		currentProject = nil
	}
	yakitHome := consts.GetDefaultYakitBaseDir()
	defaultProjectDBPath := consts.GetDefaultYakitProjectDatabase(yakitHome)
	currentProjectDBPath := consts.GetCurrentProjectDatabasePath()
	if currentProject != nil && currentProject.GetDatabasePath() != "" && projectType == yakit.TypeProject {
		currentProjectDBPath = currentProject.GetDatabasePath()
	}

	return &databaseContext{
		YakitHome:            yakitHome,
		DefaultProjectDBPath: defaultProjectDBPath,
		CurrentProjectDBPath: currentProjectDBPath,
		CurrentProfileDBPath: consts.GetCurrentProfileDatabasePath(),
		ProjectType:          projectType,
		CurrentProject:       currentProject,
	}
}

func handleGetCurrentDatabaseContext(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		projectType := normalizeProjectType(utils.MapGetString(request.Params.Arguments, "projectType"))
		return NewCommonCallToolResult(buildCurrentDatabaseContext(ctx, s, projectType))
	}
}

func handleListProjectDatabases(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var args struct {
			ProjectType string `mapstructure:"projectType"`
			Keyword     string `mapstructure:"keyword"`
			Limit       int64  `mapstructure:"limit"`
		}
		if err := mapstructure.Decode(request.Params.Arguments, &args); err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		projectType := normalizeProjectType(args.ProjectType)
		limit := args.Limit
		if limit <= 0 {
			limit = 20
		}

		resp, err := s.grpcClient.GetProjects(ctx, &ypb.GetProjectsRequest{
			ProjectName:  args.Keyword,
			Description:  args.Keyword,
			FrontendType: projectType,
			Pagination: &ypb.Paging{
				Page:    1,
				Limit:   limit,
				OrderBy: "updated_at",
				Order:   "desc",
			},
		})
		if err != nil {
			return nil, utils.Wrap(err, "failed to list project databases")
		}
		currentProject, err := s.grpcClient.GetCurrentProjectEx(ctx, &ypb.GetCurrentProjectExRequest{Type: projectType})
		currentProjectID := int64(0)
		if err == nil && currentProject != nil {
			currentProjectID = currentProject.GetId()
		}

		items := make([]*projectDatabaseItem, 0, len(resp.GetProjects()))
		for _, project := range resp.GetProjects() {
			if project == nil {
				continue
			}
			item := &projectDatabaseItem{
				ID:              project.GetId(),
				ProjectName:     project.GetProjectName(),
				Description:     project.GetDescription(),
				Type:            project.GetType(),
				DatabasePath:    project.GetDatabasePath(),
				IsCurrent:       project.GetId() == currentProjectID,
				UpdateAt:        project.GetUpdateAt(),
				FolderName:      project.GetFolderName(),
				ChildFolderName: project.GetChildFolderName(),
			}
			items = append(items, item)
		}
		return NewCommonCallToolResult(items)
	}
}

func handleSwitchCurrentProjectDatabase(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var args struct {
			ID          int64  `mapstructure:"id"`
			ProjectType string `mapstructure:"projectType"`
		}
		if err := mapstructure.Decode(request.Params.Arguments, &args); err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		if args.ID <= 0 {
			return nil, utils.Error("id must be greater than 0")
		}
		projectType := normalizeProjectType(args.ProjectType)
		_, err := s.grpcClient.SetCurrentProject(ctx, &ypb.SetCurrentProjectRequest{
			Id:   args.ID,
			Type: projectType,
		})
		if err != nil {
			return nil, utils.Wrap(err, "failed to switch current project database")
		}
		return NewCommonCallToolResult(buildCurrentDatabaseContext(ctx, s, projectType))
	}
}

func handleCreateProjectDatabase(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var args struct {
			ProjectName     string `mapstructure:"projectName"`
			Description     string `mapstructure:"description"`
			ProjectType     string `mapstructure:"projectType"`
			SwitchToCurrent *bool  `mapstructure:"switchToCurrent"`
			DatabasePath    string `mapstructure:"databasePath"`
		}
		if err := mapstructure.Decode(request.Params.Arguments, &args); err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		if args.ProjectName == "" {
			return nil, utils.Error("projectName is required")
		}

		projectType := normalizeProjectType(args.ProjectType)
		switchToCurrent := true
		if args.SwitchToCurrent != nil {
			switchToCurrent = *args.SwitchToCurrent
		}

		req := &ypb.NewProjectRequest{
			ProjectName: args.ProjectName,
			Description: args.Description,
			Type:        projectType,
			Database:    args.DatabasePath,
		}
		resp, err := s.grpcClient.NewProject(ctx, req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to create project database")
		}
		if resp == nil || resp.GetId() <= 0 {
			return nil, utils.Error("project created but response id is empty")
		}

		if switchToCurrent {
			_, err = s.grpcClient.SetCurrentProject(ctx, &ypb.SetCurrentProjectRequest{
				Id:   resp.GetId(),
				Type: projectType,
			})
			if err != nil {
				return nil, utils.Wrap(err, "project created but failed to switch current project")
			}
		}

		project, err := s.grpcClient.GetCurrentProjectEx(ctx, &ypb.GetCurrentProjectExRequest{Type: projectType})
		if err != nil || project == nil || (!switchToCurrent && project.GetId() != resp.GetId()) {
			projects, listErr := s.grpcClient.GetProjects(ctx, &ypb.GetProjectsRequest{
				ProjectName:  args.ProjectName,
				FrontendType: projectType,
				Pagination: &ypb.Paging{
					Page:    1,
					Limit:   20,
					OrderBy: "updated_at",
					Order:   "desc",
				},
			})
			if listErr == nil {
				for _, item := range projects.GetProjects() {
					if item != nil && item.GetId() == resp.GetId() {
						project = item
						break
					}
				}
			}
		}

		result := map[string]any{
			"created_project_id":   resp.GetId(),
			"created_project_name": resp.GetProjectName(),
			"project_type":         projectType,
			"switched_to_current":  switchToCurrent,
		}
		if project != nil {
			result["project"] = project
		}
		if switchToCurrent {
			result["current_database_context"] = buildCurrentDatabaseContext(ctx, s, projectType)
		}
		return NewCommonCallToolResult(result)
	}
}
