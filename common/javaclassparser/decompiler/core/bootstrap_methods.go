package core

import (
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
)

type BuildinBootstrapMethod func(d *Decompiler, sim StackSimulation, typ types.JavaType, args ...values.JavaValue) (values.JavaValue, error)

var buildinBootstrapMethods = map[string]func(args ...values.JavaValue) BuildinBootstrapMethod{
	"java.lang.invoke.StringConcatFactory.makeConcatWithConstants": func(args1 ...values.JavaValue) BuildinBootstrapMethod {
		return func(d *Decompiler, sim StackSimulation, typ types.JavaType, args2 ...values.JavaValue) (values.JavaValue, error) {
			return values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
				str1 := args1[0].String(funcCtx)
				str2 := args2[0].String(funcCtx)
				if len(str1) > 2 && str1[0] == '"' && str1[len(str1)-1] == '"' {
					str1 = string(append([]byte(str1[:len(str1)-2]), '"'))
				}
				return fmt.Sprintf("%s + %s", str1, str2)
			}, func() types.JavaType {
				return typ
			}), nil
		}
	},
	"java.lang.invoke.LambdaMetafactory.metafactory": func(args1 ...values.JavaValue) BuildinBootstrapMethod {
		return func(d *Decompiler, sim StackSimulation, typ types.JavaType, args2 ...values.JavaValue) (values.JavaValue, error) {
			classMember := args1[1].(*values.JavaClassMember)
			if classMember.Name != d.FunctionContext.ClassName {
				return nil, fmt.Errorf("call external lamada: %s.%s", classMember.Name, classMember.Member)
			}
			methodStr, err := d.DumpClassLambdaMethod(classMember.Member, classMember.Description, sim.GetVarId().Int())
			if err != nil {
				return nil, fmt.Errorf("dump lambda method `%s.%s` error: %w", classMember.Name, classMember.Member, err)
			}
			return values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
				return methodStr
			}, func() types.JavaType {
				return typ
			}), nil
		}
	},
	"defaultBootstrapMethod": func(args ...values.JavaValue) BuildinBootstrapMethod {
		return func(d *Decompiler, sim StackSimulation, typ types.JavaType, args ...values.JavaValue) (values.JavaValue, error) {
			return values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
				return "BootstrapMethod()"
			}, func() types.JavaType {
				return typ
			}), nil
		}
	},
}
