package javaclassparser

import (
	"fmt"
	"regexp"
	"strings"
	"yaklang.io/yaklang/common/utils"
)

const classTemplate = "\n//Class Declaration\n%s class %s{%s}"
const attrTemplate = `%s %s %s;`

type ClassObjectDumper struct {
	imports map[string]struct{}
	obj     *ClassObject
}

func NewClassObjectDumper(obj *ClassObject) *ClassObjectDumper {
	return &ClassObjectDumper{obj: obj, imports: make(map[string]struct{})}
}
func (c *ClassObjectDumper) DumpClass() (string, error) {
	result := classTemplate

	accessFlagsVerbose := c.obj.AccessFlagsVerbose
	if len(accessFlagsVerbose) < 1 {
		return "", utils.Error("accessFlagsVerbose is empty")
	}
	accessFlags := strings.Join(accessFlagsVerbose, " ")

	className := c.obj.GetClassName()
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
		attrs += "\t// Methods\n"
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
	return importsStr + result, nil
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
		result = append(result, fmt.Sprintf(attrTemplate, accessFlags, lastPacket, name))
	}
	return result, nil
}
func (c *ClassObjectDumper) DumpMethods() ([]string, error) {
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
		name = fmt.Sprintf("%s(%s)", name, paramsNewStr)
		result = append(result, fmt.Sprintf(attrTemplate, accessFlags, returnType, name))

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
