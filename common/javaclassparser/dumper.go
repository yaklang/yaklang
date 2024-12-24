package javaclassparser

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

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
	for _, k := range c.obj.AccessFlagsVerbose {
		if k == "interface" || k == "enum" || k == "annotation" {
			if k == "interface" {
				isInterface = true
			} else if k == "enum" {
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
	name := c.obj.GetClassName()
	splits := strings.Split(name, "/")
	packageName := strings.Join(splits[:len(splits)-1], ".")
	c.PackageName = packageName
	className := splits[len(splits)-1]
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
	if supperClassName != "java.lang.Object" {
		if isEnum && (supperClassName == "java.lang.Enum" || supperClassName == "Enum") {
			supperClassName = ""
			superStr = ""
		} else {
			funcCtx.Import(supperClassName)
			supperClassName = funcCtx.ShortTypeName(supperClassName)
			if supperClassName != "" {
				superStr += fmt.Sprintf(" extends %s", supperClassName)
			}
		}
	}

	ifaces := c.obj.Interfaces
	interfaceLists := make([]string, 0, len(ifaces))
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
	attrs := ""
	fields, err := c.DumpFields()
	if err != nil {
		return "", utils.Wrap(err, "DumpFields failed")
	}
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
			if isEnum {
				//if method.methodName == "values" {
				//	continue
				//}
				//if method.methodName == "valueOf" {
				//	continue
				//}
			}
			attrs += fmt.Sprintf("\t%s\n", method.code)
		}
	}
	var classKeyword string
	if !nonClassKeyword {
		classKeyword = " class"
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
	return packageSource + importsStr + result, nil
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
		fieldTypeStr := fieldType.String(c.FuncCtx)
		c.FuncCtx.Import(fieldTypeStr)
		lastPacket := c.FuncCtx.ShortTypeName(fieldTypeStr)

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
					valueLiteral = strconv.Quote(constStr)
				case *ConstantIntegerInfo:
					valueLiteral = strconv.Itoa(int(constVal.Value))
				case *ConstantLongInfo:
					valueLiteral = strconv.Itoa(int(constVal.Value))
					if !strings.HasSuffix(valueLiteral, "L") {
						valueLiteral += "L"
					}
				default:
					log.Errorf("when handling for fields unknown constant type: %T", constVal)
				}
			default:
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
		} else if slices.Contains(accessFlagsVerbose, "final") && c.fieldDefaultValue[name] != "" {
			dumped := dumpedFields{
				code:      fmt.Sprintf("%s %s %s = %s;", accessFlags, lastPacket, name, c.fieldDefaultValue[name]),
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
				valStr = fmt.Sprintf("\"%s\"", s)
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
			valStr = fmt.Sprintf("\"%s\"", element.Value.(string))
		case 'e':
			//val := &EnumConstValue{
			//	TypeName:  getUtf8(reader.readUint16()),
			//	ConstName: getUtf8(reader.readUint16()),
			//}
			//ele.Value = val
			enum := element.Value.(*EnumConstValue)
			valStr = fmt.Sprintf("%s.%s", enum.TypeName, enum.ConstName)
		case 'c':
			//ele.Value = getUtf8(reader.readUint16())
			valStr = element.Value.(string)
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

func (c *ClassObjectDumper) DumpMethod(methodName, desc string) (dumpedMethods, error) {
	return c.DumpMethodWithInitialId(methodName, desc, 0)
}

func (c *ClassObjectDumper) DumpMethodWithInitialId(methodName, desc string, id int) (dumpedMethods, error) {
	var method *MemberInfo
	var name, descriptor string
	var err error
	var dumped dumpedMethods
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
	//if name != "crt_data_by_Attrs" {
	//	continue
	//}
	//println(name)
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
				//if statementSet.Has(statement) {
				//	panic("statement already exists")
				//}
				statementSet.Add(statement)
				switch ret := statement.(type) {
				case *statements.AssignStatement:
					foundFieldInit := false
					if v, ok := ret.LeftValue.(*values.RefMember); ok {
						if v1, ok := v.Object.(*values.JavaRef); ok && v1.IsThis {
							foundFieldInit = true
							c.fieldDefaultValue[v.Member] = ret.JavaValue.String(funcCtx)
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
						statementStr += fmt.Sprintf("catch(%s %s){\n"+
							"%s\n"+
							c.GetTabString()+"}", ret.Exception[i].Type().String(funcCtx), ret.Exception[i].String(funcCtx), statementListToString(body))
					}
				case *statements.WhileStatement:
					statementStr = fmt.Sprintf(c.GetTabString()+"while (%s){\n"+
						"%s\n"+
						c.GetTabString()+"}", ret.ConditionValue.String(funcCtx), statementListToString(ret.Body))
				case *statements.DoWhileStatement:
					statementStr = fmt.Sprintf(c.GetTabString()+"do{\n"+
						"%s\n"+
						c.GetTabString()+"} while (%s);", statementListToString(ret.Body), ret.ConditionValue.String(funcCtx))
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
						c.GetTabString()+"}", ret.Condition.String(funcCtx), statementListToString(ret.IfBody))
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
					datas = append(datas, fmt.Sprintf("%s", ret.Condition.String(funcCtx)))
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
			for i, statement := range statementList {
				if i == len(statementList)-1 && methodType.FunctionType().ReturnType.String(funcCtx) == "void" {
					if _, ok := statement.(*statements.ReturnStatement); ok {
						continue
					}
				}
				statementStr := statementToString(statement)
				if statementStr == "" {
					continue
				}
				sourceCode += fmt.Sprintf("%s\n", statementStr)
			}
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
}

func (c *ClassObjectDumper) DumpMethods() ([]dumpedMethods, error) {
	c.Tab()
	defer c.UnTab()
	var result []dumpedMethods
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
		res, err := c.DumpMethod(name, descriptor)
		if err != nil {
			return nil, fmt.Errorf("dump method %s failed, %w", name, err)
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
