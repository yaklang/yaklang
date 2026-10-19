package scannode

import "github.com/yaklang/yaklang/common/consts"

// Parent-process maintenance configures its database independently of distyak.
// Keep this adapter helper when the child command moves to the public engine.
func applySSADatabaseFromEnv() {
	if raw := consts.GetSSADatabaseInfoFromEnv(); raw != "" {
		consts.SetSSADatabaseInfo(raw)
	}
}
