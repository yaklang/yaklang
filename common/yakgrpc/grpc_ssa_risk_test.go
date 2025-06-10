package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/yaklib"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGetSSARiskFieldGroup(t *testing.T) {

	local, err := NewLocalClient(true)
	require.NoError(t, err)

	taskID := uuid.NewString()

	defer func() {
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{
			RuntimeID: []string{taskID},
		})
	}()
	createRisk := func(filePath, serverity, risk_type string) {
		yakit.CreateSSARisk(ssadb.GetDB(), &schema.SSARisk{
			CodeSourceUrl: filePath,
			Severity:      schema.ValidSeverityType(serverity),
			RiskType:      risk_type,
			RuntimeId:     taskID,
		})
	}
	riskType1 := "risk1"
	riskType2 := "risk2"

	createRisk("ssadb://prog1/1", "high", riskType1)
	createRisk("ssadb://prog1/1", "high", riskType2)
	createRisk("ssadb://prog1/1", "low", riskType1)
	createRisk("ssadb://prog2/22", "low", riskType1)
	createRisk("ssadb://prog2/22", "low", riskType2)

	fgs, err := local.GetSSARiskFieldGroup(context.Background(), &ypb.Empty{})
	require.NoError(t, err)
	log.Infof("fgs: %v", fgs)
	// fgs.RiskTypeField
	tmp := make(map[string]struct{})
	checkField := func(fields []*ypb.FieldName) {
		for _, field := range fields {
			// check empty
			if field.Verbose == "" {
				require.Fail(t, "empty verbose")
			}

			// check total
			if field.Total == 0 {
				require.Fail(t, "empty total")
			}

			// check duplicate
			if _, ok := tmp[field.Verbose]; ok {
				require.Fail(t, "duplicate severity")
			} else {
				tmp[field.Verbose] = struct{}{}
			}
		}
	}

	checkField(fgs.SeverityField)
	checkField(fgs.RiskTypeField)

	checkFieldGroup := func(fields []*ypb.FieldGroup) {
		tmp := make(map[string]struct{})
		for _, field := range fields {
			if field.Name == "" {
				require.Fail(t, "empty name")
			}
			if field.Total == 0 {
				require.Fail(t, "empty total")
			}
			if _, ok := tmp[field.Name]; ok {
				require.Fail(t, "duplicate name")
			} else {
				tmp[field.Name] = struct{}{}
			}
		}
	}
	checkFieldGroup(fgs.FileField)
}
func TestSSARisk_MarkRead(t *testing.T) {
	taskID := uuid.NewString()

	defer func() {
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{
			RuntimeID: []string{taskID},
		})
	}()
	createRisk := func(filePath, serverity, risk_type string) {
		yakit.CreateSSARisk(ssadb.GetDB(), &schema.SSARisk{
			CodeSourceUrl: filePath,
			Severity:      schema.ValidSeverityType(serverity),
			RiskType:      risk_type,
			RuntimeId:     taskID,
		})
	}
	riskType1 := "risk1"
	riskType2 := "risk2"

	createRisk("ssadb://prog1/1", "high", riskType1)
	createRisk("ssadb://prog1/1", "high", riskType2)
	createRisk("ssadb://prog1/1", "low", riskType1)
	createRisk("ssadb://prog2/22", "low", riskType1)
	createRisk("ssadb://prog2/22", "low", riskType2)

	check := func(items []*schema.SSARisk, want bool) {
		for _, item := range items {
			require.Equal(t, item.IsRead, want)
		}
	}

	{
		// create risk and qurey, this data is un-read
		_, data, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
			RuntimeID: []string{taskID},
		}, nil)
		require.NoError(t, err)
		check(data, false)
	}

	{
		err := yakit.NewSSARiskReadRequest(ssadb.GetDB(), &ypb.SSARisksFilter{
			RiskType: []string{riskType1},
		})
		require.NoError(t, err)
	}
	{
		// check data
		_, data, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
			RiskType: []string{riskType1},
		}, nil)
		require.NoError(t, err)
		check(data, true)
	}
	{
		// risktype2 should unread
		_, data, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
			RiskType: []string{riskType2},
		}, nil)
		require.NoError(t, err)
		check(data, false)
	}
}

