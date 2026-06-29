package javaclassparser

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"runtime/debug"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	utils2 "github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type ClassObjectDumper struct {
	obj           *ClassObject
	FuncCtx       *class_context.ClassContext
	ClassName     string
	PackageName   string
	CurrentMethod *MemberInfo
	ConstantPool  []ConstantInfo
	deepStack     *utils.Stack[int]
	MethodType    *types.JavaFuncType
	lambdaMethods map[string][]string
	// lambdaCaptureCount records, per synthetic lambda impl method (keyed by name+descriptor),
	// how many leading parameters are captured variables that javac prepended to the impl
	// signature. They are not lambda parameters: DumpMethodWithInitialId drops them from the arrow
	// parameter list and renames them to capture placeholders that the invokedynamic call site
	// resolves to the actual captured values.
	lambdaCaptureCount map[string]int
	// lambdaLocalSeq hands each inlined lambda body a unique id so its own locals can be renamed
	// into a private `lv<seq>_<n>` namespace. A lambda arrow body is spliced INLINE into the
	// enclosing method, and Java forbids a local declared in the lambda body from shadowing a
	// local/parameter of the enclosing scope (or a captured variable, which resolves to an
	// enclosing `varN`). The fresh-root id namespace gives lambda locals var0,var1,... that collide
	// with the enclosing method's var0,var1,..., producing "variable varN is already defined".
	// Renaming them per-lambda eliminates the collision; nested lambdas are dumped first and already
	// carry their own `lv<innerseq>` names, so the outer rename (matching only `varN`) never touches
	// them. See renameLambdaBodyLocals.
	lambdaLocalSeq    int
	fieldDefaultValue map[string]string
	dumpedMethodsSet   map[string]*dumpedMethods
	// aggressive marks that the CURRENT method dump is a second attempt for a method whose
	// conservative decompilation already failed. While set, the decompiler enables higher-risk
	// reconstruction paths (relaxed structuring, node-duplication, synthetic rebuilds). It is
	// toggled per-method by aggressiveRedumpMethod and is otherwise always false, so methods that
	// decompile cleanly on the first pass are never affected (zero regression by construction).
	aggressive bool
	// aggressiveRetried records methods (name+desc) already attempted in aggressive mode, so a
	// method that reaches both degradation points (DumpMethods and degradeInvalidMethods) is
	// re-decompiled at most once; the aggressive path is deterministic, so repeating is pointless.
	aggressiveRetried map[string]bool
	// fieldStoreTotals counts, per field name, how many putfield/putstatic targets it has across
	// ALL of this class's <init> and <clinit> bodies. It is computed lazily (and cached) by a
	// read-only opcode pre-scan and is used to suppress field-initializer hoisting for blank-final
	// fields that are assigned in more than one place (multiple constructors or multiple branches),
	// which would otherwise emit an illegal double assignment to a final field. A nil map means the
	// pre-scan has not run yet; an entry of 0 means "not seen", so callers treat <=1 as hoistable.
	fieldStoreTotals map[string]int
	// methodReturnTypes maps a same-class method name to its rendered return type, used by the
	// generated-local safety net to recover the type of a reference local that only receives its
	// value through an embedded assignment `(v = m(...)) != null`. Names that are overloaded with
	// DIFFERENT return types are omitted (ambiguous). Built lazily and cached.
	methodReturnTypes map[string]string
	// foldSiblingResolver, when non-nil, resolves a sibling class's raw bytes by its binary internal
	// name (slash form, e.g. "ev/EnumBody$1"). It is the hook that enables enum constant-body
	// CROSS-CLASS folding: a constant-specific class body is compiled by javac into a synthetic
	// `Outer$N` subclass, and the only legal Java is to inline that subclass's members back into the
	// enum constant (`CONST { ...body... }`). The standalone single-class entry (Decompile) leaves
	// this nil, so folding is OFF and per-class output is byte-for-byte unchanged (zero regression by
	// construction); only the multi-class entry (DecompileWithResolver / jar path) sets it.
	foldSiblingResolver func(internalName string) ([]byte, bool)
}

func (c *ClassObjectDumper) GetConstructorMethodName() string {
	if c.PackageName == "" {
		return c.ClassName
	}
	after, ok := strings.CutPrefix(c.ClassName, c.PackageName+".")
	if ok {
		return after
	}
	log.Error("GetConstructorMethodName failed")
	return ""
}
func NewClassObjectDumper(obj *ClassObject) *ClassObjectDumper {
	return &ClassObjectDumper{
		obj:                obj,
		ConstantPool:       obj.ConstantPool,
		deepStack:          utils.NewStack[int](),
		lambdaMethods:      map[string][]string{},
		lambdaCaptureCount: map[string]int{},
		fieldDefaultValue:  map[string]string{},
		dumpedMethodsSet:   map[string]*dumpedMethods{},
		aggressiveRetried:  map[string]bool{},
	}
}
func (c *ClassObjectDumper) TabNumber() int {
	return c.deepStack.Peek()
}
func (c *ClassObjectDumper) GetTabString() string {
	return strings.Repeat("\t", c.deepStack.Peek())
}
func (c *ClassObjectDumper) Tab() {
	pre := c.deepStack.Peek()
	if pre == 0 {
		c.deepStack.Push(1)
	} else {
		c.deepStack.Push(pre + 1)
	}
}
func (c *ClassObjectDumper) UnTab() {
	c.deepStack.Pop()
}
// selfInnerClassAccessFlags returns the inner_class_access_flags this class carries in its own
// InnerClasses entry (the entry whose inner_class_info refers to this very class), with ok=false when
// no such entry exists. For a nested type the top-level ClassFile access_flags omit its real
// visibility (a public nested type's own access_flags lack ACC_PUBLIC); the authoritative visibility
// is in the InnerClasses attribute. javap reads exactly this to print `public` for a nested type.
func (c *ClassObjectDumper) selfInnerClassAccessFlags() (uint16, bool) {
	self := c.obj.GetClassName()
	if self == "" {
		return 0, false
	}
	for _, attr := range c.obj.Attributes {
		ic, ok := attr.(*InnerClassesAttribute)
		if !ok {
			continue
		}
		for _, e := range ic.Classes {
			if e == nil || e.InnerClassInfoIndex == 0 {
				continue
			}
			name, err := c.obj.getUtf8(e.InnerClassInfoIndex)
			if err != nil {
				continue
			}
			if name == self {
				return e.InnerClassAccessFlags, true
			}
		}
	}
	return 0, false
}

func (c *ClassObjectDumper) DumpClass() (string, error) {
	// accessFlagsVerbose := c.obj.AccessFlagsVerbose
	accessFlagsToCode := c.obj.AccessFlagsToCode

	nonClassKeyword := false
	isInterface := false
	isEnum := false
	isAnnotation := false
	syntheticEnumSubclass := false
	superRawName := strings.Replace(c.obj.GetSupperClassName(), "/", ".", -1)
	for _, k := range c.obj.AccessFlagsVerbose {
		if k == "interface" || k == "enum" || k == "annotation" {
			if k == "interface" {
				isInterface = true
			} else if k == "annotation" {
				isAnnotation = true
			} else if k == "enum" {
				// A genuine enum extends java.lang.Enum directly. Synthetic enum-constant
				// subclasses (e.g. Foo$1) carry ACC_ENUM but extend the enum type itself and
				// cannot be declared with the `enum` keyword; render them as ordinary classes.
				if superRawName != "java.lang.Enum" {
					syntheticEnumSubclass = true
					break
				}
				isEnum = true
			}

			nonClassKeyword = true
			break
		}
	}

	//if len(accessFlagsVerbose) < 1 {
	//	return "", utils.Error("accessFlagsVerbose is empty")
	//}
	accessFlags := accessFlagsToCode
	if syntheticEnumSubclass {
		// Drop the `enum` keyword so the synthetic subclass renders as a normal class.
		accessFlags = strings.TrimSpace(strings.ReplaceAll(accessFlags, "enum", ""))
	}
	name := c.obj.GetClassName()
	splits := strings.Split(name, "/")
	packageName := strings.Join(splits[:len(splits)-1], ".")
	c.PackageName = packageName
	rawClassName := splits[len(splits)-1]
	className := class_context.SafeIdentifier(rawClassName)
	// Nested/local/anonymous classes carry a '$' in their binary name (Outer$Inner). Yak emits each
	// such class as a STANDALONE top-level unit literally named `Outer$Inner` and writes it to
	// `Outer$Inner.java` ('$' is a legal Java identifier char), so a `public` modifier IS legal here
	// (Java only requires that a file's public top-level class match the file name). `protected` is
	// illegal at top level and is always dropped (demoted to package-private).
	//
	// Crucially, a nested type's REAL visibility lives in the InnerClasses attribute's
	// inner_class_access_flags, NOT in the (visibility-less) top-level ClassFile access_flags: a public
	// nested type's own access_flags lack ACC_PUBLIC, so the legacy unconditional public-stripping left
	// every public nested type package-private. Cross-package use sites then failed to recompile with
	// `... is defined in an inaccessible class or interface` and `package Outer$Inner does not exist`
	// (the single biggest fastjson2 blocker: JSONReader$Feature / JSONWriter$Feature / JSONReader$Context).
	// Recover ACC_PUBLIC from InnerClasses (this is exactly what javap consults) and keep `public` for a
	// genuinely public nested type. Kill-switch: JDEC_NESTED_PUBLIC_OFF=1 restores legacy stripping.
	if strings.Contains(rawClassName, "$") {
		accessFlags = strings.TrimSpace(strings.ReplaceAll(accessFlags, "protected", ""))
		innerPublic := false
		if os.Getenv("JDEC_NESTED_PUBLIC_OFF") == "" {
			if flags, ok := c.selfInnerClassAccessFlags(); ok && flags&0x0001 == 0x0001 {
				innerPublic = true
			}
		}
		if innerPublic {
			if !strings.Contains(accessFlags, "public") {
				accessFlags = strings.TrimSpace("public " + accessFlags)
			}
		} else {
			accessFlags = strings.TrimSpace(strings.ReplaceAll(accessFlags, "public", ""))
		}
	}
	// module-info / package-info are synthetic descriptor pseudo-classes; their internal
	// name ("module-info" / "package-info") is not a legal Java identifier, so emitting
	// `class module-info {}` yields un-parseable source. Render a valid minimal compilation
	// unit instead. (Full JPMS module / package-info annotation reconstruction is a
	// separate feature.)
	if rawClassName == "module-info" || rawClassName == "package-info" {
		var sb strings.Builder
		if rawClassName == "package-info" && packageName != "" {
			sb.WriteString(fmt.Sprintf("package %s;\n\n", packageName))
		}
		sb.WriteString(fmt.Sprintf("// decompiled from a synthetic %s descriptor\n", rawClassName))
		return sb.String(), nil
	}
	supperClassName := c.obj.GetSupperClassName()
	supperClassName = strings.Replace(supperClassName, "/", ".", -1)
	if packageName == "" {
		c.ClassName = className
	} else {
		c.ClassName = packageName + "." + className
	}
	funcCtx := &class_context.ClassContext{
		ClassName:       c.ClassName,
		SupperClassName: supperClassName,
		PackageName:     c.PackageName,
	}
	c.FuncCtx = funcCtx
	buildInLib := []string{
		//c.PackageName + ".*",
		c.ClassName,
		"java.lang.*",
		//"java.io.*",
	}
	for _, s := range buildInLib {
		funcCtx.Import(s)
	}
	// Recover generic supertypes from the class Signature attribute: the raw super_class and
	// Interfaces constant-pool entries are erased, so a class like `Ints$IntConverter extends
	// Converter<Integer, Integer>` or `enum LexicographicalComparator implements Comparator<int[]>`
	// would otherwise render with raw supertypes and fail to override the erased generic methods.
	// Keyed by the raw dotted class name so it can be matched against each erased supertype below.
	// Kill-switch: JDEC_GENERIC_SUPERS_OFF=1 restores the erased supertypes.
	genericSuperByRaw := map[string]string{}
	if os.Getenv("JDEC_GENERIC_SUPERS_OFF") == "" {
		for _, attr := range c.obj.Attributes {
			sigAttr, ok := attr.(*SignatureAttribute)
			if !ok {
				continue
			}
			sigStr, err := c.obj.getUtf8(sigAttr.SignatureIndex)
			if err != nil || sigStr == "" {
				break
			}
			sup, sigIfaces := types.ParseClassSignatureSupers(sigStr)
			recordGeneric := func(t types.JavaType) {
				raw := genericSupertypeRawName(t)
				if raw == "" {
					return
				}
				rendered := t.String(funcCtx)
				// Only override when the recovered type actually carries type arguments; a raw
				// supertype in the signature adds nothing and must not shadow the erased name.
				if strings.Contains(rendered, "<") {
					genericSuperByRaw[raw] = rendered
				}
			}
			if sup != nil {
				recordGeneric(sup)
			}
			for _, it := range sigIfaces {
				recordGeneric(it)
			}
			break
		}
	}

	superStr := ""
	ifaces := c.obj.Interfaces
	interfaceLists := make([]string, 0, len(ifaces)+1)
	if supperClassName != "java.lang.Object" {
		if isEnum && (supperClassName == "java.lang.Enum" || supperClassName == "Enum") {
			supperClassName = ""
			superStr = ""
		} else {
			funcCtx.Import(supperClassName)
			rawSuper := supperClassName
			supperClassName = funcCtx.ShortTypeName(supperClassName)
			if generic, ok := genericSuperByRaw[rawSuper]; ok {
				supperClassName = generic
			}
			if supperClassName != "" {
				if !isEnum {
					superStr += fmt.Sprintf(" extends %s", supperClassName)
				} else {
					interfaceLists = append(interfaceLists, supperClassName)
				}
			}
		}
	}

	for _, u := range ifaces {
		info, err := c.obj.getConstantInfo(u)
		if err != nil {
			continue
		}
		classInfo := info.(*ConstantClassInfo)
		name, err := c.obj.getUtf8(classInfo.NameIndex)
		if err != nil {
			continue
		}
		rawIfaceName := strings.Replace(name, "/", ".", -1)
		// An annotation type implicitly extends java.lang.annotation.Annotation; emitting it
		// explicitly ("@interface M extends Annotation") is illegal Java, so drop it.
		if isAnnotation && rawIfaceName == "java.lang.annotation.Annotation" {
			continue
		}
		name = funcCtx.ShortTypeName(rawIfaceName)
		if generic, ok := genericSuperByRaw[rawIfaceName]; ok {
			name = generic
		}
		if name != "" {
			interfaceLists = append(interfaceLists, name)

		}
	}
	if len(interfaceLists) > 0 {
		if isInterface {
			superStr += fmt.Sprintf(" extends %s", strings.Join(interfaceLists, ", "))
		} else {
			superStr += fmt.Sprintf(" implements %s", strings.Join(interfaceLists, ", "))
		}
	}

	if packageName == "" {
		packageName = "defaultpackagename"
	}
	// Extract class-level type parameters from the Signature attribute so that
	// fields/methods referencing type variables (e.g. `T value`) compile. A class
	// without generic parameters or without a Signature attribute yields "".
	classTypeParams := ""
	classSigStr := ""
	for _, attr := range c.obj.Attributes {
		if sigAttr, ok := attr.(*SignatureAttribute); ok {
			if sigStr, err := c.obj.getUtf8(sigAttr.SignatureIndex); err == nil && sigStr != "" {
				classSigStr = sigStr
				if tp := types.ParseClassSignature(sigStr); tp != "" {
					classTypeParams = tp
				}
			}
			break
		}
	}
	// A non-static inner / local / anonymous class inherits type variables from its enclosing scope.
	// When Yak flattens it to a top-level `Outer$Inner` unit, those variables lose their declaration:
	// `class AbstractMapBasedMultimap$WrappedList extends AbstractMapBasedMultimap$WrappedCollection<K, V>
	// implements List<V>` references K, V that nothing declares -> javac "cannot find symbol: class K".
	// This was the single largest remaining guava recompile blocker (~2000 undeclared type-variable
	// errors across the Multimap/Table/cache inner-class families). Recover the variables this unit
	// actually USES by scanning its own supertype + field signatures for TypeVariableSignature references
	// and declaring them on the flattened class. Those positions can only reference class- or
	// enclosing-class-level variables (never method-level ones), so this never clashes with a method's
	// own `<T>`. Bounds default to Object, matching the common unbounded enclosing variable; a bounded
	// enclosing variable used in a bound-requiring position is a known residual. Kill-switch:
	// JDEC_INNER_TYPEVAR_OFF=1.
	//
	// RESTRICTED to classes that declare NO formal type parameters of their own. For such a
	// pure-inherited inner class the flattened reference sites still carry the enclosing type arguments
	// (parseSigClassType keeps the outer args of `LOuter<..>.Inner;`), so injecting the matching free
	// variables is arity-consistent. An inner class that ALSO has its own parameters (e.g.
	// MapMakerInternalMap$HashIterator<T>) renders references with only its own-param arity, so injecting
	// the enclosing variables would make declaration and reference arities disagree ("wrong number of
	// type arguments"); those are left to the future cross-class integral rebuild. A self-contained
	// top-level class references no free variables, so this is a strict no-op for it.
	// classTypeParamNames tracks the bare names of the type variables in scope for this class
	// (its own formal parameters, or the free variables injected on a flattened inner class).
	// It is propagated to the render context so statement renderers can recognize type-variable
	// references (e.g. to emit an unchecked `(T)` cast on an erased return value).
	classTypeParamNames := types.ClassFormalTypeParamNames(classSigStr)
	if os.Getenv("JDEC_INNER_TYPEVAR_OFF") == "" && len(classTypeParamNames) == 0 {
		seen := map[string]bool{}
		var free []string
		addRef := func(n string) {
			if n == "" || seen[n] {
				return
			}
			seen[n] = true
			free = append(free, n)
		}
		if classSigStr != "" {
			for _, n := range types.FreeTypeVarRefsInClassSig(classSigStr) {
				addRef(n)
			}
		}
		for _, field := range c.obj.Fields {
			for _, fattr := range field.Attributes {
				if sa, ok := fattr.(*SignatureAttribute); ok {
					if fs, err := c.obj.getUtf8(sa.SignatureIndex); err == nil && fs != "" {
						for _, n := range types.TypeVarRefsInFieldSig(fs) {
							addRef(n)
						}
					}
					break
				}
			}
		}
		// Variables are emitted in first-seen (supertype-then-field) order. The canonical enclosing order
		// is NOT recoverable from single-class bytecode (the synthetic this$0 field is erased to the raw
		// enclosing type with no Signature, and InnerClasses carries only names), so a sibling override
		// chain can occasionally bind a variable to a swapped position; that residual is left to the
		// future cross-class integral rebuild.
		if len(free) > 0 {
			classTypeParams = "<" + strings.Join(free, ", ") + ">"
			classTypeParamNames = free
		}
	}
	if c.FuncCtx != nil {
		c.FuncCtx.TypeParams = classTypeParamNames
	}
	packageSource := fmt.Sprintf("package %s;\n\n", packageName)
	if className == "" {
		return "", utils.Error("className is empty")
	}

	annoStrs := []string{}
	for _, info := range lo.Filter(c.obj.Attributes, func(item AttributeInfo, index int) bool {
		_, ok := item.(*RuntimeVisibleAnnotationsAttribute)
		return ok
	}) {
		for _, annotation := range info.(*RuntimeVisibleAnnotationsAttribute).Annotations {
			res, err := c.DumpAnnotation(annotation)
			if err != nil {
				return "", utils.Wrap(err, "DumpAnnotation failed")
			}
			annoStrs = append(annoStrs, res)
		}
	}
	methods, err := c.DumpMethods()
	if err != nil {
		return "", utils.Wrap(err, "DumpMethods failed")
	}
	fields, err := c.DumpFields()
	if err != nil {
		return "", utils.Wrap(err, "DumpFields failed")
	}
	// Enum constant-body cross-class folding: when a multi-class resolver is available, recover each
	// constant's synthetic `Outer$N` subclass body and inline it as `CONST { ...body... }`. Computed
	// once (before assemble, which may run twice on the degradation path) so required imports are
	// merged into funcCtx ahead of the import-assembly step below. Empty (nil) on the single-class
	// path, leaving the constant render hook untouched.
	enumConstantBodies := c.foldEnumConstantBodies(isEnum)
	var classKeyword string
	if !nonClassKeyword {
		classKeyword = " class"
	}
	// assemble renders the full compilation unit from the current methods/fields. It is a
	// closure so the syntax safety net can re-render after degrading malformed members.
	assemble := func() string {
		// strings.Builder instead of `attrs += ...`: a class with many methods otherwise
		// triggers O(n^2) string concatenation (each += re-copies the whole accumulated
		// body), which profiling flagged as a top dumper allocator. The builder produces
		// the exact same bytes in O(n).
		var attrsB strings.Builder
		if len(fields) > 0 {
			attrsB.WriteString("\n\t// Fields\n")
			enumFields := make([]dumpedFields, 0, len(fields))
			ordinaryFields := make([]string, 0, len(fields))
			for _, field := range fields {
				if isEnum && field.typeName == className && (field.modifier == "public static final enum" || field.modifier == "public static final") {
					enumFields = append(enumFields, field)
					continue
				}
				ordinaryFields = append(ordinaryFields, field.code)
			}
			for idx, enumSimple := range enumFields {
				constStr := enumSimple.fieldName
				if args := c.enumConstantArgs(enumSimple.fieldName); args != "" {
					constStr += "(" + args + ")"
				}
				attrsB.WriteString("\t")
				attrsB.WriteString(constStr)
				if body := enumConstantBodies[enumSimple.fieldName]; body != "" {
					attrsB.WriteString(body)
				}
				if idx == len(enumFields)-1 {
					attrsB.WriteString(";\n")
				} else {
					attrsB.WriteString(",\n")
				}
			}
			if isEnum && len(enumFields) == 0 && (len(ordinaryFields) > 0 || len(methods) > 0) {
				// Java requires a separator before enum body declarations when the constant list is empty:
				// `enum E { ; int x; }`.
				attrsB.WriteString("\t;\n")
			}
			for _, ordinaryField := range ordinaryFields {
				attrsB.WriteString("\t")
				attrsB.WriteString(ordinaryField)
				attrsB.WriteString("\n")
			}
		}
		if isEnum && len(fields) == 0 && len(methods) > 0 {
			attrsB.WriteString("\n\t;\n")
		}
		if len(methods) > 0 {
			attrsB.WriteString("\n")
			for _, method := range methods {
				attrsB.WriteString("\t")
				attrsB.WriteString(method.code)
				attrsB.WriteString("\n")
			}
		}
		attrs := attrsB.String()
		result := fmt.Sprintf("%s%s %s%s%s {%s}", accessFlags, classKeyword, className, classTypeParams, superStr, attrs)
		if len(annoStrs) > 0 {
			result = fmt.Sprintf("%s\n%s", strings.Join(annoStrs, "\n"), result)
		}
		importsStr := ""
		for _, s := range funcCtx.GetAllImported() {
			if utils.StringSliceContain(buildInLib, s) {
				continue
			}
			// Import spelling is already normalized by GetAllImported per type kind:
			//   - EXTERNAL stdlib nested type -> reduced to the OUTER class (java.util.Map), no '$';
			//     the body renders the dotted Outer.Inner source spelling against that import.
			//   - SAME-JAR Yak flat unit -> kept as the flat `pkg.Outer$Inner` name; the body renders
			//     the matching flat `Outer$Inner` reference and the import resolves to the sibling
			//     flat unit `Outer$Inner.java` ('$' is a legal identifier char, so `import a.b.C$D;`
			//     is valid Java - verified). Rewriting '$'->'.' here (legacy behaviour, on the false
			//     premise that imports cannot carry '$') turned it into `import pkg.Outer.Inner;`, which
			//     does NOT resolve because Yak's flat `Outer.java` has no nested `Inner` (it is a
			//     separate flat unit) - the second-largest fastjson2 cross-package recompile blocker.
			// So emit the (already correct) import string verbatim.
			importsStr += fmt.Sprintf("import %s;\n", s)
		}
		if len(importsStr) > 0 {
			importsStr += "\n"
		}
		return packageSource + importsStr + result
	}

	full := assemble()
	if EnableDecompileSyntaxValidation && len(full) < 50000 {
		if err := validateJavaSyntax(full); err != nil {
			// The assembled class is not valid Java. Degrade malformed members (using the real
			// class header so interface/enum/constructor context is honored) and re-render, so a
			// single broken method/field cannot make the whole class un-parseable.
			header := fmt.Sprintf("%s%s %s%s", accessFlags, classKeyword, className, superStr)
			methods = c.degradeInvalidMethods(header, methods)
			fields = c.degradeInvalidFields(header, className, isEnum, fields)
			full = assemble()
			if err := validateJavaSyntax(full); err != nil && !isDollarIdentifierValidatorGap(full, err) {
				log.Warnf("decompiled class %s still has syntax errors after degradation: %v", c.ClassName, err)
			}
		}
	}
	return full, nil
}

