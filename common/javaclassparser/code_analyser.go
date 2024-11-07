package javaclassparser

import (
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
	"github.com/yaklang/yaklang/common/log"
	"strings"
)

func getNameAndType(pool []ConstantInfo, index uint16) (string, string) {
	indexFromPool := func(i int) ConstantInfo {
		return pool[i-1]
	}
	nameAndTypeInfo := pool[index-1].(*ConstantNameAndTypeInfo)
	name := indexFromPool(int(nameAndTypeInfo.NameIndex)).(*ConstantUtf8Info).Value
	desc := indexFromPool(int(nameAndTypeInfo.DescriptorIndex)).(*ConstantUtf8Info).Value
	return name, desc
}
func showOpcodes(codes []*core.OpCode) {
	for i, opCode := range codes {
		if opCode.Instr.Name == "if_icmpge" || opCode.Instr.Name == "goto" {
			fmt.Printf("%d %s jmpto:%d\n", i, opCode.Instr.Name, opCode.Jmp)
		} else {
			fmt.Printf("%d %s %v\n", i, opCode.Instr.Name, opCode.Data)
		}
	}
}

func GetValueFromCP(pool []ConstantInfo, index int) values.JavaValue {
	indexFromPool := func(i int) ConstantInfo {
		return pool[i-1]
	}
	constant := pool[index-1]
	getClassName := func(index uint16) string {
		classInfo := indexFromPool(int(index)).(*ConstantClassInfo)
		nameInfo := indexFromPool(int(classInfo.NameIndex)).(*ConstantUtf8Info)
		return nameInfo.Value
	}
	convertMemberInfo := func(classMemberInfo *ConstantMemberrefInfo) values.JavaValue {
		className := getClassName(classMemberInfo.ClassIndex)
		name, desc := getNameAndType(pool, classMemberInfo.NameAndTypeIndex)
		typ, err := types.ParseMethodDescriptor(desc)
		if err != nil {
			log.Errorf("parse descriptor failed:%s", desc)
		}
		return values.NewJavaClassMember(className, name, typ)
	}
	switch ret := constant.(type) {
	case *ConstantMemberrefInfo:
		return convertMemberInfo(ret)
	case *ConstantInterfaceMethodrefInfo:
		memberInfo := ret.ConstantMemberrefInfo
		return convertMemberInfo(&memberInfo)
	case *ConstantFieldrefInfo:
		classInfo := indexFromPool(int(ret.ClassIndex)).(*ConstantClassInfo)
		nameInfo := indexFromPool(int(classInfo.NameIndex)).(*ConstantUtf8Info)

		nameAndType := indexFromPool(int(ret.NameAndTypeIndex)).(*ConstantNameAndTypeInfo)
		refNameInfo := indexFromPool(int(nameAndType.NameIndex)).(*ConstantUtf8Info)
		descInfo := indexFromPool(int(nameAndType.DescriptorIndex)).(*ConstantUtf8Info)
		typeName := nameInfo.Value
		typeName = strings.Replace(typeName, "/", ".", -1)
		typ, err := types.ParseDescriptor(descInfo.Value)
		if err != nil {
			log.Errorf("parse descriptor failed:%s", descInfo.Value)
		}
		classIns := values.NewJavaClassMember(typeName, refNameInfo.Value, typ)
		return classIns
	case *ConstantMethodrefInfo:
		classInfo := indexFromPool(int(ret.ClassIndex)).(*ConstantClassInfo)
		nameInfo := indexFromPool(int(classInfo.NameIndex)).(*ConstantUtf8Info)

		nameAndType := indexFromPool(int(ret.NameAndTypeIndex)).(*ConstantNameAndTypeInfo)
		refNameInfo := indexFromPool(int(nameAndType.NameIndex)).(*ConstantUtf8Info)
		descInfo := indexFromPool(int(nameAndType.DescriptorIndex)).(*ConstantUtf8Info)
		typeName := nameInfo.Value
		typeName = strings.Replace(typeName, "/", ".", -1)
		typ, err := types.ParseMethodDescriptor(descInfo.Value)
		if err != nil {
			log.Errorf("parse descriptor failed:%s", descInfo.Value)
		}
		classIns := values.NewJavaClassMember(typeName, refNameInfo.Value, typ)
		return classIns
	case *ConstantClassInfo:
		nameInfo := indexFromPool(int(ret.NameIndex)).(*ConstantUtf8Info)
		typeName := nameInfo.Value
		typeName = strings.Replace(typeName, "/", ".", -1)
		return values.NewJavaClassValue(types.NewJavaClass(typeName))
	case *ConstantModuleInfo:
		nameInfo := indexFromPool(int(ret.NameIndex)).(*ConstantUtf8Info)
		typeName := nameInfo.Value
		typeName = strings.Replace(typeName, "/", ".", -1)
		log.Warn("TODO: the java module should be a new java type")
		return values.NewJavaClassValue(types.NewJavaClass(typeName))
	case *ConstantPackageInfo:
		nameInfo := indexFromPool(int(ret.NameIndex)).(*ConstantUtf8Info)
		typeName := nameInfo.Value
		typeName = strings.Replace(typeName, "/", ".", -1)
		log.Warn("TODO: the java module should be a new java type")
		return values.NewJavaClassValue(types.NewJavaClass(typeName))
	default:
		panic("failed")
	}
}
func GetLiteralFromCP(pool []ConstantInfo, index int) values.JavaValue {
	constant := pool[index-1]
	switch ret := constant.(type) {
	case *ConstantStringInfo:
		return values.NewJavaLiteral(pool[ret.StringIndex-1].(*ConstantUtf8Info).Value, types.NewJavaPrimer(types.JavaString))
	case *ConstantLongInfo:
		return values.NewJavaLiteral(ret.Value, types.NewJavaPrimer(types.JavaLong))
	case *ConstantIntegerInfo:
		return values.NewJavaLiteral(ret.Value, types.NewJavaPrimer(types.JavaInteger))
	case *ConstantDoubleInfo:
		return values.NewJavaLiteral(ret.Value, types.NewJavaPrimer(types.JavaDouble))
	case *ConstantFloatInfo:
		return values.NewJavaLiteral(ret.Value, types.NewJavaPrimer(types.JavaFloat))
	case *ConstantClassInfo:
		return GetValueFromCP(pool, index)
	case *ConstantModuleInfo:
		return GetValueFromCP(pool, index)
	case *ConstantPackageInfo:
		return GetValueFromCP(pool, index)
	default:
		panic("failed")
	}
}

