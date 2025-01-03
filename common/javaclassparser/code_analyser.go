package javaclassparser

import (
	"fmt"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"
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
		val := values.NewJavaClassMember(className, name, desc, typ)
		return val
	}
	switch ret := constant.(type) {
	case *ConstantMethodHandleInfo:
		return GetValueFromCP(pool, int(ret.ReferenceIndex))
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
		classIns := values.NewJavaClassMember(typeName, refNameInfo.Value, descInfo.Value, typ)
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
		classIns := values.NewJavaClassMember(typeName, refNameInfo.Value, descInfo.Value, typ)
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
	case *ConstantMethodTypeInfo:
		descInfo := indexFromPool(int(ret.DescriptorIndex)).(*ConstantUtf8Info)
		typ, err := types.ParseMethodDescriptor(descInfo.Value)
		if err != nil {
			log.Errorf("parse descriptor failed:%s", descInfo.Value)
		}
		_ = typ
		return values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
			return "<MethodType Class Instance>"
		}, func() types.JavaType {
			return types.NewJavaClass("java.lang.invoke.MethodType")
		})
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
		return GetValueFromCP(pool, index)
	}
}

type VarMap struct {
	id  int
	val values.JavaValue
}

func ParseBytesCode(dumper *ClassObjectDumper, codeAttr *CodeAttribute, id *utils.VariableId) ([]values.JavaValue, []statements.Statement, error) {
	pool := dumper.ConstantPool
	parser := core.NewDecompiler(codeAttr.Code, func(id int) values.JavaValue {
		return GetValueFromCP(dumper.ConstantPool, id)
	})
	parser.DumpClassLambdaMethod = func(name, desc string, id *utils.VariableId) (string, error) {
		dumper.lambdaMethods[name] = append(dumper.lambdaMethods[name], desc)
		dumped, err := dumper.DumpMethodWithInitialId(name, desc, id)
		if err != nil {
			return "", err
		}
		return dumped.code, nil
	}
	parser.BaseVarId = id
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
	attrInterfaces := lo.Filter(dumper.obj.Attributes, func(item AttributeInfo, index int) bool {
		_, ok := item.(*BootstrapMethodsAttribute)
		return ok
	})
	attrs := lo.Map(attrInterfaces, func(item AttributeInfo, index int) *BootstrapMethodsAttribute {
		return item.(*BootstrapMethodsAttribute)
	})
	var bootstrapMethod []*BootstrapMethod
	if len(attrs) > 0 {
		bootstrapMethod = attrs[0].BootstrapMethods
	}
	for _, method := range bootstrapMethod {
		val := GetValueFromCP(pool, int(method.BootstrapMethodRef))
		arguments := make([]values.JavaValue, len(method.BootstrapArguments))
		for i, arg := range method.BootstrapArguments {
			arguments[i] = GetLiteralFromCP(pool, int(arg))
		}
		parser.BootstrapMethods = append(parser.BootstrapMethods, &core.BootstrapMethod{
			Ref:       val,
			Arguments: arguments,
		})
	}

	parser.ConstantPoolInvokeDynamicInfo = func(index int) (uint16, string, string) {
		constant := pool[index-1]
		switch ret := constant.(type) {
		case *ConstantInvokeDynamicInfo:
			name, desc := getNameAndType(dumper.ConstantPool, ret.NameAndTypeIndex)
			return ret.BootstrapMethodAttrIndex, name, desc
		default:
			panic("error")
		}
	}
	st, err := decompiler.ParseBytesCode(parser)
	return parser.Params, st, err
}