func TestSSARisk_NewSSARisk(t *testing.T) {
	client, err := NewLocalClient(true) // use yakit handler local database, this test-case should use local grpc
	require.NoError(t, err)

	// create risk
	newrisk1 := uuid.NewString()
	yakit.CreateSSARisk(ssadb.GetDB(), &schema.SSARisk{
		Title: newrisk1,
	})
	defer func() {
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{
			Title: newrisk1,
		})
	}()

	response, err := client.QuerySSARisks(context.Background(), &ypb.QuerySSARisksRequest{
		Filter: &ypb.SSARisksFilter{},
		Pagination: &ypb.Paging{
			Limit:   1,
			Page:    1,
			Order:   "desc",
			OrderBy: "id",
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(response.Data))
	start := response.Data[0]
	require.NotNil(t, start)
	startId := start.GetId()
	require.Greater(t, startId, int64(0))
	total := response.GetTotal()
	_ = total

	// test new risk
	t.Run("test new risk", func(t *testing.T) {
		newrisk1 := uuid.NewString()
		yakit.CreateSSARisk(ssadb.GetDB(), &schema.SSARisk{
			Title: newrisk1,
		})
		defer func() {
			yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{
				Title: newrisk1,
			})
		}()

		response, err := client.QueryNewSSARisks(context.Background(), &ypb.QueryNewSSARisksRequest{
			AfterID: startId,
		})
		require.NoError(t, err)

		// check new ssa-risk
		require.Equal(t, 1, len(response.Data))
		require.Equal(t, newrisk1, response.Data[0].GetTitle())
		// check new ssa-risk count
		require.Equal(t, int64(1), response.NewRiskTotal)

		// check total
		require.Equal(t, total+1, response.GetTotal())
	})

	t.Run("test new risk with read", func(t *testing.T) {
		newrisk1 := uuid.NewString()
		yakit.CreateSSARisk(ssadb.GetDB(), &schema.SSARisk{
			Title:  newrisk1,
			IsRead: true,
		})
		defer func() {
			yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{
				Title: newrisk1,
			})
		}()

		response, err := client.QueryNewSSARisks(context.Background(), &ypb.QueryNewSSARisksRequest{
			AfterID: startId,
		})
		require.NoError(t, err)

		// check new ssa-risk
		require.Equal(t, 0, len(response.Data))
		// check new ssa-risk count
		require.Equal(t, int64(0), response.NewRiskTotal)

		// check total
		require.Equal(t, total+1, response.GetTotal())
	})

	t.Run("test risk data max is 5", func(t *testing.T) {
		for i := 0; i < 6; i++ {
			newrisk1 := uuid.NewString()
			yakit.CreateSSARisk(ssadb.GetDB(), &schema.SSARisk{
				Title: newrisk1,
			})
			defer func() {
				yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{
					Title: newrisk1,
				})
			}()
		}

		response, err := client.QueryNewSSARisks(context.Background(), &ypb.QueryNewSSARisksRequest{
			AfterID: startId,
		})
		require.NoError(t, err)

		// check new ssa-risk
		require.Equal(t, 5, len(response.Data))
		// check new ssa-risk count
		require.Equal(t, int64(6), response.NewRiskTotal)

		// check total
		require.Equal(t, total+6, response.GetTotal())
	})

}

func TestSSARiskFeedbackToOnline(t *testing.T) {
	taskID := uuid.NewString()

	defer func() {
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{
			RuntimeID: []string{taskID},
		})
	}()
	createRisk := func(filePath, serverity, risk_type string) {
		yakit.CreateSSARisk(ssadb.GetDB(), &schema.SSARisk{
			CodeSourceUrl: filePath,
			Severity:      schema.ValidSeverityType(serverity),
			RiskType:      risk_type,
			RuntimeId:     taskID,
		})
	}
	createRisk("ssadb://prog1/1", "high", "risk1")
	createRisk("ssadb://prog1/1", "low", "risk2")

	checkCount := func(items chan *schema.SSARisk, expectedCount int) {
		var results []*schema.SSARisk
		for item := range items {
			results = append(results, item)
		}
		fmt.Printf("Results: %+v\n", len(results))

		require.Len(t, results, expectedCount)
	}

	{
		data := yakit.YieldSSARisk(ssadb.GetDB(), context.Background())
		checkCount(data, 2)
	}

	mockey.PatchConvey("Test SSARiskFeedbackToOnline", t, func() {
		token := "valid_token"
		req := &ypb.SSARiskFeedbackToOnlineRequest{
			Token:  token,
			Filter: &ypb.SSARisksFilter{},
		}

		mockey.Mock(yakit.FilterSSARisk).To(func(db *gorm.DB, filter *ypb.SSARisksFilter) *gorm.DB {
			return db
		}).Build()

		mockey.Mock((*yaklib.OnlineClient).UploadToOnline).To(func(ctx context.Context, token string, raw []byte, urlStr string) error {
			assert.Equal(t, token, "valid_token")

			var reqBody yaklib.UploadOnlineRequest
			err := json.Unmarshal(raw, &reqBody)
			assert.NoError(t, err)
			assert.NotNil(t, reqBody)
			return nil
		}).Build()

		server := &TestServerWrapper{
			onlineClient: yaklib.OnlineClient{},
		}

		resp, err := server.SSARiskFeedbackToOnline(context.Background(), req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
	})

}
