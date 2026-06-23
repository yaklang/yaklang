package types

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

// SlashToDot converts a JVM internal class name ("java/lang/String") to its binary
// form ("java.lang.String"). It is on the hottest decompiler path (every descriptor and
// every constant-pool class reference).
//
// vs strings.Replace(s, "/", ".", -1): strings.Replace scans the input twice (a Count
// pre-pass plus the copy loop). We scan once with IndexByte and copy the runs between
// separators in bulk, which benchmarks at or below strings.Replace while keeping the
// zero-allocation fast path for already-dotted / separator-free names. (A naive
// byte-at-a-time loop was measurably slower than strings.Replace, so don't "simplify" to
// one.)
func SlashToDot(s string) string {
	i := strings.IndexByte(s, '/')
	if i < 0 {
		return s
	}
	var sb strings.Builder
	sb.Grow(len(s))
	for {
		sb.WriteString(s[:i])
		sb.WriteByte('.')
		s = s[i+1:]
		i = strings.IndexByte(s, '/')
		if i < 0 {
			sb.WriteString(s)
			break
		}
	}
	return sb.String()
}

// javaClassTypeCache memoizes the immutable *JavaClass produced for each JVM internal
// class name. Descriptor parsing is one of the hottest paths (every method/field/lambda
// descriptor) and the same names ("java/lang/Object", ...) recur enormously, so this
// turns a strings.Replace plus an object allocation per occurrence into one map lookup.
//
// Safety: only the *JavaClass leaf is shared. Its Name is never mutated (type-inference
// rewrites such as ResetType/ResetTypeRef/MergeTypes mutate the surrounding
// JavaTypeWrap/javaTypeRef, which callers still allocate fresh via newJavaTypeWrap on
// every parse), so sharing the leaf across callers and goroutines is race-free. The key
// is the internal ('/') form so a cache hit also skips slashToDot.
var javaClassTypeCache sync.Map // map[string]*JavaClass
var javaClassTypeCacheLen atomic.Int64

// javaClassTypeCacheCap soft-bounds the flyweight cache so Decompile stays memory-safe
// in long-running hosts (e.g. yakit) that may process unbounded distinct class names.
// The hottest names (java/lang/*, ...) recur immediately and are interned first, so the
// cap barely affects the hit rate while preventing unbounded growth. It is a soft cap:
// a benign race may let it overshoot slightly, which is harmless.
const javaClassTypeCacheCap = 1 << 16

func cachedClassType(internalName string) *JavaClass {
	if v, ok := javaClassTypeCache.Load(internalName); ok {
		return v.(*JavaClass)
	}
	jc := &JavaClass{Name: SlashToDot(internalName)}
	if javaClassTypeCacheLen.Load() >= javaClassTypeCacheCap {
		// Cache is full; return a fresh (uncached) instance. Still immutable and correct,
		// just not interned.
		return jc
	}
	actual, loaded := javaClassTypeCache.LoadOrStore(internalName, jc)
	if !loaded {
		javaClassTypeCacheLen.Add(1)
	}
	return actual.(*JavaClass)
}

func GetPrimerArrayType(id int) JavaType {
	switch id {
	case 4:
		return NewJavaPrimer(JavaBoolean)
	case 5:
		return NewJavaPrimer(JavaChar)
	case 6:
		return NewJavaPrimer(JavaFloat)
	case 7:
		return NewJavaPrimer(JavaDouble)
	case 8:
		return NewJavaPrimer(JavaByte)
	case 9:
		return NewJavaPrimer(JavaShort)
	case 10:
		return NewJavaPrimer(JavaInteger)
	case 11:
		return NewJavaPrimer(JavaLong)
	default:
		return nil
	}
}
func ParseDescriptor(descriptor string) (JavaType, error) {
	returnType, _, err := ParseJavaDescription(descriptor)
	return returnType, err
}

// ParseMethodDescriptor 解析 Java 方法描述符
func ParseMethodDescriptor(descriptor string) (JavaType, error) {
	if descriptor == "" {
		return nil, fmt.Errorf("descriptor is empty")
	}

	if descriptor[0] != '(' {
		return nil, fmt.Errorf("invalid descriptor format")
	}

	// 查找参数部分和返回类型部分
	endIndex := strings.Index(descriptor, ")")
	if endIndex == -1 {
		return nil, fmt.Errorf("invalid descriptor format")
	}

	paramDescriptor := descriptor[1:endIndex]
	returnTypeDescriptor := descriptor[endIndex+1:]

	// 解析参数类型
	paramTypes, err := parseTypes(paramDescriptor)
	if err != nil {
		return nil, err
	}

	// 解析返回类型
	returnType, _, err := ParseJavaDescription(returnTypeDescriptor)
	if err != nil {
		return nil, err
	}

	return newJavaTypeWrap(NewJavaFuncType(descriptor, paramTypes, returnType)), nil
}

// parseTypes 解析多个类型描述符
func parseTypes(descriptor string) ([]JavaType, error) {
	var types []JavaType
	for len(descriptor) > 0 {
		t, rest, err := ParseJavaDescription(descriptor)
		if err != nil {
			return nil, err
		}
		types = append(types, t)
		descriptor = rest
	}
	return types, nil
}

// ParseJavaDescription 解析单个类型描述符
func ParseJavaDescription(descriptor string) (JavaType, string, error) {
	if len(descriptor) == 0 {
		return nil, "", fmt.Errorf("empty descriptor")
	}

	switch descriptor[0] {
	case 'B':
		return NewJavaPrimer(JavaByte), descriptor[1:], nil
	case 'C':
		return NewJavaPrimer(JavaChar), descriptor[1:], nil
	case 'D':
		return NewJavaPrimer(JavaDouble), descriptor[1:], nil
	case 'F':
		return NewJavaPrimer(JavaFloat), descriptor[1:], nil
	case 'I':
		return NewJavaPrimer(JavaInteger), descriptor[1:], nil
	case 'J':
		return NewJavaPrimer(JavaLong), descriptor[1:], nil
	case 'S':
		return NewJavaPrimer(JavaShort), descriptor[1:], nil
	case 'Z':
		return NewJavaPrimer(JavaBoolean), descriptor[1:], nil
	case 'V':
		return NewJavaPrimer(JavaVoid), descriptor[1:], nil
	case 'L':
		endIndex := strings.IndexByte(descriptor, ';')
		if endIndex == -1 {
			return nil, "", fmt.Errorf("invalid class descriptor format")
		}
		return newJavaTypeWrap(cachedClassType(descriptor[1:endIndex])), descriptor[endIndex+1:], nil
	case '[':
		elemType, rest, err := ParseJavaDescription(descriptor[1:])
		if err != nil {
			return nil, "", err
		}
		return NewJavaArrayType(elemType), rest, nil
	default:
		return nil, "", fmt.Errorf("unknown type descriptor: %c", descriptor[0])
	}
}
