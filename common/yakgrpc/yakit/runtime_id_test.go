package yakit

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

func TestUsefulRuntimeId(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	require.NotNil(t, db)
	require.NoError(t, db.AutoMigrate(&schema.Risk{}, &schema.HTTPFlow{}).Error)

	riskRuntimeID := "useful-risk-" + uuid.NewString()
	httpFlowRuntimeID := "useful-httpflow-" + uuid.NewString()

	defer func() {
		require.NoError(t, db.Unscoped().Where("runtime_id IN (?)", []string{riskRuntimeID}).Delete(&schema.Risk{}).Error)
		require.NoError(t, db.Unscoped().Where("runtime_id IN (?)", []string{httpFlowRuntimeID}).Delete(&schema.HTTPFlow{}).Error)
	}()

	useful, err := UsefulRuntimeId(db, "")
	require.Error(t, err)
	require.False(t, useful)

	useful, err = UsefulRuntimeId(db, "useful-missing-"+uuid.NewString())
	require.NoError(t, err)
	require.False(t, useful)

	require.NoError(t, db.Create(&schema.Risk{
		Hash:      uuid.NewString(),
		RuntimeId: riskRuntimeID,
	}).Error)
	useful, err = UsefulRuntimeId(db, riskRuntimeID)
	require.NoError(t, err)
	require.True(t, useful)

	require.NoError(t, db.Create(&schema.HTTPFlow{
		Hash:      uuid.NewString(),
		Url:       "https://example.com/" + uuid.NewString(),
		RuntimeId: httpFlowRuntimeID,
	}).Error)
	useful, err = UsefulRuntimeId(db, httpFlowRuntimeID)
	require.NoError(t, err)
	require.True(t, useful)

}
