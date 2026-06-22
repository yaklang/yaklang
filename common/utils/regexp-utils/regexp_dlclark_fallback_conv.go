package regexp_utils

import (
	"time"

	dlclark "github.com/dlclark/regexp2"
	pcre2 "github.com/VillanCh/go-pcre2-lite/regexp2"
)

// dlclarkFallbackTimeout: dlclark 兜底路径的单次匹配超时.
//
// dlclark 默认 NoTimeout, 灾难性回溯 (如 (a+)+ 风格) 会挂死调用方. 兜底路径只为
// PCRE2 不支持的少数构造服务 (典型变长 lookbehind), 这些 pattern 本身通常不会灾难回溯,
// 但仍设上限保护, 避免恶意/失误 pattern 拖垮进程.
const dlclarkFallbackTimeout = 5 * time.Second

// pcre2OptionsToDlclark 把 go-pcre2-lite 的 RegexOptions 转换为 dlclark 的等价值.
//
// 二者常量值完全一致 (pcre2 仿 dlclark API): None=0, IgnoreCase=0x1, Multiline=0x2,
// Singleline=0x10, ... 因类型名不同需显式转换; 数值语义相同.
func pcre2OptionsToDlclark(opt pcre2.RegexOptions) dlclark.RegexOptions {
	return dlclark.RegexOptions(opt)
}