type dumpedFields struct {
	code      string
	fieldName string
	modifier  string
	typeName  string
}

func (c *ClassObjectDumper) DumpFields() ([]dumpedFields, error) {
	genuineEnum := c.isGenuineEnum()
	fields := make([]dumpedFields, 0, len(c.obj.Fields))
	for _, field := range c.obj.Fields {
		accessFlagsVerbose, accessCode := getFieldAccessFlagsVerbose(field.AccessFlags)
		//if len(accessFlagsVerbose) < 1 {
		//	return nil, utils.Error("fields accessFlagsVerbose is empty")
		//}
		_ = accessFlagsVerbose
		accessFlags := accessCode
		name, err := c.obj.getUtf8(field.NameIndex)
		if err != nil {
			return nil, err
		}
		renderName := class_context.SafeIdentifier(name)
		// $VALUES is the synthetic array backing values(); javac re-synthesizes it.
		if genuineEnum && name == "$VALUES" {
			continue
		}
		descriptor, err := c.obj.getUtf8(field.DescriptorIndex)
		if err != nil {
			return nil, err
		}
		fieldType, err := types.ParseDescriptor(descriptor)
		if err != nil {
			return nil, err
		}

		// Scan the field attributes once to find the Signature (generic) attribute before
		// rendering the type. When a parseable generic signature is present, it overrides the
		// descriptor-derived type to recover erased generics (e.g. List<String> instead of List).
		for _, attr := range field.Attributes {
			if sigAttr, ok := attr.(*SignatureAttribute); ok {
				if sigStr, err := c.obj.getUtf8(sigAttr.SignatureIndex); err == nil && sigStr != "" {
					if sigType := types.ParseSignature(sigStr); sigType != nil {
						fieldType = sigType
					}
				}
			}
		}

		// fieldType.String already registers the needed imports and shortens (or
		// FQN-disambiguates) every class component via ShortTypeName, so the rendered
		// string is the final field type. Re-running Import/ShortTypeName on that whole
		// string corrupts parameterized, array, or primitive-array types.
		lastPacket := fieldType.String(c.FuncCtx)
		valueLiteral := ""
		for _, attr := range field.Attributes {
			switch ret := attr.(type) {
			case *ConstantValueAttribute:
				value, err := c.obj.getConstantInfo(ret.ConstantValueIndex)
				if err != nil {
					log.Errorf("getConstantInfo(%d) failed", ret.ConstantValueIndex)
					continue
				}
				switch constVal := value.(type) {
				case *ConstantStringInfo:
					constStr, _ := c.obj.getUtf8(constVal.StringIndex)
					valueLiteral = values.JavaStringToLiteral(constStr)
				case *ConstantIntegerInfo:
					// boolean/char are stored as int constants in the pool; render them
					// in their declared type so the field initializer type-checks
					// (e.g. `boolean B = true` instead of the illegal `boolean B = 1`).
					switch fieldType.String(c.FuncCtx) {
					case types.NewJavaPrimer(types.JavaBoolean).String(c.FuncCtx):
						if constVal.Value == 0 {
							valueLiteral = "false"
						} else {
							valueLiteral = "true"
						}
					default:
						valueLiteral = strconv.Itoa(int(constVal.Value))
					}
				case *ConstantLongInfo:
					valueLiteral = strconv.Itoa(int(constVal.Value))
					if !strings.HasSuffix(valueLiteral, "L") {
						valueLiteral += "L"
					}
				case *ConstantFloatInfo:
					valueLiteral = javaFloatLiteral(constVal.Value)
				case *ConstantDoubleInfo:
					valueLiteral = javaDoubleLiteral(constVal.Value)
				default:
					log.Errorf("when handling for fields unknown constant type: %T", constVal)
				}
			case *SyntheticAttribute:
			// synthetic (compiler-generated) field marker; no diagnostic needed
			case *DeprecatedAttribute:
			// log.Infof("field %s is deprecated", name)
			case *SignatureAttribute:
			case *UnparsedAttribute:
				// Silently ignore unrecognized attributes (RuntimeInvisibleTypeAnnotations,
				// PermittedSubclasses, Record, NestMembers, etc.) rather than flooding logs.
			case *RuntimeVisibleAnnotationsAttribute:

			default:
				// Silently ignore unknown attribute types on fields.
			}
		}

		if valueLiteral != "" {
			fields = append(fields, dumpedFields{
				code:      fmt.Sprintf("%s %s %s = %s;", accessFlags, lastPacket, renderName, valueLiteral),
				fieldName: renderName,
				modifier:  accessFlags,
				typeName:  lastPacket,
			})
		} else if slices.Contains(accessFlagsVerbose, "final") && c.fieldDefaultValue[name] != "" {
			// A final field with a captured, hoistable initializer (constant-folded value
			// or a parameter-independent <init>/<clinit> assignment). Emit it inline.
			dumped := dumpedFields{
				code:      fmt.Sprintf("%s %s %s = %s;", accessFlags, lastPacket, renderName, c.fieldDefaultValue[name]),
				fieldName: renderName,
				modifier:  accessFlags,
				typeName:  lastPacket,
			}

			fields = append(fields, dumped)
		} else if c.isInterfaceLike() && slices.Contains(accessFlagsVerbose, "static") && slices.Contains(accessFlagsVerbose, "final") {
			fields = append(fields, dumpedFields{
				code:      fmt.Sprintf("%s %s %s = %s;", accessFlags, lastPacket, renderName, defaultInitializerForFieldType(lastPacket)),
				fieldName: renderName,
				modifier:  accessFlags,
				typeName:  lastPacket,
			})
		} else {
			// No initializer to emit (incl. blank finals assigned in the constructor /
			// static block). A bogus `= 0` here would be illegal for reference types and is
			// unnecessary: definite assignment in <init>/<clinit> keeps blank finals valid.
			fields = append(fields, dumpedFields{
				code:      fmt.Sprintf("%s %s %s;", accessFlags, lastPacket, renderName),
				fieldName: renderName,
				modifier:  accessFlags,
				typeName:  lastPacket,
			})
		}
	}
	return fields, nil
}

func defaultInitializerForFieldType(typeName string) string {
	switch strings.TrimSpace(typeName) {
	case "boolean":
		return "false"
	case "byte":
		return "(byte)0"
	case "char":
		return "(char)0"
	case "short":
		return "(short)0"
	case "int":
		return "0"
	case "long":
		return "0L"
	case "float":
		return "0.0F"
	case "double":
		return "0.0D"
	default:
		return "null"
	}
}

// genericSupertypeRawName returns the raw dotted class name of a (possibly generic) supertype type
// recovered from a class Signature, so it can be matched against the erased super_class / Interfaces
// names. Returns "" for type variables or anything that is not a class/parameterized type.
func genericSupertypeRawName(t types.JavaType) string {
	if t == nil {
		return ""
	}
	switch r := t.RawType().(type) {
	case *types.JavaParameterizedType:
		return r.RawClassName
	case *types.JavaClass:
		return r.Name
	}
	return ""
}

// javaCharLiteralFromCode renders a char annotation value (stored as an int code point) as a valid
// Java char literal: printable ASCII becomes 'x' (with the four chars that need escaping handled),
// everything else becomes a '\uXXXX' escape so the result always compiles.
func javaCharLiteralFromCode(code int) string {
	switch code {
	case '\'':
		return "'\\''"
	case '\\':
		return "'\\\\'"
	case '\n':
		return "'\\n'"
	case '\r':
		return "'\\r'"
	case '\t':
		return "'\\t'"
	}
	if code >= 0x20 && code <= 0x7e {
		return fmt.Sprintf("'%c'", rune(code))
	}
	return fmt.Sprintf("'\\u%04x'", code&0xffff)
}

// formatAnnotationElementValue renders a single annotation element_value (the right-hand side of an
// element-value pair, or an AnnotationDefault's default value) into its Java source form. Extracted
// from DumpAnnotation so the annotation-default renderer (`@interface` element `default <value>`)
// can reuse the exact same value formatting.
func (c *ClassObjectDumper) formatAnnotationElementValue(element *ElementValuePairAttribute) (string, error) {
	valStr := ""
	switch element.Tag {
	case 'B', 'C', 'D', 'F', 'I', 'J', 'S', 'Z':
		constant := element.Value.(ConstantInfo)
		switch ret := constant.(type) {
		case *ConstantStringInfo:
			s, err := c.obj.getUtf8(ret.StringIndex)
			if err != nil {
				return "", err
			}
			valStr = values.JavaStringToLiteral(s)
		case *ConstantLongInfo:
			valStr = fmt.Sprintf("%dL", ret.Value)
		case *ConstantIntegerInfo:
			if os.Getenv("JDEC_ANNO_LITERAL_OFF") == "" && element.Tag == 'Z' {
				if ret.Value == 0 {
					valStr = "false"
				} else {
					valStr = "true"
				}
			} else if os.Getenv("JDEC_ANNO_LITERAL_OFF") == "" && element.Tag == 'C' {
				valStr = javaCharLiteralFromCode(int(ret.Value))
			} else {
				valStr = fmt.Sprintf("%d", ret.Value)
			}
		case *ConstantDoubleInfo:
			valStr = fmt.Sprintf("%f", ret.Value)
		case *ConstantFloatInfo:
			valStr = fmt.Sprintf("%f", ret.Value)
		default:
			return "", errors.New("parse annotation error, unknown constant type")
		}
	case 's':
		valStr = values.JavaStringToLiteral(element.Value)
	case 'c':
		descStr, _ := element.Value.(string)
		classTyp, perr := types.ParseDescriptor(descStr)
		if perr != nil || classTyp == nil {
			fallback := strings.TrimSuffix(strings.TrimPrefix(descStr, "L"), ";")
			valStr = strings.Replace(fallback, "/", ".", -1) + ".class"
		} else {
			typeStr := classTyp.String(c.FuncCtx)
			if !classTyp.IsArray() {
				c.FuncCtx.Import(typeStr)
				typeStr = c.FuncCtx.ShortTypeName(typeStr)
			}
			valStr = typeStr + ".class"
		}
	case '@':
		annotation := element.Value.(*AnnotationAttribute)
		res, err := c.DumpAnnotation(annotation)
		if err != nil {
			return "", err
		}
		valStr = res
	case '[':
		l := element.Value.([]*ElementValuePairAttribute)
		eleList := []string{}
		for _, e := range l {
			res, err := c.formatAnnotationElementValue(e)
			if err != nil {
				return "", err
			}
			eleList = append(eleList, res)
		}
		valStr = fmt.Sprintf("{%s}", strings.Join(eleList, ", "))
	case 'e':
		switch ret := element.Value.(type) {
		case *EnumConstValue:
			if len(ret.TypeName) <= 2 {
				return "", fmt.Errorf("parse annotation error, invalid enum type name: %s", ret.TypeName)
			}
			fullqualifiedName := ret.TypeName[1 : len(ret.TypeName)-1]
			fullqualifiedName = strings.Replace(fullqualifiedName, "/", ".", -1)
			c.FuncCtx.Import(fullqualifiedName)
			last := strings.LastIndex(fullqualifiedName, ".")
			if last == -1 {
				return fullqualifiedName + "." + ret.ConstName, nil
			}
			return fullqualifiedName[last+1:] + "." + ret.ConstName, nil
		default:
			return "", fmt.Errorf("parse annotation error, unknown tag: %c, ret: %T", element.Tag, ret)
		}
	default:
		return "", fmt.Errorf("parse annotation error, unknown tag: %c", element.Tag)
	}
	return valStr, nil
}

// annotationElementDefaultClause returns the ` default <value>` suffix for an annotation element
// method (an abstract method of an @interface) when it carries an AnnotationDefault attribute, or
// "" otherwise. Without it javac rejects any use site that omits the element. Kill-switch:
// JDEC_ANNO_DEFAULT_OFF=1.
func (c *ClassObjectDumper) annotationElementDefaultClause(method *MemberInfo) string {
	if method == nil || os.Getenv("JDEC_ANNO_DEFAULT_OFF") != "" {
		return ""
	}
	for _, attr := range method.Attributes {
		ad, ok := attr.(*AnnotationDefaultAttribute)
		if !ok || ad.DefaultValue == nil {
			continue
		}
		val, err := c.formatAnnotationElementValue(ad.DefaultValue)
		if err != nil || val == "" {
			return ""
		}
		return " default " + val
	}
	return ""
}

func (c *ClassObjectDumper) DumpAnnotation(anno *AnnotationAttribute) (string, error) {
	result := ""

	annoName := anno.TypeName
	typ, err := types.ParseDescriptor(annoName)
	if err != nil {
		return "", fmt.Errorf("parse annotation error, %w", err)
	}
	classIns, ok := typ.RawType().(*types.JavaClass)
	if !ok {
		return "", errors.New("invalid annotation type")
	}
	annoName = c.FuncCtx.ShortTypeName(classIns.Name)
	var parseElement func(element *ElementValuePairAttribute) (string, error)
	parseElement = func(element *ElementValuePairAttribute) (string, error) {
		valStr := ""
		switch element.Tag {
		case 'B', 'C', 'D', 'F', 'I', 'J', 'S', 'Z':
			constant := element.Value.(ConstantInfo)
			switch ret := constant.(type) {
			case *ConstantStringInfo:
				s, err := c.obj.getUtf8(ret.StringIndex)
				if err != nil {
					return "", err
				}
				valStr = values.JavaStringToLiteral(s)
			case *ConstantLongInfo:
				valStr = fmt.Sprintf("%dL", ret.Value)
			case *ConstantIntegerInfo:
				// boolean/char/byte/short/int annotation members ALL store a CONSTANT_Integer in the
				// pool; the element TAG carries the real Java type. Without dispatching on the tag a
				// boolean member emits `1`/`0` (javac: "int cannot be converted to boolean") and a char
				// member emits its code point (`59` instead of `';'`), so the decompiled annotation does
				// not compile. Render by tag. Kill-switch: JDEC_ANNO_LITERAL_OFF=1 restores raw ints.
				if os.Getenv("JDEC_ANNO_LITERAL_OFF") == "" && element.Tag == 'Z' {
					if ret.Value == 0 {
						valStr = "false"
					} else {
						valStr = "true"
					}
				} else if os.Getenv("JDEC_ANNO_LITERAL_OFF") == "" && element.Tag == 'C' {
					valStr = javaCharLiteralFromCode(int(ret.Value))
				} else {
					valStr = fmt.Sprintf("%d", ret.Value)
				}
			case *ConstantDoubleInfo:
				valStr = fmt.Sprintf("%f", ret.Value)
			case *ConstantFloatInfo:
				valStr = fmt.Sprintf("%f", ret.Value)
			default:
				return "", errors.New("parse annotation error, unknown constant type")
			}
		case 's':
			valStr = values.JavaStringToLiteral(element.Value) // fmt.Sprintf("\"%s\"", element.Value.(string))
		case 'c':
			// class element value: the raw value is a field descriptor like
			// "Lcom/example/Foo;" or "[I"; render it as a Java class literal "Foo.class".
			descStr, _ := element.Value.(string)
			classTyp, perr := types.ParseDescriptor(descStr)
			if perr != nil || classTyp == nil {
				fallback := strings.TrimSuffix(strings.TrimPrefix(descStr, "L"), ";")
				valStr = strings.Replace(fallback, "/", ".", -1) + ".class"
			} else {
				typeStr := classTyp.String(c.FuncCtx)
				if !classTyp.IsArray() {
					c.FuncCtx.Import(typeStr)
					typeStr = c.FuncCtx.ShortTypeName(typeStr)
				}
				valStr = typeStr + ".class"
			}
		case '@':
			//ele.Value = ParseAnnotation(cp)
			annotation := element.Value.(*AnnotationAttribute)
			res, err := c.DumpAnnotation(annotation)
			if err != nil {
				return "", err
			}
			valStr = res
		case '[':
			//length := reader.readUint16()
			//l := []any{}
			//for k := 0; k < int(length); k++ {
			//	val := ParseAnnotationElementValue(cp)
			//	l = append(l, val)
			//}
			//ele.Value = l
			l := element.Value.([]*ElementValuePairAttribute)
			eleList := []string{}
			for _, e := range l {
				res, err := parseElement(e)
				if err != nil {
					return "", err
				}
				eleList = append(eleList, res)
			}
			valStr = fmt.Sprintf("{%s}", strings.Join(eleList, ", "))
		case 'e':
			// fullname
			switch ret := element.Value.(type) {
			case *EnumConstValue:
				if len(ret.TypeName) <= 2 {
					return "", fmt.Errorf("parse annotation error, invalid enum type name: %s", ret.TypeName)
				}
				fullqualifiedName := ret.TypeName[1 : len(ret.TypeName)-1]
				fullqualifiedName = strings.Replace(fullqualifiedName, "/", ".", -1)
				c.FuncCtx.Import(fullqualifiedName)
				last := strings.LastIndex(fullqualifiedName, ".")
				if last == -1 {
					return fullqualifiedName + "." + ret.ConstName, nil
				}
				return fullqualifiedName[last+1:] + "." + ret.ConstName, nil
			default:
				return "", fmt.Errorf("parse annotation error, unknown tag: %c, ret: %T", element.Tag, ret)
			}
		default:
			return "", fmt.Errorf("parse annotation error, unknown tag: %c", element.Tag)
		}
		return valStr, nil
	}
	elementStrList := []string{}
	for _, element := range anno.ElementValuePairs {
		str, err := parseElement(element)
		if err != nil {
			return "", err
		}
		elementStrList = append(elementStrList, fmt.Sprintf("%s=%s", element.Name, str))
	}
	result = fmt.Sprintf("@%s(%s)", annoName, strings.Join(elementStrList, ", "))
	return result, nil
}