type VarMap struct {
	id  int
	val values.JavaValue
}

func ParseBytesCode(dumper *ClassObjectDumper, codeAttr *CodeAttribute) ([]statements.Statement, error) {
	pool := dumper.ConstantPool
	parser := core.NewDecompiler(codeAttr.Code, func(id int) values.JavaValue {
		return GetValueFromCP(dumper.ConstantPool, id)
	})
	parser.FunctionContext = dumper.FuncCtx
	parser.FunctionType = dumper.MethodType
	//parser.FunctionContext.FunctionName
	parser.ConstantPoolLiteralGetter = func(id int) values.JavaValue {
		return GetLiteralFromCP(dumper.ConstantPool, id)
	}
	for _, entry := range codeAttr.ExceptionTable {
		parser.ExceptionTable = append(parser.ExceptionTable, &core.ExceptionTableEntry{
			StartPc:   entry.StartPc,
			EndPc:     entry.EndPc,
			HandlerPc: entry.HandlerPc,
			CatchType: entry.CatchType,
		})
	}

	parser.ConstantPoolInvokeDynamicInfo = func(index int) (string, string) {
		indexFromPool := func(i int) ConstantInfo {
			return pool[i-1]
		}
		constant := pool[index-1]
		switch ret := constant.(type) {
		case *ConstantInvokeDynamicInfo:
			nameAndTypeInfo := indexFromPool(int(ret.NameAndTypeIndex)).(*ConstantNameAndTypeInfo)
			name := indexFromPool(int(nameAndTypeInfo.NameIndex)).(*ConstantUtf8Info).Value
			desc := indexFromPool(int(nameAndTypeInfo.DescriptorIndex)).(*ConstantUtf8Info).Value
			return name, desc
		default:
			panic("error")
		}
	}
	return decompiler.ParseBytesCode(parser)
}
