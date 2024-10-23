package class_context

import (
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"strings"
)

type ClassContext struct {
	ClassName        string
	FunctionName     string
	FunctionType     any
	PackageName      string
	BuildInLibsMap   map[string][]string
	Arguments        []string
	IsStatic         bool
	GetTypeShortName func(rawName string) string
}

func (f *ClassContext) GetAllImported() []string {
	imports := []string{}
	for pkg, classes := range f.BuildInLibsMap {
		for _, className := range classes {
			imports = append(imports, pkg+"."+className)
		}
	}
	return imports
}
func (f *ClassContext) Import(name string) {
	if f.BuildInLibsMap == nil {
		f.BuildInLibsMap = make(map[string][]string)
	}
	pkg, className := SplitPackageClassName(name)
	f.BuildInLibsMap[pkg] = append(f.BuildInLibsMap[pkg], className)
}
func (f *ClassContext) ShortTypeName(name string) string {
	if f.BuildInLibsMap == nil {
		return name
	}
	pkg, className := SplitPackageClassName(name)
	if pkg == "" {
		return className
	}
	libs := f.BuildInLibsMap[pkg]
	if len(libs) > 0 && (funk.Contains(libs, className) || libs[0] == "*") {
		return className
	}
	f.BuildInLibsMap[pkg] = append(f.BuildInLibsMap[pkg], className)
	return className
}

func SplitPackageClassName(s string) (string, string) {
	splits := strings.Split(s, ".")
	if len(splits) > 0 {
		return strings.Join(splits[:len(splits)-1], "."), splits[len(splits)-1]
	}
	log.Errorf("split package name and class name failed: %v", s)
	return "", ""
}

//func GetShortName(ctx *ClassContext, name string) string {
//	libs := append(ctx.BuildInLibs, ctx.ClassName)
//	for _, lib := range libs {
//		pkg, className := SplitPackageClassName(lib)
//		fpkg, fclassName := SplitPackageClassName(name)
//		if fpkg == pkg && (className == "*" || fclassName == className) {
//			return fclassName
//		}
//	}
//	return name
//}
