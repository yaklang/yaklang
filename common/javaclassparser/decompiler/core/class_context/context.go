package class_context

import (
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/omap"
	"strings"
)

type ClassContext struct {
	ClassName        string
	FunctionName     string
	FunctionType     any
	PackageName      string
	BuildInLibsMap   *omap.OrderedMap[string, []string]
	Arguments        []string
	IsStatic         bool
}

func (f *ClassContext) GetAllImported() []string {
	imports := []string{}
	f.BuildInLibsMap.ForEach(func(pkg string, classes []string) bool {
		for _, className := range classes {
			imports = append(imports, pkg+"."+className)
		}
		return true
	})
	return imports
}
func (f *ClassContext) Import(name string) {
	if f.BuildInLibsMap == nil {
		f.BuildInLibsMap = omap.NewEmptyOrderedMap[string, []string]()
	}
	pkg, className := SplitPackageClassName(name)
	f.BuildInLibsMap.Set(pkg, append(f.BuildInLibsMap.GetMust(pkg), className))
}
func (f *ClassContext) ShortTypeName(name string) string {
	if f.BuildInLibsMap == nil {
		return name
	}
	pkg, className := SplitPackageClassName(name)
	if pkg == "" {
		return className
	}
	libs := f.BuildInLibsMap.GetMust(pkg)
	if len(libs) > 0 && (funk.Contains(libs, className) || libs[0] == "*") {
		return className
	}
	f.BuildInLibsMap.Set(pkg, append(f.BuildInLibsMap.GetMust(pkg), className))
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
