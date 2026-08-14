package yakit

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestQueryPayloadAcceptsNilPaging(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.Payload{}).Error)

	pagination, payloads, err := QueryPayload(db, "", "", "", nil)
	require.NoError(t, err)
	require.Empty(t, payloads)
	require.Equal(t, 1, pagination.Page)
	require.Equal(t, 30, pagination.Limit)
}
