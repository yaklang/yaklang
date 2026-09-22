package yakit

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestCreateSSARisk_LaterModeCoversEarlierInSameScan(t *testing.T) {
	db := ssadb.GetDB()
	require.NotNil(t, db)
	program := "cover-" + utils.RandStringBytes(8)
	runtime := "runtime-" + utils.RandStringBytes(8)
	t.Cleanup(func() {
		_ = DeleteSSARisks(db, &ypb.SSARisksFilter{
			ProgramName: []string{program},
			RuntimeID:   []string{runtime},
		})
	})

	require.NoError(t, CreateSSARisk(db, &schema.SSARisk{
		Title:           "source floor",
		ProgramName:     program,
		RuntimeId:       runtime,
		ScanMode:        "source",
		RiskType:        "sql-injection",
		RiskFeatureHash: "feature-mysql",
		CodeSourceUrl:   "a.php",
		Line:            4,
		FromRule:        "source-php-mysql-query",
		Variable:        "hit",
		Index:           0,
	}))
	require.NoError(t, CreateSSARisk(db, &schema.SSARisk{
		Title:           "ssa precise",
		ProgramName:     program,
		RuntimeId:       runtime,
		ScanMode:        "ssa",
		RiskType:        "sql-injection",
		RiskFeatureHash: "feature-mysql",
		CodeSourceUrl:   "a.php",
		Line:            4,
		FromRule:        "ssa-php-mysql-query",
		Variable:        "call",
		Index:           0,
	}))

	_, risks, err := QuerySSARisk(db, &ypb.SSARisksFilter{
		ProgramName: []string{program},
		RuntimeID:   []string{runtime},
	}, nil)
	require.NoError(t, err)
	require.Len(t, risks, 1)
	require.Equal(t, "ssa", risks[0].ScanMode)
	require.Equal(t, "ssa precise", risks[0].Title)
}

func TestCreateSSARisk_EarlierModeDoesNotCoverLater(t *testing.T) {
	db := ssadb.GetDB()
	require.NotNil(t, db)
	program := "cover-back-" + utils.RandStringBytes(8)
	runtime := "runtime-" + utils.RandStringBytes(8)
	t.Cleanup(func() {
		_ = DeleteSSARisks(db, &ypb.SSARisksFilter{
			ProgramName: []string{program},
			RuntimeID:   []string{runtime},
		})
	})

	require.NoError(t, CreateSSARisk(db, &schema.SSARisk{
		Title:           "ssa precise",
		ProgramName:     program,
		RuntimeId:       runtime,
		ScanMode:        "ssa",
		RiskType:        "sql-injection",
		RiskFeatureHash: "feature-mysql",
		CodeSourceUrl:   "a.php",
		Line:            4,
		FromRule:        "ssa-php-mysql-query",
		Index:           1,
	}))
	require.NoError(t, CreateSSARisk(db, &schema.SSARisk{
		Title:           "source floor",
		ProgramName:     program,
		RuntimeId:       runtime,
		ScanMode:        "source",
		RiskType:        "sql-injection",
		RiskFeatureHash: "feature-mysql",
		CodeSourceUrl:   "a.php",
		Line:            4,
		FromRule:        "source-php-mysql-query",
		Index:           2,
	}))

	_, risks, err := QuerySSARisk(db, &ypb.SSARisksFilter{
		ProgramName: []string{program},
		RuntimeID:   []string{runtime},
	}, nil)
	require.NoError(t, err)
	require.Len(t, risks, 1)
	require.Equal(t, "ssa", risks[0].ScanMode)
}

func TestCreateSSARisk_OtherScanIsNotCovered(t *testing.T) {
	db := ssadb.GetDB()
	require.NotNil(t, db)
	program := "cover-other-" + utils.RandStringBytes(8)
	t.Cleanup(func() {
		_ = DeleteSSARisks(db, &ypb.SSARisksFilter{ProgramName: []string{program}})
	})

	require.NoError(t, CreateSSARisk(db, &schema.SSARisk{
		Title:           "old scan",
		ProgramName:     program,
		RuntimeId:       "scan-a",
		ScanMode:        "source",
		RiskType:        "sql-injection",
		RiskFeatureHash: "feature-mysql",
		CodeSourceUrl:   "a.php",
		Line:            4,
		Index:           1,
	}))
	require.NoError(t, CreateSSARisk(db, &schema.SSARisk{
		Title:           "new scan",
		ProgramName:     program,
		RuntimeId:       "scan-b",
		ScanMode:        "ssa",
		RiskType:        "sql-injection",
		RiskFeatureHash: "feature-mysql",
		CodeSourceUrl:   "a.php",
		Line:            4,
		Index:           2,
	}))

	_, risks, err := QuerySSARisk(db, &ypb.SSARisksFilter{ProgramName: []string{program}}, nil)
	require.NoError(t, err)
	require.Len(t, risks, 2)
}