// normalizeCatchClauseType keeps a catch clause's declared type a legal reference type. A catch
// type must be a subtype of Throwable; when upstream type inference degrades the exception
// variable to a primitive (e.g. "boolean" from a reused slot) or an array, fall back to Throwable
// so the emitted Java stays syntactically valid.
func normalizeCatchClauseType(excType string) string {
	if strings.HasSuffix(excType, "[]") {
		return "Throwable"
	}
	switch excType {
	case "boolean", "byte", "char", "short", "int", "long", "float", "double", "void":
		return "Throwable"
	}
	return excType
}

// mergeNestedSameTypeCatches collapses the decompiler-synthesized "two catch clauses of the same
// type" shape. Java forbids a try from declaring two handlers of the same exception type, but it is
// exactly what javac's try-with-resources / try-catch-finally desugaring produces: an inner handler
// (e.g. the try-with-resources `catch (Throwable t) { primaryExc = t; throw t; }` that records the
// primary exception) whose protected region is itself covered by an outer Throwable cleanup ("any")
// handler. At runtime the inner handler runs first and unconditionally rethrows its caught
// exception, which the outer handler then catches; so the two handlers are sequential, not
// alternative. Reconstruct that ordering by concatenating the first handler's body (minus its
// trailing `throw e`) with the second handler's body, under the first handler's catch variable.
//
// The merge only fires on ADJACENT handlers of the same (normalized) type whose first member ends
// by unconditionally rethrowing its own caught variable. That signature is unique to the synthesized
// illegal shape, so the pass never reorders or merges distinct user-written handlers (which can
// never share a type anyway). Chains of three or more (nested try-with-resources) collapse by
// repeatedly merging the leading pair.
func mergeNestedSameTypeCatches(funcCtx *class_context.ClassContext, exceptions []*values.JavaRef, bodies [][]statements.Statement) ([]*values.JavaRef, [][]statements.Statement) {
	if len(bodies) < 2 || len(exceptions) != len(bodies) {
		return exceptions, bodies
	}
	catchTypeKey := func(ref *values.JavaRef) string {
		if ref == nil {
			return ""
		}
		t := ref.Type()
		if t == nil {
			return "Throwable"
		}
		return normalizeCatchClauseType(t.String(funcCtx))
	}
	// lastMeaningfulStmt returns the rendered text and index of the body's last statement that the
	// dumper would actually emit (MiddleStatement / StackAssignStatement are dropped at render time).
	lastMeaningfulStmt := func(body []statements.Statement) (string, int) {
		for i := len(body) - 1; i >= 0; i-- {
			switch body[i].(type) {
			case *statements.MiddleStatement, *statements.StackAssignStatement:
				continue
			}
			return strings.TrimSpace(body[i].String(funcCtx)), i
		}
		return "", -1
	}
	exc := append([]*values.JavaRef{}, exceptions...)
	bod := append([][]statements.Statement{}, bodies...)
	i := 0
	for i+1 < len(bod) {
		sameType := catchTypeKey(exc[i]) != "" && catchTypeKey(exc[i]) == catchTypeKey(exc[i+1])
		rethrows := false
		var throwIdx int
		if sameType && exc[i] != nil {
			varName := strings.TrimSpace(exc[i].String(funcCtx))
			lastStr, lastIdx := lastMeaningfulStmt(bod[i])
			if lastIdx >= 0 && varName != "" && lastStr == "throw "+varName {
				rethrows = true
				throwIdx = lastIdx
			}
		}
		if sameType && rethrows {
			merged := append([]statements.Statement{}, bod[i][:throwIdx]...)
			merged = append(merged, bod[i+1]...)
			bod[i] = merged
			exc = append(exc[:i+1], exc[i+2:]...)
			bod = append(bod[:i+1], bod[i+2:]...)
			// Do not advance: the merged handler may chain into a further same-type handler.
			continue
		}
		i++
	}
	return exc, bod
}

// isUnconditionalTerminalStatement reports whether st unconditionally transfers control out of
// the current block: return / throw / break / continue (with or without a label). In valid Java
// any sibling statement that follows such a statement at the same nesting level is unreachable and
// is rejected by javac as a compile error. The decompiler occasionally synthesizes a structural
// jump (e.g. a `break;` to leave a loop) right after a real `return`/`throw`; emitting it would
// make the output uncompilable, so callers stop rendering a statement list once this returns true.
func isUnconditionalTerminalStatement(st statements.Statement, funcCtx *class_context.ClassContext) bool {
	switch s := st.(type) {
	case *statements.ReturnStatement:
		return true
	case *statements.CustomStatement:
		t := strings.TrimSpace(s.String(funcCtx))
		switch {
		case t == "break", t == "continue", t == "return":
			return true
		case strings.HasPrefix(t, "break "), strings.HasPrefix(t, "continue "), strings.HasPrefix(t, "throw "):
			return true
		}
	case *statements.DoWhileStatement:
		// An infinite loop (condition is the constant true) that never breaks back to its own
		// successor transfers control away forever, so any sibling after it is unreachable.
		// This is common after CFG structuring: a nested loop's exit is wired straight to the
		// outer loop's `continue LABEL`, leaving the inner do-while(true) with no break and a
		// dangling `continue;` behind it that javac rejects as an unreachable statement.
		if loopConditionIsConstTrue(s.ConditionValue, funcCtx) &&
			!loopBodyHasEscapingBreak(s.Body, s.Label, true, funcCtx) {
			return true
		}
	case *statements.WhileStatement:
		if loopConditionIsConstTrue(s.ConditionValue, funcCtx) &&
			!loopBodyHasEscapingBreak(s.Body, "", true, funcCtx) {
			return true
		}
	}
	return false
}

func needsTrailingIncompleteControlFlowThrow(statementList []statements.Statement, returnType types.JavaType, funcCtx *class_context.ClassContext) bool {
	if returnType == nil || returnType.String(funcCtx) == "void" {
		return false
	}
	for i := len(statementList) - 1; i >= 0; i-- {
		st := statementList[i]
		switch st.(type) {
		case *statements.MiddleStatement, *statements.StackAssignStatement:
			continue
		}
		switch s := st.(type) {
		case *statements.DoWhileStatement:
			return loopConditionIsConstTrue(s.ConditionValue, funcCtx) &&
				loopBodyHasEscapingBreak(s.Body, s.Label, true, funcCtx)
		case *statements.WhileStatement:
			return loopConditionIsConstTrue(s.ConditionValue, funcCtx) &&
				loopBodyHasEscapingBreak(s.Body, "", true, funcCtx)
		default:
			return false
		}
	}
	return false
}

// loopConditionIsConstTrue reports whether a loop condition is the literal true (an infinite loop).
func loopConditionIsConstTrue(cond values.JavaValue, funcCtx *class_context.ClassContext) bool {
	return cond != nil && strings.TrimSpace(cond.String(funcCtx)) == "true"
}

// loopBodyHasEscapingBreak reports whether body (the body of a loop whose label is loopLabel)
// contains a break that hands control to the statement following THAT loop: an unlabeled `break`
// that is not nested inside a deeper loop or switch, or a `break <loopLabel>` at any depth. continue
// statements and breaks targeting other constructs do not return control to this loop's successor,
// so they are not counted. directlyInLoop becomes false once the walk descends into a nested loop or
// switch, where an unlabeled break belongs to that inner construct instead of to our loop. The
// walker covers every statement kind that can hold a nested break; leaf statements without nested
// bodies cannot contain one.
func loopBodyHasEscapingBreak(body []statements.Statement, loopLabel string, directlyInLoop bool, funcCtx *class_context.ClassContext) bool {
	for _, st := range body {
		switch s := st.(type) {
		case *statements.CustomStatement:
			t := strings.TrimSpace(s.String(funcCtx))
			if directlyInLoop && t == "break" {
				return true
			}
			if loopLabel != "" && t == "break "+loopLabel {
				return true
			}
		case *statements.IfStatement:
			if loopBodyHasEscapingBreak(s.IfBody, loopLabel, directlyInLoop, funcCtx) ||
				loopBodyHasEscapingBreak(s.ElseBody, loopLabel, directlyInLoop, funcCtx) {
				return true
			}
		case *statements.TryCatchStatement:
			if loopBodyHasEscapingBreak(s.TryBody, loopLabel, directlyInLoop, funcCtx) {
				return true
			}
			for _, cb := range s.CatchBodies {
				if loopBodyHasEscapingBreak(cb, loopLabel, directlyInLoop, funcCtx) {
					return true
				}
			}
		case *statements.SynchronizedStatement:
			if loopBodyHasEscapingBreak(s.Body, loopLabel, directlyInLoop, funcCtx) {
				return true
			}
		case *statements.DoWhileStatement:
			// Nested loop: an unlabeled break is its own; only `break <loopLabel>` escapes to us.
			if loopBodyHasEscapingBreak(s.Body, loopLabel, false, funcCtx) {
				return true
			}
		case *statements.WhileStatement:
			if loopBodyHasEscapingBreak(s.Body, loopLabel, false, funcCtx) {
				return true
			}
		case *statements.SwitchStatement:
			for _, c := range s.Cases {
				if loopBodyHasEscapingBreak(c.Body, loopLabel, false, funcCtx) {
					return true
				}
			}
		}
	}
	return false
}

func (c *ClassObjectDumper) DumpMethod(methodName, desc string) (*dumpedMethods, error) {
	return c.DumpMethodWithInitialId(methodName, desc, utils2.NewRootVariableId())
}

