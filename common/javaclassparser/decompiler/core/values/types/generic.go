package types

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
)

// JavaParameterizedType represents a parameterized (generic) class type, e.g.
// BiFunction<Integer, Integer, Integer>. It wraps a raw class name and carries
// concrete type arguments recovered from the Signature attribute.
type JavaParameterizedType struct {
	RawClassName string
	TypeArgs     []JavaType
}

func NewParameterizedType(rawClassName string, typeArgs []JavaType) JavaType {
	return newJavaTypeWrap(&JavaParameterizedType{
		RawClassName: rawClassName,
		TypeArgs:     typeArgs,
	})
}

func (j *JavaParameterizedType) String(funcCtx *class_context.ClassContext) string {
	base := funcCtx.ShortTypeName(j.RawClassName)
	if len(j.TypeArgs) == 0 {
		return base
	}
	parts := make([]string, len(j.TypeArgs))
	for i, ta := range j.TypeArgs {
		parts[i] = ta.String(funcCtx)
	}
	return fmt.Sprintf("%s<%s>", base, strings.Join(parts, ", "))
}

func (j *JavaParameterizedType) IsJavaType() {}

var _ javaType = &JavaParameterizedType{}

// ParseSignature parses a JVM Signature attribute string and returns the
// parameterized JavaType. Returns nil if parsing fails.
func ParseSignature(sig string) JavaType {
	t, _, ok := parseSigType(sig)
	if !ok {
		return nil
	}
	return t
}

func parseSigType(sig string) (JavaType, string, bool) {
	if len(sig) == 0 {
		return nil, "", false
	}
	switch sig[0] {
	case 'B', 'C', 'D', 'F', 'I', 'J', 'S', 'Z', 'V':
		return NewJavaPrimer(primerForSig(sig[0])), sig[1:], true
	case 'L':
		return parseSigClassType(sig)
	case 'T':
		end := strings.IndexByte(sig, ';')
		if end < 0 {
			return nil, "", false
		}
		return newJavaTypeWrap(&JavaClass{Name: sig[1:end]}), sig[end+1:], true
	case '[':
		elem, rest, ok := parseSigType(sig[1:])
		if !ok {
			return nil, "", false
		}
		return NewJavaArrayType(elem), rest, true
	default:
		return nil, "", false
	}
}

