package class_context

import (
	"os"
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
	// TypeParams holds the bare names of the type variables in scope for the class being
	// rendered (its formal type parameters, plus any free variables injected on a flattened
	// inner class). It lets renderers tell a type-variable reference (e.g. `T`/`K`/`V`) apart
	// from an ordinary bare-named class so they can, for instance, emit an unchecked cast when
	// a value erased to a bound is returned from a now-type-variable-typed method.
	TypeParams []string
	// FieldTypeVars maps a (same-class) field's safe identifier to the bare class-scope type
	// variable it is declared as (e.g. `key` -> `K` for `private final K key;`). It is recovered
	// from each field's generic Signature (`TK;`) once per class. A store into such a field whose
	// right-hand side erased to Object/the bound needs an explicit unchecked `(K)` cast to
	// recompile (bytecode erases the field type to its bound, so `this.key = objExpr` otherwise
	// fails "Object cannot be converted to K"). Empty when the class has no type variables.
	FieldTypeVars map[string]string
}

// FieldTypeVar reports the bare class-scope type variable a same-class field is declared as, or ""
// when name is not a recorded type-variable field (see FieldTypeVars).
func (f *ClassContext) FieldTypeVar(name string) string {
	if f == nil || name == "" || f.FieldTypeVars == nil {
		return ""
	}
	return f.FieldTypeVars[name]
}

