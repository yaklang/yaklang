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
	require.NoError(t, db.AutoMigrate(&schema.GeneralStorage{}).Error)
	return db
}

func TestListAIProviders_FromGlobalConfig(t *testing.T) {
	db := setupAIProviderTestDB(t)
	defer db.Close()

	cfg := &ypb.AIGlobalConfig{
		IntelligentModels: []*ypb.AIModelConfig{
			{
				ModelName: "gpt-4o",
				Provider: &ypb.ThirdPartyApplicationConfig{
					Type:   "openai",
					APIKey: "key-1",
					Domain: "api.openai.com",
				},
			},
		},
		LightweightModels: []*ypb.AIModelConfig{
			{
				ModelName: "gpt-4o-mini",
				Provider: &ypb.ThirdPartyApplicationConfig{
					Type:   "azure",
					APIKey: "key-2",
					Domain: "azure.example.com",
				},
			},
		},
	}
	_, err := SetAIGlobalConfig(db, cfg)
	require.NoError(t, err)

	providers, err := ListAIProviders(db)
	require.NoError(t, err)
	assert.Len(t, providers, 2)
}

func TestQueryAIProviders_Filter(t *testing.T) {
	db := setupAIProviderTestDB(t)
	defer db.Close()

	cfg := &ypb.AIGlobalConfig{
		IntelligentModels: []*ypb.AIModelConfig{
			{
				ModelName: "gpt-4o",
				Provider: &ypb.ThirdPartyApplicationConfig{
					Type:   "openai",
					APIKey: "key-1",
					Domain: "api.openai.com",
				},
			},
		},
		LightweightModels: []*ypb.AIModelConfig{
			{
				ModelName: "gpt-4o-mini",
				Provider: &ypb.ThirdPartyApplicationConfig{
					Type:   "openai",
					APIKey: "key-2",
					Domain: "api.openai.com",
				},
			},
			{
				ModelName: "azure-mini",
				Provider: &ypb.ThirdPartyApplicationConfig{
					Type:   "azure",
					APIKey: "key-3",
					Domain: "azure.example.com",
				},
			},
		},
	}
	_, err := SetAIGlobalConfig(db, cfg)
	require.NoError(t, err)

	pag, providers, err := QueryAIProviders(db, &ypb.AIProviderFilter{
		AIType: []string{"openai"},
	}, &ypb.Paging{Page: 1, Limit: 10, OrderBy: "id", Order: "asc"})
	require.NoError(t, err)
	require.NotNil(t, pag)
	assert.Equal(t, 2, pag.TotalRecord)
	assert.Len(t, providers, 2)
}

func TestUpsertAndDeleteAIProvider_Deprecated(t *testing.T) {
	db := setupAIProviderTestDB(t)
	defer db.Close()

	_, err := UpsertAIProvider(db, &ypb.AIProvider{
		Config: &ypb.ThirdPartyApplicationConfig{
			Type:   "openai",
			APIKey: "key-1",
			Domain: "api.openai.com",
		},
	})
	require.Error(t, err)

	require.Error(t, DeleteAIProvider(db, 1))
}