func (c *ClassObjectDumper) DumpMethodWithInitialId(methodName, desc string, id *utils2.VariableId) (*dumpedMethods, error) {
	traitId := fmt.Sprintf("name:%s,desc:%s", methodName, desc)
	if v, ok := c.dumpedMethodsSet[traitId]; ok {
		return v, nil
	}
	var method *MemberInfo
	var name, descriptor string
	var err error
	var dumped = &dumpedMethods{}

	debugMode := false
	defer func() {
		if debugMode && method != nil {
			log.Info("DumpMethodWithInitialId done")
			log.Info("\n" + dumped.code)
		}
	}()

	c.dumpedMethodsSet[traitId] = dumped
	for _, info := range c.obj.Methods {
		name, err = c.obj.getUtf8(info.NameIndex)
		if err != nil {
			return dumped, utils.Wrapf(err, "getUtf8(%v) failed", info.NameIndex)
		}
		descriptor, err = c.obj.getUtf8(info.DescriptorIndex)
		if err != nil {
			return dumped, utils.Wrapf(err, "getUtf8(%v) failed", info.DescriptorIndex)
		}
		if name == methodName && descriptor == desc {
			method = info
			break
		}
	}
	if method == nil {
		return dumped, fmt.Errorf("method %s not found", methodName)
	}

	var isLambda bool
	if v := c.lambdaMethods[name]; slices.Contains(v, descriptor) {
		isLambda = true
	}

	c.FuncCtx.IsStatic = method.AccessFlags&StaticFlag == StaticFlag
	accessFlagsVerbose, accessFlagCode := getMethodAccessFlagsVerbose(method.AccessFlags)

	var isVarArgs bool
	var abstractMethod bool
	accessFlagsVerbose = lo.Filter(accessFlagsVerbose, func(item string, index int) bool {
		if item == "varargs" {
			isVarArgs = true
			return false
		}
		if item == "abstract" {
			abstractMethod = true
		}
		return true
	})
	_ = abstractMethod

	accessFlags := accessFlagCode
	methodType, err := types.ParseMethodDescriptor(descriptor)
	if err != nil {
		return dumped, utils.Wrapf(err, "ParseMethodDescriptor(%v) failed", descriptor)
	}
	descriptorParamTypes := slices.Clone(methodType.FunctionType().ParamTypes)
	descriptorParamCount := len(descriptorParamTypes)
	// Override the descriptor-derived method type with generic information from the
	// Signature attribute, if present and parseable. This recovers erased generics on
	// method parameters and return types (e.g. BiFunction<Integer,Integer,Integer> vs raw
	// BiFunction). Falls back silently to the descriptor if the signature cannot be parsed.
	//
	// methodTypeParams is the method's own formal type-parameter section ("<T>", "<K, V>"), rendered
	// from a signature that BEGINS with "<...>" (a generic method like `<T> T checkNotNull(T)`).
	// ParseMethodSignatureFull (unlike the old ParseMethodSignature) does not bail on that leading
	// section, so such a method's params/return are now recovered as the type variables (T) instead of
	// staying erased to Object - the dominant guava `base` recompile blocker after the synchronized
	// scope fix (Preconditions.checkNotNull rendered `Object checkNotNull(Object)`, so every
	// `predicate.apply(checkNotNull(x))` failed "Object cannot be converted to CAP#1"). The header
	// then emits the `<T>` declaration before the return type. Kill-switch: JDEC_METHOD_TYPEPARAMS_OFF.
	methodTypeParams := ""
	var methodTypeParamNames []string
	for _, attr := range method.Attributes {
		if sigAttr, ok := attr.(*SignatureAttribute); ok {
			if sigStr, err := c.obj.getUtf8(sigAttr.SignatureIndex); err == nil && sigStr != "" {
				tps, sigParams, sigRet := types.ParseMethodSignatureFull(sigStr, c.FuncCtx)
				// Gate on sigRet (not sigParams): a zero-arg generic method like
				// `()TK;` (Map.Entry.getKey) parses to (nil params, K return). The old
				// `sigParams != nil` guard skipped exactly these, leaving the return type
				// erased to Object so the override of an interface method failed to compile
				// ("return type Object is not compatible with V"). sigRet==nil still means a
				// genuine parse failure, so we fall back to the descriptor as before. Kill-switch
				// JDEC_METHOD_SIG_RET_OFF restores the legacy sigParams!=nil gate.
				applicable := sigRet != nil
				if os.Getenv("JDEC_METHOD_SIG_RET_OFF") != "" {
					applicable = sigParams != nil
				}
				if applicable {
					mt := methodType.FunctionType()
					// Only override when the param count matches (signature may include
					// formal type parameters that shift the count; skip those for safety).
					if len(sigParams) == len(mt.ParamTypes) {
						if sigParams != nil {
							mt.ParamTypes = sigParams
						}
						mt.ReturnType = sigRet
					}
				}
				if os.Getenv("JDEC_METHOD_TYPEPARAMS_OFF") == "" {
					methodTypeParams = tps
					methodTypeParamNames = types.MethodFormalTypeParamNames(sigStr)
				}
			}
			break
		}
	}
	// A synthetic access-bridge constructor carries NO Signature attribute, so its parameters stay
	// erased to their descriptor types (Object / raw). Its only body is `this(args...)` forwarding to
	// the private target ctor, so when that target declares a type variable (e.g. guava
	// Equivalence.Wrapper(Equivalence, Object, Equivalence$1) -> this(var1, var2) into the private
	// (Equivalence<? super T>, T) ctor) the bare `Object` arg fails "Object cannot be converted to T".
	// Lift the target ctor's GENERIC param types onto the bridge: the bridge erases to the same
	// descriptor (byte-faithful) and its (raw) call sites still type-check. This recurs across every
	// nested class with a private generic ctor reached from its encloser, so the fix is broad.
	// Kill-switch: JDEC_NO_SYN_BRIDGE_CTOR_RETYPE=1.
	if name == "<init>" && os.Getenv("JDEC_NO_SYN_BRIDGE_CTOR_RETYPE") == "" &&
		c.isSyntheticAccessBridgeCtor(descriptor, method.AccessFlags) {
		c.reTypeSyntheticBridgeCtorParams(descriptor, methodType.FunctionType())
	}
	// Bring the method's own type-variable names into scope for the duration of this method's render
	// so renderers (e.g. typeVarReturnCast) recognize them like class-level ones, then restore the
	// class-scope set afterward so they never leak into sibling members.
	if len(methodTypeParamNames) > 0 && c.FuncCtx != nil {
		savedTypeParams := c.FuncCtx.TypeParams
		c.FuncCtx.TypeParams = append(slices.Clone(savedTypeParams), methodTypeParamNames...)
		defer func() { c.FuncCtx.TypeParams = savedTypeParams }()
	}
	c.MethodType = methodType.FunctionType()
	returnTypeStr := methodType.FunctionType().ReturnType.String(c.FuncCtx)
	code := ""
	c.Tab()
	c.CurrentMethod = method
	funcCtx := c.FuncCtx
	funcCtx.FunctionName = name
	//if name != "scope" {
	//	return &dumpedMethods{}, nil
	//}
	//println(name)
	finalFieldMap := map[string]struct{}{}
	finalFieldRenderNameToRaw := map[string]string{}
	classStaticInitializersMustHoist := slices.Contains(c.obj.AccessFlagsVerbose, "interface") || slices.Contains(c.obj.AccessFlagsVerbose, "annotation")
	for _, field := range c.obj.Fields {
		var finalFalg uint16 = 0x0010
		if field.AccessFlags&finalFalg == finalFalg {
			rawName := c.obj.ConstantPoolManager.GetUtf8(int(field.NameIndex)).Value
			finalFieldMap[rawName] = struct{}{}
			finalFieldRenderNameToRaw[class_context.SafeIdentifier(rawName)] = rawName
		}
	}
	annoStrs := []string{}
	funcCtx.FunctionType = c.MethodType
	var paramsNewStr string
	var exceptions string
	for _, attribute := range method.Attributes {
		if exceptionAttr, ok := attribute.(*ExceptionsAttribute); ok {
			exceptions = " throws "
			expList := []string{}
			for _, u := range exceptionAttr.ExceptionIndexTable {
				info, err := c.obj.getConstantInfo(u)
				if err != nil {
					continue
				}
				classInfo := info.(*ConstantClassInfo)
				name, err := c.obj.getUtf8(classInfo.NameIndex)
				if err != nil {
					continue
				}
				name = strings.Replace(name, "/", ".", -1)
				funcCtx.Import(name)
				name = funcCtx.ShortTypeName(name)
				if name != "" {
					expList = append(expList, name)
				}
			}
			exceptions += strings.Join(expList, ", ")
		}
		if anno, ok := attribute.(*RuntimeVisibleAnnotationsAttribute); ok {
			for _, annotation := range anno.Annotations {
				res, err := c.DumpAnnotation(annotation)
				if err != nil {
					return dumped, err
				}
				annoStrs = append(annoStrs, res)
			}
		}
		if codeAttr, ok := attribute.(*CodeAttribute); ok {
			params, statementList, err := ParseBytesCode(c, codeAttr, id)
			if err != nil {
				return dumped, utils.Wrap(err, "ParseBytesCode failed")
			}
			thisRemoved := false
			if len(params) > 0 {
				if v, ok := params[0].(*values.JavaRef); ok && v.IsThis {
					params = params[1:]
					thisRemoved = true
				}
			}
			// For a synthetic lambda body, the leading parameters are captured variables that
			// javac prepended to the impl signature; they are not lambda parameters. Drop them from
			// the arrow list and rename each to a capture placeholder so every body reference resolves
			// to the captured value at the invokedynamic call site (see bootstrap_methods.go). For an
			// instance lambda the receiver was captured as the first dynamic arg but is represented by
			// the impl method's `this` (already stripped above), so its placeholder index is offset.
			samParams := params
			if isLambda {
				if n := c.lambdaCaptureCount[name+descriptor]; n > 0 {
					capArgOffset := 0
					if thisRemoved {
						capArgOffset = 1
					}
					drop := n - capArgOffset
					if drop > 0 && drop <= len(params) {
						for i := 0; i < drop; i++ {
							if ref, ok := params[i].(*values.JavaRef); ok && ref.Id != nil {
								ref.Id.SetName(fmt.Sprintf("\x00LCAP%d\x00", i+capArgOffset))
							}
						}
						samParams = params[drop:]
					}
				}
			}
			// A genuine enum constructor carries two synthetic leading parameters (String
			// name, int ordinal) that javac injects and forbids in source. Drop them from the
			// rendered signature; the synthetic super(name, ordinal) call is stripped from the
			// body below.
			isEnumCtor := name == "<init>" && c.isGenuineEnum()
			if isEnumCtor && len(samParams) >= 2 {
				samParams = samParams[2:]
			}
			// A lambda body is emitted as an arrow expression `(Type p0, Type p1) -> ...`
			// inline in the enclosing method, so Java requires its parameter names to be unique
			// across the entire method scope (no shadowing). The fresh root namespace gives
			// them var0, var1, ... which can still collide with the enclosing method's own
			// params/locals (var1, var2, ...). Rename each SAM param to an `l<N>` name that the
			// slot-based scheme never generates, eliminating the collision while keeping the body
			// consistent (every body reference shares the same JavaRef/Id).
			if isLambda {
				for i, val := range samParams {
					if ref, ok := val.(*values.JavaRef); ok && ref.Id != nil && !ref.IsThis {
						ref.Id.SetName(fmt.Sprintf("l%d", i))
					}
				}
			}
			ensureUniqueParameterNames(samParams, funcCtx)
			paramsNewStrList := []string{}
			if !isLambda && name != "<init>" && name != "<clinit>" && len(samParams) != descriptorParamCount {
				paramSlotOffset := 0
				if !funcCtx.IsStatic {
					paramSlotOffset = 1
				}
				for i, pt := range descriptorParamTypes {
					paramName := fmt.Sprintf("var%d", i+paramSlotOffset)
					if i == len(descriptorParamTypes)-1 && isVarArgs && pt.IsArray() {
						paramsNewStrList = append(paramsNewStrList, fmt.Sprintf("%s... %s", pt.ElementType().String(c.FuncCtx), paramName))
					} else {
						paramsNewStrList = append(paramsNewStrList, fmt.Sprintf("%s %s", pt.String(c.FuncCtx), paramName))
					}
				}
			} else {
				// samParams and descriptorParamTypes share the SAME trailing source parameters; any
				// synthetic prefix (enum ctor's String,int) lives only at the FRONT of the descriptor
				// list and was sliced off samParams above, so align them from the tail.
				descTailOffset := descriptorParamCount - len(samParams)
				for i, val := range samParams {
					typ := val.Type()
					if i == len(samParams)-1 && isVarArgs && typ != nil && typ.IsArray() {
						paramsNewStrList = append(paramsNewStrList, fmt.Sprintf("%s... %s", typ.ElementType().String(c.FuncCtx), val.String(c.FuncCtx)))
					} else {
						typName := "java.lang.Object"
						if typ != nil {
							typName = typ.String(c.FuncCtx)
						}
						// A narrow int-category parameter (char/byte/short) whose slot was widened to
						// int by an in-body int reassignment must still be DECLARED with its authoritative
						// descriptor type, or assigning it to a same-typed field/return is a "possible
						// lossy conversion from int to char" javac error (e.g. guava ArrayBasedCharEscaper's
						// `char safeMin` ctor). The descriptor is the ground truth for primitive params.
						if !isLambda && descTailOffset >= 0 {
							if dt := paramDescriptorNarrowType(descriptorParamTypes, descTailOffset+i, typ); dt != nil {
								typName = dt.String(c.FuncCtx)
							}
						}
						paramsNewStrList = append(paramsNewStrList, fmt.Sprintf("%s %s", typName, val.String(c.FuncCtx)))
					}
				}
			}
			c.MethodType = methodType.FunctionType()
			paramsNewStr = strings.Join(paramsNewStrList, ", ")

			// Rename locals whose slot-derived names collide across nested scopes (e.g. two
			// nested catch parameters both named var4) so the emitted Java is re-compilable.
			resolveLocalNameCollisions(params, statementList)

			// Per-constructor/<clinit> count of field assignments. A final field is only safe to
			// lift into a field initializer when assigned exactly once in this body; a blank final
			// assigned across multiple branches must keep its in-body assignments (see
			// countConstructorFieldAssignments).
			ctorFieldAssignCount := countConstructorFieldAssignments(statementList, funcCtx.ClassName)

			// Cross-constructor/<clinit> totals: a final field assigned exactly once HERE may still
			// be assigned in another overloaded constructor. Hoisting it then double-assigns a final
			// field, so the hoist guards below additionally require the class-wide store count <= 1.
			fieldStoreTotal := c.constructorFieldStoreTotals()

			sourceCode := "\n"
			hoistableStaticInitLocals := map[string]string{}
			// Contiguous-prefix hoist barrier for <clinit>. The dumper emits every lifted field
			// initializer as a field declaration ABOVE the static{} block, so a <clinit> assignment
			// may only be lifted while every preceding top-level <clinit> statement was also lifted.
			// Once a side-effecting / non-hoistable statement stays in the static block (a loop, a
			// `someField.set(...)` call, a branch), any later field initializer that reads the state
			// those statements produced (e.g. commons-codec URLCodec `WWW_FORM_URL = (BitSet)
			// WWW_FORM_URL_SAFE.clone()` emitted after all the `WWW_FORM_URL_SAFE.set(...)` calls) must
			// stay too: lifting it would reorder the read ahead of the writes AND forward-reference a
			// field declared later. Kept as a blank-final store in the static block instead.
			// staticHoistAllowedHere defaults true so non-<clinit> bodies and the pre-barrier prefix
			// are unaffected. Interfaces/annotations (classStaticInitializersMustHoist) are EXCLUDED:
			// they cannot declare static{} blocks, so every constant initializer MUST be lifted to its
			// declaration and there is no place to leave a deferred store — the barrier only applies to
			// ordinary classes, which can hold blank-final stores in a static block.
			// Kill-switch: JDEC_NO_CLINIT_HOIST_BARRIER=1 restores the old behavior.
			clinitHoistBarrierOn := funcCtx.FunctionName == "<clinit>" && !classStaticInitializersMustHoist && os.Getenv("JDEC_NO_CLINIT_HOIST_BARRIER") == ""
			staticHoistBarrierHit := false
			staticHoistAllowedHere := true
			hoistEventCount := 0
			statementSet := utils.NewSet[statements.Statement]()
			var statementToString func(statement statements.Statement) string
			var statementListToString func(statements []statements.Statement) string
			statementListToString = func(statementList []statements.Statement) string {
				c.Tab()
				defer c.UnTab()
				var res []string
				for i, statement := range statementList {
					if _, ok := statement.(*statements.MiddleStatement); ok {
						continue
					}
					_, ok := statement.(*statements.StackAssignStatement)
					if ok {
						continue
					}
					// A static initializer block (<clinit> -> `static {}`) cannot contain a `return`
					// statement (javac: "return outside of method"). Source cannot express an early
					// return in a <clinit>, so javac never emits one; a void `return;` sitting at the
					// tail of ANY block (top-level or inside the normal-completion try body) is just
					// the terminal flow-exit opcode. Dropping it preserves semantics and yields legal
					// Java (e.g. commons-codec DaitchMokotoffSoundex's twr <clinit> rendered a bare
					// `return;` inside `try{...}` which javac rejected). Restricting to the block tail
					// avoids enabling any dead trailing siblings. Kill-switch:
					// JDEC_NO_CLINIT_RETURN_DROP=1. (Bug AC)
					if funcCtx.FunctionName == "<clinit>" && os.Getenv("JDEC_NO_CLINIT_RETURN_DROP") == "" {
						if rs, ok := statement.(*statements.ReturnStatement); ok && rs.JavaValue == nil && i == len(statementList)-1 {
							break
						}
					}
					res = append(res, statementToString(statement))
					// Drop unreachable trailing siblings: once an unconditional terminal
					// (return/throw/break/continue) is emitted, anything after it in the same
					// block is dead code that javac would reject (e.g. a synthetic `break;`
					// appended after a `return;` by the loop rewriter).
					if isUnconditionalTerminalStatement(statement, funcCtx) {
						break
					}
				}
				return strings.Join(res, "\n")
			}
			statementToString = func(statement statements.Statement) (statementStr string) {
				defer func() {
					if debugMode {
						log.Info("\n" + statementStr)
					}
				}()
				//if statementSet.Has(statement) {
				//	panic("statement already exists")
				//}
				statementSet.Add(statement)
				switch ret := statement.(type) {
				case *statements.AssignStatement:
					foundFieldInit := false
					if ret.LeftValue != nil && ret.JavaValue != nil && funcCtx.FunctionName == "<clinit>" && classStaticInitializersMustHoist {
						if ref, ok := ret.LeftValue.(*values.JavaRef); ok && !ref.IsThis {
							if rhs := ret.JavaValue.String(funcCtx); staticHoistAllowedHere && canHoistFieldValueInitializer(ret.JavaValue, rhs) {
								hoistableStaticInitLocals[strings.TrimSpace(ref.String(funcCtx))] = rhs
								foundFieldInit = true
								hoistEventCount++
							}
						}
					}
					if v, ok := ret.LeftValue.(*values.RefMember); ok && ret.JavaValue != nil {
						obj := core.UnpackSoltValue(v.Object)
						if v1, ok := obj.(*values.JavaRef); ok && v1.IsThis && (funcCtx.FunctionName == "<init>" || funcCtx.FunctionName == funcCtx.ClassName) {
							if _, ok := finalFieldMap[v.Member]; ok {
								if rhs := ret.JavaValue.String(funcCtx); canHoistFieldValueInitializer(ret.JavaValue, rhs) &&
									(!EnableFieldInitHoistGuard || (ctorFieldAssignCount[v.Member] == 1 && crossCtorStoreOK(fieldStoreTotal, v.Member) && !rhsReadsInstanceField(rhs))) {
									foundFieldInit = true
									c.fieldDefaultValue[v.Member] = rhs
								}
							}
						}
					} else if v, ok := ret.LeftValue.(*values.JavaClassMember); ok && ret.JavaValue != nil {
						if (funcCtx.FunctionName == "<clinit>" && classStaticInitializersMustHoist) || v.Name == funcCtx.ClassName {
							if _, ok := finalFieldMap[v.Member]; ok {
								if rhs := ret.JavaValue.String(funcCtx); staticHoistAllowedHere && canHoistFieldValueInitializer(ret.JavaValue, rhs) &&
									(!EnableFieldInitHoistGuard || (ctorFieldAssignCount[v.Member] <= 1 && crossCtorStoreOK(fieldStoreTotal, v.Member))) {
									foundFieldInit = true
									c.fieldDefaultValue[v.Member] = rhs
									hoistEventCount++
								}
							}
						}
					}
					if !foundFieldInit && ret.LeftValue != nil && ret.JavaValue != nil && funcCtx.FunctionName == "<clinit>" && classStaticInitializersMustHoist {
						lhs := strings.TrimSpace(ret.LeftValue.String(funcCtx))
						if strings.HasPrefix(lhs, c.GetConstructorMethodName()+".") {
							lhs = strings.TrimPrefix(lhs, c.GetConstructorMethodName()+".")
						}
						if rawName, ok := finalFieldRenderNameToRaw[lhs]; ok {
							rhs := ret.JavaValue.String(funcCtx)
							if ref, ok := values.UnpackSoltValue(ret.JavaValue).(*values.JavaRef); ok {
								if localInit, ok := hoistableStaticInitLocals[strings.TrimSpace(ref.String(funcCtx))]; ok {
									rhs = localInit
								}
							}
							if staticHoistAllowedHere && canHoistFieldValueInitializer(ret.JavaValue, rhs) &&
								(!EnableFieldInitHoistGuard || (ctorFieldAssignCount[rawName] <= 1 && crossCtorStoreOK(fieldStoreTotal, rawName))) {
								foundFieldInit = true
								c.fieldDefaultValue[rawName] = rhs
								hoistEventCount++
							}
						}
					}
					if !foundFieldInit {
						statementStr = c.GetTabString() + statement.String(funcCtx) + ";"
					}
				case *statements.SynchronizedStatement:
					// A field lock desugars to `getfield; dup; astore tmp; monitorenter`; the
					// synthetic temp backs the implicit finally's monitorexit. After the
					// synchronized rewriter removes that monitorexit the temp is dead, but it
					// survives in the monitor position as an inline `tmp = lock` assignment,
					// which references an undeclared variable. Strip it back to the lock
					// expression (safe: the temp has no other use).
					arg := monitorTempAssignRe.ReplaceAllString(ret.Argument.String(funcCtx), "$1")
					statementStr = fmt.Sprintf(c.GetTabString()+"synchronized(%s){\n"+
						"%s\n"+
						c.GetTabString()+"}", arg, statementListToString(ret.Body))
				case *statements.TryCatchStatement:
					statementStr = fmt.Sprintf(c.GetTabString()+"try{\n"+
						"%s\n"+
						c.GetTabString()+"}", statementListToString(ret.TryBody))
					// Two catch handlers of the SAME type are illegal Java (a try may not declare two
					// handlers of the same exception type), but they are exactly what bytecode emits for
					// try-with-resources / try-catch-finally: a Throwable primaryExc-capture handler whose
					// region is nested inside a Throwable cleanup ("any") handler. Collapse such adjacent
					// pairs back into one handler so the source recompiles. Kill-switch:
					// JDEC_NO_CATCH_MERGE=1 restores the raw duplicate-catch output.
					catchExc := ret.Exception
					catchBodies := ret.CatchBodies
					if os.Getenv("JDEC_NO_CATCH_MERGE") == "" {
						catchExc, catchBodies = mergeNestedSameTypeCatches(funcCtx, catchExc, catchBodies)
					}
					for i, body := range catchBodies {
						excType := normalizeCatchClauseType(catchExc[i].Type().String(funcCtx))
						statementStr += fmt.Sprintf("catch(%s %s){\n"+
							"%s\n"+
							c.GetTabString()+"}", excType, catchExc[i].String(funcCtx), statementListToString(body))
					}
					haveCatch := len(catchBodies) > 0
					if !haveCatch {
						body := statementListToString(ret.TryBody)
						if canFlattenNoCatchTry(body) {
							// A try without catch/finally has no Java-level effect. Some legacy bytecode
							// patterns (for example an EOFException edge inside a loop with an enclosing
							// IOException handler) can lose the inner handler during CFG structuring while
							// the body itself is still sound. Preserve the executable statements instead of
							// stubbing the method.
							statementStr = body
						} else {
							// A try with no catch/finally is malformed (structuring failed). Emit the
							// internal marker so the method degrades to a stub rather than leaking the
							// broken body that produced this bare try.
							statementStr += "catch(Exception e) { throw e; /* " + malformedTryNoCatchMarker + " */ }"
						}
					}
				case *statements.WhileStatement:
					statementStr = fmt.Sprintf(c.GetTabString()+"while (%s){\n"+
						"%s\n"+
						c.GetTabString()+"}", values.SimplifyConditionValue(ret.ConditionValue).String(funcCtx), statementListToString(ret.Body))
				case *statements.DoWhileStatement:
					body := normalizeDoWhileBreakGuardSource(statementListToString(statements.NormalizeDoWhileDecrementGuard(ret.Body, funcCtx)))
					statementStr = fmt.Sprintf(c.GetTabString()+"do{\n"+
						"%s\n"+
						c.GetTabString()+"} while (%s);", body, values.SimplifyConditionValue(ret.ConditionValue).String(funcCtx))
					if ret.Label != "" {
						statementStr = fmt.Sprintf("%s%s:\n%s", c.GetTabString(), ret.Label, statementStr)
					}
				case *statements.SwitchStatement:
					getBody := func(caseItems []*statements.CaseItem) string {
						var res []string
						for _, st := range caseItems {
							if st.IsDefault {
								res = append(res, c.GetTabString()+fmt.Sprintf("default:\n%s", statementListToString(st.Body)))
								continue
							}
							res = append(res, c.GetTabString()+fmt.Sprintf("case %d:\n%s", st.IntValue, statementListToString(st.Body)))
						}
						return strings.Join(res, "\n")
					}
					statementStr = fmt.Sprintf(c.GetTabString()+"switch (%s){\n"+
						"%s\n"+
						c.GetTabString()+"}", ret.Value.String(funcCtx), getBody(ret.Cases))
				case *statements.IfStatement:
					if isEmptyAssertionsDisabledGuard(ret, funcCtx) {
						statementStr = ""
						break
					}
					if stmt := buildReturnFromEmptyGuardTernary(ret, funcCtx); stmt != "" {
						statementStr = c.GetTabString() + stmt + ";"
						break
					}
					// Recover short-circuit boolean returns: when a method returns boolean and the
					// if-then is empty (or only a `return true`) while the else is `return expr`,
					// rewrite to `return condition || expr`. This is the simplest case of the
					// boolean short-circuit DAG where the true arm shares a constant leaf.
					if isBoolReturnIfElse(ret, funcCtx) {
						if stmt := buildBoolReturnFromIfElse(ret, funcCtx); stmt != "" {
							statementStr = c.GetTabString() + stmt + ";"
							break
						}
					}
					statementStr = fmt.Sprintf(c.GetTabString()+"if (%s){\n"+
						"%s\n"+
						c.GetTabString()+"}", values.SimplifyConditionValue(ret.Condition).String(funcCtx), statementListToString(ret.IfBody))
					if len(ret.ElseBody) > 0 {
						statementStr += fmt.Sprintf("else{\n"+
							"%s\n"+
							c.GetTabString()+"}", statementListToString(ret.ElseBody))
					}
				case *statements.ReturnStatement:
					statementStr = c.GetTabString() + statement.String(funcCtx) + ";"
				case *statements.ForStatement:
					datas := []string{}
					datas = append(datas, ret.InitVar.String(funcCtx))
					datas = append(datas, fmt.Sprintf("%s", values.SimplifyConditionValue(ret.Condition.Condition).String(funcCtx)))
					datas = append(datas, ret.EndExp.String(funcCtx))
					var lines []string
					for _, subStatement := range ret.SubStatements {
						lines = append(lines, c.GetTabString()+"\t"+subStatement.String(funcCtx)+";")
					}
					s := fmt.Sprintf("%sfor(%s; %s; %s) {\n%s\n%s}", c.GetTabString(), datas[0], datas[1], datas[2], strings.Join(lines, "\n"), c.GetTabString())
					statementStr = s
				default:
					statementStr = c.GetTabString() + statement.String(funcCtx) + ";"
				}
				return statementStr
			}
			statementCodes := []string{}
			supperInvokeStr := ""
			for i, statement := range statementList {
				if i == len(statementList)-1 && methodType.FunctionType().ReturnType.String(funcCtx) == "void" {
					if _, ok := statement.(*statements.ReturnStatement); ok {
						continue
					}
				}
				if v, ok := statement.(*statements.ExpressionStatement); ok {
					if v1, ok := v.Expression.(*values.FunctionCallExpression); ok && v1.IsSupperConstructorInvoke(funcCtx) {
						supperInvokeStr = fmt.Sprintf("%s\n", statementToString(statement))
						continue
					}
				}
				if clinitHoistBarrierOn {
					staticHoistAllowedHere = !staticHoistBarrierHit
				}
				hoistBefore := hoistEventCount
				statementStr := statementToString(statement)
				if clinitHoistBarrierOn && !staticHoistBarrierHit {
					switch statement.(type) {
					case *statements.MiddleStatement, *statements.StackAssignStatement:
						// Structural markers are skipped from the body (see statementListToString);
						// treat them as transparent so they never trip the barrier.
					default:
						// A top-level <clinit> statement that produced no hoist event stays in the
						// static block; from here on nothing may be lifted ahead of it.
						if hoistEventCount == hoistBefore {
							staticHoistBarrierHit = true
						}
					}
				}
				if statementStr == "" {
					continue
				}
				statementCodes = append(statementCodes, fmt.Sprintf("%s\n", statementStr))
			}

			if isEnumCtor {
				// The only super() call in an enum constructor is the synthetic
				// super(name, ordinal); enum constructors cannot call super explicitly.
				supperInvokeStr = ""
			}
			if name != "<init>" && name != "<clinit>" &&
				needsTrailingIncompleteControlFlowThrow(statementList, methodType.FunctionType().ReturnType, funcCtx) {
				statementCodes = append(statementCodes, fmt.Sprintf("%sthrow new RuntimeException(\"incomplete control flow\");\n", c.GetTabString()))
			}
			sourceCode += supperInvokeStr + strings.Join(statementCodes, "")
			receiverType := ""
			if !funcCtx.IsStatic && name != "<clinit>" {
				receiverType = c.GetConstructorMethodName()
			}
			sourceCode = hoistCastGuardedEscapedLocals(sourceCode)
			sourceCode = addMissingGeneratedLocalDecls(sourceCode, paramsNewStr, receiverType, c.methodReturnTypeByName())
			code = sourceCode
		}
	}
	c.UnTab()

	if paramsNewStr == "" && abstractMethod {
		paramList := []string{}
		// fetch from method type
		paramTypes := methodType.FunctionType().ParamTypes
		for idx, t := range paramTypes {
			typeName := t.String(funcCtx)
			// 末参为 varargs 时必须渲染成「元素类型 + ...」(如 Feature...), 不能是「数组类型 + ...」
			// (Feature[]...)。后者会被 javac 当成 Feature[] 的 varargs (descriptor [[LFeature;), 与子类
			// 重写的 Feature... (descriptor [LFeature;) 不再 override-equivalent → 子类报「is not abstract
			// and does not override」。这里此前漏掉了 ElementType 剥离 (拼接式方法/lambda/stub 路径都对),
			// 是 fastjson2 JSONPath.set 等抽象 varargs 方法的整族重编译失败根因。
			if isVarArgs && idx == len(paramTypes)-1 && t.IsArray() && os.Getenv("JDEC_VARARGS_ABSTRACT_FIX_OFF") == "" {
				paramList = append(paramList, fmt.Sprintf("%s... var%d", t.ElementType().String(funcCtx), idx))
			} else if isVarArgs && idx == len(paramTypes)-1 {
				paramList = append(paramList, fmt.Sprintf("%s... var%d", typeName, idx))
			} else {
				paramList = append(paramList, fmt.Sprintf("%s var%d", typeName, idx))
			}
		}
		paramsNewStr = strings.Join(paramList, ", ")
	}
	if isLambda {
		// A lambda arrow body is spliced inline into the enclosing method. Lift its own locals into a
		// private `lv<seq>_N` namespace so they never shadow an enclosing local/parameter or a captured
		// variable (Java: "variable varN is already defined in method"). Nested lambda bodies were
		// dumped earlier and already carry their own `lv<innerseq>` names, so this outer rewrite (which
		// only matches `varN`) leaves them untouched.
		c.lambdaLocalSeq++
		code = renameLambdaBodyLocals(code, c.lambdaLocalSeq)
		res := fmt.Sprintf("(%s) -> {%s", paramsNewStr, code)
		res += strings.Repeat("\t", c.TabNumber()) + "}"
		dumped.methodName = name
		dumped.code = res
		dumped.bodyCode = code
		return dumped, nil
	}
	if name == "<clinit>" && strings.TrimSpace(code) == "" {
		dumped.methodName = name
		dumped.code = ""
		dumped.bodyCode = code
		return dumped, nil
	}
	methodSourceBuffer := strings.Builder{}
	isInterfaceType := slices.Contains(c.obj.AccessFlagsVerbose, "interface")
	writeAccessFlags := func(buffer io.Writer) {
		if accessFlags != "" {
			methodSourceBuffer.Write([]byte(accessFlags + " "))
		}
		// A non-abstract, non-static instance method declared in an interface is a default
		// method and must carry the `default` keyword, otherwise javac rejects the body.
		if isInterfaceType && !abstractMethod && name != "<init>" && name != "<clinit>" && !strings.Contains(accessFlags, "static") {
			methodSourceBuffer.Write([]byte("default "))
		}
	}
	writeName := func(buffer io.Writer) {
		if name == "<init>" {
			methodSourceBuffer.Write([]byte(c.GetConstructorMethodName()))
		} else {
			methodSourceBuffer.Write([]byte(class_context.SafeIdentifier(name)))
		}
	}
	writeArguments := func(buffer io.Writer) {
		methodSourceBuffer.Write([]byte(fmt.Sprintf("(%s)%s", paramsNewStr, exceptions)))
	}
	writeBlock := func(buffer io.Writer) {
		if abstractMethod {
			// An abstract method of an @interface is an annotation element; if it carries an
			// AnnotationDefault attribute we must re-emit its `default <value>` clause, otherwise
			// any use site that omits the element fails javac ("missing a default value").
			methodSourceBuffer.Write([]byte(c.annotationElementDefaultClause(method) + ";"))
		} else if code == "" {
			methodSourceBuffer.Write([]byte(" {}"))
		} else {
			body := fmt.Sprintf(" {%s%s}", code, strings.Repeat("\t", c.TabNumber()))
			methodSourceBuffer.WriteString(body)
		}
	}
	writeReturnType := func(buffer io.Writer) {
		methodSourceBuffer.Write([]byte(returnTypeStr + " "))
	}
	// writeMethodTypeParams emits a generic method's own formal type-parameter declaration ("<T> ")
	// after the access flags and before the return type, e.g. `public static <T> T checkNotNull(T x)`.
	writeMethodTypeParams := func(buffer io.Writer) {
		if methodTypeParams != "" {
			methodSourceBuffer.Write([]byte(methodTypeParams + " "))
		}
	}
	var writerSeq []func(io.Writer)
	switch name {
	case "<init>":
		writerSeq = []func(io.Writer){
			writeAccessFlags,
			writeMethodTypeParams,
			writeName,
			writeArguments,
			writeBlock,
		}
	case "<clinit>":
		writerSeq = []func(io.Writer){
			writeAccessFlags,
			writeBlock,
		}
	default:
		writerSeq = []func(io.Writer){
			writeAccessFlags,
			writeMethodTypeParams,
			writeReturnType,
			writeName,
			writeArguments,
			writeBlock,
		}
	}
	methodSource := ""
	for _, writer := range writerSeq {
		writer(&methodSourceBuffer)
	}
	methodSource = methodSourceBuffer.String()
	if len(annoStrs) == 0 {
		dumped.code = methodSource
		dumped.methodName = name
		dumped.bodyCode = code
		return dumped, nil
	} else {
		c.Tab()
		annoStr := strings.Join(annoStrs, c.GetTabString()+"\n")
		c.UnTab()
		originCode := annoStr + "\n" + c.GetTabString() + methodSource
		dumped.code = originCode
		dumped.methodName = name
		dumped.bodyCode = code
		return dumped, nil
	}
}

