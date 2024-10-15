package class_context

import (
	"github.com/yaklang/yaklang/common/log"
	"strings"
)

type FunctionContext struct {
	ClassName    string
	FunctionName string
	PackageName  string
	BuildInLibs  []string
}

func (f *FunctionContext) ShortTypeName(s string) string {
	return GetShortName(f, s)
}

func SplitPackageClassName(s string) (string, string) {
	splits := strings.Split(s, ".")
	if len(splits) > 0 {
		return strings.Join(splits[:len(splits)-1], "."), splits[len(splits)-1]
	}
	log.Errorf("split package name and class name failed: %v", s)
	return "", ""
}

func GetShortName(ctx *FunctionContext, name string) string {
	libs := append(ctx.BuildInLibs, ctx.ClassName)
	for _, lib := range libs {
		pkg, className := SplitPackageClassName(lib)
		fpkg, fclassName := SplitPackageClassName(name)
		if fpkg == pkg && (className == "*" || fclassName == className) {
			return fclassName
		}
	}
	return name
}