// IsTypeParam reports whether name is one of the class-scope type variables (see TypeParams).
func (f *ClassContext) IsTypeParam(name string) bool {
	if f == nil || name == "" {
		return false
	}
	for _, p := range f.TypeParams {
		if p == name {
			return true
		}
	}
	return false
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
	if name == "" {
		return "_"
	}
	var b strings.Builder
	for i, r := range name {
		valid := r == '_' || r == '$' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && r >= '0' && r <= '9')
		if valid {
			b.WriteRune(r)
			continue
		}
		if i == 0 && r >= '0' && r <= '9' {
			b.WriteByte('_')
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	name = b.String()
	if _, ok := javaKeywords[name]; ok {
		return name + "_"
	}
	return name
}

func (f *ClassContext) GetAllImported() []string {
	imports := []string{}
	seen := map[string]struct{}{}
	f.BuildInLibsMap.ForEach(func(pkg string, classes []string) bool {
		if pkg == f.PackageName || pkg == "java.lang" {
			return true
		}
		for _, className := range classes {
			// A nested type may have been registered under its binary name (Outer$Inner). How to import
			// it depends on how its REFERENCE site is spelled (see ShortTypeName):
			//   - EXTERNAL stdlib nested type (java.*/javax.*/...): the reference uses the dotted source
			//     spelling (Map.Entry), so import the OUTER class (java.util.Map).
			//   - SAME-JAR Yak-emitted nested type: it is a STANDALONE flat top-level unit literally named
			//     `Outer$Inner` and the reference uses that exact flat name, so the import MUST carry the
			//     flat `$` name too (`import pkg.Outer$Inner;` is legal Java - '$' is an identifier char).
			//     Importing only the OUTER class (legacy behaviour, on the false premise that imports cannot
			//     contain '$') left `Outer$Inner.X` unresolved at every cross-package use site -> javac read
			//     it as `package Outer$Inner` (the single biggest fastjson2 cross-package recompile blocker:
			//     JSONReader$Feature / JSONWriter$Feature). Kill-switch: JDEC_NESTED_FLAT_IMPORT_OFF=1.
			// Anonymous/local segments ($1) are never importable, so drop them in either branch.
			if strings.Contains(className, "$") {
				if _, ok := binaryNestedNameToSource(className); !ok {
					continue
				}
				stdlibOrLegacy := isStdlibNestedDottedPackage(pkg) || os.Getenv("JDEC_NESTED_FLAT_IMPORT_OFF") != ""
				if stdlibOrLegacy {
					src, _ := binaryNestedNameToSource(className)
					outer := src
					if i := strings.IndexByte(src, '.'); i >= 0 {
						outer = src[:i]
					}
					className = outer
				}
				// else: keep the flat `Outer$Inner` name so the import matches the flat reference.
			}
			imp := pkg + "." + className
			if _, dup := seen[imp]; dup {
				continue
			}
			seen[imp] = struct{}{}
			imports = append(imports, imp)
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
	if pkg == "" || pkg == "java.lang" {
		return
	}
	if className != "*" {
		className = SafeIdentifier(className)
	}
	if f.KeySet.Has(className) {
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
// stdlibNestedDottedPackages enumerates the package prefixes whose nested types are guaranteed to be
// JDK / standard-library types (never a Yak-emitted flat unit, and always present on the compile
// classpath as genuinely nested Outer.Inner). For these a nested-type REFERENCE must use the dotted
// Java source spelling (Map.Entry), not the binary flat name (Map$Entry) Yak uses for its own units.
func isStdlibNestedDottedPackage(pkg string) bool {
	switch {
	case pkg == "java" || strings.HasPrefix(pkg, "java."):
		return true
	case pkg == "javax" || strings.HasPrefix(pkg, "javax."):
		return true
	case pkg == "jdk" || strings.HasPrefix(pkg, "jdk."):
		return true
	case pkg == "sun" || strings.HasPrefix(pkg, "sun."):
		return true
	case strings.HasPrefix(pkg, "com.sun."):
		return true
	case strings.HasPrefix(pkg, "org.w3c."):
		return true
	case strings.HasPrefix(pkg, "org.xml."):
		return true
	case strings.HasPrefix(pkg, "org.ietf."):
		return true
	case strings.HasPrefix(pkg, "org.omg."):
		return true
	}
	return false
}

func (f *ClassContext) ShortTypeName(name string) string {
	pkg, className := SplitPackageClassName(name)
	className = SafeIdentifier(className)
	if pkg == "" {
		return className
	}
	// A reference to an EXTERNAL standard-library nested type must use the dotted Java source spelling
	// (java.util.Map.Entry -> Map.Entry), never the binary flat name (Map$Entry). Yak emits its OWN
	// nested classes as standalone flat `Outer$Inner` units and references them by that same flat name
	// so the whole decompiled set recompiles together; but a JDK/stdlib nested type is only present on
	// the compile classpath as a genuinely nested Outer.Inner and is unresolvable as `Outer$Inner` in
	// source (this was the single largest guava/spring recompile blocker - hundreds of `Map$Entry`
	// "cannot find symbol"). java.*/javax.*/... can never be a Yak unit, so the conversion is always
	// safe. The import statement still carries the OUTER class (see GetAllImported). Kill-switch:
	// JDEC_STDLIB_NESTED_DOT_OFF=1 restores the legacy flat spelling.
	dotted := className
	if strings.Contains(className, "$") && os.Getenv("JDEC_STDLIB_NESTED_DOT_OFF") == "" && isStdlibNestedDottedPackage(pkg) {
		if src, ok := binaryNestedNameToSource(className); ok {
			dotted = src
		}
	}
	if pkg == f.PackageName || pkg == "java.lang" {
		return dotted
	}
	f.Import(name)
	if f.BuildInLibsMap == nil {
		f.BuildInLibsMap = omap.NewEmptyOrderedMap[string, []string]()
	}
	libs := f.BuildInLibsMap.GetMust(pkg)
	if len(libs) > 0 && (funk.Contains(libs, className) || libs[0] == "*") {
		return dotted
	}
	//f.BuildInLibsMap.Set(pkg, append(f.BuildInLibsMap.GetMust(pkg), className))
	return pkg + "." + dotted
}

// binaryNestedNameToSource converts a binary nested class simple name (Outer$Inner$Deeper) into its
// Java source spelling (Outer.Inner.Deeper). It returns ok=false when the name is not nested or when
// any segment is anonymous/local (a segment that is empty or begins with a digit, e.g. Outer$1).
// NOTE: Yak emits each nested class as a STANDALONE top-level unit literally named `Outer$Inner`
// ('$' is a legal Java identifier char) and references it by that same flat name, which is internally
// consistent and recompiles when the whole decompiled source set is compiled together (the standard
// decompiler round-trip). This helper is therefore only used to keep import statements legal (an
// import line cannot contain '$'); type references stay flat to match the flat declarations.
func binaryNestedNameToSource(className string) (string, bool) {
	if !strings.Contains(className, "$") {
		return className, false
	}
	parts := strings.Split(className, "$")
	for _, p := range parts {
		if p == "" || (p[0] >= '0' && p[0] <= '9') {
			return className, false
		}
	}
	return strings.Join(parts, "."), true
}

func SplitPackageClassName(s string) (string, string) {
	splits := strings.Split(s, ".")
	if len(splits) > 0 {
		return strings.Join(splits[:len(splits)-1], "."), splits[len(splits)-1]
	}
	log.Errorf("split package name and class name failed: %v", s)
	return "", ""
}
