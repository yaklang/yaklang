package yakgrpc

import (
	"context"
	"encoding/json"
	"github.com/bytedance/mockey"
	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestHotPatchTemplate(t *testing.T) {
	local, err := NewLocalClient()
	require.NoError(t, err)

	times := 10
	typ := "test" + uuid.NewString()
	names := make([]string, 0, times)
	contents := make([]string, 0, times)
	types := make([]string, 0, times)
	for i := 0; i < times; i++ {
		names = append(names, uuid.NewString())
		contents = append(contents, "prefix"+uuid.NewString())
		types = append(types, typ)
	}
	checkYpbHotPatchTemplate := func(t *testing.T, index int, got *ypb.HotPatchTemplate) {
		require.Equal(t, names[index], got.Name)
		require.Equal(t, contents[index], got.Content)
		require.Equal(t, types[index], got.Type)
	}

	// create
	ctx := utils.TimeoutContextSeconds(8)
	for i := 0; i < times; i++ {
		_, err = local.CreateHotPatchTemplate(ctx, &ypb.HotPatchTemplate{
			Name:    names[i],
			Content: contents[i],
			Type:    types[i],
		})
		require.NoError(t, err)
	}

	t.Cleanup(func() {
		// delete
		// one
		deleteResp, err := local.DeleteHotPatchTemplate(ctx, &ypb.DeleteHotPatchTemplateRequest{
			Condition: &ypb.HotPatchTemplateRequest{
				Name: []string{names[0]},
			},
		})
		require.NoError(t, err)
		require.Equal(t, int64(1), deleteResp.GetMessage().EffectRows)

		// all names
		deleteResp, err = local.DeleteHotPatchTemplate(ctx, &ypb.DeleteHotPatchTemplateRequest{
			Condition: &ypb.HotPatchTemplateRequest{
				Name: names,
			},
		})
		require.NoError(t, err)
		require.Equal(t, int64(times-1), deleteResp.GetMessage().EffectRows)
	})

	// query
	// type
	queryResp, err := local.QueryHotPatchTemplate(ctx, &ypb.HotPatchTemplateRequest{
		Type: typ,
	})
	require.NoError(t, err)

	gots := queryResp.GetData()
	require.Len(t, gots, times)
	for i := 0; i < times; i++ {
		checkYpbHotPatchTemplate(t, i, gots[i])
	}

	// content keyword
	queryResp, err = local.QueryHotPatchTemplate(ctx, &ypb.HotPatchTemplateRequest{
		ContentKeyword: []string{contents[0]},
	})
	require.NoError(t, err)

	gots = queryResp.GetData()
	require.Len(t, gots, 1)
	checkYpbHotPatchTemplate(t, 0, gots[0])

	// name
	// one
	queryResp, err = local.QueryHotPatchTemplate(ctx, &ypb.HotPatchTemplateRequest{
		Name: []string{names[0]},
	})
	require.NoError(t, err)

	gots = queryResp.GetData()
	require.Len(t, gots, 1)
	checkYpbHotPatchTemplate(t, 0, gots[0])

	// all names
	queryResp, err = local.QueryHotPatchTemplate(ctx, &ypb.HotPatchTemplateRequest{
		Name: names,
	})
	require.NoError(t, err)

	gots = queryResp.GetData()
	require.Len(t, gots, times)
	for i := 0; i < times; i++ {
		checkYpbHotPatchTemplate(t, i, gots[i])
	}

	// list
	listResp, err := local.QueryHotPatchTemplateList(ctx, &ypb.QueryHotPatchTemplateListRequest{
		Type: typ,
	})
	require.NoError(t, err)
	require.ElementsMatch(t, listResp.GetName(), names)

	// update
	// content
	newContent := "new" + contents[0]
	updateResp, err := local.UpdateHotPatchTemplate(ctx, &ypb.UpdateHotPatchTemplateRequest{
		Condition: &ypb.HotPatchTemplateRequest{
			Name: []string{names[0]},
		},
		Data: &ypb.HotPatchTemplate{
			Content: newContent,
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), updateResp.GetMessage().EffectRows)
	contents[0] = newContent

	// query new content
	queryResp, err = local.QueryHotPatchTemplate(ctx, &ypb.HotPatchTemplateRequest{
		ContentKeyword: []string{contents[0]},
	})
	require.NoError(t, err)

	gots = queryResp.GetData()
	checkYpbHotPatchTemplate(t, 0, gots[0])

}

type TestServerWrapper struct {
	*Server
	onlineClient yaklib.OnlineClient
}

func TestUploadHotPatchTemplateToOnline(t *testing.T) {
	mockey.PatchConvey("Test UploadHotPatchTemplateToOnline", t, func() {
		token := "test-token"
		template := &schema.HotPatchTemplate{
			Name:    "test-template",
			Content: "test-content",
			Type:    "test-type",
		}

		mockey.Mock(yakit.GetHotPatchTemplate).To(func(database *gorm.DB, req *ypb.UploadHotPatchTemplateToOnlineRequest) (*schema.HotPatchTemplate, error) {
			assert.Equal(t, req.Name, "test-template")
			assert.Equal(t, req.Type, "test-type")
			return template, nil
		}).Build()

		mockey.Mock((*yaklib.OnlineClient).UploadHotPatchTemplateToOnline).To(func(ctx context.Context, token string, data []byte) error {
			assert.Equal(t, token, "test-token")

			var reqBody schema.HotPatchTemplate
			err := json.Unmarshal(data, &reqBody)
			assert.NoError(t, err)

			assert.Equal(t, reqBody.Name, "test-template")
			assert.Equal(t, reqBody.Content, "test-content")
			assert.Equal(t, reqBody.Type, "test-type")

			log.Infof("reqBody: %+v", reqBody)

			return nil
		}).Build()

		server := &TestServerWrapper{
			onlineClient: yaklib.OnlineClient{},
		}

		req := &ypb.UploadHotPatchTemplateToOnlineRequest{
			Token: token,
			Name:  "test-template",
			Type:  "test-type",
		}

		resp, err := server.UploadHotPatchTemplateToOnline(context.Background(), req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
	})
}

func TestDownloadHotPatchTemplate(t *testing.T) {
	mockey.PatchConvey("Test DownloadHotPatchTemplate", t, func() {
		token := "test-token"
		name := "test-template"
		templateType := "test-type"
		template := &yaklib.HotPatchTemplate{
			Name:         name,
			Content:      "test-content",
			TemplateType: templateType,
		}

		mockey.Mock((*yaklib.OnlineClient).DownloadHotPatchTemplate).To(func(clientToken, clientName, clientTemplateType string) (*yaklib.HotPatchTemplate, error) {
			assert.Equal(t, token, clientToken) // 验证传入的token
			assert.Equal(t, name, clientName)
			assert.Equal(t, templateType, clientTemplateType)
			return template, nil
		}).Build()

		mockey.Mock(yakit.CreateOrUpdateHotPatchTemplate).To(func(db *gorm.DB, name, templateType, content string) error {
			assert.Equal(t, name, "test-template")
			assert.Equal(t, templateType, "test-type")
			assert.Equal(t, content, "test-content")
			return nil
		}).Build()

		server := &TestServerWrapper{
			onlineClient: yaklib.OnlineClient{},
		}

		req := &ypb.DownloadHotPatchTemplateRequest{
			Token: token, // 添加token到请求
			Name:  name,
			Type:  templateType,
		}

		resp, err := server.DownloadHotPatchTemplate(context.Background(), req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
	})
}
