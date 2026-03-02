package yakit

import (
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func setupAIProviderTestDB(t *testing.T) *gorm.DB {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AIThirdPartyConfig{}).Error)
	return db
}

func TestEnsureAIBalanceProviderConfig(t *testing.T) {
	db := setupAIProviderTestDB(t)
	defer db.Close()

	EnsureAIBalanceProviderConfig(db)
	providers, err := ListAIProviders(db)
	require.NoError(t, err)
	assert.Len(t, providers, 1)
	assert.Equal(t, "aibalance", providers[0].Type)

	// Idempotent
	EnsureAIBalanceProviderConfig(db)
	providers, err = ListAIProviders(db)
	require.NoError(t, err)
	assert.Len(t, providers, 1)
}

func TestQueryAIProviders(t *testing.T) {
	db := setupAIProviderTestDB(t)
	defer db.Close()

	p1 := &schema.AIThirdPartyConfig{Type: "openai", APIKey: "key-1", Domain: "api.openai.com"}
	p2 := &schema.AIThirdPartyConfig{Type: "azure", APIKey: "key-2", Domain: "azure.example.com"}
	p3 := &schema.AIThirdPartyConfig{Type: "openai", APIKey: "key-3", Domain: "api.openai.com"}

	require.NoError(t, CreateAIProvider(db, p1))
	require.NoError(t, CreateAIProvider(db, p2))
	require.NoError(t, CreateAIProvider(db, p3))

	pag, providers, err := QueryAIProviders(db, nil, &ypb.Paging{Page: 1, Limit: 2, OrderBy: "id", Order: "asc"})
	require.NoError(t, err)
	require.NotNil(t, pag)
	assert.Equal(t, 3, pag.TotalRecord)
	require.Len(t, providers, 2)
	assert.Equal(t, p1.ID, providers[0].ID)
	assert.Equal(t, p2.ID, providers[1].ID)

	pag, providers, err = QueryAIProviders(db, &ypb.AIProviderFilter{
		AIType: []string{"openai"},
	}, &ypb.Paging{Page: 1, Limit: 10, OrderBy: "id", Order: "asc"})
	require.NoError(t, err)
	require.NotNil(t, pag)
	assert.Equal(t, 2, pag.TotalRecord)
	require.Len(t, providers, 2)
	assert.Equal(t, p1.ID, providers[0].ID)
	assert.Equal(t, p3.ID, providers[1].ID)

	pag, providers, err = QueryAIProviders(db, &ypb.AIProviderFilter{
		Ids: []int64{int64(p2.ID)},
	}, nil)
	require.NoError(t, err)
	require.NotNil(t, pag)
	assert.Equal(t, 1, pag.TotalRecord)
	require.Len(t, providers, 1)
	assert.Equal(t, p2.ID, providers[0].ID)
}