func parseSigClassType(sig string) (JavaType, string, bool) {
	rest := sig[1:]
	hasTypeArgs := false
	lt := strings.IndexByte(rest, '<')
	sc := strings.IndexByte(rest, ';')
	nameEnd := len(rest)
	if lt >= 0 && (sc < 0 || lt < sc) {
		nameEnd = lt
		hasTypeArgs = true
	} else if sc >= 0 {
		nameEnd = sc
	} else {
		return nil, "", false
	}
	rawName := SlashToDot(rest[:nameEnd])
	rest = rest[nameEnd:]
	var typeArgs []JavaType
	if hasTypeArgs {
		rest = rest[1:]
		for len(rest) > 0 && rest[0] != '>' {
			// Wildcard type arguments: '*' = "?", '+' = "? extends X", '-' = "? super X".
			// '=' is a CaptureMarker used by javac for capture-of; treat as a plain wildcard.
			if rest[0] == '*' || rest[0] == '=' {
				typeArgs = append(typeArgs, &JavaWildcardType{})
				rest = rest[1:]
				continue
			}
			if rest[0] == '+' || rest[0] == '-' {
				variant := "extends"
				if rest[0] == '-' {
					variant = "super"
				}
				rest = rest[1:]
				ta, remaining, ok := parseSigType(rest)
				if !ok {
					return nil, "", false
				}
				typeArgs = append(typeArgs, &JavaWildcardType{Variant: variant, Bound: ta})
				rest = remaining
				continue
			}
			ta, remaining, ok := parseSigType(rest)
			if !ok {
				return nil, "", false
			}
			typeArgs = append(typeArgs, ta)
			rest = remaining
		}
		if len(rest) == 0 || rest[0] != '>' {
			return nil, "", false
		}
		rest = rest[1:]
	}
	for len(rest) > 0 && rest[0] == '.' {
		innerEnd := 1
		for innerEnd < len(rest) && rest[innerEnd] != ';' && rest[innerEnd] != '<' && rest[innerEnd] != '.' {
			innerEnd++
		}
		rawName += "$" + rest[1:innerEnd]
		rest = rest[innerEnd:]
		if len(rest) > 0 && rest[0] == '<' {
			rest = rest[1:]
			var innerArgs []JavaType
			for len(rest) > 0 && rest[0] != '>' {
				if rest[0] == '*' || rest[0] == '=' {
					innerArgs = append(innerArgs, &JavaWildcardType{})
					rest = rest[1:]
					continue
				}
				if rest[0] == '+' || rest[0] == '-' {
					variant := "extends"
					if rest[0] == '-' {
						variant = "super"
					}
					rest = rest[1:]
					ta, remaining, ok := parseSigType(rest)
					if !ok {
						return nil, "", false
					}
					innerArgs = append(innerArgs, &JavaWildcardType{Variant: variant, Bound: ta})
					rest = remaining
					continue
				}
				ta, remaining, ok := parseSigType(rest)
				if !ok {
					return nil, "", false
				}
				innerArgs = append(innerArgs, ta)
				rest = remaining
			}
			if len(rest) > 0 && rest[0] == '>' {
				rest = rest[1:]
			}
			typeArgs = innerArgs
		}
	}
	if len(rest) == 0 || rest[0] != ';' {
		return nil, "", false
	}
	rest = rest[1:]
	if len(typeArgs) > 0 {
		return newJavaTypeWrap(&JavaParameterizedType{
			RawClassName: rawName,
			TypeArgs:     typeArgs,
		}), rest, true
	}
	return newJavaTypeWrap(&JavaClass{Name: rawName}), rest, true
}

func primerForSig(c byte) string {
	switch c {
	case 'B':
		return JavaByte
	case 'C':
		return JavaChar
	case 'D':
		return JavaDouble
	case 'F':
		return JavaFloat
	case 'I':
		return JavaInteger
	case 'J':
		return JavaLong
	case 'S':
		return JavaShort
	case 'Z':
		return JavaBoolean
	case 'V':
		return JavaVoid
	}
	return JavaInteger
}

func ParseMethodSignature(sig string) ([]JavaType, JavaType) {
	if len(sig) == 0 || sig[0] != '(' {
		return nil, nil
	}
	rest := sig[1:]
	var params []JavaType
	for len(rest) > 0 && rest[0] != ')' {
		t, remaining, ok := parseSigType(rest)
		if !ok {
			return nil, nil
		}
		params = append(params, t)
		rest = remaining
	}
	if len(rest) == 0 || rest[0] != ')' {
		return nil, nil
	}
	rest = rest[1:]
	retType, _, ok := parseSigType(rest)
	if !ok {
		return nil, nil
	}
	return params, retType
}

// ParseClassSignature extracts the type parameters declaration from a class
// signature, e.g. from "<T:Ljava/lang/Object;>Ljava/lang/Object;" returns
// "<T>". Also handles bounds like "<T::Ljava/lang/Comparable<TT;>;>" -> "<T extends Comparable<T>>".
// Returns "" if the class has no type parameters or parsing fails.
func ParseClassSignature(sig string) string {
	if len(sig) == 0 || sig[0] != '<' {
		return ""
	}
	rest := sig[1:]
	var params []string
	for len(rest) > 0 && rest[0] != '>' {
		colonIdx := strings.IndexByte(rest, ':')
		if colonIdx < 0 {
			return ""
		}
		typeParamName := rest[:colonIdx]
		rest = rest[colonIdx:]
		var bounds []string
		for len(rest) > 0 && rest[0] == ':' {
			rest = rest[1:] // skip ':'
			// After skipping ':', if the next char is ':' or '>', the class bound is empty
			// (e.g. "<T::Lcomparable;>" means T has no class bound, only an interface bound).
			if len(rest) > 0 && (rest[0] == ':' || rest[0] == '>') {
				continue
			}
			boundType, remaining, ok := parseSigType(rest)
			if !ok {
				return ""
			}
			rest = remaining
			bounds = append(bounds, boundType.String(&class_context.ClassContext{}))
		}
		if len(bounds) > 0 {
			params = append(params, fmt.Sprintf("%s extends %s", typeParamName, strings.Join(bounds, " & ")))
		} else {
			params = append(params, typeParamName)
		}
	}
	if len(rest) == 0 || rest[0] != '>' {
		return ""
	}
	return "<" + strings.Join(params, ", ") + ">"
}

