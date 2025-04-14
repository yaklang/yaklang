package yso

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/javaclassparser/attribute_info"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
)

func JavaClassModifySuperClass(ins *javaclassparser.ClassObject, className string) error {
	supperClassIndex := ins.ConstantPoolManager.AddNewClassInfo(className)
	index := ins.ConstantPoolManager.AddNewMethodInfo(className, "<init>", "()V")
	ins.SuperClass = uint16(supperClassIndex)
	constructorMethod := lo.Filter(ins.Methods, func(item *javaclassparser.MemberInfo, index int) bool {
		strIns := ins.ConstantPoolManager.GetUtf8(int(item.NameIndex))
		if strIns.Value != "<init>" {
			return false
		}
		return true
	})
	for _, method := range constructorMethod {
		codeAttrs := lo.Filter(method.Attributes, func(item attribute_info.AttributeInfo, index int) bool {
			_, ok := item.(*attribute_info.CodeAttribute)
			return ok
		})
		var codeAttr *attribute_info.CodeAttribute
		if len(codeAttrs) == 0 {
			continue
		}
		codeAttr = codeAttrs[0].(*attribute_info.CodeAttribute)
		decompiler := core.NewDecompiler(codeAttr.Code, nil)
		err := decompiler.ParseOpcode()
		if err != nil {
			return err
		}
		if len(codeAttr.Code) >= 4 {
			if codeAttr.Code[0] == core.OP_ALOAD_0 && codeAttr.Code[1] == core.OP_INVOKESPECIAL {
				codeAttr.Code[2] = byte(index >> 8)
				codeAttr.Code[3] = byte(index)
			}
		}
	}
	return nil
}
