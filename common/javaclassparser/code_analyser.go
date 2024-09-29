package javaclassparser

import (
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
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

func GetValueFromCP(pool []ConstantInfo, index int) core.JavaValue {
	indexFromPool := func(i int) ConstantInfo {
		return pool[i-1]
	}
	constant := pool[index-1]
	getClassName := func(index uint16) string {
		classInfo := indexFromPool(int(index)).(*ConstantClassInfo)
		nameInfo := indexFromPool(int(classInfo.NameIndex)).(*ConstantUtf8Info)
		return nameInfo.Value
	}
	convertMemberInfo := func(classMemberInfo *ConstantMemberrefInfo) core.JavaValue {
		className := getClassName(classMemberInfo.ClassIndex)
		name, desc := getNameAndType(pool, classMemberInfo.NameAndTypeIndex)
		return core.NewJavaClassMember(className, name, desc)
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
		classIns := core.NewJavaClassMember(typeName, refNameInfo.Value, descInfo.Value)
		return classIns
	case *ConstantMethodrefInfo:
		classInfo := indexFromPool(int(ret.ClassIndex)).(*ConstantClassInfo)
		nameInfo := indexFromPool(int(classInfo.NameIndex)).(*ConstantUtf8Info)

		nameAndType := indexFromPool(int(ret.NameAndTypeIndex)).(*ConstantNameAndTypeInfo)
		refNameInfo := indexFromPool(int(nameAndType.NameIndex)).(*ConstantUtf8Info)
		descInfo := indexFromPool(int(nameAndType.DescriptorIndex)).(*ConstantUtf8Info)
		typeName := nameInfo.Value
		typeName = strings.Replace(typeName, "/", ".", -1)
		classIns := core.NewJavaClassMember(typeName, refNameInfo.Value, descInfo.Value)
		return classIns
	case *ConstantClassInfo:
		nameInfo := indexFromPool(int(ret.NameIndex)).(*ConstantUtf8Info)
		typeName := nameInfo.Value
		typeName = strings.Replace(typeName, "/", ".", -1)
		return core.NewJavaClass(typeName)
	default:
		panic("failed")
	}
}
func GetLiteralFromCP(pool []ConstantInfo, index int) *core.JavaLiteral {
	constant := pool[index-1]
	switch ret := constant.(type) {
	case *ConstantStringInfo:
		return core.NewJavaLiteral(pool[ret.StringIndex-1].(*ConstantUtf8Info).Value, core.JavaString)
	case *ConstantLongInfo:
		return core.NewJavaLiteral(ret.Value, core.JavaLong)
	case *ConstantIntegerInfo:
		return core.NewJavaLiteral(ret.Value, core.JavaInteger)
	default:
		panic("failed")
	}
}

type VarMap struct {
	id  int
	val core.JavaValue
}

func ParseBytesCode(dumper *ClassObjectDumper, codeAttr *CodeAttribute) ([]core.Statement, error) {
	pool := dumper.ConstantPool
	parser := core.NewDecompiler(codeAttr.Code, func(id int) core.JavaValue {
		return GetValueFromCP(dumper.ConstantPool, id)
	})
	parser.ConstantPoolLiteralGetter = func(id int) *core.JavaLiteral {
		return GetLiteralFromCP(dumper.ConstantPool, id)
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
