package yakgrpc

import (
	"context"
	"github.com/bytedance/mockey"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"math/rand"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_HTTPFuzzer_DeleteHistory(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	targetHost, targetPort := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("Hello"))
	})

	checkFuzzerTask := func(t *testing.T, ctx context.Context, fuzzerTabIndex string, wantNum int) {
		t.Helper()
		var queryFuzzerTaskRsp *ypb.HistoryHTTPFuzzerTasksResponse
		var err error
		err = utils.AttemptWithDelayFast(func() error {
			queryFuzzerTaskRsp, err = c.QueryHistoryHTTPFuzzerTaskEx(ctx, &ypb.QueryHistoryHTTPFuzzerTaskExParams{
				FuzzerTabIndex: fuzzerTabIndex,
				Pagination: &ypb.Paging{
					Page:  1,
					Limit: 10,
				},
			})
			if err != nil {
				return err
			}
			if int(queryFuzzerTaskRsp.Total) != wantNum {
				return utils.Errorf("want %d, got %d", wantNum, int(queryFuzzerTaskRsp.Total))
			}
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, wantNum, int(queryFuzzerTaskRsp.Total))
	}

	checkFuzzerResponse := func(t *testing.T, ctx context.Context, taskID int64, wantNum int) {
		t.Helper()

		var queryFuzzerResponseRsp *ypb.QueryHTTPFuzzerResponseByTaskIdResponse
		var err error
		err = utils.AttemptWithDelayFast(func() error {
			queryFuzzerResponseRsp, err = c.QueryHTTPFuzzerResponseByTaskId(ctx, &ypb.QueryHTTPFuzzerResponseByTaskIdRequest{
				TaskId: taskID,
				Pagination: &ypb.Paging{
					Page:  1,
					Limit: 10,
				},
			})
			if err != nil {
				return err
			}
			if int(queryFuzzerResponseRsp.Total) != wantNum {
				return utils.Errorf("want %d, got %d", wantNum, int(queryFuzzerResponseRsp.Total))
			}
			return nil
		})

		require.NoError(t, err)
		require.Equal(t, wantNum, int(queryFuzzerResponseRsp.Total))
	}

	t.Run("id", func(t *testing.T) {
		fuzzerTabIndex := uuid.NewString()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client, err := c.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
			Request: `GET / HTTP/1.1
Host: ` + utils.HostPort(targetHost, targetPort) + `
`,
			FuzzerTabIndex: fuzzerTabIndex,
		})
		require.NoError(t, err)

		var taskID int64 = 0
		for {
			rsp, err := client.Recv()
			if err != nil {
				break
			}
			if taskID == 0 {
				taskID = rsp.GetTaskId()
			}
		}

		require.NotEqual(t, taskID, 0, "No Response")

		// before delete
		checkFuzzerTask(t, ctx, fuzzerTabIndex, 1)
		checkFuzzerResponse(t, ctx, taskID, 1)

		// delete
		c.DeleteHistoryHTTPFuzzerTask(ctx, &ypb.DeleteHistoryHTTPFuzzerTaskRequest{
			Id: int32(taskID),
		})
		// check
		checkFuzzerTask(t, ctx, fuzzerTabIndex, 0)
		checkFuzzerResponse(t, ctx, taskID, 0)
	})

	t.Run("fuzzer index", func(t *testing.T) {
		fuzzerTabIndex := uuid.NewString()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client, err := c.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
			Request: `GET /?c={{int(1-10)}} HTTP/1.1
Host: ` + utils.HostPort(targetHost, targetPort) + `
`,
			FuzzerTabIndex: fuzzerTabIndex,
			ForceFuzz:      true,
			Concurrent:     10,
		})
		require.NoError(t, err)

		var taskID int64 = 0
		for {
			rsp, err := client.Recv()
			if err != nil {
				break
			}
			if taskID == 0 {
				taskID = rsp.GetTaskId()
			}
		}

		require.NotEqual(t, taskID, 0, "No Response")

		// before delete
		checkFuzzerTask(t, ctx, fuzzerTabIndex, 1)
		checkFuzzerResponse(t, ctx, taskID, 10)

		// delete
		c.DeleteHistoryHTTPFuzzerTask(ctx, &ypb.DeleteHistoryHTTPFuzzerTaskRequest{
			WebFuzzerIndex: fuzzerTabIndex,
		})
		// check
		checkFuzzerTask(t, ctx, fuzzerTabIndex, 0)
		checkFuzzerResponse(t, ctx, taskID, 0)
	})
}

