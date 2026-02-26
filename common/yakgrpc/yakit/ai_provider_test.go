package yakit

import (
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
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
