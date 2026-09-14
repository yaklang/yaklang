package reactservice

import "github.com/yaklang/yaklang/common/consts"

var defaultService = New(consts.GetGormProjectDatabase)

// Default returns the process-wide ReAct service used by the normal Yak engine.
// Explicitly configured embedded/test servers may inject an isolated Service.
func Default() *Service {
	return defaultService
}
