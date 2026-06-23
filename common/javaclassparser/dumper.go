package javaclassparser

import (
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/davecgh/go-spew/spew"
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
	obj               *ClassObject
	FuncCtx           *class_context.ClassContext
	ClassName         string
	PackageName       string
	CurrentMethod     *MemberInfo
	ConstantPool      []ConstantInfo
	deepStack         *utils.Stack[int]
	MethodType        *types.JavaFuncType
	lambdaMethods     map[string][]string
	fieldDefaultValue map[string]string
	dumpedMethodsSet  map[string]*dumpedMethods
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
		obj:               obj,
		ConstantPool:      obj.ConstantPool,
		deepStack:         utils.NewStack[int](),
		lambdaMethods:     map[string][]string{},
		fieldDefaultValue: map[string]string{},
		dumpedMethodsSet:  map[string]*dumpedMethods{},
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
func (c *ClassObjectDumper) DumpClass() (string, error) {
	// accessFlagsVerbose := c.obj.AccessFlagsVerbose
	accessFlagsToCode := c.obj.AccessFlagsToCode

	nonClassKeyword := false
	isInterface := false
	isEnum := false
	syntheticEnumSubclass := false
	superRawName := strings.Replace(c.obj.GetSupperClassName(), "/", ".", -1)
	for _, k := range c.obj.AccessFlagsVerbose {
		if k == "interface" || k == "enum" || k == "annotation" {
			if k == "interface" {
				isInterface = true
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
	className := splits[len(splits)-1]
	// module-info / package-info are synthetic descriptor pseudo-classes; their internal
	// name ("module-info" / "package-info") is not a legal Java identifier, so emitting
	// `class module-info {}` yields un-parseable source. Render a valid minimal compilation
	// unit instead. (Full JPMS module / package-info annotation reconstruction is a
	// separate feature.)
	if className == "module-info" || className == "package-info" {
		var sb strings.Builder
		if className == "package-info" && packageName != "" {
			sb.WriteString(fmt.Sprintf("package %s;\n\n", packageName))
		}
		sb.WriteString(fmt.Sprintf("// decompiled from a synthetic %s descriptor\n", className))
		return sb.String(), nil
	}
	supperClassName := c.obj.GetSupperClassName()
	supperClassName = strings.Replace(supperClassName, "/", ".", -1)
	c.ClassName = strings.Replace(name, "/", ".", -1)
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
	superStr := ""
	ifaces := c.obj.Interfaces
	interfaceLists := make([]string, 0, len(ifaces)+1)
	if supperClassName != "java.lang.Object" {
		if isEnum && (supperClassName == "java.lang.Enum" || supperClassName == "Enum") {
			supperClassName = ""
			superStr = ""
		} else {
			funcCtx.Import(supperClassName)
			supperClassName = funcCtx.ShortTypeName(supperClassName)
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
		name = funcCtx.ShortTypeName(strings.Replace(name, "/", ".", -1))
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
	var classKeyword string
	if !nonClassKeyword {
		classKeyword = " class"
	}
	// assemble renders the full compilation unit from the current methods/fields. It is a
	// closure so the syntax safety net can re-render after degrading malformed members.
	assemble := func() string {
		attrs := ""
		if len(fields) > 0 {
			attrs += "\n\t// Fields\n"
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
				attrs += fmt.Sprintf("\t%s", enumSimple.fieldName)
				if idx == len(enumFields)-1 {
					attrs += ";\n"
				} else {
					attrs += ",\n"
				}
			}
			for _, ordinaryField := range ordinaryFields {
				attrs += fmt.Sprintf("\t%s\n", ordinaryField)
			}
		}
		if len(methods) > 0 {
			attrs += "\n"
			for _, method := range methods {
				attrs += fmt.Sprintf("\t%s\n", method.code)
			}
		}
		result := fmt.Sprintf("%s%s %s%s {%s}", accessFlags, classKeyword, className, superStr, attrs)
		if len(annoStrs) > 0 {
			result = fmt.Sprintf("%s\n%s", strings.Join(annoStrs, "\n"), result)
		}
		importsStr := ""
		for _, s := range funcCtx.GetAllImported() {
			if utils.StringSliceContain(buildInLib, s) {
				continue
			}
			importsStr += fmt.Sprintf("import %s;\n", s)
		}
		if len(importsStr) > 0 {
			importsStr += "\n"
		}
		return packageSource + importsStr + result
	}

	full := assemble()
	if EnableDecompileSyntaxValidation {
		if err := validateJavaSyntax(full); err != nil {
			// The assembled class is not valid Java. Degrade malformed members (using the real
			// class header so interface/enum/constructor context is honored) and re-render, so a
			// single broken method/field cannot make the whole class un-parseable.
			header := fmt.Sprintf("%s%s %s%s", accessFlags, classKeyword, className, superStr)
			methods = c.degradeInvalidMethods(header, methods)
			fields = c.degradeInvalidFields(header, className, isEnum, fields)
			full = assemble()
			if err := validateJavaSyntax(full); err != nil {
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
		descriptor, err := c.obj.getUtf8(field.DescriptorIndex)
		if err != nil {
			return nil, err
		}
		fieldType, err := types.ParseDescriptor(descriptor)
		if err != nil {
			return nil, err
		}

		lastPacket := ""
		if fieldType.IsArray() {
			javaTyp := fieldType.RawType().(*types.JavaArrayType)
			fieldTypeStr := javaTyp.JavaType.String(c.FuncCtx)
			c.FuncCtx.Import(fieldTypeStr)
			shortName := c.FuncCtx.ShortTypeName(fieldTypeStr)
			originalType := javaTyp.JavaType
			javaTyp.JavaType = types.NewJavaClass(shortName)
			lastPacket = javaTyp.JavaType.String(c.FuncCtx)
			javaTyp.JavaType = originalType
		} else {
			fieldTypeStr := fieldType.String(c.FuncCtx)
			c.FuncCtx.Import(fieldTypeStr)
			lastPacket = c.FuncCtx.ShortTypeName(fieldTypeStr)
		}
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
					valueLiteral = strconv.Itoa(int(constVal.Value))
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
				log.Infof("field %s is synthetic", name)
			case *DeprecatedAttribute:
			// log.Infof("field %s is deprecated", name)
			case *SignatureAttribute:

			case *UnparsedAttribute:
				log.Error("cannot handle attribute type: UnparsedAttribute")
				spew.Dump(ret)
			case *RuntimeVisibleAnnotationsAttribute:

			default:
				log.Info(spew.Sdump(ret))
				log.Errorf("when handling for fields unknown attribute type: %T", ret)
			}
		}

		if valueLiteral != "" {
			fields = append(fields, dumpedFields{
				code:      fmt.Sprintf("%s %s %s = %s;", accessFlags, lastPacket, name, valueLiteral),
				fieldName: name,
				modifier:  accessFlags,
				typeName:  lastPacket,
			})
		} else if slices.Contains(accessFlagsVerbose, "final") {
			defaultValue := "0"
			if c.fieldDefaultValue[name] != "" {
				defaultValue = c.fieldDefaultValue[name]
			}
			dumped := dumpedFields{
				code:      fmt.Sprintf("%s %s %s = %s;", accessFlags, lastPacket, name, defaultValue),
				fieldName: name,
				modifier:  accessFlags,
				typeName:  lastPacket,
			}

			fields = append(fields, dumped)
		} else {
			fields = append(fields, dumpedFields{
				code:      fmt.Sprintf("%s %s %s;", accessFlags, lastPacket, name),
				fieldName: name,
				modifier:  accessFlags,
				typeName:  lastPacket,
			})
		}
	}
	return fields, nil
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
				valStr = fmt.Sprintf("%d", ret.Value)
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
	for _, field := range c.obj.Fields {
		var finalFalg uint16 = 0x0010
		if field.AccessFlags&finalFalg == finalFalg {
			finalFieldMap[c.obj.ConstantPoolManager.GetUtf8(int(field.NameIndex)).Value] = struct{}{}
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
			if len(params) > 0 {
				if v, ok := params[0].(*values.JavaRef); ok && v.IsThis {
					params = params[1:]
				}
			}
			paramsNewStrList := []string{}
			for i, val := range params {
				if i == len(params)-1 && isVarArgs {
					paramsNewStrList = append(paramsNewStrList, fmt.Sprintf("%s... %s", val.Type().ElementType().String(c.FuncCtx), val.String(c.FuncCtx)))
				} else {
					paramsNewStrList = append(paramsNewStrList, fmt.Sprintf("%s %s", val.Type().String(c.FuncCtx), val.String(c.FuncCtx)))
				}
			}
			c.MethodType = methodType.FunctionType()
			paramsNewStr = strings.Join(paramsNewStrList, ", ")

			sourceCode := "\n"
			statementSet := utils.NewSet[statements.Statement]()
			var statementToString func(statement statements.Statement) string
			var statementListToString func(statements []statements.Statement) string
			statementListToString = func(statementList []statements.Statement) string {
				c.Tab()
				defer c.UnTab()
				var res []string
				for _, statement := range statementList {
					if _, ok := statement.(*statements.MiddleStatement); ok {
						continue
					}
					_, ok := statement.(*statements.StackAssignStatement)
					if ok {
						continue
					}
					res = append(res, statementToString(statement))
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
					if v, ok := ret.LeftValue.(*values.RefMember); ok {
						obj := core.UnpackSoltValue(v.Object)
						if v1, ok := obj.(*values.JavaRef); ok && v1.IsThis && (funcCtx.FunctionName == "<cinit>" || funcCtx.FunctionName == "<init>" || funcCtx.FunctionName == funcCtx.ClassName) {
							if _, ok := finalFieldMap[v.Member]; ok {
								foundFieldInit = true
								c.fieldDefaultValue[v.Member] = ret.JavaValue.String(funcCtx)
							}
						}
					} else if v, ok := ret.LeftValue.(*values.JavaClassMember); ok {
						if funcCtx.FunctionName == "<cinit>" || v.Name == funcCtx.ClassName {
							if _, ok := finalFieldMap[v.Member]; ok {
								foundFieldInit = true
								c.fieldDefaultValue[v.Member] = ret.JavaValue.String(funcCtx)
							}
						}
					}
					if !foundFieldInit {
						statementStr = c.GetTabString() + statement.String(funcCtx) + ";"
					}
				case *statements.SynchronizedStatement:
					statementStr = fmt.Sprintf(c.GetTabString()+"synchronized(%s){\n"+
						"%s\n"+
						c.GetTabString()+"}", ret.Argument.String(funcCtx), statementListToString(ret.Body))
				case *statements.TryCatchStatement:
					statementStr = fmt.Sprintf(c.GetTabString()+"try{\n"+
						"%s\n"+
						c.GetTabString()+"}", statementListToString(ret.TryBody))
					for i, body := range ret.CatchBodies {
						excType := ret.Exception[i].Type().String(funcCtx)
						// A catch clause type must be a reference type (subtype of Throwable).
						// When upstream type inference degrades the exception variable to a
						// primitive (e.g. "boolean" from a reused slot), fall back to Throwable
						// so the output stays syntactically valid.
						switch excType {
						case "boolean", "byte", "char", "short", "int", "long", "float", "double", "void":
							excType = "Throwable"
						}
						statementStr += fmt.Sprintf("catch(%s %s){\n"+
							"%s\n"+
							c.GetTabString()+"}", excType, ret.Exception[i].String(funcCtx), statementListToString(body))
					}
					haveCatch := len(ret.CatchBodies) > 0
					if !haveCatch {
						statementStr += "catch(Exception e) { throw e; }"
					}
				case *statements.WhileStatement:
					statementStr = fmt.Sprintf(c.GetTabString()+"while (%s){\n"+
						"%s\n"+
						c.GetTabString()+"}", values.SimplifyConditionValue(ret.ConditionValue).String(funcCtx), statementListToString(ret.Body))
				case *statements.DoWhileStatement:
					statementStr = fmt.Sprintf(c.GetTabString()+"do{\n"+
						"%s\n"+
						c.GetTabString()+"} while (%s);", statementListToString(ret.Body), values.SimplifyConditionValue(ret.ConditionValue).String(funcCtx))
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
				statementStr := statementToString(statement)
				if statementStr == "" {
					continue
				}
				statementCodes = append(statementCodes, fmt.Sprintf("%s\n", statementStr))
			}

			sourceCode += supperInvokeStr + strings.Join(statementCodes, "")
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
			if isVarArgs && idx == len(paramTypes)-1 {
				paramList = append(paramList, fmt.Sprintf("%s... var%d", typeName, idx))
			} else {
				paramList = append(paramList, fmt.Sprintf("%s var%d", typeName, idx))
			}
		}
		paramsNewStr = strings.Join(paramList, ", ")
	}
	if isLambda {
		res := fmt.Sprintf("(%s) -> {%s", paramsNewStr, code)
		res += strings.Repeat("\t", c.TabNumber()) + "}"
		dumped.methodName = name
		dumped.code = res
		dumped.bodyCode = code
		return dumped, nil
	}
	methodSourceBuffer := strings.Builder{}
	writeAccessFlags := func(buffer io.Writer) {
		if accessFlags != "" {
			methodSourceBuffer.Write([]byte(accessFlags + " "))
		}
	}
	writeName := func(buffer io.Writer) {
		if name == "<init>" {
			methodSourceBuffer.Write([]byte(c.GetConstructorMethodName()))
		} else {
			methodSourceBuffer.Write([]byte(name))
		}
	}
	writeArguments := func(buffer io.Writer) {
		methodSourceBuffer.Write([]byte(fmt.Sprintf("(%s)%s", paramsNewStr, exceptions)))
	}
	writeBlock := func(buffer io.Writer) {
		if abstractMethod {
			methodSourceBuffer.Write([]byte(";"))
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
	var writerSeq []func(io.Writer)
	switch name {
	case "<init>":
		writerSeq = []func(io.Writer){
			writeAccessFlags,
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

// safeDumpMethod wraps DumpMethod with panic recovery and tab-state restoration so a
// single broken method cannot abort the whole class. DumpMethod uses a non-deferred
// Tab()/UnTab() pair, which leaves the indentation stack unbalanced if it panics midway;
// we rewind it here.
func (c *ClassObjectDumper) safeDumpMethod(name, descriptor string) (res *dumpedMethods, err error) {
	tabSaved := c.deepStack.Len()
	defer func() {
		if rec := recover(); rec != nil {
			err = utils.Errorf("panic: %v", rec)
		}
		for c.deepStack.Len() > tabSaved {
			c.deepStack.Pop()
		}
	}()
	return c.DumpMethod(name, descriptor)
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

func (c *ClassObjectDumper) DumpMethods() ([]*dumpedMethods, error) {
	c.Tab()
	defer c.UnTab()
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
		// if name != "isSymlink" {
		// 	continue
		// }
		res, err := c.safeDumpMethod(name, descriptor)
		if err == nil && res != nil && strings.Contains(res.code, values.EmptySlotValuePlaceholder) {
			// The decompiled body leaked an internal placeholder ("empty slot value"),
			// which means the stack simulation was incomplete and the emitted source is
			// not valid Java. Degrade to a stub instead of producing un-compilable code.
			err = utils.Errorf("incomplete stack simulation: empty stack slot leaked into method body")
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
			if !slices.Contains(accessFlagsVerbose, "abstract") && !slices.Contains(accessFlagsVerbose, "annotation") && !slices.Contains(accessFlagsVerbose, "interface") && !slices.Contains(accessFlagsVerbose, "enum") {
				continue
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
