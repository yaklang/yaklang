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

// ParseMethodSignatureTypeParams extracts formal type parameters from a method
// signature, e.g. "<E:Ljava/lang/Object;>(LList<TE;>;)TE;" returns "<E>".
// Returns "" if the method has no type parameters or parsing fails.
func ParseMethodSignatureTypeParams(sig string) string {
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
			rest = rest[1:]
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
