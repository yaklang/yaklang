package aibalance

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSchemaModelsTableNamesLocked 锁定 aibalance 专属模型的物理表名。
//
// 背景：这些模型原先定义在 common/schema/ai_infra.go，后搬回 aibalance 包内自治管理。
// GORM 把数据写到哪张物理表只取决于 TableName()（显式）或结构体名的默认推导（与所在 Go 包无关），
// 因此「换包」本身不会动数据；唯一的数据丢失风险是「表名被无意改掉」——那样旧数据会被遗留在旧表里，
// 新代码读写的是另一张空表。本测试把每个表名钉死成搬迁前的历史值，任何后续重命名都会让测试失败，
// 从而杜绝「因模型挪位置导致数据丢失」。
//
// 注意：下列字面量必须与搬迁前 common/schema 中的历史表名严格一致，不得随结构体改名而改动。
// 关键词: aibalance 表名锁定, schema 归位数据安全, TableName 回归守卫, 防止数据丢失
func TestSchemaModelsTableNamesLocked(t *testing.T) {
	db := GetDB()
	if db == nil {
		t.Skip("profile database not available")
	}

	cases := []struct {
		model     interface{}
		tableName string
	}{
		// B 类（无显式 TableName，依赖 GORM 默认推导；历史上登记在 schema.ProfileTables）
		{&AiProvider{}, "ai_providers"},
		{&AiApiKeys{}, "ai_api_keys"},
		{&LoginSession{}, "login_sessions"},
		{&OpsUser{}, "ops_users"},
		{&OpsActionLog{}, "ops_action_logs"},
		{&WebSearchApiKey{}, "web_search_api_keys"},

		// A 类（显式 TableName）
		{&AiBalanceRateLimitConfig{}, "ai_balance_rate_limit_configs"},
		{&AiBalanceClientVersionStat{}, "ai_balance_client_versions"},
		{&WebSearchConfig{}, "web_search_configs"},
		{&AmapConfig{}, "amap_configs"},
		{&AmapApiKey{}, "amap_api_keys"},
		{&AiProviderHealthRecord{}, "ai_provider_health_records"},
		{&AiDailyCacheStat{}, "ai_daily_cache_stats"},
		{&AiDailyUserSeen{}, "ai_daily_user_seen"},
		{&AiDailySummary{}, "ai_daily_summary"},
		{&FreeUserDailyTokenUsage{}, "free_user_daily_token_usage"},
	}

	for _, tc := range cases {
		got := db.NewScope(tc.model).TableName()
		assert.Equalf(t, tc.tableName, got,
			"table name for %T must stay %q to keep existing data accessible (got %q)",
			tc.model, tc.tableName, got)
	}
}
