package yakit

import (
	"sync"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/configtest"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

// Tests can register extra AI gateways or change timeout globals. Freeze the
// clean default comparison before those fixtures so later tests still compare
// against the same baseline. The factory does no work in production.
var configurationTestNetworkDefaults = sync.OnceValue(GetDefaultNetworkConfig)

// GuardConfigurationForTest must be called before tests mutate the shared
// profile configuration. Refuse user data rather than backing it up and
// overwriting it. Local fixture tests should use configtest.RequirePristine
// directly so they do not initialize any global database.
func GuardConfigurationForTest(t configtest.TestingT) {
	t.Helper()
	configtest.RequireHome(t)
	db := consts.GetGormProfileDatabase()
	project := consts.GetGormProjectDatabase()
	defaults := proto.Clone(configurationTestNetworkDefaults()).(*ypb.GlobalNetworkConfig)
	defaultAI := DefaultAIGlobalConfig()
	migrateAIGlobalConfigBaseURLs(defaultAI)
	baseline := configtest.BuiltinDefaults{Network: defaults, AI: defaultAI}
	configtest.RequirePristine(t, db, baseline)
	configtest.RequirePristine(t, project, nil)
	cached, tiered := GetCachedAIGlobalConfig(), consts.GetTieredAIConfig()
	configtest.RequirePublicRuntime(t, cached, tiered)
	configtest.Guard(t, db, baseline)
	configtest.Guard(t, project, nil)
	// Restore process-local consumers as well as the SQLite records. Registered
	// after Guard so this runs before the original stored rows are restored.
	network := GetNetworkConfig()
	if network == nil {
		network = defaults
	}
	hotPatch, hotPatchCode := GetGlobalHotPatchRuntimeCache()
	t.Cleanup(func() {
		ConfigureNetWork(network)
		setCachedAIGlobalConfig(cached)
		consts.SetTieredAIConfig(tiered)
		SetGlobalHotPatchRuntimeCache(hotPatch, hotPatchCode)
	})
}
