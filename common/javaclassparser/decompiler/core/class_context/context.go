package class_context

import (
	"slices"
	"strings"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type ClassContext struct {
	ClassName       string
	FunctionName    string
	SupperClassName string
	FunctionType    any
	PackageName     string
	BuildInLibsMap  *omap.OrderedMap[string, []string]
	KeySet          *utils.Set[string]
	Arguments       []string
	IsStatic        bool
	IsVarArgs       bool
}

var javaKeywords = map[string]struct{}{
	"abstract": {}, "assert": {}, "boolean": {}, "break": {}, "byte": {}, "case": {}, "catch": {},
	"char": {}, "class": {}, "const": {}, "continue": {}, "default": {}, "do": {}, "double": {},
	"else": {}, "enum": {}, "extends": {}, "final": {}, "finally": {}, "float": {}, "for": {},
	"goto": {}, "if": {}, "implements": {}, "import": {}, "instanceof": {}, "int": {}, "interface": {},
	"long": {}, "native": {}, "new": {}, "package": {}, "private": {}, "protected": {}, "public": {},
	"return": {}, "short": {}, "static": {}, "strictfp": {}, "super": {}, "switch": {}, "synchronized": {},
	"this": {}, "throw": {}, "throws": {}, "transient": {}, "try": {}, "void": {}, "volatile": {}, "while": {},
	"true": {}, "false": {}, "null": {}, "_": {},
}

func SafeIdentifier(name string) string {
	if _, ok := javaKeywords[name]; ok {
		return name + "_"
	}
	return name
}

func (f *ClassContext) GetAllImported() []string {
	imports := []string{}
	f.BuildInLibsMap.ForEach(func(pkg string, classes []string) bool {
		if pkg == f.PackageName {
			return true
		}
		for _, className := range classes {
			imports = append(imports, pkg+"."+className)
		}
		return true
	})
	return imports
}
func (f *ClassContext) Import(name string) {
	if f.KeySet == nil {
		f.KeySet = utils.NewSet[string]()
	}
	if f.BuildInLibsMap == nil {
		f.BuildInLibsMap = omap.NewEmptyOrderedMap[string, []string]()
	}
	pkg, className := SplitPackageClassName(name)
	if f.KeySet.Has(className) {
		return
	}
	if pkg == "" {
		return
	}
	key, ok := f.BuildInLibsMap.Get(pkg)
	if ok {
		if slices.Contains(key, className) || slices.Contains(key, "*") {
			return
		}
	}
	f.BuildInLibsMap.Set(pkg, append(f.BuildInLibsMap.GetMust(pkg), className))
	f.KeySet.Add(className)
}
func (f *ClassContext) ShortTypeName(name string) string {
	f.Import(name)
	pkg, className := SplitPackageClassName(name)
	if pkg == "" {
		return className
	}
	if pkg == f.PackageName {
		return className
	}
	if f.BuildInLibsMap == nil {
		f.BuildInLibsMap = omap.NewEmptyOrderedMap[string, []string]()
	}
	libs := f.BuildInLibsMap.GetMust(pkg)
	if len(libs) > 0 && (funk.Contains(libs, className) || libs[0] == "*") {
		return className
	}
	//f.BuildInLibsMap.Set(pkg, append(f.BuildInLibsMap.GetMust(pkg), className))
	return name
}

func SplitPackageClassName(s string) (string, string) {
	splits := strings.Split(s, ".")
	if len(splits) > 0 {
		return strings.Join(splits[:len(splits)-1], "."), splits[len(splits)-1]
	}
	log.Errorf("split package name and class name failed: %v", s)
	return "", ""
}
