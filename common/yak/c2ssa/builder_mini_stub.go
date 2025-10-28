//go:build no_language
// +build no_language

package c2ssa

// Stub implementation when language support is excluded
// 语言支持被排除时的桩实现 - C 语言支持被排除

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// CreateBuilder 桩实现 - no_language 版本不支持 C
func CreateBuilder() ssa.Builder {
	log.Warn("C language support is excluded in no_language build. Please use the full version.")
	return nil
}

// IsCSupported 返回 C 是否被支持
func IsCSupported() bool {
	return false
}
