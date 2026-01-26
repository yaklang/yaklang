//go:build yakit_exclude

package yakgrpc

import "github.com/yaklang/yaklang/common/yak"

// enabledHooks is defined here for builds with yakit_exclude tag
// where grpc_mitm.go (which normally defines it) is excluded
var enabledHooks = yak.MITMAndPortScanHooks
