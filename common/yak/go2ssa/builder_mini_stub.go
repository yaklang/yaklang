//go:build no_language
// +build no_language

package go2ssa

// Stub implementation when language support is excluded
// 语言支持被排除时的桩实现 - Go 语言支持被排除

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// CreateBuilder 桩实现 - no_language 版本不支持 Go
func CreateBuilder() ssa.Builder {
	log.Warn("Go language support is excluded in no_language build. Please use the full version.")
	return nil
}

// IsGoSupported 返回 Go 是否被支持
func IsGoSupported() bool {
	return false
}
