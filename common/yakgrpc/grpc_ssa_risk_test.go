package yakgrpc

import (
	"context"
	"testing"

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
