package cpp2ssa

import (
	"strings"

	cppparser "github.com/yaklang/yaklang/common/yak/cpp/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// declTypeInfo is the resolved meaning of a declSpecifierSeq: the base type plus
// the declaration modifiers that change how declarators are read.
type declTypeInfo struct {
	typ       ssa.Type
	text      string
	isTypedef bool
	isStatic  bool
	isAuto    bool
	className string
}

// typeSpecifierInfo resolves a declSpecifierSeq. A class, struct or union
// definition inside the specifier sequence is built here, because C++ allows
// `struct Point { int x; } p;` without a separate declaration.
func (b *singleFileBuilder) typeSpecifierInfo(raw cppparser.IDeclSpecifierSeqContext) declTypeInfo {
	info := declTypeInfo{typ: ssa.CreateAnyType()}
	seq, ok := raw.(*cppparser.DeclSpecifierSeqContext)
	if !ok || seq == nil {
		return info
	}
	var spellings []string
	for _, rawSpecifier := range seq.AllDeclSpecifier() {
		specifier, ok := rawSpecifier.(*cppparser.DeclSpecifierContext)
		if !ok || specifier == nil {
			continue
		}
		switch {
		case specifier.Typedef() != nil:
			info.isTypedef = true
		case specifier.StorageClassSpecifier() != nil:
			if specifier.StorageClassSpecifier().GetText() == "static" {
				info.isStatic = true
			}
		case specifier.TypeSpecifier() != nil:
			typeSpecifier, ok := specifier.TypeSpecifier().(*cppparser.TypeSpecifierContext)
			if !ok || typeSpecifier == nil {
				continue
			}
			switch {
			case typeSpecifier.ClassSpecifier() != nil:
				if bp := b.visitClassSpecifier(typeSpecifier.ClassSpecifier()); bp != nil {
					info.typ = bp
					info.className = bp.Name
				}
			case typeSpecifier.EnumSpecifier() != nil:
				info.typ = ssa.CreateNumberType()
				info.text = typeSpecifier.EnumSpecifier().GetText()
			case typeSpecifier.TrailingTypeSpecifier() != nil:
				// const/volatile do not change the SSA type.
				if trailing, ok := typeSpecifier.TrailingTypeSpecifier().(*cppparser.TrailingTypeSpecifierContext); ok && trailing != nil && trailing.CvQualifier() != nil {
					break
				}
				spellings = append(spellings, typeSpecifier.TrailingTypeSpecifier().GetText())
			}
		case specifier.FunctionSpecifier() != nil, specifier.Constexpr() != nil:
			// inline/virtual/explicit/constexpr do not change the SSA type.
		}
	}
	if info.className == "" && len(spellings) > 0 {
		info.text = strings.Join(spellings, " ")
		info.typ = b.typeFromText(info.text)
		normalized := normalizeTypeText(info.text)
		info.isAuto = normalized == "auto" || strings.HasPrefix(normalized, "decltype(")
	}
	return info
}

// typeFromText resolves a C++ type spelling to an SSA type. Unknown names become
// placeholder blueprints instead of nil so later member access has a target.
func (b *singleFileBuilder) typeFromText(text string) ssa.Type {
	text = normalizeTypeText(text)
	if text == "" {
		return ssa.CreateAnyType()
	}
	for i := 0; i < 8; i++ {
		alias, ok := b.typeAliases[text]
		if !ok {
			break
		}
		text = normalizeTypeText(alias)
	}

	switch text {
	case "void":
		return ssa.CreateAnyType()
	case "bool":
		return ssa.CreateBooleanType()
	case "char", "wchar_t", "char16_t", "char32_t", "signedchar", "unsignedchar", "char8_t":
		return ssa.CreateByteType()
	case "short", "shortint", "unsignedshort", "unsignedshortint",
		"int", "signed", "signedint", "unsigned", "unsignedint",
		"long", "longint", "unsignedlong", "unsignedlongint",
		"longlong", "longlongint", "unsignedlonglong", "unsignedlonglongint",
		"size_t", "ssize_t", "ptrdiff_t", "intptr_t", "uintptr_t",
		"int8_t", "int16_t", "int32_t", "int64_t", "uint8_t", "uint16_t", "uint32_t", "uint64_t":
		return ssa.CreateNumberType()
	case "float", "double", "longdouble":
		return ssa.CreateFloatType()
	case "auto", "decltype(auto)", "any":
		return ssa.CreateAnyType()
	}

	base, args := splitTemplateSpelling(text)
	elem := ssa.CreateAnyType()
	if len(args) > 0 {
		elem = b.typeFromText(args[0])
	}
	baseName := stdlibBaseName(base)
	switch baseName {
	case "string", "basic_string", "wstring", "u16string", "u32string", "string_view":
		return b.ensureStdlibType(baseName)
	case "vector", "array", "deque", "list", "forward_list", "initializer_list", "stack", "queue", "priority_queue":
		return ssa.NewSliceType(elem)
	case "map", "unordered_map", "multimap", "unordered_multimap":
		valueType := ssa.CreateAnyType()
		if len(args) > 1 {
			valueType = b.typeFromText(args[1])
		}
		return ssa.NewMapType(elem, valueType)
	case "set", "unordered_set", "multiset", "unordered_multiset":
		return ssa.NewMapType(elem, ssa.CreateBooleanType())
	case "pair", "tuple":
		return b.newTupleType(args)
	case "unique_ptr", "shared_ptr", "weak_ptr", "auto_ptr", "optional":
		return b.typeFromText(args[0])
	}
	if entry := lookupStdlibType(baseName); entry != nil {
		if bp := b.ensureStdlibBlueprint(entry, baseName); bp != nil {
			return bp
		}
	}
	if templateType, ok := b.templateTypes[baseName]; ok {
		return templateType
	}
	if bp := b.ensureClassBlueprint(base); bp != nil {
		return bp
	}
	return ssa.CreateAnyType()
}

func (b *singleFileBuilder) ensureStdlibType(name string) ssa.Type {
	entry := lookupStdlibType(name)
	if entry == nil {
		return ssa.CreateStringType()
	}
	if bp := b.ensureStdlibBlueprint(entry, name); bp != nil {
		return bp
	}
	return ssa.CreateStringType()
}

func (b *singleFileBuilder) newTupleType(args []string) ssa.Type {
	objectType := ssa.NewObjectType()
	objectType.AddField(b.EmitConstInst("first"), b.typeFromText(argText(args, 0)))
	objectType.AddField(b.EmitConstInst("second"), b.typeFromText(argText(args, 1)))
	return objectType
}

func argText(args []string, index int) string {
	if index < len(args) {
		return args[index]
	}
	return ""
}

// ensureClassBlueprint returns the blueprint for a class spelling, creating a
// placeholder when no declaration has been seen yet.
func (b *singleFileBuilder) ensureClassBlueprint(text string) *ssa.Blueprint {
	name := normalizeTypeName(text)
	if name == "" {
		return nil
	}
	if bp := b.GetBluePrint(name); bp != nil {
		return bp
	}
	for _, using := range b.usingNamespaces {
		qualified := using + "::" + name
		if bp := b.GetBluePrint(qualified); bp != nil {
			return bp
		}
	}
	bp := b.CreateBlueprint(name)
	bp.SetKind(ssa.BlueprintClass)
	return bp
}

// typeQualifierPrefixes are stripped only when they form a leading word, so a
// class named Constant is not read as const + ant.
var typeQualifierPrefixes = []string{
	"constexpr", "consteval", "constinit", "const", "volatile", "typename",
	"struct", "class", "union", "enum", "static", "extern", "register",
	"mutable", "thread_local",
}

// normalizeTypeText removes whitespace, leading type qualifiers and reference
// markers. It must not rewrite substrings: type names such as UnsignedValue must
// survive intact.
func normalizeTypeText(text string) string {
	text = strings.TrimSpace(text)
	for {
		trimmed := strings.TrimSpace(text)
		stripped := false
		for _, prefix := range typeQualifierPrefixes {
			if !strings.HasPrefix(trimmed, prefix) {
				continue
			}
			rest := trimmed[len(prefix):]
			if rest == "" {
				text = ""
				stripped = true
				break
			}
			if isTypeBoundary(rest[0]) {
				text = strings.TrimSpace(rest)
				stripped = true
				break
			}
		}
		if !stripped {
			break
		}
	}
	text = strings.NewReplacer(" ", "", "\t", "", "\n", "").Replace(text)
	text = strings.TrimSuffix(text, "&&")
	text = strings.TrimSuffix(text, "&")
	text = strings.TrimPrefix(text, "::")
	return text
}

func isTypeBoundary(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '*', '&', ':', '<':
		return true
	}
	return false
}

