//go:build no_language
// +build no_language

package java2ssa

// Stub implementation when language support is excluded
// 语言支持被排除时的桩实现 - Java 语言支持被排除

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// CreateBuilder 桩实现 - no_language 版本不支持 Java
func CreateBuilder() ssa.Builder {
	log.Warn("Java language support is excluded in no_language build. Please use the full version.")
	return nil
}

// IsJavaSupported 返回 Java 是否被支持
func IsJavaSupported() bool {
	return false
}

