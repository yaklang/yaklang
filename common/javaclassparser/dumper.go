package javaclassparser

import (
	"errors"
	"fmt"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

const classTemplate = "%s class %s%s {%s}"
const attrTemplate = `%s %s %s {%s}`

type ClassObjectDumper struct {
	imports       map[string]struct{}
	obj           *ClassObject
	FuncCtx       *class_context.ClassContext
	ClassName     string
	PackageName   string
	CurrentMethod *MemberInfo
	ConstantPool  []ConstantInfo
	deepStack     *utils.Stack[int]
	MethodType    *types.JavaFuncType
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
		obj:          obj,
		ConstantPool: obj.ConstantPool,
		imports:      make(map[string]struct{}),
		deepStack:    utils.NewStack[int](),
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
	result := classTemplate
	accessFlagsVerbose := c.obj.AccessFlagsVerbose
	//if len(accessFlagsVerbose) < 1 {
	//	return "", utils.Error("accessFlagsVerbose is empty")
	//}
	accessFlags := strings.Join(accessFlagsVerbose, " ")
	name := c.obj.GetClassName()
	splits := strings.Split(name, "/")
	packageName := strings.Join(splits[:len(splits)-1], ".")
	c.PackageName = packageName
	className := splits[len(splits)-1]
	c.ClassName = strings.Replace(name, "/", ".", -1)
	funcCtx := &class_context.ClassContext{
		ClassName:   c.ClassName,
		PackageName: c.PackageName,
	}
	c.FuncCtx = funcCtx
	buildInLib := []string{
		c.PackageName + ".*",
		"java.lang.*",
		//"java.io.*",
	}
	for _, s := range buildInLib {
		funcCtx.Import(s)
	}
	superStr := ""
	supperClassName := c.obj.GetSupperClassName()
	supperClassName = strings.Replace(supperClassName, "/", ".", -1)
	if supperClassName != "java.lang.Object" {
		funcCtx.Import(supperClassName)
		supperClassName = funcCtx.ShortTypeName(supperClassName)
		if supperClassName != "" {
			superStr += fmt.Sprintf(" extends %s", supperClassName)
		}
	}
	for _, u := range c.obj.Interfaces {
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
			superStr += fmt.Sprintf(" implements %s", name)
		}
	}

	packageSource := fmt.Sprintf("package %s;\n\n", packageName)
	if className == "" {
		return "", utils.Error("className is empty")
	}
	attrs := ""
	fields, err := c.DumpFields()
	if err != nil {
		return "", utils.Wrap(err, "DumpFields failed")
	}
	if len(fields) > 0 {
		attrs += "\n\t// Fields\n"
		for _, field := range fields {
			attrs += fmt.Sprintf("\t%s\n", field)
		}
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
	if len(methods) > 0 {
		attrs += "\n"
		for _, method := range methods {
			attrs += fmt.Sprintf("\t%s\n", method)
		}
	}

	result = fmt.Sprintf(result, accessFlags, className, superStr, attrs)
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
	for lib, _ := range c.imports {
		if strings.HasPrefix(lib, "java.lang") {
			continue
		}
		importsStr += fmt.Sprintf("import %s;\n", lib)
	}
	//constantPool, err := c.dumpConstantPool()
	//if err != nil {
	//	return "", err
	//}
	//constantPoolStr := strings.Join(constantPool, "\n// ")
	//constantPoolStr = "\n// Constant Pool\n// " + constantPoolStr
	return packageSource + importsStr + result, nil
}
func (c *ClassObjectDumper) DumpFields() ([]string, error) {
	result := []string{}
	for _, field := range c.obj.Fields {
		accessFlagsVerbose := getAccessFlagsVerbose(field.AccessFlags)
		//if len(accessFlagsVerbose) < 1 {
		//	return nil, utils.Error("fields accessFlagsVerbose is empty")
		//}
		accessFlags := strings.Join(accessFlagsVerbose, " ")
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
		lastPacket := c.parseImportCLass(fieldType.String(c.FuncCtx))
		result = append(result, fmt.Sprintf("%s %s %s;", accessFlags, lastPacket, name))
	}
	return result, nil
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

func (c *ClassObjectDumper) DumpMethods() ([]string, error) {
	c.Tab()
	defer c.UnTab()
	result := []string{}
	for _, method := range c.obj.Methods {
		c.FuncCtx.IsStatic = method.AccessFlags&StaticFlag == StaticFlag
		accessFlagsVerbose := getAccessFlagsVerbose(method.AccessFlags)
		//if len(accessFlagsVerbose) < 1 {
		//	return nil, utils.Error("method accessFlagsVerbose is empty")
		//}
		accessFlags := strings.Join(accessFlagsVerbose, " ")
		name, err := c.obj.getUtf8(method.NameIndex)
		if err != nil {
			return nil, utils.Wrapf(err, "getUtf8(%v) failed", method.NameIndex)
		}
		descriptor, err := c.obj.getUtf8(method.DescriptorIndex)
		if err != nil {
			return nil, utils.Wrapf(err, "getUtf8(%v) failed", method.DescriptorIndex)
		}
		methodType, err := types.ParseMethodDescriptor(descriptor)
		if err != nil {
			return nil, utils.Wrapf(err, "ParseMethodDescriptor(%v) failed", descriptor)
		}
		paramsNewStrList := []string{}
		for i, paramsType := range methodType.FunctionType().ParamTypes {
			paramsNewStrList = append(paramsNewStrList, fmt.Sprintf("%s var%d", paramsType.String(c.FuncCtx), i+1))
		}
		c.MethodType = methodType.FunctionType()
		returnTypeStr := methodType.FunctionType().ReturnType.String(c.FuncCtx)
		paramsNewStr := strings.Join(paramsNewStrList, ", ")
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
		for _, attribute := range method.Attributes {
			if anno, ok := attribute.(*RuntimeVisibleAnnotationsAttribute); ok {
				for _, annotation := range anno.Annotations {
					res, err := c.DumpAnnotation(annotation)
					if err != nil {
						return nil, err
					}
					annoStrs = append(annoStrs, res)
				}
			}
			if codeAttr, ok := attribute.(*CodeAttribute); ok {
				if name == "main" {
					log.Debug("decompile main func")
				}
				statementList, err := ParseBytesCode(c, codeAttr)
				if err != nil {
					return nil, utils.Wrap(err, "ParseBytesCode failed")
				}
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
				for _, statement := range statementList {
					statementStr := statementToString(statement)
					sourceCode += fmt.Sprintf("%s\n", statementStr)
				}
				code = sourceCode
			}
		}
		c.UnTab()
		methodSource := ""
		switch name {
		case "<init>":
			name = fmt.Sprintf("%s(%s)", c.GetConstructorMethodName(), paramsNewStr)
			methodSource = fmt.Sprintf("%s %s {%s", accessFlags, name, code)
		case "<clinit>":
			methodSource = fmt.Sprintf("%s {%s", accessFlags, code)
		default:
			name = fmt.Sprintf("%s(%s)", name, paramsNewStr)
			methodSource = fmt.Sprintf(`%s %s %s {%s`, accessFlags, returnTypeStr, name, code)
		}
		methodSource += strings.Repeat("\t", c.TabNumber()) + "}"
		if len(annoStrs) == 0 {
			result = append(result, methodSource)
		} else {
			c.Tab()
			annoStr := strings.Join(annoStrs, c.GetTabString()+"\n")
			c.UnTab()
			result = append(result, annoStr+"\n"+c.GetTabString()+methodSource)
		}
	}
	return result, nil
}
func (c *ClassObjectDumper) parseImportCLass(name string) string {
	packageName, className := core.SplitPackageClassName(name)
	if packageName != "" {
		c.imports[packageName] = struct{}{}
	}
	return className
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