type dumpedMethods struct {
	methodName string
	code       string
	bodyCode   string
	// member/descriptor are retained so the post-decompile syntax safety net can rebuild a
	// stub for a method whose generated body turns out to be un-parseable.
	member     *MemberInfo
	descriptor string
}

// javaFloatLiteral renders a float constant as a valid Java float literal (with the
// mandatory 'F' suffix), handling NaN/Infinity which have no plain literal form.
// localDeclVarId returns the VariableId of a local-variable value (var0, var1, ...),
// or nil for `this`, fields, statics, or values that do not render via their slot id.
func localDeclVarId(v values.JavaValue) *utils2.VariableId {
	if v == nil {
		return nil
	}
	ref, ok := values.UnpackSoltValue(v).(*values.JavaRef)
	if !ok || ref == nil || ref.IsThis || ref.Id == nil {
		return nil
	}
	// CustomValue/StackVar refs do not render via the slot id, so renaming the id would not
	// change the emitted text; skip them.
	if ref.CustomValue != nil || ref.StackVar != nil {
		return nil
	}
	return ref.Id
}

// declareLocalInScope records a local declaration in the current scope, renaming it when its
// generated name (varN, derived from slot depth) already belongs to a *different* variable
// that is still live in an enclosing scope. The JVM reuses local slots, so two distinct
// variables in nested source scopes can collapse to the same varN, which javac rejects
// ("variable varN is already defined"). The rename uses a `_<n>` suffix the decompiler never
// generates, guaranteeing it cannot clash with a real slot name.
func declareLocalInScope(id *utils2.VariableId, live map[string]*utils2.VariableId) {
	if id == nil {
		return
	}
	name := id.String()
	if existing, ok := live[name]; ok && existing != id {
		for i := 1; ; i++ {
			cand := fmt.Sprintf("%s_%d", name, i)
			if _, taken := live[cand]; !taken {
				id.SetName(cand)
				name = cand
				break
			}
		}
	}
	live[name] = id
}

// declareCatchParamInScope registers a catch parameter in its catch-block scope, resolving the two
// distinct ways its generated name can collide with an enclosing local. Java forbids a catch
// parameter from shadowing a variable declared in an enclosing block, yet JVM slot reuse routinely
// gives a catch parameter and an unrelated local the same var<slot> name.
//
//   - Distinct ids, same printed name: the catch parameter owns its VariableId and merely renders the
//     same varN as a still-live enclosing local. declareLocalInScope renames it in place (its own id
//     gets a `_<n>` suffix). This is the common case and matches the pre-existing behavior.
//   - Shared id: slot-reuse variable merging unified the catch slot with an enclosing local that
//     occupies the same JVM slot, so they share ONE VariableId AND, in practice, the same JavaRef
//     OBJECT (the decompiler reuses one ref per slot, repointed in place by the rewriter). Renaming
//     the shared id - or mutating that ref's Id - would rename every other use of the enclosing local
//     too, leaving the clash in place and corrupting unrelated lines. The catch parameter is instead
//     split off by replacing only the exception SLICE ENTRY with a fresh clone ref that carries a new,
//     uniquely-named id. The shared object is left untouched, so the enclosing local is unaffected and
//     only the printed catch-parameter name changes.
func declareCatchParamInScope(exSlot **values.JavaRef, enclosing, inner map[string]*utils2.VariableId) {
	ex := *exSlot
	id := localDeclVarId(ex)
	if id == nil {
		return
	}
	if existing, ok := enclosing[id.String()]; ok && existing == id {
		fresh := &utils2.VariableId{}
		fresh.SetName(freshScopedName(id.String(), enclosing, inner))
		clone := *ex
		clone.Id = fresh
		*exSlot = &clone
		inner[fresh.String()] = fresh
		return
	}
	declareLocalInScope(id, inner)
}

// freshScopedName returns base with the first `_<n>` suffix that is unused in both scope maps.
func freshScopedName(base string, a, b map[string]*utils2.VariableId) string {
	for i := 1; ; i++ {
		cand := fmt.Sprintf("%s_%d", base, i)
		if _, taken := a[cand]; taken {
			continue
		}
		if _, taken := b[cand]; taken {
			continue
		}
		return cand
	}
}

func cloneScope(live map[string]*utils2.VariableId) map[string]*utils2.VariableId {
	out := make(map[string]*utils2.VariableId, len(live)+4)
	for k, v := range live {
		out[k] = v
	}
	return out
}

// resolveLocalNameCollisions walks the method body in lexical-scope order and renames any
// local declaration whose slot-derived name collides with a still-live variable from an
// enclosing scope (see declareLocalInScope). Renaming only fires on a genuine collision, so
// output for the overwhelmingly common non-colliding case is byte-for-byte unchanged. This
// fixes nested catch parameters and reused slots that javac would otherwise reject.
func resolveLocalNameCollisions(params []values.JavaValue, body []statements.Statement) {
	live := map[string]*utils2.VariableId{}
	for _, p := range params {
		if id := localDeclVarId(p); id != nil {
			live[id.String()] = id
		}
	}
	renameStatementsInScope(body, live)
}

// paramDescriptorNarrowType returns the parameter's authoritative descriptor type when that type is a
// narrow integer-category primitive (char/byte/short) but the inferred slot type has been widened to
// int. It returns nil otherwise. The JVM stores char/byte/short locals in int-sized slots and an
// in-body reassignment (e.g. `var2 = 65535`) makes the decompiler infer the slot as int; declaring the
// parameter as int then breaks a same-typed field/return assignment with "possible lossy conversion".
// The method descriptor is the ground truth for a primitive parameter's declared type, so we trust it.
// Kill-switch: JDEC_PARAM_DESC_NARROW_OFF.
func paramDescriptorNarrowType(descTypes []types.JavaType, idx int, inferred types.JavaType) types.JavaType {
	if os.Getenv("JDEC_PARAM_DESC_NARROW_OFF") != "" {
		return nil
	}
	if idx < 0 || idx >= len(descTypes) || descTypes[idx] == nil || inferred == nil {
		return nil
	}
	dp, ok := descTypes[idx].RawType().(*types.JavaPrimer)
	if !ok {
		return nil
	}
	switch dp.Name {
	case types.JavaChar, types.JavaByte, types.JavaShort:
	default:
		return nil
	}
	ip, ok := inferred.RawType().(*types.JavaPrimer)
	if !ok || ip.Name != types.JavaInteger {
		return nil
	}
	return descTypes[idx]
}

func ensureUniqueParameterNames(params []values.JavaValue, funcCtx *class_context.ClassContext) {
	seen := map[string]bool{}
	for i, p := range params {
		name := ""
		if p != nil {
			name = p.String(funcCtx)
		}
		if name == "" || seen[name] {
			id := localDeclVarId(p)
			if id == nil {
				continue
			}
			base := name
			if base == "" {
				base = fmt.Sprintf("var%d", i)
			}
			for suffix := 1; ; suffix++ {
				candidate := fmt.Sprintf("%s_%d", base, suffix)
				if !seen[candidate] {
					id.SetName(candidate)
					name = candidate
					break
				}
			}
		}
		seen[name] = true
	}
}

func renameStatementsInScope(stmts []statements.Statement, live map[string]*utils2.VariableId) {
	for _, st := range stmts {
		switch s := st.(type) {
		case *statements.AssignStatement:
			if (s.IsFirst || s.IsDeclare) && s.ArrayMember == nil {
				declareLocalInScope(localDeclVarId(s.LeftValue), live)
			}
		case *statements.IfStatement:
			renameStatementsInScope(s.IfBody, cloneScope(live))
			renameStatementsInScope(s.ElseBody, cloneScope(live))
		case *statements.DoWhileStatement:
			renameStatementsInScope(s.Body, cloneScope(live))
		case *statements.WhileStatement:
			renameStatementsInScope(s.Body, cloneScope(live))
		case *statements.ForStatement:
			inner := cloneScope(live)
			if s.InitVar != nil {
				renameStatementsInScope([]statements.Statement{s.InitVar}, inner)
			}
			renameStatementsInScope(s.SubStatements, inner)
		case *statements.SwitchStatement:
			// Java switch cases share a single block scope (fallthrough), so declarations in
			// one case are visible to later cases: use one shared inner scope.
			inner := cloneScope(live)
			for _, c := range s.Cases {
				renameStatementsInScope(c.Body, inner)
			}
		case *statements.SynchronizedStatement:
			renameStatementsInScope(s.Body, cloneScope(live))
		case *statements.TryCatchStatement:
			renameStatementsInScope(s.TryBody, cloneScope(live))
			for i, body := range s.CatchBodies {
				inner := cloneScope(live)
				if i < len(s.Exception) && s.Exception[i] != nil {
					declareCatchParamInScope(&s.Exception[i], live, inner)
				}
				renameStatementsInScope(body, inner)
			}
		}
	}
}

// localSlotRefRe matches a decompiler-generated local/parameter reference (var0, var1, ...) INCLUDING
// the collision-renamed form `varN_M` (resolveLocalNameCollisions disambiguates two same-slot-name
// locals as varN / varN_1). `this`, instance fields (this.x), and static members (Class.x) never
// render this way, so a match means the expression depends on a method-scoped value. The `(?:_\d+)*`
// suffix is load-bearing: the bare `\bvar\d+\b` failed to match `var7_1` (the `_` after the digits is
// a word char, so there is no `\b` there), letting a final field assigned from a renamed constructor
// local (`this.hashCode64 = var7_1`) be wrongly lifted to `final long hashCode64 = var7_1;` -> the
// initializer references an out-of-scope local (fastjson2 SymbolTable.hashCode64 / FactoryFunction.function).
var localSlotRefRe = regexp.MustCompile(`\bvar\d+(?:_\d+)*\b`)
var generatedLocalRefRe = regexp.MustCompile(`\bvar\d+(?:_\d+)?\b`)

// lambdaLocalRe captures the numeric tail of a slot-derived local reference (var9, var9_1) so a
// lambda body's own locals can be lifted into a private namespace. It is applied ONLY to a fully
// rendered lambda arrow body, where every other `varN`-shaped token has already been resolved away:
// lambda parameters were renamed to `l0,l1,...`, captured variables to `\x00LCAP%d\x00` placeholders,
// and any nested lambda body already carries its own `lv<seq>_` names. What remains is exactly the
// lambda's own locals, which must not shadow the enclosing scope they are spliced into.
var lambdaLocalRe = regexp.MustCompile(`\bvar(\d+(?:_\d+)?)\b`)

// renameLambdaBodyLocals rewrites a rendered lambda body's own local references from the slot-derived
// `varN` form into a per-lambda `lv<seq>_N` namespace that the enclosing slot/parameter schemes never
// produce. This is the structural fix for "variable varN is already defined in method": an inlined
// lambda arrow body cannot legally declare a local that shadows an enclosing local/parameter or a
// captured variable (which resolves to the enclosing `varN`). Kill-switch: JDEC_NO_LAMBDA_LOCAL_RENAME.
func renameLambdaBodyLocals(body string, seq int) string {
	if os.Getenv("JDEC_NO_LAMBDA_LOCAL_RENAME") != "" {
		return body
	}
	prefix := fmt.Sprintf("lv%d_", seq)
	return lambdaLocalRe.ReplaceAllString(body, prefix+"$1")
}
// The optional `(?:\s*\.\.\.)?` after the type recognizes a varargs parameter declaration
// (`int[]... var0`, `String... var1`). Without it the ellipsis broke the `Type varN` match, so a
// varargs parameter was treated as an UNDECLARED local and addMissingGeneratedLocalDecls injected a
// bogus `Object varN = null;` that shadowed the real parameter (guava Ints.concat / Longs.concat).
var generatedLocalDeclRe = regexp.MustCompile(`\b(?:boolean|byte|char|short|int|long|float|double|String|[A-Za-z_$][A-Za-z0-9_$.<>?,]*(?:\[\])*)(?:\s*\.\.\.)?\s+(var\d+(?:_\d+)?)\b`)
var mismatchedDoWhileIndexDeclRe = regexp.MustCompile(`int\s+(var\d+(?:_\d+)?)\s*=\s*0;\n(\s*)do\{\n(\s*)if \(\((var\d+)\) <`)

// monitorTempAssignRe matches a dead synthetic monitor temp left in the synchronized()
// argument position, e.g. `var2 = this.lock`, capturing the lock expression itself.
var monitorTempAssignRe = regexp.MustCompile(`^var\d+ = (.+)$`)
var doWhileBreakGuardRe = regexp.MustCompile(`^(\s*)if \(([^\n{}]*)\)\{\n\s*break;\n\s*\}else\{`)

// methodReturnTypeByName builds (and caches) a same-class method-name -> rendered-return-type map.
// Constructors and void methods are skipped; a name overloaded with conflicting return types is
// dropped so the safety net never guesses wrong. The return types are read from the authoritative
// method descriptors, not by parsing rendered source.
func (c *ClassObjectDumper) methodReturnTypeByName() map[string]string {
	if c.methodReturnTypes != nil {
		return c.methodReturnTypes
	}
	m := map[string]string{}
	ambiguous := map[string]bool{}
	for _, info := range c.obj.Methods {
		name, err := c.obj.getUtf8(info.NameIndex)
		if err != nil || name == "<init>" || name == "<clinit>" {
			continue
		}
		descriptor, err := c.obj.getUtf8(info.DescriptorIndex)
		if err != nil {
			continue
		}
		mt, perr := types.ParseMethodDescriptor(descriptor)
		if perr != nil || mt == nil {
			continue
		}
		ret := mt.FunctionType().ReturnType.String(c.FuncCtx)
		if ret == "" || ret == "void" {
			continue
		}
		if existing, ok := m[name]; ok && existing != ret {
			ambiguous[name] = true
			continue
		}
		m[name] = ret
	}
	for name := range ambiguous {
		delete(m, name)
	}
	c.methodReturnTypes = m
	return m
}

func addMissingGeneratedLocalDecls(body, params, receiverType string, methodReturnTypes map[string]string) string {
	body = repairMismatchedDoWhileIndexDecls(body)
	declared := map[string]bool{}
	for _, match := range generatedLocalDeclRe.FindAllStringSubmatch(params+"\n"+body, -1) {
		if len(match) > 1 {
			declared[match[1]] = true
		}
	}
	// Every varN token in the rendered parameter list IS, by construction, a declared parameter
	// (the param list contains only `Type varN` pairs; a type's generic args never contain a `var<digits>`
	// token). Relying solely on generatedLocalDeclRe to spot them is brittle: its type prefix cannot match
	// a parameter whose final type token before the name is a wildcard (`Map<?, ?> var2` -> the token `?>`
	// has no leading identifier char) or otherwise carries spaces, so such a parameter looked UNDECLARED
	// and a bogus `Object varN = null;` was injected that shadowed it (guava base Joiner$MapJoiner: every
	// `appendTo(StringBuilder, Map<?, ?>)` / `join(Iterable<? extends Entry<?, ?>>)` body). Mark all
	// parameter slot names declared directly so wildcard-typed parameters are never re-declared.
	for _, name := range generatedLocalRefRe.FindAllString(params, -1) {
		declared[name] = true
	}
	missing := []string{}
	seen := map[string]bool{}
	for _, name := range generatedLocalRefRe.FindAllString(body, -1) {
		if (name != "var0" || receiverType == "") && declared[name] || seen[name] {
			continue
		}
		seen[name] = true
		missing = append(missing, name)
	}
	if len(missing) == 0 {
		return body
	}
	sort.Slice(missing, func(i, j int) bool {
		return missing[i] < missing[j]
	})
	lines := make([]string, 0, len(missing))
	for _, name := range missing {
		typ, zero := "Object", "null"
		if name == "var0" && receiverType != "" {
			typ, zero = receiverType, "this"
		} else if generatedLocalLooksInt(body, name) {
			typ, zero = "int", "0"
		} else if rt := inferGeneratedLocalRefType(body, params, name, methodReturnTypes); rt != "" {
			// A REFERENCE-typed local whose value only arrives through an embedded
			// assignment in a condition ((s = next(...)) != null); without a recovered type the
			// default `Object` makes every member access (s.length()) fail to recompile.
			typ, zero = rt, "null"
		}
		lines = append(lines, fmt.Sprintf("\t%s %s = %s;\n", typ, name, zero))
	}
	return "\n" + strings.Join(lines, "") + strings.TrimPrefix(body, "\n")
}

