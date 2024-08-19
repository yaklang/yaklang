package java2ssa

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

var SpringFrameworkAnnotationMap = map[string]bool{
	"CrossOrigin":                          true,
	"InitBinder":                           true,
	"ExceptionHandlerReflectiveProcessor":  true,
	"RequestBody":                          true,
	"PathVariable":                         true,
	"package-info":                         true,
	"ModelAttribute":                       true,
	"RequestAttribute":                     true,
	"RequestHeader":                        true,
	"ExceptionHandler":                     true,
	"ControllerMappingReflectiveProcessor": true,
	"GetMapping":                           true,
	"Mapping":                              true,
	"MatrixVariable":                       true,
	"DeleteMapping":                        true,
	"CookieValue":                          true,
	"BindParam":                            true,
	"PostMapping":                          true,
	"PutMapping":                           true,
	"ControllerAdvice":                     true,
	"PatchMapping":                         true,
	"RequestMapping":                       true,
	"RequestMethod":                        true,
	"RequestParam":                         true,
	"RequestPart":                          true,
	"ResponseBody":                         true,
	"ResponseStatus":                       true,
	"RestController":                       true,
	"RestControllerAdvice":                 true,
	"SessionAttribute":                     true,
	"SessionAttributes":                    true,
	"ValueConstants":                       true,
}

var ServletAnnotationMap = map[string]bool{
	"HandlesTypes":         true,
	"HttpConstraint":       true,
	"HttpMethodConstraint": true,
	"MultipartConfig":      true,
	"ServletSecurity":      true,
	"WebFilter":            true,
	"WebInitParam":         true,
	"WebListener":          true,
	"WebServlet":           true,
}

func (y *builder) AddFullTypeNameRaw(typName string, typ ssa.Type) ssa.Type {
	if b, ok := ssa.ToBasicType(typ); ok {
		typ = ssa.NewBasicType(b.Kind, b.GetName())
		typ.SetFullTypeNames(b.GetFullTypeNames())
	}

	if typ == nil {
		return ssa.GetAnyType()
	}
	typ.AddFullTypeName(typName)
	return typ
}

func (y *builder) AddFullTypeNameFromMap(typName string, typ ssa.Type) ssa.Type {
	if b, ok := ssa.ToBasicType(typ); ok {
		typ = ssa.NewBasicType(b.Kind, b.GetName())
		typ.SetFullTypeNames(b.GetFullTypeNames())
	}

	if typ == nil {
		return ssa.GetAnyType()
	}

	typStr := typName
	if ft, ok := y.fullTypeNameMap[typName]; ok {
		typStr = strings.Join(ft, ".")
		for i := len(ft) - 1; i > 0; i-- {
			version := y.GetPkgSCAVersion(strings.Join(ft[:i], "."))
			if version != "" {
				typStr = (fmt.Sprintf("%s:%s", typStr, version))
				break
			}
		}
		typ.AddFullTypeName(typStr)
		return typ
	} else {
		return y.AddFullTypeNameForAllImport(typName, typ)
	}

}

func (y *builder) MergeFullTypeNameForType(allTypName []string, typ ssa.Type) ssa.Type {
	if b, ok := ssa.ToBasicType(typ); ok {
		typ = ssa.NewBasicType(b.Kind, b.GetName())
		typ.SetFullTypeNames(b.GetFullTypeNames())
	}

	if typ == nil {
		return ssa.GetAnyType()
	}
	for _, typStr := range allTypName {
		if !utils.ContainsAll[string](typ.GetFullTypeNames(), typStr) {
			typ.AddFullTypeName(typStr)
		}
	}
	return typ
}

func (y *builder) AddFullTypeNameForAllImport(typName string, typ ssa.Type) ssa.Type {
	for _, ft := range y.allImportPkgSlice {
		typStr := strings.Join(ft[:len(ft)-1], ".")
		var typStrWithVersion string
		for i := len(ft) - 1; i > 0; i-- {
			version := y.GetPkgSCAVersion(strings.Join(ft[:i], "."))
			if version != "" {
				typStrWithVersion = (fmt.Sprintf("%s.%s:%s", typStr, typName, version))
				break
			}
		}
		if typStrWithVersion != "" {
			typ.AddFullTypeName(typStrWithVersion)
		} else {
			typStr = fmt.Sprintf("%s.%s", typStr, typName)
			typ.AddFullTypeName(typStr)
		}
	}
	if len(y.selfPkgPath) != 0 {
		typStr := strings.Join(y.selfPkgPath[:len(y.selfPkgPath)-1], ".")
		typStr = fmt.Sprintf("%s.%s", typStr, typName)
		typ.AddFullTypeName(typStr)
	}
	return typ
}

func (y *builder) GetPkgSCAVersion(pkgName string) string {
	sca := y.GetProgram().GetApplication().GetSCAPackageByName(pkgName)
	if sca != nil {
		return sca.Version
	}
	return ""
}

func (y *builder) AddFullTypeNameFromAnnotationMap(typName string, typ ssa.Type) ssa.Type {
	if b, ok := ssa.ToBasicType(typ); ok {
		typ = ssa.NewBasicType(b.Kind, b.GetName())
		typ.SetFullTypeNames(b.GetFullTypeNames())
	}

	if typ == nil {
		return ssa.GetAnyType()
	}

	for _, p := range y.allImportPkgSlice {
		str := strings.Join(p[:len(p)-1], ".")
		switch str {
		case "org.springframework.web.bind.annotation":
			ok := SpringFrameworkAnnotationMap[typName]
			if ok {
				return y.AddFullTypeNameRaw(fmt.Sprintf("%s.%s", str, typName), typ)
			}
		case "javax.servlet.annotation":
			ok := ServletAnnotationMap[typName]
			if ok {
				return y.AddFullTypeNameRaw(fmt.Sprintf("%s.%s", str, typName), typ)
			}
		default:
			return y.AddFullTypeNameForAllImport(typName, typ)
		}
	}

	return y.AddFullTypeNameForAllImport(typName, typ)
}
