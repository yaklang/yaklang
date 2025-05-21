package yakgrpc

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestServer_UpdateProject(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	u := uuid.New()
	pjc, err := client.NewProject(context.Background(), &ypb.NewProjectRequest{
		ProjectName:   u.String(),
		Description:   "hello",
		Type:          yakit.TypeProject,
		ChildFolderId: 0,
		FolderId:      0,
	})
	require.NoError(t, err)

	want := uuid.NewString()
	_, err = client.UpdateProject(context.Background(), &ypb.NewProjectRequest{
		Id:            pjc.Id,
		ProjectName:   u.String(),
		Description:   want,
		Type:          yakit.TypeProject,
		ChildFolderId: 0,
		FolderId:      0,
	})
	require.NoError(t, err)
	detail, err := client.QueryProjectDetail(context.Background(), &ypb.QueryProjectDetailRequest{Id: pjc.Id})
	require.NoError(t, err)
	assert.Equal(t, want, detail.Description)
	_, err = client.DeleteProject(context.Background(), &ypb.DeleteProjectRequest{Id: pjc.Id, IsDeleteLocal: true})
	require.NoError(t, err)
}

func TestServer_TestSSAProject(t *testing.T) {
	client, err := NewLocalClient(true) // local grpc server not global
	require.NoError(t, err)

	// create project
	projectName := uuid.NewString()
	project, err := client.NewProject(context.Background(), &ypb.NewProjectRequest{
		ProjectName: projectName,
		Description: "Test SSA Project",
		Type:        yakit.TypeSSAProject,
	})
	require.NoError(t, err)
	require.NotNil(t, project)

	// get current project
	currentProject, err := client.GetCurrentProjectEx(context.Background(), &ypb.GetCurrentProjectExRequest{Type: yakit.TypeSSAProject})
	require.NoError(t, err)
	require.NotNil(t, currentProject)
	require.Greater(t, currentProject.Id, int64(0))

	// get default project
	defaultProject, err := client.GetDefaultProjectEx(context.Background(), &ypb.GetDefaultProjectExRequest{Type: yakit.TypeSSAProject})
	require.NoError(t, err)
	require.NotNil(t, defaultProject)
	require.Greater(t, defaultProject.Id, int64(0))
	require.Equal(t, defaultProject.Type, yakit.TypeSSAProject)

	// get project by id
	projectDetail, err := client.QueryProjectDetail(context.Background(), &ypb.QueryProjectDetailRequest{Id: project.Id})
	require.NoError(t, err)
	require.Equal(t, projectName, projectDetail.ProjectName)

	// set current project

	// test set ssa-project for current yak-project // should fail
	_, err = client.SetCurrentProject(context.Background(), &ypb.SetCurrentProjectRequest{
		Id: project.Id,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "type not match")
	// set current project
	_, err = client.SetCurrentProject(context.Background(), &ypb.SetCurrentProjectRequest{
		Id:   project.Id,
		Type: yakit.TypeSSAProject,
	})
	require.NoError(t, err)

	// check current project
	newCurrentProject, err := client.GetCurrentProjectEx(context.Background(), &ypb.GetCurrentProjectExRequest{Type: yakit.TypeSSAProject})
	require.NoError(t, err)
	require.Equal(t, project.Id, newCurrentProject.Id)

	// delete project
	_, err = client.DeleteProject(context.Background(), &ypb.DeleteProjectRequest{
		Id:            project.Id,
		IsDeleteLocal: true,
	})
	require.NoError(t, err)

	// check current project is default
	afterDeleteCurrent, err := client.GetCurrentProjectEx(context.Background(), &ypb.GetCurrentProjectExRequest{Type: yakit.TypeSSAProject})
	require.NoError(t, err)
	require.Equal(t, defaultProject.Id, afterDeleteCurrent.Id)

	// check file is deleted
	// check projectDetail.DatabasePath  file exist
	info, err := os.Stat(projectDetail.DatabasePath)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
	_ = info
}

func TestServer_Project_ExportAndImportProject(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx := utils.TimeoutContext(20 * time.Second) // set timeout
	token := utils.RandStringBytes(10)
	newProjectResp, err := client.NewProject(ctx, &ypb.NewProjectRequest{
		ProjectName:   token,
		Description:   "hello",
		Type:          yakit.TypeProject,
		ChildFolderId: 0,
		FolderId:      0,
	})
	require.NoError(t, err)
	defer func() {
		yakit.DeleteProjectByProjectName(consts.GetGormProfileDatabase(), token)
	}()

	t.Run("export project and import project (encrypt type)", func(t *testing.T) {
		passwd := utils.RandStringBytes(10)
		exportStream, err := client.ExportProject(ctx, &ypb.ExportProjectRequest{
			ProjectName: token,
			Password:    passwd,
			Id:          newProjectResp.Id,
		})
		require.NoError(t, err)

		exportPath := ""
		for {
			rsp, err := exportStream.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					t.Fatal(err)
				}
				break
			}
			if rsp.TargetPath != "" {
				exportPath = rsp.TargetPath
			}
			spew.Dump(rsp)
		}

		newProjectName := utils.RandStringBytes(10)
		importStream, err := client.ImportProject(ctx, &ypb.ImportProjectRequest{
			ProjectFilePath:  exportPath,
			Password:         passwd,
			LocalProjectName: newProjectName,
		})
		require.NoError(t, err)

		for {
			rsp, err := importStream.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				t.Fatal(err)
			}
			spew.Dump(rsp)
		}

		newProject, err := yakit.GetProjectByName(consts.GetGormProfileDatabase(), newProjectName)
		require.NoError(t, err)
		require.Equal(t, newProject.ProjectName, newProjectName)
		defer func() {
			yakit.DeleteProjectByProjectName(consts.GetGormProfileDatabase(), newProjectName)
		}()

		_, err = gorm.Open(consts.SQLite, newProject.DatabasePath) // check db whether it is damaged
		require.NoError(t, err)
	})

	t.Run("import project (unencrypt type)", func(t *testing.T) {
		exportStream, err := client.ExportProject(ctx, &ypb.ExportProjectRequest{
			ProjectName: token,
			Id:          newProjectResp.Id,
		})
		require.NoError(t, err)

		exportPath := ""
		for {
			rsp, err := exportStream.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					t.Fatal(err)
				}
				break
			}
			if rsp.TargetPath != "" {
				exportPath = rsp.TargetPath
			}
			spew.Dump(rsp)
		}

		newProjectName := utils.RandStringBytes(10)
		importStream, err := client.ImportProject(ctx, &ypb.ImportProjectRequest{
			ProjectFilePath:  exportPath,
			LocalProjectName: newProjectName,
		})
		require.NoError(t, err)

		for {
			rsp, err := importStream.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				t.Fatal(err)
			}
			spew.Dump(rsp)
		}

		newProject, err := yakit.GetProjectByName(consts.GetGormProfileDatabase(), newProjectName)
		require.NoError(t, err)
		require.Equal(t, newProject.ProjectName, newProjectName)
		defer func() {
			yakit.DeleteProjectByProjectName(consts.GetGormProfileDatabase(), newProjectName)
		}()

		_, err = gorm.Open(consts.SQLite, newProject.DatabasePath) // check db whether it is damaged
		require.NoError(t, err)
	})

	t.Run("import project DB file", func(t *testing.T) {
		projectInfo, err := yakit.GetProjectByID(consts.GetGormProfileDatabase(), newProjectResp.Id)
		require.NoError(t, err)

		newProjectName := utils.RandStringBytes(10)
		importStream, err := client.ImportProject(ctx, &ypb.ImportProjectRequest{
			ProjectFilePath:  projectInfo.DatabasePath,
			LocalProjectName: newProjectName,
		})
		require.NoError(t, err)

		for {
			rsp, err := importStream.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				t.Fatal(err)
			}
			spew.Dump(rsp)
		}

		newProject, err := yakit.GetProjectByName(consts.GetGormProfileDatabase(), newProjectName)
		require.NoError(t, err)
		require.Equal(t, newProject.ProjectName, newProjectName)
		defer func() {
			yakit.DeleteProjectByProjectName(consts.GetGormProfileDatabase(), newProjectName)
		}()

		_, err = gorm.Open(consts.SQLite, newProject.DatabasePath) // check db whether it is damaged
		require.NoError(t, err)
	})

}

