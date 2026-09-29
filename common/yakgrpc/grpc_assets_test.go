package yakgrpc

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
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

	var gotRiskReq struct {
		Hash            []string `json:"hash"`
		SetTags         []string `json:"setTags"`
		VerifierUid     string   `json:"verifierUid"`
		FixTime         int64    `json:"fixTime"`
		FixSuggestion   string   `json:"fixSuggestion"`
		TagReason       string   `json:"tagReason"`
		RiskTypeVerbose string   `json:"riskTypeVerbose"`
		SetSeverity     string   `json:"setSeverity"`
		SeverityScore   float64  `json:"severityScore"`
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/set/risk/tags", r.URL.Path)
		require.Equal(t, "test-token", r.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotRiskReq))
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()
	oldBase := consts.GetOnlineBaseUrl()
	consts.SetOnlineBaseUrl(ts.URL)
	t.Cleanup(func() { consts.SetOnlineBaseUrl(oldBase) })

	server := &Server{}

	req := &ypb.BatchSetRiskTagsRequest{
		Hashes:          []string{risk.Hash},
		SetTags:         []string{"confirmed", "verified"},
		VerifierUid:     "123456",
		FixTime:         1757174400,
		FixSuggestion:   "upgrade dependency",
		TagReason:       "disposal done",
		RiskTypeVerbose: "ssrf-patched",
		SetSeverity:     "critical",
		SeverityScore:   9.5,
		Token:           "test-token",
	}

	resp, err := server.BatchSetRiskTags(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Greater(t, resp.UpdatedCount, int64(0))

	assert.NotEmpty(t, gotRiskReq.Hash)
	assert.Contains(t, gotRiskReq.Hash, risk.Hash)
	assert.Equal(t, []string{"confirmed", "verified"}, gotRiskReq.SetTags)
	assert.Equal(t, "123456", gotRiskReq.VerifierUid)
	assert.Equal(t, "upgrade dependency", gotRiskReq.FixSuggestion)
	assert.Equal(t, "disposal done", gotRiskReq.TagReason)
	assert.Equal(t, "ssrf-patched", gotRiskReq.RiskTypeVerbose)
	assert.Equal(t, "critical", gotRiskReq.SetSeverity)
	assert.EqualValues(t, 1757174400, gotRiskReq.FixTime)
	assert.Equal(t, 9.5, gotRiskReq.SeverityScore)

	updated, err := yakit.GetRiskByIDOrHash(db, 0, risk.Hash)
	require.NoError(t, err)
	assert.Equal(t, "confirmed|verified", updated.Tags)
	assert.Equal(t, "123456", updated.VerifierUid)
	assert.True(t, updated.FixTime.Equal(time.Unix(1757174400, 0)))
	assert.Equal(t, "upgrade dependency", updated.FixSuggestion)
	assert.Equal(t, "disposal done", updated.TagReason)
	assert.Equal(t, "ssrf-patched", updated.RiskTypeVerbose)
	assert.Equal(t, "critical", updated.Severity)
	assert.Equal(t, 9.5, updated.SeverityScore)
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

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/risk/download", r.URL.Path)
		require.Equal(t, "test-token", r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"pagemeta": map[string]any{"page": 1, "total": 2, "total_page": 1, "limit": 30},
			"data": []map[string]any{
				{
					"hash":            existingRisk.Hash,
					"title":           existingRisk.Title,
					"tags":            "new-tag",
					"verifierUid":     "123456",
					"fixTime":         1757174400,
					"fixSuggestion":   "upgrade dependency",
					"tagReason":       "verified and fixed",
					"severity":        "critical",
					"riskTypeVerbose": "ssrf-confirmed",
					"severityScore":   8.5,
				},
				{
					"hash":          newHash,
					"title":         "downloaded-risk-" + token,
					"severity":      "medium",
					"tags":          "downloaded",
					"severityScore": 5.0,
				},
			},
		})
	}))
	defer ts.Close()
	oldBase := consts.GetOnlineBaseUrl()
	consts.SetOnlineBaseUrl(ts.URL)
	t.Cleanup(func() { consts.SetOnlineBaseUrl(oldBase) })

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

	updated, err := yakit.GetRiskByIDOrHash(db, 0, existingRisk.Hash)
	require.NoError(t, err)
	assert.Equal(t, "new-tag", updated.Tags)
	assert.Equal(t, "123456", updated.VerifierUid)
	assert.True(t, updated.FixTime.Equal(time.Unix(1757174400, 0)))
	assert.Equal(t, "upgrade dependency", updated.FixSuggestion)
	assert.Equal(t, "verified and fixed", updated.TagReason)
	assert.Equal(t, "critical", updated.Severity)
	assert.Equal(t, "ssrf-confirmed", updated.RiskTypeVerbose)
	assert.Equal(t, 8.5, updated.SeverityScore)

	newRisk, err := yakit.GetRiskByIDOrHash(db, 0, newHash)
	require.NoError(t, err)
	assert.Equal(t, "downloaded-risk-"+token, newRisk.Title)
	assert.Equal(t, "medium", newRisk.Severity)
	assert.Equal(t, "downloaded", newRisk.Tags)
	assert.Equal(t, 5.0, newRisk.SeverityScore)

	yakit.DeleteRiskByID(db, int64(newRisk.ID))
}
