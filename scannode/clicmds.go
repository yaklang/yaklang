package scannode

import "github.com/yaklang/yaklang/common/yak/distyak"

// DistYakCommand preserves the legacy node entrypoint while the shared script
// runner remains available to the public CLI and downstream node modules.
var DistYakCommand = distyak.DistYakCommand
