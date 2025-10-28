//go:build no_language
// +build no_language

package php2ssa

// Stub implementation when language support is excluded
// 语言支持被排除时的桩实现 - PHP 语言支持被排除

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// CreateBuilder 桩实现 - no_language 版本不支持 PHP
func CreateBuilder() ssa.Builder {
	log.Warn("PHP language support is excluded in no_language build. Please use the full version.")
	return nil
}

// IsPHPSupported 返回 PHP 是否被支持
func IsPHPSupported() bool {
	return false
}