// ParseClassSignatureSupers parses a class Signature attribute and returns the (possibly generic)
// superclass type followed by the (possibly generic) directly-implemented interface types. The raw
// super_class and Interfaces constant-pool entries are erased; this recovers the type arguments so a
// generic supertype renders as `extends Converter<Integer, Integer>` / `implements Comparator<int[]>`
// instead of the raw form (which fails to override the erased generic methods). Returns (nil, nil) on
// any parse failure so the caller can fall back to the erased names.
func ParseClassSignatureSupers(sig string) (JavaType, []JavaType) {
	rest := sig
	if len(rest) > 0 && rest[0] == '<' {
		r, ok := skipAngleSection(rest)
		if !ok {
			return nil, nil
		}
		rest = r
	}
	sup, rest, ok := parseSigType(rest)
	if !ok {
		return nil, nil
	}
	var ifaces []JavaType
	for len(rest) > 0 {
		it, remaining, ok := parseSigType(rest)
		if !ok {
			return sup, ifaces
		}
		ifaces = append(ifaces, it)
		rest = remaining
	}
	return sup, ifaces
}

// skipAngleSection consumes a leading '<' ... matching '>' run (honoring nested angle brackets) and
// returns the remainder after the matching '>'. Used to skip a class signature's formal type
// parameter section, whose ':' bound syntax parseSigType does not handle.
func skipAngleSection(sig string) (string, bool) {
	if len(sig) == 0 || sig[0] != '<' {
		return sig, false
	}
	depth := 0
	for i := 0; i < len(sig); i++ {
		switch sig[i] {
		case '<':
			depth++
		case '>':
			depth--
			if depth == 0 {
				return sig[i+1:], true
			}
		}
	}
	return "", false
}

// ClassFormalTypeParamNames returns the bare names declared in a leading formal type parameter
// section of a class signature, e.g. "<K:Ljava/lang/Object;V:Ljava/lang/Object;>L..." -> ["K","V"].
// Returns nil when the signature has no leading "<...>" section or cannot be parsed.
func ClassFormalTypeParamNames(sig string) []string {
	if len(sig) == 0 || sig[0] != '<' {
		return nil
	}
	rest := sig[1:]
	var names []string
	for len(rest) > 0 && rest[0] != '>' {
		colonIdx := strings.IndexByte(rest, ':')
		if colonIdx < 0 {
			return names
		}
		names = append(names, rest[:colonIdx])
		rest = rest[colonIdx:]
		for len(rest) > 0 && rest[0] == ':' {
			rest = rest[1:]
			if len(rest) > 0 && (rest[0] == ':' || rest[0] == '>') {
				continue
			}
			_, remaining, ok := parseSigType(rest)
			if !ok {
				return names
			}
			rest = remaining
		}
	}
	return names
}

// scanTypeVarRefs consumes exactly one type signature at sig and appends every TypeVariableSignature
// name (the `T<name>;` form, including those nested inside type arguments and array element types) to
// *out. It mirrors parseSigType's grammar so a `T` is only treated as a type-variable tag when it
// actually starts a type (never when it merely appears inside a class internal name like
// Lcom/example/TestClass;). Returns the remaining string and whether parsing succeeded.
func scanTypeVarRefs(sig string, out *[]string) (string, bool) {
	if len(sig) == 0 {
		return "", false
	}
	switch sig[0] {
	case 'B', 'C', 'D', 'F', 'I', 'J', 'S', 'Z', 'V':
		return sig[1:], true
	case 'T':
		end := strings.IndexByte(sig, ';')
		if end < 0 {
			return "", false
		}
		*out = append(*out, sig[1:end])
		return sig[end+1:], true
	case '[':
		return scanTypeVarRefs(sig[1:], out)
	case 'L':
		return scanClassTypeVarRefs(sig, out)
	default:
		return "", false
	}
}

