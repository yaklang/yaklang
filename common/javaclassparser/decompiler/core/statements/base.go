package statements

import "github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"

type Statement interface {
	String(funcCtx *class_context.ClassContext) string
}
