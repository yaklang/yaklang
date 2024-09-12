package javaclassparser

import (
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"regexp"
	"strings"
)

const classTemplate = "%s class %s {%s}"
const attrTemplate = `%s %s %s {%s}`

type ClassObjectDumper struct {
	imports       map[string]struct{}
	obj           *ClassObject
	ClassName     string
	PackageName   string
	CurrentMethod *MemberInfo
	ConstantPool  []ConstantInfo
	deepStack     *utils.Stack[int]
}

func (c *ClassObjectDumper) GetConstructorMethodName() string {
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
	if len(accessFlagsVerbose) < 1 {
		return "", utils.Error("accessFlagsVerbose is empty")
	}
	accessFlags := strings.Join(accessFlagsVerbose, " ")
	name := c.obj.GetClassName()
	splits := strings.Split(name, "/")
	packageName := strings.Join(splits[:len(splits)-1], ".")
	c.PackageName = packageName
	className := splits[len(splits)-1]
	c.ClassName = strings.Replace(name, "/", ".", -1)
	packageSource := fmt.Sprintf("package %s;\n\n", packageName)
	if className == "" {
		return "", utils.Error("className is empty")
	}
	attrs := ""
	fields, err := c.DumpFields()
	if err != nil {
		return "", err
	}
	if len(fields) > 0 {
		attrs += "\n\t// Fields\n"
		for _, field := range fields {
			attrs += fmt.Sprintf("\t%s\n", field)
		}
	}

	methods, err := c.DumpMethods()
	if err != nil {
		return "", err
	}
	if len(methods) > 0 {
		attrs += "\n"
		for _, method := range methods {
			attrs += fmt.Sprintf("\t%s\n", method)
		}
	}
	result = fmt.Sprintf(result, accessFlags, className, attrs)
	importsStr := ""
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
		if len(accessFlagsVerbose) < 1 {
			return nil, utils.Error("fields accessFlagsVerbose is empty")
		}
		accessFlags := strings.Join(accessFlagsVerbose, " ")
		name, err := c.obj.getUtf8(field.NameIndex)
		if err != nil {
			return nil, err
		}
		descriptor, err := c.obj.getUtf8(field.DescriptorIndex)
		if err != nil {
			return nil, err
		}
		lastPacket := c.parseImportCLass(descriptor)
		result = append(result, fmt.Sprintf("%s %s %s;", accessFlags, lastPacket, name))
	}
	return result, nil
}
func (c *ClassObjectDumper) DumpMethods() ([]string, error) {
	c.Tab()
	defer c.UnTab()
	result := []string{}
	for _, method := range c.obj.Methods {
		accessFlagsVerbose := getAccessFlagsVerbose(method.AccessFlags)
		if len(accessFlagsVerbose) < 1 {
			return nil, utils.Error("method accessFlagsVerbose is empty")
		}
		accessFlags := strings.Join(accessFlagsVerbose, " ")
		name, err := c.obj.getUtf8(method.NameIndex)
		if err != nil {
			return nil, err
		}
		descriptor, err := c.obj.getUtf8(method.DescriptorIndex)
		if err != nil {
			return nil, err
		}
		r, err := regexp.Compile("\\((.*)\\)(.+?)")
		if err != nil {
			return nil, err
		}

		matchRes := r.FindAllStringSubmatch(descriptor, -1)
		if len(matchRes) != 1 {
			return nil, utils.Error("method descriptor is invalid")
		}
		matchResOne := matchRes[0]
		if len(matchResOne) != 3 {
			return nil, utils.Error("method descriptor is invalid")
		}
		paramsStr := matchResOne[1]
		params := strings.Split(paramsStr, ";")
		paramsNewStrList := []string{}
		returnType := matchResOne[2]
		if returnType == "V" {
			returnType = "void"
		}
		for i, param := range params {
			array := ""
			if param == "" {
				continue
			}
			lastPacket := c.parseImportCLass(param)
			paramsNewStrList = append(paramsNewStrList, fmt.Sprintf("%s%s var%d", lastPacket, array, i))
		}
		paramsNewStr := strings.Join(paramsNewStrList, ", ")
		code := ""
		c.Tab()
		c.CurrentMethod = method
		funcCtx := &decompiler.FunctionContext{
			ClassName:    c.ClassName,
			FunctionName: name,
			PackageName:  c.PackageName,
			BuildInLibs: []string{
				"java.lang.*",
			},
		}
		for _, attribute := range method.Attributes {
			if codeAttr, ok := attribute.(*CodeAttribute); ok {
				statements, err := ParseBytesCode(c, codeAttr)
				if err != nil {
					return nil, err
				}
				sourceCode := "\n"
				for _, statement := range statements {
					var statementStr string
					switch ret := statement.(type) {
					case *decompiler.IfStatement:
						getBody := func(sts []decompiler.Statement) string {
							c.Tab()
							defer c.UnTab()
							var res []string
							for _, st := range sts {
								res = append(res, c.GetTabString()+st.String(funcCtx))
							}
							return strings.Join(res, "\n")
						}
						statementStr = fmt.Sprintf(c.GetTabString()+"if (%s){\n"+
							"%s\n"+
							c.GetTabString()+"}else{\n"+
							"%s\n"+
							c.GetTabString()+"}", ret.Condition.String(funcCtx), getBody(ret.IfBody), getBody(ret.ElseBody))
					case *decompiler.ExpressionStatement:
						if funcCtx.FunctionName == "<init>" {
							if v, ok := ret.Expression.(*decompiler.FunctionCallExpression); ok {
								if IsJavaSupperRef(v.Object) && v.FunctionName == "<init>" {
									continue
								}
							}
						}
						statementStr = c.GetTabString() + statement.String(funcCtx) + ";"
					case *decompiler.ReturnStatement:
						if funcCtx.FunctionName == "<init>" {
							continue
						}
						statementStr = c.GetTabString() + statement.String(funcCtx) + ";"
					case *decompiler.ForStatement:
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
			methodSource = fmt.Sprintf(`%s %s %s {%s`, accessFlags, returnType, name, code)
		}
		methodSource += strings.Repeat("\t", c.TabNumber()) + "}"
		result = append(result, methodSource)
	}
	return result, nil
}
func (c *ClassObjectDumper) parseImportCLass(name string) string {
	if name[len(name)-1] == ';' {
		name = name[:len(name)-1]
	}
	array := ""
	if name[0] == '[' {
		name = name[1:]
		array = "[]"
	}
	if name[0] == 'L' {
		name = name[1:]
	}
	paramSplit := strings.Split(name, "/")
	lastPacket := paramSplit[len(paramSplit)-1]
	c.imports[strings.Join(paramSplit, ".")] = struct{}{}
	return lastPacket + array
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
		}
	}
	return result, nil
}
