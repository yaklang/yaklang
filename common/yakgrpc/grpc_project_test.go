package yakgrpc

import (
	"context"
	"errors"
	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"testing"
	"time"
)

func TestServer_UpdateProject(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	u := uuid.New()
	pjc, err := client.NewProject(context.Background(), &ypb.NewProjectRequest{
		ProjectName:   u.String(),
		Description:   "hello",
		Type:          yakit.TypeProject,
		ChildFolderId: 0,
		FolderId:      0,
	})
	if err != nil {
		panic(err)
	}
	_, err = client.UpdateProject(context.Background(), &ypb.NewProjectRequest{
		Id:            pjc.Id,
		ProjectName:   u.String(),
		Description:   "",
		Type:          yakit.TypeProject,
		ChildFolderId: 0,
		FolderId:      0,
	})
	if err != nil {
		panic(err)
	}
	detail, err := client.QueryProjectDetail(context.Background(), &ypb.QueryProjectDetailRequest{Id: pjc.Id})
	if err != nil {
		panic(err)
	}
	assert.True(t, detail.Description == "")
	_, err = client.DeleteProject(context.Background(), &ypb.DeleteProjectRequest{Id: pjc.Id, IsDeleteLocal: true})
	if err != nil {
		panic(err)
	}
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
