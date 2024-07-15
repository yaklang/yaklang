package yakgrpc

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
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
