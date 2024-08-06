package java2ssa

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

var SpringFrameworkAnnotationMap = map[string]bool{
	"CrossOrigin":true,
	"InitBinder":true,
	"ExceptionHandlerReflectiveProcessor":true,
	"RequestBody":true,
	"PathVariable":true,
	"package-info":true,
	"ModelAttribute":true,
	"RequestAttribute":true,
	"RequestHeader":true,
	"ExceptionHandler":true,
	"ControllerMappingReflectiveProcessor":true,
	"GetMapping":true,
	"Mapping":true,
	"MatrixVariable":true,
	"DeleteMapping":true,
	"CookieValue":true,
	"BindParam":true,
	"PostMapping":true,
	"PutMapping":true,
	"ControllerAdvice":true,
	"PatchMapping":true,
	"RequestMapping":true,
	"RequestMethod":true,
	"RequestParam":true,
	"RequestPart":true,
	"ResponseBody":true,
	"ResponseStatus":true,
	"RestController":true,
	"RestControllerAdvice":true,
	"SessionAttribute":true,
	"SessionAttributes":true,
	"ValueConstants":true,
}

var ServletAnnotationMap = map[string]bool{
	"HandlesTypes":true,
	"HttpConstraint":true,
	"HttpMethodConstraint":true,
	"MultipartConfig":true,
	"ServletSecurity":true,
	"WebFilter":true,
	"WebInitParam":true,
	"WebListener":true,
	"WebServlet":true,
}

func (y *builder) AddFullTypeNameRaw(typName string, typ ssa.Type) ssa.Type {
	newTyp,_ :=y.AddFullTypeNameForType(typName, typ, true)
	return newTyp
}

func (y *builder) AddFullTypeNameFromMap(typName string, typ ssa.Type) (newTyp ssa.Type, fromMap bool) {
	return y.AddFullTypeNameForType(typName, typ, false)
}

// AddFullTypeNameForType用于将FullTypeName设置到Type中。其中当Type是BasicType时，会创建新的Type，避免修改原来的Type。
// isFullName表示是否是完整的FullTypeName，如果不是，则会从fullTypeNameMap寻找完整的FullTypeName。
func (y *builder) AddFullTypeNameForType(typName string, typ ssa.Type, isFullName bool) (newTyp ssa.Type,fromMap bool)  {
	if b, ok := ssa.ToBasicType(typ); ok {
		typ = ssa.NewBasicType(b.Kind, b.GetName())
	}

	if typ == nil {
		return ssa.GetAnyType(),false
	}

	typStr := typName
	if !isFullName {
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
			return typ,true
		}
	} else {
		typ.AddFullTypeName(typStr)
	}
	return typ,false
}

func (y *builder) CopyFullTypeNameForType(allTypName []string, typ ssa.Type) ssa.Type {
	if b, ok := ssa.ToBasicType(typ); ok {
		typ = ssa.NewBasicType(b.Kind, b.GetName())
	}

	if typ == nil {
		return ssa.GetAnyType()
	}
	for _, typStr := range allTypName {
		typ.AddFullTypeName(typStr)
	}
	return typ
}

func (y *builder) AddFullTypeNameForAllImport(typName string, typ ssa.Type) ssa.Type {
	for _, ft := range y.allImportPkgSlice {
		typStr := strings.Join(ft[:len(ft)-1], ".")
		for i := len(ft) - 1; i > 0; i-- {
			version := y.GetPkgSCAVersion(strings.Join(ft[:i], "."))
			if version != "" {
				typStr = (fmt.Sprintf("%s.%s:%s", typStr, typName, version))
				break
			}
		}
		typStr = fmt.Sprintf("%s.%s", typStr, typName)
		typ.AddFullTypeName(typStr)
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