package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/bytedance/mockey"
	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
	"testing"
)

func TestUploadPayloadToOnline(t *testing.T) {
	mockey.PatchConvey("Test UploadPayloadToOnline", t, func() {
		defer mockey.UnPatchAll()

		server := &Server{}

		mockPayloads := []*schema.Payload{
			{
				Group:   "test-group",
				Folder:  pointer("test-folder"),
				Content: pointer("test-content-path"),
				IsFile:  pointer(true),
			},
		}

		mockey.Mock((*Server).GetProfileDatabase).Return(&gorm.DB{}).Build()

		mockey.Mock(bizhelper.ExactQueryString).To(func(db *gorm.DB, key, value string) *gorm.DB {
			return db
		}).Build()

		mockey.Mock((*gorm.DB).Find).To(func(_ *gorm.DB, dest interface{}, conds ...interface{}) *gorm.DB {
			ptr := dest.(*[]*schema.Payload)
			*ptr = mockPayloads
			return &gorm.DB{}
		}).Build()

		mockey.Mock(GetPayloadFile).To(func(ctx context.Context, path string) ([]byte, bool, error) {
			assert.Equal(t, path, "test-content-path")
			return []byte("mock file content"), false, nil
		}).Build()

		mockey.Mock((*yaklib.OnlineClient).UploadPayloadsToOnline).To(func(ctx context.Context, token string, data, fileContent []byte) error {
			assert.Equal(t, token, "fake-token")
			assert.Equal(t, string(fileContent), "mock file content")

			var p schema.Payload
			err := json.Unmarshal(data, &p)
			assert.NoError(t, err)
			assert.Equal(t, p.Group, "test-group")
			return nil
		}).Build()

		stream := &fakeUploadStream{}

		req := &ypb.UploadPayloadToOnlineRequest{
			Token:  "fake-token",
			Group:  "test-group",
			Folder: "test-folder",
		}

		err := server.UploadPayloadToOnline(req, stream)
		assert.NoError(t, err)

		assert.NotEmpty(t, stream.SendMessages)
	})
}

func pointer[T any](v T) *T {
	return &v
}

type fakeUploadStream struct {
	grpc.ServerStream
	SendMessages []*ypb.DownloadProgress
}

func (f *fakeUploadStream) Send(resp *ypb.DownloadProgress) error {
	f.SendMessages = append(f.SendMessages, resp)
	fmt.Printf("Progress: %.2f, Message: %s [%s]\n", resp.Progress, resp.Message, resp.MessageType)
	return nil
}

func (f *fakeUploadStream) Context() context.Context {
	return context.Background()
}

func TestDownloadPayload(t *testing.T) {
	mockey.PatchConvey("Test DownloadPayload", t, func() {
		defer mockey.UnPatchAll()

		server := &Server{}

		// 模拟下载到的 payload 数据
		mockPayload := &yaklib.OnlinePayload{
			Group:       "test-group",
			Folder:      "test-folder",
			Content:     "test content",
			FileContent: nil,
			IsFile:      false,
			Hash:        "mockhash",
		}

		ch := make(chan *yaklib.OnlinePayloadItem, 1)
		ch <- &yaklib.OnlinePayloadItem{
			PayloadData: mockPayload,
			Total:       1,
		}
		close(ch)

		mockey.Mock((*yaklib.OnlineClient).DownloadBatchPayloads).To(func(ctx context.Context, token, group, folder string) *yaklib.OnlineDownloadPayloadStream {
			return &yaklib.OnlineDownloadPayloadStream{
				Chan:  ch,
				Total: 1,
			}
		}).Build()

		mockey.Mock((*yaklib.OnlineClient).SavePayload).To(func(db *gorm.DB, payload ...*yaklib.OnlinePayload) error {
			assert.Equal(t, "test-group", payload[0].Group)
			assert.Equal(t, "test-folder", payload[0].Folder)
			return nil
		}).Build()

		stream := &fakeDownloadStream{
			SendMessages: make([]*ypb.DownloadProgress, 0),
		}

		req := &ypb.DownloadPayloadRequest{
			Token:  "fake-token",
			Group:  "test-group",
			Folder: "test-folder",
		}

		err := server.DownloadPayload(req, stream)
		assert.NoError(t, err)

		assert := assert.New(t)
		assert.GreaterOrEqual(len(stream.SendMessages), 2)

		assert.Contains(stream.SendMessages[0].Message, "开始下载payload")
		assert.Contains(stream.SendMessages[1].Message, "保存成功")
		assert.Equal("success", stream.SendMessages[1].MessageType)
	})
}

type fakeDownloadStream struct {
	grpc.ServerStream
	SendMessages []*ypb.DownloadProgress
}

func (f *fakeDownloadStream) Send(resp *ypb.DownloadProgress) error {
	f.SendMessages = append(f.SendMessages, resp)
	fmt.Printf("Progress: %.2f, Message: %s [%s]\n", resp.Progress, resp.Message, resp.MessageType)
	return nil
}

func (f *fakeDownloadStream) Context() context.Context {
	return context.Background()
}