// castEscapeDeclLineRe matches a single rendered line that DECLARES a generated local
// (`<type> varN = ...` or `<type> varN;`), capturing the indent (1), the type expression (2), the
// slot name (3) and the `= rhs` / `;` tail (4). The type char class deliberately allows spaces,
// `<>?,` and `[]` so generic and array types (`Map<?, ?> var2`, `int[] var3`) are recognized; it
// excludes `()` and `=` so a cast/return/call line (`return (T) (var2);`, `this.items.add(var8);`)
// can never be mistaken for a declaration.
var castEscapeDeclLineRe = regexp.MustCompile(`^(\s*)([A-Za-z_$][A-Za-z0-9_$.<>?,\[\] ]*?)\s+(var\d+(?:_\d+)?)(\s*[=;].*)$`)

// castEscapeTypeKeywords are leading words that look like a type to castEscapeDeclLineRe but are not:
// `return varN;` / `throw varN;` etc. must be classified as USES, never declarations.
var castEscapeTypeKeywords = map[string]bool{
	"return": true, "throw": true, "new": true, "instanceof": true,
	"else": true, "assert": true, "case": true, "yield": true,
	"break": true, "continue": true,
}

var castEscapeScalarPrimitives = map[string]bool{
	"boolean": true, "byte": true, "char": true, "short": true,
	"int": true, "long": true, "float": true, "double": true,
}

func castEscapeFirstToken(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " \t<.[("); i >= 0 {
		return s[:i]
	}
	return s
}

