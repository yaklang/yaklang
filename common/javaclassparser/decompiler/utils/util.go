package utils

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
)


func LinkNode(src, target *core.Node) {
	target.Source = append(target.Source, src)
	src.Next = append(src.Next, target)
}
