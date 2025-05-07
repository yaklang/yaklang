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
		return ssa.CreateAnyType()
	}
	typ.AddFullTypeName(typName)
	return typ
}

// func (y *builder) AddFullTypeNameForSubType(typeName string,  )
func (y *builder) CreateSubType(subName string, typ ssa.Type) ssa.Type {
	// 1. split subName by "."
	parts := strings.Split(subName, ".")

	newType := ssa.NewBasicType(ssa.AnyTypeKind, subName)

	// Get the full type names from the original type
	fullTypeNames := typ.GetFullTypeNames()
	newFullTypeNames := make([]string, 0, len(fullTypeNames))

	// 1.1 if len(parts) == 1 just append this subName to each fullTypeName and create new type
	if len(parts) == 1 {
		for _, ftn := range fullTypeNames {
			newFullTypeNames = append(newFullTypeNames, ftn+"."+subName)
		}
	} else {
		// 1.2 if len(parts) > 1,
		// check if which part matches with existing in type.FullTypeName end
		// if match, append rest to fullTypeName
		for _, ftn := range fullTypeNames {
			// Split the full type name by "."
			fullParts := strings.Split(ftn, ".")

			i := 0
			for {
				// check is match end of fullParts and start of parts
				if fullParts[len(fullParts)-1-i] == parts[i] {
					parts := append(fullParts[len(fullParts)-1-i:], parts...)
					newFullTypeNames = append(newFullTypeNames, strings.Join(parts, "."))
					break
				}
				if i > len(fullParts)-1 || i > len(parts)-1 {
					break
				}
				i++
			}
		}
	}

	newType.SetFullTypeNames(newFullTypeNames)
	return newType
}

func (y *builder) AddFullTypeNameFromMap(typName string, typ ssa.Type) ssa.Type {
	if b, ok := ssa.ToBasicType(typ); ok {
		typ = ssa.NewBasicType(b.Kind, b.GetName())
		typ.SetFullTypeNames(b.GetFullTypeNames())
	}

	if typ == nil {
		typ = ssa.CreateAnyType()
	}

	typStr := typName
	if ft, ok := y.fullTypeNameMap[typName]; ok {
		typStr = strings.Join(ft, ".")
		for i := len(ft) - 1; i > 0; i-- {
			version := y.GetPkgSCAVersion(strings.Join(ft[:i], "."))
			if version != "" {
				typStr = fmt.Sprintf("%s:%s", typStr, version)
				break
			}
		}
		typ.AddFullTypeName(typStr)
		return typ
	} else if strings.Contains(typName, ".") {
		// 如果是全名，直接添加
		typ.AddFullTypeName(typName)
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
		return ssa.CreateAnyType()
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

func (y *builder) HaveCastType(typ ssa.Type) bool {
	if typ == nil {
		return false
	}
	fts := typ.GetFullTypeNames()
	if len(fts) == 0 {
		return false
	}
	return fts[0] == "__castType__"
}

func (y *builder) SetCastTypeFlag(typ ssa.Type) ssa.Type {
	if typ == nil {
		return nil
	}
	fts := typ.GetFullTypeNames()
	if len(fts) == 0 {
		return typ
	}
	newFts := utils.InsertSliceItem[string](fts, "__castType__", 0)
	typ.SetFullTypeNames(newFts)
	return typ
}

func (y *builder) RemoveCastTypeFlag(typ ssa.Type) ssa.Type {
	if typ == nil {
		return nil
	}
	fts := typ.GetFullTypeNames()
	if len(fts) == 0 {
		return typ
	}
	newFts := utils.RemoveSliceItem[string](fts, "__castType__")
	typ.SetFullTypeNames(newFts)
	return typ
}