// scanClassTypeVarRefs consumes one ClassTypeSignature (Lpkg/Name<args>.Inner<args>;) and records the
// type-variable names found in its (possibly nested) type arguments. It does not record the class name
// itself, which is never a type variable.
func scanClassTypeVarRefs(sig string, out *[]string) (string, bool) {
	rest := sig[1:]
	lt := strings.IndexByte(rest, '<')
	sc := strings.IndexByte(rest, ';')
	if lt >= 0 && (sc < 0 || lt < sc) {
		rest = rest[lt:]
	} else if sc >= 0 {
		rest = rest[sc:]
	} else {
		return "", false
	}
	scanArgs := func(r string) (string, bool) {
		r = r[1:] // skip '<'
		for len(r) > 0 && r[0] != '>' {
			switch r[0] {
			case '*':
				r = r[1:]
				continue
			case '+', '-', '=':
				r = r[1:]
				if len(r) > 0 && (r[0] == '>' || r[0] == '*') {
					continue
				}
			}
			remaining, ok := scanTypeVarRefs(r, out)
			if !ok {
				return "", false
			}
			r = remaining
		}
		if len(r) == 0 || r[0] != '>' {
			return "", false
		}
		return r[1:], true
	}
	if len(rest) > 0 && rest[0] == '<' {
		r, ok := scanArgs(rest)
		if !ok {
			return "", false
		}
		rest = r
	}
	for len(rest) > 0 && rest[0] == '.' {
		innerEnd := 1
		for innerEnd < len(rest) && rest[innerEnd] != ';' && rest[innerEnd] != '<' && rest[innerEnd] != '.' {
			innerEnd++
		}
		rest = rest[innerEnd:]
		if len(rest) > 0 && rest[0] == '<' {
			r, ok := scanArgs(rest)
			if !ok {
				return "", false
			}
			rest = r
		}
	}
	if len(rest) == 0 || rest[0] != ';' {
		return "", false
	}
	return rest[1:], true
}

// TypeVarRefsInFieldSig returns the type-variable names referenced in a single field type signature,
// in first-seen order (with duplicates preserved; the caller dedups). E.g. "Ljava/util/List<TV;>;"
// -> ["V"]; "TK;" -> ["K"].
func TypeVarRefsInFieldSig(sig string) []string {
	var out []string
	if _, ok := scanTypeVarRefs(sig, &out); !ok {
		return nil
	}
	return out
}

// FreeTypeVarRefsInClassSig returns the type-variable names referenced in the SUPERTYPE portion of a
// class signature (the superclass + interfaces that follow the formal type parameter section), in
// first-seen order. Subtracting ClassFormalTypeParamNames from these yields the "free" variables a
// nested/inner/anonymous class inherits from an enclosing scope.
func FreeTypeVarRefsInClassSig(sig string) []string {
	rest := sig
	if len(rest) > 0 && rest[0] == '<' {
		r, ok := skipAngleSection(rest)
		if !ok {
			return nil
		}
		rest = r
	}
	var out []string
	for len(rest) > 0 {
		remaining, ok := scanTypeVarRefs(rest, &out)
		if !ok {
			break
		}
		rest = remaining
	}
	return out
}

// ParseMethodSignatureTypeParams extracts formal type parameters from a method
// signature, e.g. "<E:Ljava/lang/Object;>(LList<TE;>;)TE;" returns "<E>".
// Returns "" if the method has no type parameters or parsing fails.
func ParseMethodSignatureTypeParams(sig string) string {
	return parseFormalTypeParams(sig, &class_context.ClassContext{})
}