func TestSetTagForRisk(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	r := yakit.CreateRisk("http://127.0.0.1")
	err = yakit.SaveRisk(r)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.SetTagForRisk(context.Background(), &ypb.SetTagForRiskRequest{
		Hash: r.Hash,
		Tags: []string{"误报, 忽略, 待处理"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestQueryRiskTags(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.QueryRiskTags(context.Background(), &ypb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRiskFieldGroup(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.RiskFieldGroup(context.Background(), &ypb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestQueryRisks(t *testing.T) {
	testRuntimeId := uuid.New().String()
	randInt := rand.Intn(10) + 1
	for i := 0; i < randInt; i++ {
		err := yakit.SaveRisk(&schema.Risk{
			RuntimeId: testRuntimeId,
		})
		require.NoError(t, err)
	}
	defer func() {
		yakit.DeleteRisk(consts.GetGormProjectDatabase(), &ypb.QueryRisksRequest{
			RuntimeId: testRuntimeId,
		})
	}()
	client, err := NewLocalClient()
	require.NoError(t, err)
	res, err := client.QueryRisks(context.Background(), &ypb.QueryRisksRequest{
		RuntimeId: testRuntimeId,
	})
	require.NoError(t, err)
	require.Equal(t, int64(randInt), res.Total)
}

func TestQueryRisksWithRuntimeIds(t *testing.T) {
	testRuntimeId := uuid.New().String()
	testRuntimeId2 := uuid.New().String()
	randInt := rand.Intn(10) + 1
	for i := 0; i < randInt; i++ {
		err := yakit.SaveRisk(&schema.Risk{
			RuntimeId: testRuntimeId,
		})
		require.NoError(t, err)
	}

	for i := 0; i < randInt; i++ {
		err := yakit.SaveRisk(&schema.Risk{
			RuntimeId: testRuntimeId2,
		})
		require.NoError(t, err)
	}
	defer func() {
		yakit.DeleteRisk(consts.GetGormProjectDatabase(), &ypb.QueryRisksRequest{
			RuntimeId: testRuntimeId,
		})
		yakit.DeleteRisk(consts.GetGormProjectDatabase(), &ypb.QueryRisksRequest{
			RuntimeId: testRuntimeId2,
		})
	}()
	client, err := NewLocalClient()
	require.NoError(t, err)
	res, err := client.QueryRisks(context.Background(), &ypb.QueryRisksRequest{
		RuntimeIds: []string{testRuntimeId, testRuntimeId2},
	})
	require.NoError(t, err)
	require.Equal(t, int64(randInt)*2, res.Total)
}

func TestBatchSetRiskTags(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	token := utils.RandStringBytes(5)

	risk := &schema.Risk{
		Title:    "test-risk-" + token,
		Severity: "high",
		RiskType: "ssrf",
	}
	require.NoError(t, yakit.SaveRisk(risk))
	defer yakit.DeleteRiskByID(db, int64(risk.ID))

	mockey.PatchConvey("skip online sync, test local batch update", t, func() {
		mockClient := new(yaklib.OnlineClient)

		mockey.Mock((*yaklib.OnlineClient).SetRiskTagsToOnline).
			To(func(_ *yaklib.OnlineClient, ctx context.Context, token string, hashes []string, tags, tagsDescription, riskTypeVerbose, severity string, severityScore float64) error {
				assert.NotEmpty(t, hashes)
				assert.Equal(t, "confirmed|verified", tags)
				assert.Equal(t, `{"verifier":"admin"}`, tagsDescription)
				assert.Equal(t, "ssrf-patched", riskTypeVerbose)
				assert.Equal(t, "critical", severity)
				assert.Equal(t, 9.5, severityScore)
				return nil
			}).Build()

		mockey.Mock(yaklib.NewOnlineClient).
			To(func(baseUrl string) *yaklib.OnlineClient {
				return mockClient
			}).Build()

		server := &TestServerWrapper{
			Server:       &Server{},
			onlineClient: yaklib.OnlineClient{},
		}

		req := &ypb.BatchSetRiskTagsRequest{
			Hashes:          []string{risk.Hash},
			Tags:            "confirmed|verified",
			TagsDescription: `{"verifier":"admin"}`,
			RiskTypeVerbose: "ssrf-patched",
			Severity:        "critical",
			SeverityScore:   9.5,
			Token:           "test-token",
		}

		resp, err := server.BatchSetRiskTags(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Greater(t, resp.UpdatedCount, int64(0))

		updated, err := yakit.GetRiskByIDOrHash(db, 0, risk.Hash)
		require.NoError(t, err)
		assert.Equal(t, "confirmed|verified", updated.Tags)
		assert.Equal(t, `{"verifier":"admin"}`, updated.TagsDescription)
		assert.Equal(t, "ssrf-patched", updated.RiskTypeVerbose)
		assert.Equal(t, "critical", updated.Severity)
		assert.Equal(t, 9.5, updated.SeverityScore)
	})
}

func TestRisksFromOnline(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	token := utils.RandStringBytes(5)

	existingRisk := &schema.Risk{
		Title:    "existing-risk-" + token,
		Severity: "high",
		Tags:     "old-tag",
	}
	require.NoError(t, yakit.SaveRisk(existingRisk))
	defer yakit.DeleteRiskByID(db, int64(existingRisk.ID))

	newHash := "new-download-" + token
	mockItems := []*yaklib.DownloadRiskStreamItem{
		{
			Risk: &yaklib.DownloadRiskItem{
				Hash:            existingRisk.Hash,
				Title:           existingRisk.Title,
				Tags:            "new-tag",
				TagsDescription: `{"verifier":"admin"}`,
				Severity:        "critical",
				RiskTypeVerbose: "ssrf-confirmed",
				SeverityScore:   8.5,
			},
			Total: 2,
		},
		{
			Risk: &yaklib.DownloadRiskItem{
				Hash:          newHash,
				Title:         "downloaded-risk-" + token,
				Severity:      "medium",
				Tags:          "downloaded",
				SeverityScore: 5.0,
			},
			Total: 2,
		},
	}

	mockey.PatchConvey("mock download from online", t, func() {
		mockClient := new(yaklib.OnlineClient)

		mockey.Mock((*yaklib.OnlineClient).DownloadRisks).
			To(func(_ *yaklib.OnlineClient, ctx context.Context, tk string) (chan *yaklib.DownloadRiskStreamItem, error) {
				ch := make(chan *yaklib.DownloadRiskStreamItem, len(mockItems))
				for _, item := range mockItems {
					ch <- item
				}
				close(ch)
				return ch, nil
			}).Build()

		mockey.Mock(yaklib.NewOnlineClient).
			To(func(baseUrl string) *yaklib.OnlineClient {
				return mockClient
			}).Build()

		mockey.Mock(yaklib.DownloadOnlineAuthProxy).
			To(func(baseUrl string) error {
				return nil
			}).Build()

		client, err := NewLocalClient()
		require.NoError(t, err)

		stream, err := client.RisksFromOnline(context.Background(), &ypb.RisksFromOnlineRequest{
			Token: "test-token",
		})
		require.NoError(t, err)

		var progressLogs []string
		for {
			msg, err := stream.Recv()
			if err != nil {
				break
			}
			progressLogs = append(progressLogs, msg.Log)
		}

		foundUpdate := false
		foundInsert := false
		for _, logMsg := range progressLogs {
			if logMsg == "update ["+existingRisk.Hash+"] finished" {
				foundUpdate = true
			}
			if logMsg == "insert ["+newHash+"] finished" {
				foundInsert = true
			}
		}
		assert.True(t, foundUpdate, "should have updated existing risk")
		assert.True(t, foundInsert, "should have inserted new risk")

		// 验证已存在的 risk 字段已更新
		updated, err := yakit.GetRiskByIDOrHash(db, 0, existingRisk.Hash)
		require.NoError(t, err)
		assert.Equal(t, "new-tag", updated.Tags)
		assert.Equal(t, `{"verifier":"admin"}`, updated.TagsDescription)
		assert.Equal(t, "critical", updated.Severity)
		assert.Equal(t, "ssrf-confirmed", updated.RiskTypeVerbose)
		assert.Equal(t, 8.5, updated.SeverityScore)

		// 验证新 risk 已写入
		newRisk, err := yakit.GetRiskByIDOrHash(db, 0, newHash)
		require.NoError(t, err)
		assert.Equal(t, "downloaded-risk-"+token, newRisk.Title)
		assert.Equal(t, "medium", newRisk.Severity)
		assert.Equal(t, "downloaded", newRisk.Tags)
		assert.Equal(t, 5.0, newRisk.SeverityScore)

		// 清理新建的 risk
		yakit.DeleteRiskByID(db, int64(newRisk.ID))
	})
}