func castEscapeLastToken(s string) string {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

// castEscapeClassifyUse classifies a single NON-declaration occurrence of a generated local by the
// text immediately around it: 1 = explicit cast (`(X)(name)`, `(X) (name)`, `(X)name` - the run just
// before name reduces to a closing `)`), 2 = bare assignment LHS (`name = ...`, preceded only by
// whitespace, a single `=`), 0 = anything else (member access `name.f`, index `name[i]`, an uncast
// argument/return, an arithmetic/relational operand) - i.e. a use for which an `Object` declaration
// would be unsound.
func castEscapeClassifyUse(pre, post string) int {
	q := strings.TrimRight(pre, " \t")
	if strings.HasSuffix(q, "(") {
		q = strings.TrimRight(q[:len(q)-1], " \t")
	}
	if strings.HasSuffix(q, ")") {
		return 1
	}
	if strings.TrimLeft(pre, " \t") == "" {
		t := strings.TrimLeft(post, " \t")
		if strings.HasPrefix(t, "=") && !strings.HasPrefix(t, "==") {
			return 2
		}
	}
	return 0
}

// hoistCastGuardedEscapedLocals closes the "if/else parallel-phi orphan read, DIFFERENT-rendered-type
// subfamily" (the least-upper-bound subfamily of Bug AL) that the AST pass parallelArmDeclHoist
// cannot: a JVM slot reused for logically-one variable that is first-declared INSIDE two or more arms
// of an if/else (possibly nested) with DIFFERENT rendered types - e.g.
// `ParameterizedType var3 = ...` vs `ParameterizedTypeImpl var3 = ...` (fastjson2
// ObjectWriters.fieldWriterList), or `Object`/`List`/`Object var2` across three nested arms
// (JSONStreamReaderUTF8.readLineObject) - and then READ after the join. The arms carry different
// VarUids, and parallelArmDeclHoist only merges arms whose rendered type tokens AGREE (widening
// genuinely-different types would need a common-supertype facility this decompiler does not have), so
// each arm keeps its own decl, the post-join read binds a slot name whose every declaration lives in
// a non-dominating inner scope, and javac rejects it as "cannot find symbol: variable varN".
//
// Computing the true least-upper-bound of the arm types requires a cross-class hierarchy the
// decompiler cannot resolve, so this pass NEVER guesses a join type. It fires ONLY on the shape where
// `Object varN` is PROVABLY sound regardless of the LUB: every non-declaration use of the escaped slot
// is an explicit CAST (`(X)(varN)`) or a bare assignment, and no declaration of it is a scalar
// primitive. For that shape an `Object varN = null;` at method top is always type-correct - each arm
// store accepts any reference value and each read down-casts from Object - while every store keeps its
// own RHS type. Any other use (member access, index, uncast argument/return, arithmetic) makes Object
// unsound and leaves the slot untouched, so the file simply keeps its one pre-existing error (no
// regression). The transform demotes each inner `T varN = rhs` to `varN = rhs`, drops a bare
// `T varN;`, and injects the single `Object varN = null;`; addMissingGeneratedLocalDecls (run next)
// then sees the name declared and adds nothing.
//
// "Escaped" is detected by indentation: the dumper indents one tab per nesting level, so a cast-use
// whose leading-whitespace depth is SHALLOWER than every declaration of the same name is, by
// construction, outside all the arms that declare it - exactly the orphan read. A slot whose
// declaration already dominates its reads (decl depth <= every read depth) is never shallower-read and
// is left alone. Kill-switch: JDEC_CAST_ESCAPE_HOIST_OFF=1.
func hoistCastGuardedEscapedLocals(body string) string {
	if os.Getenv("JDEC_CAST_ESCAPE_HOIST_OFF") != "" {
		return body
	}
	lines := strings.Split(body, "\n")
	declAt := make([]string, len(lines))
	primDecl := map[string]bool{}
	for i, ln := range lines {
		m := castEscapeDeclLineRe.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		if castEscapeTypeKeywords[castEscapeFirstToken(m[2])] {
			continue
		}
		declAt[i] = m[3]
		typeTok := strings.TrimSpace(m[2])
		if !strings.Contains(typeTok, "[") && castEscapeScalarPrimitives[castEscapeLastToken(typeTok)] {
			primDecl[m[3]] = true
		}
	}

	type rec struct {
		declDepths []int
		castDepths []int
		bad        bool
	}
	recs := map[string]*rec{}
	get := func(name string) *rec {
		r := recs[name]
		if r == nil {
			r = &rec{}
			recs[name] = r
		}
		return r
	}
	for i, ln := range lines {
		depth := len(ln) - len(strings.TrimLeft(ln, " \t"))
		for _, loc := range generatedLocalRefRe.FindAllStringIndex(ln, -1) {
			name := ln[loc[0]:loc[1]]
			if declAt[i] == name {
				tail := strings.TrimLeft(ln[loc[1]:], " \t")
				if strings.HasPrefix(tail, "=") || strings.HasPrefix(tail, ";") {
					get(name).declDepths = append(get(name).declDepths, depth)
					continue
				}
			}
			switch castEscapeClassifyUse(ln[:loc[0]], ln[loc[1]:]) {
			case 1:
				get(name).castDepths = append(get(name).castDepths, depth)
			case 2:
				// benign assignment LHS - neither proves escape nor unsoundness
			default:
				get(name).bad = true
			}
		}
	}

	fired := map[string]bool{}
	for name, r := range recs {
		if r.bad || primDecl[name] || len(r.declDepths) == 0 || len(r.castDepths) == 0 {
			continue
		}
		minDecl := r.declDepths[0]
		for _, d := range r.declDepths[1:] {
			if d < minDecl {
				minDecl = d
			}
		}
		for _, d := range r.castDepths {
			if d < minDecl {
				fired[name] = true
				break
			}
		}
	}
	if len(fired) == 0 {
		return body
	}

	out := make([]string, 0, len(lines)+len(fired))
	for i, ln := range lines {
		if name := declAt[i]; name != "" && fired[name] {
			m := castEscapeDeclLineRe.FindStringSubmatch(ln)
			if strings.HasPrefix(strings.TrimLeft(m[4], " \t"), "=") {
				out = append(out, m[1]+name+m[4])
			}
			// a bare `T varN;` is dropped: the injected `Object varN = null;` carries the slot
			continue
		}
		out = append(out, ln)
	}

	names := make([]string, 0, len(fired))
	for n := range fired {
		names = append(names, n)
	}
	sort.Strings(names)
	// Insert after any leading blank lines (keep the body's leading newline) and after a
	// constructor's super()/this() chain call (which must remain the first statement). The injected
	// declarations adopt the indentation of the first real statement so they align with the body.
	insertIdx := 0
	for insertIdx < len(out) && strings.TrimSpace(out[insertIdx]) == "" {
		insertIdx++
	}
	indent := "\t"
	if insertIdx < len(out) {
		if t := strings.TrimSpace(out[insertIdx]); strings.HasPrefix(t, "super(") || strings.HasPrefix(t, "this(") {
			insertIdx++
		}
		if insertIdx < len(out) {
			indent = out[insertIdx][:len(out[insertIdx])-len(strings.TrimLeft(out[insertIdx], " \t"))]
		}
	}
	inject := make([]string, 0, len(names))
	for _, n := range names {
		inject = append(inject, indent+"Object "+n+" = null;")
	}
	merged := make([]string, 0, len(out)+len(inject))
	merged = append(merged, out[:insertIdx]...)
	merged = append(merged, inject...)
	merged = append(merged, out[insertIdx:]...)
	return strings.Join(merged, "\n")
}

// repairMismatchedDoWhileIndexDecls repairs the narrow case where the decompiler mis-named the
// declaration of a do-while loop index: `int X = 0;\ndo{\n if ((Y) < ...` where the loop body
// actually iterates on Y, X is a stale name the rest of the body never uses, and Y has no other
// declaration. There the `int X = 0` is the index initializer wearing the wrong name, so renaming
// it to `int Y = 0` makes the source compile.
//
// It must NOT fire on a continued-variable tail loop, where the declaration immediately before the
// do-while is a DIFFERENT, legitimate local than the one the condition tests - e.g.
// `int j = 0; do { if (i < n) { ...; j++; } }` keeps incrementing the outer index `i` while a new
// `j` is initialized first. There Y (`i`/var1) is already declared above and X (`j`/var3) is used
// inside the loop body, so the old unconditional rewrite both dropped `j`'s declaration and aliased
// it onto the already-live `i`, producing a duplicate `int var1 = 0` plus a phantom hoisted
// `int var3 = 0` (Bug C). The two guards below skip exactly that shape: only a genuinely misnamed,
// otherwise-unused declaration of an otherwise-undeclared index is rewritten. Kill-switch:
// JDEC_DOWHILE_INDEX_REPAIR_OFF=1.
func repairMismatchedDoWhileIndexDecls(body string) string {
	if os.Getenv("JDEC_DOWHILE_INDEX_REPAIR_OFF") != "" {
		return body
	}
	return mismatchedDoWhileIndexDeclRe.ReplaceAllStringFunc(body, func(match string) string {
		parts := mismatchedDoWhileIndexDeclRe.FindStringSubmatch(match)
		if len(parts) != 5 || parts[1] == parts[4] {
			return match
		}
		declaredName, indexName := parts[1], parts[4]
		// The loop index Y must be otherwise undeclared: if it already has a declaration
		// (a continued outer index), renaming X to Y would duplicate/alias a live variable.
		if generatedLocalIsDeclared(body, indexName) {
			return match
		}
		// The declared name X must be a stale name used nowhere else: a single occurrence in
		// the whole body is the declaration itself. More than one means X is a real, separate
		// variable (e.g. `j` read/incremented inside the loop) that must keep its declaration.
		if generatedLocalOccurrences(body, declaredName) > 1 {
			return match
		}
		return fmt.Sprintf("int %s = 0;\n%sdo{\n%sif ((%s) <", indexName, parts[2], parts[3], indexName)
	})
}

// generatedLocalIsDeclared reports whether name has any `T name` declaration in body.
func generatedLocalIsDeclared(body, name string) bool {
	for _, match := range generatedLocalDeclRe.FindAllStringSubmatch(body, -1) {
		if len(match) > 1 && match[1] == name {
			return true
		}
	}
	return false
}

// generatedLocalOccurrences counts whole-token references to a generated local name in body.
func generatedLocalOccurrences(body, name string) int {
	return len(regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`).FindAllString(body, -1))
}

func generatedLocalLooksInt(body, name string) bool {
	quoted := regexp.QuoteMeta(name)
	patterns := []string{
		`\b` + quoted + `\s*(?:\+\+|--)`,
		`(?:\+\+|--)\s*` + quoted + `\b`,
		// A bare RELATIONAL comparison (`(v) < n`) is numeric-only in Java -> int category for
		// any right operand.
		`\(` + quoted + `\)\s*(?:<|>|<=|>=)`,
		// A bare EQUALITY comparison (`(v) == X` / `(v) != X`) is also valid for REFERENCES, so it
		// only proves int when the right operand is a numeric literal (`(0)`, `-1`). A reference
		// comparison such as `(v) != (HashMap.class)` or `(v) != (var2)` must NOT be read as int,
		// otherwise an `objectClass`-style local (fastjson2 JSONWriter.checkAndWriteTypeName, whose
		// value is `(v = obj.getClass()) != type`) is mis-declared `int v = 0` and every Class
		// comparison/use fails to recompile.
		`\(` + quoted + `\)\s*(?:==|!=)\s*\(?-?\d`,
		`\[\s*` + quoted + `\s*\]`,
	}
	// Embedded-assignment-in-condition form produced by the dup-collapse, e.g.
	// `(var4 = expr) == (0)` / `(var4 = expr) < (n)` (commons-codec Metaphone /
	// MatchRatingApproachEncoder, and the synthetic EmbeddedAssignDecl battery). Such a variable has
	// no ordinary `T v = ...` declaration, so the safety net must synthesize one; without these
	// signals it guessed `Object v = null`, which breaks the int store and any arithmetic read.
	//   - A RELATIONAL comparison (`< > <= >=`) is numeric-only in Java, so the embedded-assign
	//     target is int-category regardless of the right operand.
	//   - An EQUALITY comparison (`== !=`) is numeric ONLY when the right operand is a numeric
	//     literal (`(0)`, `-1`); it must NOT match a reference null-check like
	//     `(o = foo()) != null`, which legitimately compiles with the `Object o = null` default.
	// Kill-switch: JDEC_NO_EMBED_ASSIGN_INT=1 restores the pre-fix (Object-defaulting) behavior.
	if os.Getenv("JDEC_NO_EMBED_ASSIGN_INT") == "" {
		patterns = append(patterns,
			`\(`+quoted+` = [^()]*\)\s*(?:<|>|<=|>=)`,
			`\(`+quoted+` = [^()]*\)\s*(?:==|!=)\s*\(?-?\d`,
		)
	}
	for _, pattern := range patterns {
		if regexp.MustCompile(pattern).MatchString(body) {
			return true
		}
	}
	return false
}

// embeddedAssignRHSRe / arrayLoadRHSRe / bareCallRHSRe / typedArrayDeclRe / methodReturnDeclRe back
// inferGeneratedLocalRefType. They are package-level so the regex compiles once.
var arrayLoadRHSRe = regexp.MustCompile(`^([A-Za-z_$][A-Za-z0-9_$]*)\[.+\]$`)
var bareCallRHSRe = regexp.MustCompile(`^([A-Za-z_$][A-Za-z0-9_$]*)\(.*\)$`)

// embeddedAssignRHS returns the right-hand side of the FIRST embedded assignment to name, i.e. the
// balanced expression X in `(name = X)`. It scans with explicit paren-depth tracking because the RHS
// itself may contain parentheses (a method call), which a regex cannot balance.
func embeddedAssignRHS(body, name string) (string, bool) {
	marker := "(" + name + " = "
	idx := strings.Index(body, marker)
	if idx < 0 {
		return "", false
	}
	start := idx + len(marker)
	depth := 1 // the '(' that opened the embedded-assignment group
	for i := start; i < len(body); i++ {
		switch body[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return strings.TrimSpace(body[start:i]), true
			}
		}
	}
	return "", false
}

// declaredArrayElementType returns the element type of an array local/param named arr by reading its
// `T[]... arr` declaration from text, or "" when none is found. For a multi-dimensional array it
// strips exactly one dimension (String[][] arr -> String[]).
func declaredArrayElementType(text, arr string) string {
	re := regexp.MustCompile(`\b([A-Za-z_$][A-Za-z0-9_$.<>?,]*)\s*((?:\[\s*\])+)\s+` + regexp.QuoteMeta(arr) + `\b`)
	m := re.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	base := m[1]
	dims := strings.Count(m[2], "[")
	if dims <= 1 {
		return base
	}
	return base + strings.Repeat("[]", dims-1)
}

// declaredMethodReturnType returns the declared return type of an in-class method named method by
// matching its `T method(params) {` definition in body, or "" when not found. The trailing `{`
// requirement distinguishes a method DEFINITION from a call site (`return method(...)` has no brace),
// and a leading `.` rules out an instance-call spelled `recv.method(`.
func declaredMethodReturnType(body, method string) string {
	re := regexp.MustCompile(`(?:^|[^.\w$])([A-Za-z_$][A-Za-z0-9_$.<>?,\[\]]*)\s+` + regexp.QuoteMeta(method) + `\s*\([^;{}]*\)\s*\{`)
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		ret := m[1]
		switch ret {
		case "return", "new", "throw", "else", "instanceof", "case":
			continue
		}
		return ret
	}
	return ""
}

// inferGeneratedLocalRefType recovers the reference type of an undeclared local that only receives
// its value through an embedded assignment in a condition, e.g.
//
//	if ((var4 = next(a, b)) != null) { var4.length(); }     // -> next()'s return type
//	while ((var4 = arr[i]) != null) { ... }                 // -> arr's element type
//
// The dup-collapse drops the standalone `T var4 = ...` declaration, so the string-level safety net
// would default it to `Object` and every member access fails to recompile (`cannot find symbol`).
// The type is recovered using ONLY information already present in the rendered body (the array's own
// declaration / the in-class method's signature), so it needs no symbol table and stays self
// contained. Anything it cannot resolve confidently returns "" so the caller keeps the safe `Object`
// default - so this can only turn a non-compiling unit into a compiling one, never the reverse.
// Kill-switch: JDEC_NO_EMBED_ASSIGN_REF=1 restores the pre-fix (Object-defaulting) behavior.
func inferGeneratedLocalRefType(body, params, name string, methodReturnTypes map[string]string) string {
	if os.Getenv("JDEC_NO_EMBED_ASSIGN_REF") != "" {
		return ""
	}
	rhs, ok := embeddedAssignRHS(body, name)
	if !ok {
		return ""
	}
	// `recv.getClass()` always yields java.lang.Class; the raw `Class` type recompiles for every
	// use (Class comparisons, Class-typed arguments). This is the single most common reference
	// embedded-assign RHS whose type a textual scan can resolve without a symbol table
	// (fastjson2 JSONWriter.checkAndWriteTypeName `objectClass = obj.getClass()`).
	if strings.HasSuffix(rhs, ".getClass()") {
		return "Class"
	}
	if m := arrayLoadRHSRe.FindStringSubmatch(rhs); m != nil {
		return declaredArrayElementType(params+"\n"+body, m[1])
	}
	if m := bareCallRHSRe.FindStringSubmatch(rhs); m != nil {
		// Prefer the authoritative class method table; fall back to an in-body declaration scan
		// (covers methods rendered in the same unit that are not in the descriptor map).
		if methodReturnTypes != nil {
			if ret := methodReturnTypes[m[1]]; ret != "" {
				return ret
			}
		}
		return declaredMethodReturnType(body, m[1])
	}
	return ""
}

// canHoistFieldInitializer reports whether a `final`-field assignment found inside <init>/
// <clinit> may be lifted into a field initializer. A real field initializer cannot reference
// constructor parameters or local variables; the JVM models those as slot locals that the
// decompiler renders as varN. If the right-hand side mentions any such local, lifting it would
// emit illegal Java (e.g. `final String id = var1;` where var1 is a constructor parameter), so
// the assignment is kept in the constructor/static block instead. Erring toward NOT hoisting is
// always safe: `this.f = expr;` / `f = expr;` compiles whether or not it could have been an
// initializer.
func canHoistFieldInitializer(rhs string) bool {
	// Kill-switch: restore the legacy narrow `\bvar\d+\b` matcher (the `_M` hole) so the
	// renamed-local mis-hoist reproduces for the load-bearing test.
	if os.Getenv("JDEC_FIELD_HOIST_RENAMED_LOCAL_OFF") != "" {
		return !localSlotRefReNarrowLegacy.MatchString(rhs)
	}
	return !localSlotRefRe.MatchString(rhs)
}

// localSlotRefReNarrowLegacy is the pre-fix matcher that misses the collision-renamed `varN_M` form;
// retained only behind the JDEC_FIELD_HOIST_RENAMED_LOCAL_OFF kill-switch for the load-bearing test.
var localSlotRefReNarrowLegacy = regexp.MustCompile(`\bvar\d+\b`)

func canHoistFieldValueInitializer(value values.JavaValue, rhs string) bool {
	if canHoistFieldInitializer(rhs) {
		return true
	}
	if cv, ok := values.UnpackSoltValue(value).(*values.CustomValue); ok && cv.Flag == "lambda" && cv.NoOuterCapture {
		return true
	}
	return false
}

// EnableFieldInitHoistGuard gates the safety guard that prevents a constructor/<clinit> field
// assignment from being wrongly lifted into a field initializer. Set to false to restore the
// legacy (over-eager) hoisting behavior for debugging/regression bisection.
var EnableFieldInitHoistGuard = true

// EnableCrossConstructorHoistGuard gates ONLY the class-wide (cross-constructor) half of the hoist
// guard: a blank final assigned exactly once per constructor body but in several overloaded
// constructors must still not be hoisted. Set to false to drop just this cross-constructor check
// (keeping the per-body guard) for debugging/regression bisection; the BlankFinalMultiCtor battery
// regresses when it is off, proving the check is load-bearing.
var EnableCrossConstructorHoistGuard = true

// crossCtorStoreOK reports whether the class-wide store count permits hoisting field name. When the
// cross-constructor guard is disabled it is a no-op (always true), isolating its effect.
func crossCtorStoreOK(totals map[string]int, name string) bool {
	if !EnableCrossConstructorHoistGuard {
		return true
	}
	return totals[name] <= 1
}

// rhsReadsInstanceField reports whether a candidate field-initializer right-hand side reads
// another instance field via `this.`. A real field initializer may reference earlier-declared
// fields, but the decompiler cannot prove the referenced field already holds its final value at
// initializer time (a blank final assigned later in the constructor still reads its default 0
// here). Lifting such an assignment silently changes the value, so we keep it in the constructor
// body where the referenced field is already assigned — always safe and value-preserving.
func rhsReadsInstanceField(rhs string) bool {
	return strings.Contains(rhs, "this.")
}

// countConstructorFieldAssignments walks a constructor (<init>) or static-initializer (<clinit>)
// body, recursing into every nested block, and counts how many times each field is assigned:
// instance fields via `this.f` and same-class static fields via `Class.f`. A genuine field
// initializer is assigned exactly once (javac copies it into the constructor prologue); a blank
// final assigned across multiple conditional branches is assigned 2+ times. Only a single
// assignment may be lifted into a field initializer — otherwise the remaining branch assignments
// stay in the body and javac rejects the now double-assigned final ("cannot assign a value to
// final variable").
func countConstructorFieldAssignments(stmts []statements.Statement, className string) map[string]int {
	counts := map[string]int{}
	tally := func(st *statements.AssignStatement) {
		if st.LeftValue == nil {
			return
		}
		switch lv := st.LeftValue.(type) {
		case *values.RefMember:
			if ref, ok := core.UnpackSoltValue(lv.Object).(*values.JavaRef); ok && ref.IsThis {
				counts[lv.Member]++
			}
		case *values.JavaClassMember:
			if lv.Name == className {
				counts[lv.Member]++
			}
		}
	}
	var walk func(list []statements.Statement)
	walkOne := func(st statements.Statement) {
		if st != nil {
			walk([]statements.Statement{st})
		}
	}
	walk = func(list []statements.Statement) {
		for _, st := range list {
			switch s := st.(type) {
			case *statements.AssignStatement:
				tally(s)
			case *statements.IfStatement:
				walk(s.IfBody)
				walk(s.ElseBody)
			case *statements.ForStatement:
				walkOne(s.InitVar)
				walk(s.SubStatements)
				walkOne(s.EndExp)
			case *statements.WhileStatement:
				walk(s.Body)
			case *statements.DoWhileStatement:
				walk(s.Body)
			case *statements.TryCatchStatement:
				walk(s.TryBody)
				for _, b := range s.CatchBodies {
					walk(b)
				}
			case *statements.SwitchStatement:
				for _, c := range s.Cases {
					walk(c.Body)
				}
			case *statements.SynchronizedStatement:
				walk(s.Body)
			}
		}
	}
	walk(stmts)
	return counts
}

// constructorFieldStoreTotals returns, per field name, how many putfield/putstatic targets it has
// across EVERY <init> and <clinit> body of this class. The result is computed once via a read-only
// opcode pre-scan (core.Decompiler.CountFieldStores) and cached on the dumper.
//
// This complements the per-body countConstructorFieldAssignments: a blank-final field may be
// assigned exactly once in each of two overloaded constructors (per-body count == 1 in both), yet
// hoisting it into a field initializer is still illegal, because every constructor would then carry
// both the initializer copy and its own assignment, double-assigning a final field. Only a field
// assigned in a single place across the whole class is safe to hoist, so callers gate hoisting on
// this total being <= 1.
//
// The scan is best-effort: if a constructor body fails to parse, its counts are simply skipped, so
// the totals never over-report (they can only under-report), and an under-report degrades to the
// pre-existing per-body guard rather than to an incorrect hoist.
func (c *ClassObjectDumper) constructorFieldStoreTotals() map[string]int {
	if c.fieldStoreTotals != nil {
		return c.fieldStoreTotals
	}
	totals := map[string]int{}
	c.fieldStoreTotals = totals
	if c.obj == nil {
		return totals
	}
	for _, info := range c.obj.Methods {
		name, err := c.obj.getUtf8(info.NameIndex)
		if err != nil || (name != "<init>" && name != "<clinit>") {
			continue
		}
		var codeAttr *CodeAttribute
		for _, attribute := range info.Attributes {
			if ca, ok := attribute.(*CodeAttribute); ok {
				codeAttr = ca
				break
			}
		}
		if codeAttr == nil {
			continue
		}
		func() {
			defer func() { recover() }()
			parser := core.NewDecompiler(codeAttr.Code, func(id int) values.JavaValue {
				return GetValueFromCP(c.ConstantPool, id)
			})
			counts, err := parser.CountFieldStores()
			if err != nil {
				return
			}
			for k, v := range counts {
				totals[k] += v
			}
		}()
	}
	return totals
}

func javaFloatLiteral(f float32) string {
	v := float64(f)
	switch {
	case math.IsNaN(v):
		return "Float.NaN"
	case math.IsInf(v, 1):
		return "Float.POSITIVE_INFINITY"
	case math.IsInf(v, -1):
		return "Float.NEGATIVE_INFINITY"
	}
	return strconv.FormatFloat(v, 'g', -1, 32) + "F"
}

// javaDoubleLiteral renders a double constant as a valid Java double literal (with a
// 'D' suffix so an integral value is not mistaken for an int), handling NaN/Infinity.
func javaDoubleLiteral(f float64) string {
	switch {
	case math.IsNaN(f):
		return "Double.NaN"
	case math.IsInf(f, 1):
		return "Double.POSITIVE_INFINITY"
	case math.IsInf(f, -1):
		return "Double.NEGATIVE_INFINITY"
	}
	return strconv.FormatFloat(f, 'g', -1, 64) + "D"
}

// DecompileStubMarker tags a method body that could not be decompiled and was replaced by a
// throwing stub (graceful degradation). Tooling such as the jdsc self-check can scan decompiled
// output for this marker to detect partial results and keep surfacing method-level bugs.
const DecompileStubMarker = "yak-decompiler:"

// malformedTryNoCatchMarker is an internal sentinel emitted when a TryCatchStatement ends up with
// no catch (or finally) handler. That is always a structuring failure -- e.g. a value-producing
// ternary inside the try region confuses the CFG and the catch handler is mis-attributed, leaking
// broken Java like `Exception v = Exception;` that the ANTLR syntax net still accepts. Detecting
// the marker degrades the whole method to an honest stub instead of emitting silently-wrong code.
// It never survives into final output because the offending method is re-rendered as a stub.
const malformedTryNoCatchMarker = "yak-decompiler-internal: try without catch handler"

func normalizeDoWhileBreakGuardSource(body string) string {
	match := doWhileBreakGuardRe.FindStringSubmatchIndex(body)
	if len(match) < 6 {
		return body
	}
	conditionStart, conditionEnd := match[4], match[5]
	condition := strings.TrimSpace(body[conditionStart:conditionEnd])
	if !shouldInvertDoWhileBreakGuard(condition) {
		return body
	}
	return body[:conditionStart] + "!(" + condition + ")" + body[conditionEnd:]
}

func shouldInvertDoWhileBreakGuard(condition string) bool {
	condition = strings.TrimSpace(condition)
	if condition == "" || strings.HasPrefix(condition, "!") {
		return false
	}
	// Only invert the common structured-loop shape where the positive loop/body
	// condition (`i < n` / `i <= n`) was attached to the synthetic break arm.
	// Already-negative break guards such as `i >= n` are semantically correct as-is.
	if strings.Contains(condition, ">=") || strings.Contains(condition, ">") ||
		strings.Contains(condition, "==") || strings.Contains(condition, "!=") {
		return false
	}
	return strings.Contains(condition, "<")
}

func canFlattenNoCatchTry(body string) bool {
	body = strings.TrimSpace(body)
	if body == "" {
		return false
	}
	if strings.Contains(body, malformedTryNoCatchMarker) ||
		strings.Contains(body, values.EmptySlotValuePlaceholder) ||
		strings.Contains(body, "= Exception;") ||
		strings.Contains(body, "= Exception\n") {
		return false
	}
	return true
}

// safeDumpMethod wraps DumpMethod with panic recovery and tab-state restoration so a
// single broken method cannot abort the whole class. DumpMethod uses a non-deferred
// Tab()/UnTab() pair, which leaves the indentation stack unbalanced if it panics midway;
// we rewind it here.
func (c *ClassObjectDumper) safeDumpMethod(name, descriptor string) (res *dumpedMethods, err error) {
	tabSaved := c.deepStack.Len()
	defer func() {
		if rec := recover(); rec != nil {
			if os.Getenv("DEC_PANIC_STACK") != "" {
				err = utils.Errorf("panic: %v\n%s", rec, debug.Stack())
			} else {
				err = utils.Errorf("panic: %v", rec)
			}
		}
		for c.deepStack.Len() > tabSaved {
			c.deepStack.Pop()
		}
	}()
	return c.DumpMethod(name, descriptor)
}

// aggressiveRedumpMethod re-decompiles a SINGLE method in aggressive mode. It is the gated retry at
// the heart of the longtail strategy: it is only ever called after the conservative dump already
// failed/degraded for this method, so methods that decompile cleanly never reach it (zero regression
// by construction). It toggles the per-dumper aggressive flag for the duration, evicts the method's
// cache entry so the re-dump actually re-runs, and returns the fresh result only if it is "clean"
// (no error, no leaked internal placeholder / malformed-try marker). On any failure it restores the
// pre-retry cache entry and returns nil, so the caller falls back to its normal stub degradation.
//
// A method is retried at most once (aggressiveRetried guard): it may reach both degradation points
// (DumpMethods and degradeInvalidMethods), but the aggressive path is deterministic so repeating it
// would only waste work and produce the same outcome.
func (c *ClassObjectDumper) aggressiveRedumpMethod(name, descriptor string) *dumpedMethods {
	traitId := fmt.Sprintf("name:%s,desc:%s", name, descriptor)
	if c.aggressiveRetried[traitId] {
		return nil
	}
	c.aggressiveRetried[traitId] = true

	savedAggressive := c.aggressive
	savedEntry, hadEntry := c.dumpedMethodsSet[traitId]
	c.aggressive = true
	delete(c.dumpedMethodsSet, traitId)
	defer func() { c.aggressive = savedAggressive }()

	res, err := c.safeDumpMethod(name, descriptor)
	clean := err == nil && res != nil &&
		!strings.Contains(res.code, values.EmptySlotValuePlaceholder) &&
		!strings.Contains(res.code, malformedTryNoCatchMarker) &&
		// Reject results that are syntactically valid but reference a local before its declaration
		// (a slot-reuse renaming bug). Adopting such a result would replace an honest stub with
		// silently-wrong code; keeping the stub upholds the never-emit-broken-code contract until the
		// underlying data-flow bug is fixed.
		!usesLocalBeforeDeclaration(res.code) &&
		// Reject results containing an empty `{ }` block: in aggressive structuring this is the
		// fingerprint of a dropped statement (the assert-ternary idiom collapsing into an empty if
		// body with a leaked unconditional throw). Such output is valid Java but semantically wrong.
		!containsEmptyControlBlock(res.bodyCode)
	if !clean {
		// Restore the exact pre-retry cache state so downstream rendering is unchanged.
		if hadEntry {
			c.dumpedMethodsSet[traitId] = savedEntry
		} else {
			delete(c.dumpedMethodsSet, traitId)
		}
		return nil
	}
	return res
}

// dumpStubMethod builds a syntactically-valid placeholder for a method whose body could
// not be decompiled. It reconstructs the signature purely from the access flags and the
// method descriptor (independent of the bytecode), so a single un-decompilable method
// degrades gracefully instead of failing the entire class. Returns nil when even the
// signature cannot be derived, in which case the caller should drop the method.
func (c *ClassObjectDumper) dumpStubMethod(method *MemberInfo, name, descriptor, reason string) (stub *dumpedMethods) {
	defer func() {
		if rec := recover(); rec != nil {
			stub = nil
		}
	}()
	methodType, perr := types.ParseMethodDescriptor(descriptor)
	if perr != nil || methodType == nil || methodType.FunctionType() == nil {
		return nil
	}
	ft := methodType.FunctionType()
	funcCtx := c.FuncCtx
	funcCtx.IsStatic = method.AccessFlags&StaticFlag == StaticFlag
	accessFlagsVerbose, accessFlags := getMethodAccessFlagsVerbose(method.AccessFlags)
	isVarArgs := slices.Contains(accessFlagsVerbose, "varargs")
	isAbstract := slices.Contains(accessFlagsVerbose, "abstract") || slices.Contains(accessFlagsVerbose, "native")
	isInterface := slices.Contains(c.obj.AccessFlagsVerbose, "interface")

	paramList := []string{}
	for idx, pt := range ft.ParamTypes {
		if isVarArgs && idx == len(ft.ParamTypes)-1 && pt.IsArray() {
			paramList = append(paramList, fmt.Sprintf("%s... var%d", pt.ElementType().String(funcCtx), idx))
		} else {
			paramList = append(paramList, fmt.Sprintf("%s var%d", pt.String(funcCtx), idx))
		}
	}
	paramsStr := strings.Join(paramList, ", ")

	// sanitize the failure reason so it can live inside a block comment on one line
	reason = strings.ReplaceAll(reason, "*/", "* /")
	reason = strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(reason)
	if len(reason) > 160 {
		reason = reason[:160]
	}

	prefix := ""
	if accessFlags != "" {
		prefix = accessFlags + " "
	}
	// A non-abstract, non-static interface method is a default method.
	if isInterface && !isAbstract && name != "<clinit>" && !strings.Contains(prefix, "static") {
		prefix += "default "
	}
	throwBody := fmt.Sprintf(" { throw new RuntimeException(%s); /* %s %s */ }",
		strconv.Quote(DecompileStubMarker+" undecompilable method body"), DecompileStubMarker, reason)

	var src string
	switch name {
	case "<clinit>":
		src = fmt.Sprintf("static { /* %s undecompilable <clinit>: %s */ }", DecompileStubMarker, reason)
	case "<init>":
		src = fmt.Sprintf("%s%s(%s)%s", prefix, c.GetConstructorMethodName(), paramsStr, throwBody)
	default:
		if isAbstract {
			src = fmt.Sprintf("%s%s %s(%s);", prefix, ft.ReturnType.String(funcCtx), name, paramsStr)
		} else {
			src = fmt.Sprintf("%s%s %s(%s)%s", prefix, ft.ReturnType.String(funcCtx), name, paramsStr, throwBody)
		}
	}
	return &dumpedMethods{methodName: name, code: src, bodyCode: "stub"}
}

// isGenuineEnum reports whether this class is a real `enum` declaration (ACC_ENUM and a
// direct java.lang.Enum supertype), as opposed to a synthetic enum-constant subclass.
func (c *ClassObjectDumper) isGenuineEnum() bool {
	if !slices.Contains(c.obj.AccessFlagsVerbose, "enum") {
		return false
	}
	sup := strings.Replace(c.obj.GetSupperClassName(), "/", ".", -1)
	return sup == "java.lang.Enum"
}

// isSyntheticEnumMethod reports whether a method is one javac auto-generates for every enum
// (values(), valueOf(String), $values()). These must not be emitted: javac re-synthesizes
// them, and emitting them yields "method X is already defined".
func (c *ClassObjectDumper) isSyntheticEnumMethod(name, descriptor string) bool {
	if name == "$values" {
		return true
	}
	selfDesc := "L" + c.obj.GetClassName() + ";"
	if name == "values" && descriptor == "()["+selfDesc {
		return true
	}
	if name == "valueOf" && descriptor == "(Ljava/lang/String;)"+selfDesc {
		return true
	}
	// Synthetic "marker" constructor javac emits for enums that have constant-specific bodies:
	// `<init>(String name, int ordinal, <Enum>$N marker)`. Its sole purpose is to give the constant-body
	// subclasses an accessible super-ctor; its body just forwards to the real `<init>(String,int)`.
	// Emitting it is ALWAYS wrong (it references a synthetic `$N` type that no longer exists once bodies
	// are folded, and renders an illegal `this(...)` after local declarations); javac re-synthesizes it
	// from the folded constant bodies on recompile. Identified by a trailing parameter typed as this
	// enum's OWN anonymous subclass `L<self>$<digits>;` -- a shape impossible to write in source, so the
	// match is exact. Kill-switch JDEC_NO_ENUM_MARKER_CTOR restores the raw (broken) emission.
	if name == "<init>" && os.Getenv("JDEC_NO_ENUM_MARKER_CTOR") == "" && c.isEnumMarkerCtorDescriptor(descriptor) {
		return true
	}
	return false
}

// isEnumMarkerCtorDescriptor reports whether descriptor's LAST parameter is an anonymous synthetic
// class `L<binary>$<digits>;` (e.g. "Lcodec/AccEnum$1;" OR "Lcom/google/common/base/Predicates$1;"),
// the signature of the javac-generated enum marker constructor. The marker's owner is whichever class
// javac happened to allocate the anonymous slot in: for a TOP-LEVEL enum it is the enum's own
// `<self>$N`, but for an enum NESTED in another class (e.g. Predicates.ObjectPredicate) javac names it
// after the ENCLOSING class (`Predicates$1`). So we only require the trailing param to be SOME
// anonymous class (simple name = all digits) -- a shape impossible to write in source, so the match is
// unambiguous. This check is reached only for genuine enums (see DumpMethods), so it never affects
// ordinary classes.
func (c *ClassObjectDumper) isEnumMarkerCtorDescriptor(descriptor string) bool {
	params := methodParamFieldDescriptors(descriptor)
	if len(params) == 0 {
		return false
	}
	last := params[len(params)-1]
	// Must be a plain (non-array) object type Lxxx; .
	if !strings.HasPrefix(last, "L") || !strings.HasSuffix(last, ";") {
		return false
	}
	bin := last[1 : len(last)-1]
	dollar := strings.LastIndexByte(bin, '$')
	if dollar < 0 || dollar == len(bin)-1 {
		return false
	}
	digits := bin[dollar+1:]
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return false
		}
	}
	return true
}

// methodParamFieldDescriptors splits a method descriptor's parameter list into raw JVM field
// descriptors (e.g. "(Ljava/lang/String;I[B)V" -> ["Ljava/lang/String;", "I", "[B"]).
func methodParamFieldDescriptors(descriptor string) []string {
	open := strings.IndexByte(descriptor, '(')
	closeIdx := strings.IndexByte(descriptor, ')')
	if open < 0 || closeIdx < 0 || closeIdx < open {
		return nil
	}
	params := descriptor[open+1 : closeIdx]
	var out []string
	for i := 0; i < len(params); {
		start := i
		for i < len(params) && params[i] == '[' {
			i++
		}
		if i >= len(params) {
			break
		}
		if params[i] == 'L' {
			semi := strings.IndexByte(params[i:], ';')
			if semi < 0 {
				break
			}
			i += semi + 1
		} else {
			i++
		}
		out = append(out, params[start:i])
	}
	return out
}

// enumConstantArgs derives the explicit constructor arguments for an enum constant from the
// `new <EnumType>(name, ordinal, args...)` expression captured in <clinit>. The first two
// arguments are the synthetic name/ordinal javac injects; the remainder are the source-level
// arguments (e.g. PLANET(mass, radius)). Returns "" for a plain constant with no extra args.
func (c *ClassObjectDumper) enumConstantArgs(name string) string {
	raw := strings.TrimSpace(c.fieldDefaultValue[name])
	if !strings.HasPrefix(raw, "new ") || !strings.HasSuffix(raw, ")") {
		return ""
	}
	open := strings.Index(raw, "(")
	if open < 0 {
		return ""
	}
	parts := splitTopLevelArgs(raw[open+1 : len(raw)-1])
	if len(parts) <= 2 {
		return ""
	}
	return strings.Join(parts[2:], ", ")
}

// splitTopLevelArgs splits a comma-separated argument list, ignoring commas nested inside
// (), [], {} or string/char literals.
func splitTopLevelArgs(s string) []string {
	var parts []string
	depth := 0
	start := 0
	var quote byte
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if quote != 0 {
			if ch == '\\' {
				i++
			} else if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '"', '\'':
			quote = ch
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	if tail := strings.TrimSpace(s[start:]); tail != "" || len(parts) > 0 {
		parts = append(parts, tail)
	}
	return parts
}

// isSyntheticAccessBridgeCtor reports whether a method is the synthetic access-bridge constructor
// javac emits (pre-nestmates) so an enclosing class can reach a nested class's PRIVATE constructor: an
// ACC_SYNTHETIC `<init>` whose LAST parameter is a synthetic anonymous marker class (binary name
// Outer$N, N all digits). A source-declared constructor can never have an anonymous-class parameter
// type, so this shape is unambiguous.
func (c *ClassObjectDumper) isSyntheticAccessBridgeCtor(descriptor string, accessFlags uint16) bool {
	if !isSyntheticMethod(accessFlags) {
		return false
	}
	mt, err := types.ParseMethodDescriptor(descriptor)
	if err != nil || mt == nil || mt.FunctionType() == nil {
		return false
	}
	pts := mt.FunctionType().ParamTypes
	if len(pts) == 0 {
		return false
	}
	cls, ok := pts[len(pts)-1].RawType().(*types.JavaClass)
	if !ok {
		return false
	}
	name := cls.Name
	if i := strings.LastIndexAny(name, "./"); i >= 0 {
		name = name[i+1:]
	}
	d := strings.LastIndexByte(name, '$')
	if d < 0 || d == len(name)-1 {
		return false
	}
	for _, r := range name[d+1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// reTypeSyntheticBridgeCtorParams replaces a synthetic access-bridge constructor's erased parameter
// types with the GENERIC parameter types of the private target constructor it forwards to. The bridge
// (last param is an anonymous marker class) carries no Signature attribute; its non-marker parameters
// therefore render as their erased descriptor types. We locate the unique non-synthetic `<init>` whose
// erased parameter list equals the bridge's leading (marker-stripped) parameters, parse its Signature
// attribute, and adopt those generic types in-place on ft.ParamTypes. The marker parameter is left
// untouched. No-op when no matching target or signature is found (output then stays as before).
func (c *ClassObjectDumper) reTypeSyntheticBridgeCtorParams(bridgeDesc string, ft *types.JavaFuncType) {
	if ft == nil {
		return
	}
	bridgeParams := methodParamFieldDescriptors(bridgeDesc)
	targetArity := len(bridgeParams) - 1 // drop the trailing synthetic marker parameter
	if targetArity <= 0 {
		return
	}
	prefix := bridgeParams[:targetArity]
	for _, m := range c.obj.Methods {
		if isSyntheticMethod(m.AccessFlags) {
			continue
		}
		if n, _ := c.obj.getUtf8(m.NameIndex); n != "<init>" {
			continue
		}
		d, _ := c.obj.getUtf8(m.DescriptorIndex)
		if !slices.Equal(methodParamFieldDescriptors(d), prefix) {
			continue
		}
		for _, attr := range m.Attributes {
			sigAttr, ok := attr.(*SignatureAttribute)
			if !ok {
				continue
			}
			sigStr, e := c.obj.getUtf8(sigAttr.SignatureIndex)
			if e != nil || sigStr == "" {
				return
			}
			_, sigParams, _ := types.ParseMethodSignatureFull(sigStr, c.FuncCtx)
			if len(sigParams) != targetArity {
				return
			}
			for i := 0; i < targetArity && i < len(ft.ParamTypes); i++ {
				if sigParams[i] != nil {
					ft.ParamTypes[i] = sigParams[i]
				}
			}
			return
		}
		return
	}
}

func (c *ClassObjectDumper) DumpMethods() ([]*dumpedMethods, error) {
	c.Tab()
	defer c.UnTab()
	genuineEnum := c.isGenuineEnum()
	var result []*dumpedMethods
	for _, method := range c.obj.Methods {
		name, err := c.obj.getUtf8(method.NameIndex)
		if err != nil {
			return nil, utils.Wrapf(err, "getUtf8(%v) failed", method.NameIndex)
		}
		descriptor, err := c.obj.getUtf8(method.DescriptorIndex)
		if err != nil {
			return nil, utils.Wrapf(err, "getUtf8(%v) failed", method.DescriptorIndex)
		}
		if genuineEnum && c.isSyntheticEnumMethod(name, descriptor) {
			continue
		}
		if v := c.lambdaMethods[name]; slices.Contains(v, descriptor) {
			continue
		}
		// Synthetic lambda bodies (javac emits "lambda$...") must never be dumped as
		// standalone methods: they are only valid inlined as lambda expressions.
		// Dumping them here would also poison the method cache with a method-declaration
		// form, breaking later inline rendering at the invokedynamic call site.
		if strings.HasPrefix(name, "lambda$") && isSyntheticMethod(method.AccessFlags) {
			continue
		}
		// Compiler-generated bridge methods (ACC_BRIDGE, always also ACC_SYNTHETIC) implement
		// covariant returns and generic erasure. They are not source-level declarations; dumping
		// them yields illegal Java (two methods differing only by return type, e.g. `String build()`
		// plus a synthetic `Object build()`). Suppress them so the output mirrors the original
		// source. CFR and Vineflower suppress bridge methods as well.
		if isBridgeMethod(method.AccessFlags) {
			continue
		}
		// if name != "isSymlink" {
		// 	continue
		// }
		res, err := c.safeDumpMethod(name, descriptor)
		if err == nil && res != nil && name == "<clinit>" && c.isInterfaceLike() && isIgnorableAssertionOnlyClinit(res.code) {
			continue
		}
		if err == nil && res != nil && strings.Contains(res.code, values.EmptySlotValuePlaceholder) {
			// The decompiled body leaked an internal placeholder ("empty slot value"),
			// which means the stack simulation was incomplete and the emitted source is
			// not valid Java. Degrade to a stub instead of producing un-compilable code.
			if os.Getenv("DEBUG_EMPTYSLOT") == "" {
				err = utils.Errorf("incomplete stack simulation: empty stack slot leaked into method body")
			} else {
				log.Errorf("DEBUG_EMPTYSLOT method %s%s:\n%s", name, descriptor, res.code)
			}
		}
		if err == nil && res != nil && strings.Contains(res.code, malformedTryNoCatchMarker) {
			// The try-region structuring failed and produced a try with no catch handler,
			// which means the body is semantically corrupted (e.g. the caught-exception
			// placeholder leaked into the try). Degrade to a stub.
			if os.Getenv("DEBUG_TRYNOCATCH") != "" {
				log.Errorf("DEBUG_TRYNOCATCH method %s%s:\n%s", name, descriptor, res.code)
			}
			err = utils.Errorf("try-region structuring failed: try without catch handler")
		}
		if err != nil {
			// Gated aggressive retry: this method failed conservative decompilation (error, leaked
			// empty slot, or malformed try). Re-decompile ONLY this method in aggressive mode and
			// adopt the result if it now produces a clean body. Whole-class syntax validation still
			// runs afterwards, so an aggressive result that is clean-looking but invalid Java is
			// caught and re-degraded at the degradeInvalidMethods stage.
			if retry := c.aggressiveRedumpMethod(name, descriptor); retry != nil {
				log.Infof("aggressive retry recovered method %s%s", name, descriptor)
				res = retry
				err = nil
			}
		}
		if name == "<clinit>" && c.isInterfaceLike() {
			// Interfaces and annotations cannot declare a source-level static initializer.
			// DumpMethodWithInitialId has already hoisted any representable final-field
			// assignments into field initializers; leftover helper-array stores have no legal
			// method form and must not be emitted only to be dropped by the syntax safety net.
			continue
		}
		if err != nil {
			// Graceful degradation: an un-decompilable method body must not fail the whole
			// class. Emit a stub method (correct signature, throwing body) so the rest of
			// the class still decompiles.
			log.Warnf("decompile method %s%s failed, emitting stub: %v", name, descriptor, err)
			stub := c.dumpStubMethod(method, name, descriptor, err.Error())
			if stub == nil {
				// even the signature could not be derived; drop the method to keep output valid
				log.Warnf("stub for method %s%s could not be built, skipping", name, descriptor)
				continue
			}
			traitId := fmt.Sprintf("name:%s,desc:%s", name, descriptor)
			c.dumpedMethodsSet[traitId] = stub
			res = stub
		}
		accessFlagsVerbose, _ := getMethodAccessFlagsVerbose(method.AccessFlags)
		if strings.TrimSpace(res.bodyCode) == "" {
			// A synthetic access-bridge constructor whose body decompiled to empty (its `this()`
			// delegation to a trivial no-arg ctor was stripped) must be KEPT, not dropped. javac emits
			// this package-private bridge (pre-nestmates) so an enclosing class can reach a nested
			// class's PRIVATE no-arg constructor; the call site is `new Outer$Inner((Outer$N)null)`.
			// Once nested classes are decompiled as flat top-level `Outer$Inner` units, dropping the
			// bridge leaves that call resolving to no constructor ("constructor cannot be applied to
			// given types" - the single largest guava `base` recompile blocker via
			// Platform$JdkPatternCompiler). The empty body implicitly calls super() exactly as the
			// no-arg target did, so it is semantically faithful. Kill-switch: JDEC_NO_SYN_BRIDGE_CTOR=1.
			isSynBridgeCtor := name == "<init>" && os.Getenv("JDEC_NO_SYN_BRIDGE_CTOR") == "" &&
				c.isSyntheticAccessBridgeCtor(descriptor, method.AccessFlags)
			// A genuinely-declared constructor whose body decompiled to just the implicit super()
			// must be KEPT unless it is indistinguishable from the constructor javac auto-generates
			// when a class declares none. Dropping a programmer-declared no-arg ctor while OTHER
			// ctors exist removes it from the API (e.g. guava `VerifyException()` -> `new
			// VerifyException()` no longer resolves); dropping a non-public SOLE ctor (singleton
			// `private Foo(){}`) silently widens accessibility; dropping a PARAMETERIZED empty-body
			// ctor (`Foo(int){ super(); }`) deletes a real overload. All are semantic regressions
			// the syntax safety net cannot catch. Kill-switch: JDEC_NO_KEEP_DECLARED_CTOR=1.
			keepDeclaredCtor := name == "<init>" && !isSynBridgeCtor &&
				os.Getenv("JDEC_NO_KEEP_DECLARED_CTOR") == "" &&
				!c.isOmittableDefaultCtor(descriptor, accessFlagsVerbose)
			if isSynBridgeCtor || keepDeclaredCtor {
				// keep res as the empty-body constructor (faithful: empty body == implicit super())
			} else if !slices.Contains(accessFlagsVerbose, "abstract") && !slices.Contains(accessFlagsVerbose, "annotation") && !slices.Contains(accessFlagsVerbose, "interface") && !slices.Contains(accessFlagsVerbose, "enum") {
				methodType, perr := types.ParseMethodDescriptor(descriptor)
				descBroken := perr != nil || methodType == nil || methodType.FunctionType() == nil
				isVoid := !descBroken && methodType.FunctionType().ReturnType.String(c.FuncCtx) == "void"
				if descBroken || isVoid {
					// A genuinely-empty void method (its bytecode is just `return`) is a faithful
					// `void f(...) {}` and MUST be emitted, not dropped: dropping silently removes the
					// method from the API and, when it overrides an abstract method (a no-op override
					// such as ObjectWriterBaseModule$VoidObjectWriter.write(JSONWriter,Object,Object,
					// Type,long){}), makes the subclass "not abstract and does not override". Only the
					// trivial-return shape is kept; a void body that decompiled to empty but is NOT
					// backed by a bare return (real content lost) keeps the legacy drop so no half-
					// decompiled body is emitted as if empty. Kill-switch: JDEC_NO_EMIT_EMPTY_VOID=1.
					if isVoid && os.Getenv("JDEC_NO_EMIT_EMPTY_VOID") == "" && methodBodyIsTriviallyEmpty(method) {
						// keep res: renders as the faithful empty-body `void f(...) {}`
					} else {
						continue
					}
				} else {
					stub := c.dumpStubMethod(method, name, descriptor, "empty method body after decompilation")
					if stub == nil {
						continue
					}
					traitId := fmt.Sprintf("name:%s,desc:%s", name, descriptor)
					c.dumpedMethodsSet[traitId] = stub
					res = stub
				}
			}
		}
		// retain identity so the syntax safety net can re-derive a stub if needed
		if res.member == nil {
			res.member = method
		}
		if res.descriptor == "" {
			res.descriptor = descriptor
		}
		result = append(result, res)
	}
	return result, nil
}

func (c *ClassObjectDumper) isInterfaceLike() bool {
	return slices.Contains(c.obj.AccessFlagsVerbose, "interface") || slices.Contains(c.obj.AccessFlagsVerbose, "annotation")
}

// methodBodyIsTriviallyEmpty reports whether the method's bytecode is a genuinely empty body: only
// `nop` (0x00) padding plus exactly one `return` (0xb1, the void return). Such a method is a faithful
// `void f(...) {}` whose empty decompiled body is correct, so it must be emitted rather than dropped.
// The {nop,return}-only test is sound: any opcode carrying operand bytes (one of which could happen to
// be 0xb1) is itself outside {0x00,0xb1}, so its presence is detected and excludes the method, leaving
// only truly empty bodies. A method that decompiled to empty but has richer bytecode (real content the
// decompiler failed to recover) returns false and keeps the legacy drop behavior.
func methodBodyIsTriviallyEmpty(method *MemberInfo) bool {
	if method == nil {
		return false
	}
	var code []byte
	for _, attr := range method.Attributes {
		if ca, ok := attr.(*CodeAttribute); ok {
			code = ca.Code
			break
		}
	}
	if len(code) == 0 {
		return false
	}
	returns := 0
	for _, b := range code {
		switch b {
		case 0x00: // nop
		case 0xb1: // return (void)
			returns++
		default:
			return false
		}
	}
	return returns == 1
}

// isOmittableDefaultCtor reports whether an empty-body constructor (its body decompiled to just the
// implicit super()) is indistinguishable from the no-arg constructor javac auto-generates when a
// class declares NONE, so dropping it is loss-less (javac regenerates an identical one). This holds
// ONLY when all of the following are true:
//   - it takes no parameters (descriptor "()V") -- javac never auto-generates a parameterized ctor;
//   - it is the class's SOLE constructor -- if other ctors exist, no default is generated, so this
//     no-arg ctor was written explicitly and is part of the public API;
//   - its accessibility matches the implicit default's (public for a public class, package-private
//     otherwise) -- a non-public sole ctor (singleton pattern) must be kept or accessibility widens.
//
// Any empty-body constructor failing these is programmer-declared and MUST be emitted.
func (c *ClassObjectDumper) isOmittableDefaultCtor(descriptor string, ctorAccessVerbose []string) bool {
	if descriptor != "()V" {
		return false
	}
	ctorCount := 0
	for _, m := range c.obj.Methods {
		if n, _ := c.obj.getUtf8(m.NameIndex); n == "<init>" {
			ctorCount++
		}
	}
	if ctorCount != 1 {
		return false
	}
	if slices.Contains(ctorAccessVerbose, "protected") || slices.Contains(ctorAccessVerbose, "private") {
		return false
	}
	return slices.Contains(c.obj.AccessFlagsVerbose, "public") == slices.Contains(ctorAccessVerbose, "public")
}

func isIgnorableAssertionOnlyClinit(code string) bool {
	body := strings.TrimSpace(code)
	if !strings.Contains(body, "$assertionsDisabled") {
		return false
	}
	body = strings.TrimPrefix(body, "static")
	body = strings.TrimSpace(body)
	body = strings.TrimPrefix(body, "{")
	body = strings.TrimSuffix(body, "}")
	body = strings.TrimSpace(body)
	body = strings.ReplaceAll(body, "\n", "")
	body = strings.ReplaceAll(body, "\t", "")
	body = strings.ReplaceAll(body, " ", "")
	return body == "" ||
		body == "if(Record$1.$assertionsDisabled){}else{return;}" ||
		strings.HasPrefix(body, "if(") && strings.Contains(body, "$assertionsDisabled)") &&
			strings.HasSuffix(body, "{}else{return;}")
}

func (c *ClassObjectDumper) dumpConstantPool() ([]string, error) {
	result := []string{}
	for _, constant := range c.obj.ConstantPool {
		switch ret := constant.(type) {
		case *ConstantIntegerInfo:
		case *ConstantFloatInfo:
		case *ConstantLongInfo:
		case *ConstantDoubleInfo:
		case *ConstantUtf8Info:
			result = append(result, ret.Value)
		case *ConstantStringInfo:
		case *ConstantClassInfo:
		case *ConstantFieldrefInfo:
		case *ConstantMethodrefInfo:
		case *ConstantInterfaceMethodrefInfo:
		case *ConstantNameAndTypeInfo:
		case *ConstantMethodTypeInfo:
		case *ConstantMethodHandleInfo:
		case *ConstantInvokeDynamicInfo:
		case *ConstantModuleInfo:
		case *ConstantPackageInfo:
		}
	}
	return result, nil
}

// isBoolReturnIfElse detects the pattern where an if-then-else in a boolean-returning
// method has an empty (or trivially `return true`) then-body and a boolean return in the
// else-body. This is the simplest manifestation of the boolean short-circuit DAG where the
// compiler shared a constant true leaf across both the short-circuit and the fallback.
// We can recover `return cond || elseReturnExpr` from it.
func isBoolReturnIfElse(ifSt *statements.IfStatement, funcCtx *class_context.ClassContext) bool {
	// Only applies to boolean-returning methods.
	if funcCtx.FunctionType == nil {
		return false
	}
	retType := ""
	if ft, ok := funcCtx.FunctionType.(*types.JavaFuncType); ok {
		retType = ft.ReturnType.String(funcCtx)
	}
	if retType != "boolean" {
		return false
	}
	// Then-body must be empty or contain only `return true`.
	thenIsTrue := len(ifSt.IfBody) == 0
	if !thenIsTrue && len(ifSt.IfBody) == 1 {
		if rs, ok := ifSt.IfBody[0].(*statements.ReturnStatement); ok {
			thenIsTrue = rs.JavaValue != nil && rs.JavaValue.String(funcCtx) == "true"
		}
	}
	if !thenIsTrue {
		return false
	}
	// Else-body must end with a boolean return.
	if len(ifSt.ElseBody) == 0 {
		return false
	}
	lastElse := ifSt.ElseBody[len(ifSt.ElseBody)-1]
	rs, ok := lastElse.(*statements.ReturnStatement)
	if !ok || rs.JavaValue == nil {
		return false
	}
	return true
}

func buildReturnFromEmptyGuardTernary(ifSt *statements.IfStatement, funcCtx *class_context.ClassContext) string {
	if !isEffectivelyEmptyBody(ifSt.IfBody) || ifSt.Condition == nil {
		return ""
	}
	meaningfulElse := meaningfulStatements(ifSt.ElseBody)
	if len(meaningfulElse) != 1 {
		return ""
	}
	ret, ok := meaningfulElse[0].(*statements.ReturnStatement)
	if !ok || ret.JavaValue == nil {
		return ""
	}
	tern, ok := values.UnpackSoltValue(ret.JavaValue).(*values.TernaryExpression)
	if !ok || tern.Condition == nil || tern.TrueValue == nil || tern.FalseValue == nil {
		return ""
	}
	slot, ok := tern.Condition.(*values.SlotValue)
	if !ok || slot.GetValue() != nil {
		return ""
	}
	guard := values.SimplifyConditionValue(values.NewUnaryExpression(
		ifSt.Condition,
		values.Not,
		types.NewJavaPrimer(types.JavaBoolean),
	))
	return fmt.Sprintf("return (%s) ? (%s) : (%s)", guard.String(funcCtx), tern.TrueValue.String(funcCtx), tern.FalseValue.String(funcCtx))
}

func isEffectivelyEmptyBody(body []statements.Statement) bool {
	return len(meaningfulStatements(body)) == 0
}

func isEmptyAssertionsDisabledGuard(ifSt *statements.IfStatement, funcCtx *class_context.ClassContext) bool {
	if ifSt == nil || ifSt.Condition == nil {
		return false
	}
	if !isEffectivelyNoOpBody(ifSt.IfBody) || !isEffectivelyNoOpBody(ifSt.ElseBody) {
		return false
	}
	return strings.Contains(ifSt.Condition.String(funcCtx), "$assertionsDisabled")
}

func isEffectivelyNoOpBody(body []statements.Statement) bool {
	for _, st := range meaningfulStatements(body) {
		ret, ok := st.(*statements.ReturnStatement)
		if !ok || ret.JavaValue != nil {
			return false
		}
	}
	return true
}

func meaningfulStatements(body []statements.Statement) []statements.Statement {
	var out []statements.Statement
	for _, st := range body {
		switch st.(type) {
		case *statements.MiddleStatement, *statements.StackAssignStatement:
			continue
		default:
			out = append(out, st)
		}
	}
	return out
}

// buildBoolReturnFromIfElse emits `return cond || elseExpr` from the detected if-else pattern.
func buildBoolReturnFromIfElse(ifSt *statements.IfStatement, funcCtx *class_context.ClassContext) string {
	cond := values.SimplifyConditionValue(ifSt.Condition).String(funcCtx)
	// Extract the return expression from the else body.
	lastElse := ifSt.ElseBody[len(ifSt.ElseBody)-1]
	rs := lastElse.(*statements.ReturnStatement)
	elseExpr := rs.JavaValue.String(funcCtx)
	// If the else body has statements before the return, we can't fold into a single
	// expression; fall back to emitting the if-else as-is.
	if len(ifSt.ElseBody) > 1 {
		return "" // signal: caller should use normal rendering
	}
	return fmt.Sprintf("return (%s) || (%s)", cond, elseExpr)
}