// normalizeTypeName keeps the qualified name but drops template arguments.
func normalizeTypeName(text string) string {
	base, _ := splitTemplateSpelling(normalizeTypeText(text))
	if base == "" {
		return ""
	}
	base = strings.TrimPrefix(base, "::")
	if strings.ContainsAny(base, "()[]{}*&,") {
		return ""
	}
	return base
}

// splitTemplateSpelling splits "std::vector<int>" into ("std::vector", ["int"]).
// Only the outermost angle brackets are removed; nested arguments keep theirs.
func splitTemplateSpelling(text string) (string, []string) {
	text = strings.TrimSpace(text)
	start := strings.Index(text, "<")
	if start < 0 || !strings.HasSuffix(text, ">") {
		return text, nil
	}
	depth := 0
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '<':
			depth++
		case '>':
			depth--
			if depth == 0 && i != len(text)-1 {
				return text, nil
			}
			if depth < 0 {
				return text, nil
			}
		}
	}
	if depth != 0 {
		return text, nil
	}
	base := strings.TrimSpace(text[:start])
	body := text[start+1 : len(text)-1]
	return base, splitTemplateArguments(body)
}

func splitTemplateArguments(body string) []string {
	var args []string
	depth := 0
	last := 0
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case '<', '(', '[':
			depth++
		case '>', ')', ']':
			depth--
		case ',':
			if depth == 0 {
				args = append(args, strings.TrimSpace(body[last:i]))
				last = i + 1
			}
		}
	}
	if tail := strings.TrimSpace(body[last:]); tail != "" {
		args = append(args, tail)
	}
	return args
}

// typeFromTypeIDText maps a type-id spelling (including pointer/reference
// decorations) to an SSA type. References use the pointer kind: the SSA core has
// no separate reference type, and pointer kind preserves aliasing information.
func (b *singleFileBuilder) typeFromTypeIDText(text string) ssa.Type {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "const ")
	text = strings.TrimSpace(strings.TrimSuffix(text, " const"))
	depth := 0
	for {
		switch {
		case strings.HasSuffix(text, "*"):
			depth++
			text = strings.TrimSpace(strings.TrimSuffix(text, "*"))
		case strings.HasSuffix(text, "&"):
			depth++
			text = strings.TrimSpace(strings.TrimSuffix(text, "&"))
		default:
			return wrapPointerType(b.typeFromText(text), depth)
		}
	}
}
