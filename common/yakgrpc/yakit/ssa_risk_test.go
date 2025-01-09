package yakit

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestSSARisk_CURD(t *testing.T) {
	db := ssadb.GetDB()
	programNameToken := utils.RandStringBytes(10)
	riskType1 := utils.RandStringBytes(10)
	riskType2 := utils.RandStringBytes(10)
	for i := 0; i < 5; i++ {
		err := CreateSSARisk(db, &schema.SSARisk{
			ProgramName: programNameToken,
			RiskType:    riskType1,
		})
		require.NoError(t, err)
	}

	for i := 0; i < 5; i++ {
		err := CreateSSARisk(db, &schema.SSARisk{
			ProgramName: programNameToken,
			RiskType:    riskType2,
		})
		require.NoError(t, err)
	}

	t.Cleanup(func() {
		DeleteSSARisks(db, &ypb.SSARisksFilter{ProgramName: []string{programNameToken}})
	})

	_, risks, err := QuerySSARisk(db, &ypb.SSARisksFilter{ProgramName: []string{programNameToken}}, nil)
	require.NoError(t, err)
	require.Len(t, risks, 10)

	_, risks, err = QuerySSARisk(db, &ypb.SSARisksFilter{RiskType: []string{riskType1}}, nil)
	require.NoError(t, err)
	require.Len(t, risks, 5)

	tagTestRisk := risks[0]
	tagToken := utils.RandStringBytes(10)
	err = UpdateSSARiskTags(db, int64(tagTestRisk.ID), []string{tagToken})
	require.NoError(t, err)

	newRisk, err := GetSSARiskByID(db, int64(tagTestRisk.ID))
	require.NoError(t, err)
	require.Equal(t, tagToken, newRisk.Tags)
}

func TestSSARisk_GroupCount(t *testing.T) {
	db, err := consts.GetTempSSADataBase()
	require.NoError(t, err)

	programNameToken1 := utils.RandStringBytes(10)
	riskTypeToken1 := utils.RandStringBytes(10)
	severityToke1 := utils.RandStringBytes(10)
	for i := 0; i < 10; i++ {
		err := CreateSSARisk(db, &schema.SSARisk{
			ProgramName: programNameToken1,
			RiskType:    riskTypeToken1,
			Severity:    severityToke1,
		})
		require.NoError(t, err)
	}

	t.Cleanup(func() {
		DeleteSSARisks(db, &ypb.SSARisksFilter{ProgramName: []string{programNameToken1}})
	})
	programNameToken2 := utils.RandStringBytes(10)
	riskTypeToken2 := utils.RandStringBytes(10)
	severityToke2 := utils.RandStringBytes(10)
	for i := 0; i < 10; i++ {
		err := CreateSSARisk(db, &schema.SSARisk{
			ProgramName: programNameToken2,
			RiskType:    riskTypeToken2,
			Severity:    severityToke2,
		})
		require.NoError(t, err)
	}

	t.Cleanup(func() {
		DeleteSSARisks(db, &ypb.SSARisksFilter{ProgramName: []string{programNameToken2}})
	})

	check := func(fieldGroup []*ypb.FieldGroup) {
		require.Len(t, fieldGroup, 2)
		for i := 0; i < 2; i++ {
			require.Equal(t, int(fieldGroup[i].Total), 10)
		}
	}

	check(SSARiskColumnGroupCount(db, "program_name"))
	check(SSARiskColumnGroupCount(db, "risk_type"))
	check(SSARiskColumnGroupCount(db, "severity"))

}