func TestServer_Project_DefaultProject(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx := utils.TimeoutContext(20 * time.Second) // set timeout
	token := utils.RandStringBytes(10)
	newProjectResp, err := client.NewProject(ctx, &ypb.NewProjectRequest{
		ProjectName: token,
		Description: "hello",
		Type:        yakit.TypeProject,
	})
	require.NoError(t, err)
	defer func() {
		yakit.DeleteProjectByProjectName(consts.GetGormProfileDatabase(), token)
	}()

	getDefaultPath := func() string {
		projects, err := client.GetProjects(context.Background(), &ypb.GetProjectsRequest{
			Type: yakit.TypeProject,
		})
		require.NoError(t, err)
		log.Info("projects: ", projects)
		for _, project := range projects.Projects {
			if project.GetProjectName() == "[default]" {
				return project.GetDatabasePath()
			}
		}
		return ""
	}

	defaultID := getDefaultPath()
	require.NotEqual(t, defaultID, "")

	t.Run("set default project", func(t *testing.T) {
		_, err = client.SetCurrentProject(context.Background(), &ypb.SetCurrentProjectRequest{
			Id:   newProjectResp.Id,
			Type: yakit.TypeProject,
		})
		require.NoError(t, err)
		_, err := client.SetCurrentProject(context.Background(), &ypb.SetCurrentProjectRequest{
			Id:   0,
			Type: yakit.TypeProject,
		})
		require.NoError(t, err)
		require.Equal(t, getDefaultPath(), defaultID)
	})

}
