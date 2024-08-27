package yakgrpc

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/utils/multipart"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//go:embed grpc_mitm_upload_test_embed_file.jpg
var embedJPEG []byte

func TestMITM_UploadFile(t *testing.T) {
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		return []byte("HTTP/1.1 200 OK\n\n")
	})
	target := utils.HostPort(host, port)

	mitmPort := 0

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = cancel
	hash1 := codec.Md5(string(embedJPEG))
	uid := uuid.New().String()
	NewMITMTestCase(t,
		CaseWithContext(ctx),
		CaseWithPort(func(i int) {
			mitmPort = i
		}),
		CaseWithServerStart(func() {
			rsp, _, err := poc.DoGET(
				`http://`+target+"/"+uid,
				poc.WithAppendHttpPacketUploadFile("file", "test.jpg", string(embedJPEG), "text/plain", "test"),
				poc.WithProxy("http://127.0.0.1:"+fmt.Sprint(mitmPort)), poc.WithSave(false),
			)
			if err != nil {
				t.Fatal(err)
			}
			_, reqBody := lowhttp.SplitHTTPPacketFast(rsp.RawRequest)
			reader := multipart.NewReader(bytes.NewReader(reqBody))
			for {
				part, err := reader.NextPart()
				if err != nil {
					break
				}
				fileContentRequest, _ := io.ReadAll(part)
				if len(fileContentRequest) <= 0 {
					continue
				}
				if ret := codec.Md5(string(fileContentRequest)); ret != hash1 {
					fmt.Println("origin  len: ", len(embedJPEG), "hash", hash1)
					fmt.Println("request len: ", len(fileContentRequest), "hash", ret)
					t.Fatal("build packet error")
				}
			}
			log.Info("Start to check request in table")
			flowMsg, err := QueryHTTPFlows(utils.TimeoutContextSeconds(2), localClient, &ypb.QueryHTTPFlowRequest{Keyword: uid, SourceType: "mitm"}, 1)
			require.NoError(t, err)
			flow := flowMsg.Data[0]
			log.Info("check flow in mitm")
			_, reqBody = lowhttp.SplitHTTPPacketFast(flow.Request)
			for {
				part, err := reader.NextPart()
				if err != nil {
					break
				}
				fileContentRequest, _ := io.ReadAll(part)
				if len(fileContentRequest) <= 0 {
					continue
				}
				if ret := codec.Md5(string(fileContentRequest)); ret != hash1 {
					fmt.Println("origin       len: ", len(embedJPEG), "hash", hash1)
					fmt.Println("mitm request len: ", len(fileContentRequest), "hash", ret)
					t.Fatal("build packet error")
				}
			}
			log.Info("finished")
			cancel()
		}))
}