// ParseMethodSignatureTypeParamsCtx is ParseMethodSignatureTypeParams but renders bound types with
// the given render context so that bound classes in other packages register an import and resolve at
// recompile time (the empty-context form only ever produces the bare name).
func ParseMethodSignatureTypeParamsCtx(sig string, funcCtx *class_context.ClassContext) string {
	return parseFormalTypeParams(sig, funcCtx)
}

// parseFormalTypeParams renders a leading formal type-parameter section ("<T:...>") to Java source
// ("<T>", "<T extends Comparable<T>>", "<K, V>"). A sole `extends Object`/`extends java.lang.Object`
// bound is dropped because it is always redundant (every type variable implicitly extends Object) and
// the bare `<T>` form matches what javac and mature decompilers emit. Returns "" when there is no
// leading section or parsing fails.
func parseFormalTypeParams(sig string, funcCtx *class_context.ClassContext) string {
	if len(sig) == 0 || sig[0] != '<' {
		return ""
	}
	if funcCtx == nil {
		funcCtx = &class_context.ClassContext{}
	}
	rest := sig[1:]
	var params []string
	for len(rest) > 0 && rest[0] != '>' {
		colonIdx := strings.IndexByte(rest, ':')
		if colonIdx < 0 {
			return ""
		}
		typeParamName := rest[:colonIdx]
		rest = rest[colonIdx:]
		var bounds []string
		for len(rest) > 0 && rest[0] == ':' {
			rest = rest[1:]
			if len(rest) > 0 && (rest[0] == ':' || rest[0] == '>') {
				continue
			}
			boundType, remaining, ok := parseSigType(rest)
			if !ok {
				return ""
			}
			rest = remaining
			rendered := boundType.String(funcCtx)
			if rendered == "Object" || rendered == "java.lang.Object" {
				// A `<T extends Object>` bound is always redundant; drop it for the canonical `<T>`.
				continue
			}
			bounds = append(bounds, rendered)
		}
		if len(bounds) > 0 {
			params = append(params, fmt.Sprintf("%s extends %s", typeParamName, strings.Join(bounds, " & ")))
		} else {
			params = append(params, typeParamName)
		}
	}
	if len(rest) == 0 || rest[0] != '>' {
		return ""
	}
	return "<" + strings.Join(params, ", ") + ">"
}

// ParseMethodSignatureFull parses a method Signature attribute that MAY begin with a formal
// type-parameter section ("<T:...>"), which ParseMethodSignature rejects outright. It returns the
// rendered type-parameter string (e.g. "<T>", "<K, V>"; "" when there are none), the parameter types
// and the return type - the latter two with type variables preserved (TT; -> JavaClass{Name:"T"}).
// Returns ("", nil, nil) on any parse failure. For a signature WITHOUT a leading "<...>" section it is
// exactly ParseMethodSignature, so non-generic methods are unaffected. Bound types in the type-param
// string are rendered with funcCtx so other-package bounds register an import.
func ParseMethodSignatureFull(sig string, funcCtx *class_context.ClassContext) (string, []JavaType, JavaType) {
	typeParams := ""
	rest := sig
	if len(rest) > 0 && rest[0] == '<' {
		typeParams = parseFormalTypeParams(rest, funcCtx)
		r, ok := skipAngleSection(rest)
		if !ok {
			return "", nil, nil
		}
		rest = r
	}
	params, ret := ParseMethodSignature(rest)
	if ret == nil {
		return "", nil, nil
	}
	return typeParams, params, ret
}

// MethodFormalTypeParamNames returns the bare names declared in a method signature's leading formal
// type-parameter section, e.g. "<T:Ljava/lang/Object;>(TT;)TT;" -> ["T"]. Returns nil when there is
// no leading "<...>" section. The grammar of a method's formal type-parameter section is identical to
// a class's, so this delegates to ClassFormalTypeParamNames.
func MethodFormalTypeParamNames(sig string) []string {
	return ClassFormalTypeParamNames(sig)
}
