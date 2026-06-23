package core

import (
	"fmt"
	"strings"

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

				for i := 0; i < len(args2); i++ {
					idx := len(args2) - 1 - i
					if idx < 0 || idx >= len(args2) {
						break
					}
					arg := args2[idx]
					newStr := arg.String(funcCtx)
					tag := `\u0001`
					str1 = strings.Replace(str1, tag, `" + `+newStr+` + "`, 1)
				}

				if strings.HasSuffix(str1, ` + ""`) {
					str1 = strings.TrimSuffix(str1, ` + ""`)
				}
				return str1

				//str2 := args2[0].String(funcCtx)
				//if len(str1) > 2 && str1[0] == '"' && str1[len(str1)-1] == '"' && strings.HasSuffix(str1, `\u0000`) {
				//	var ok bool
				//	str1, ok = strings.CutSuffix(str1, `\u0000"`)
				//	if ok {
				//		str1 = str1 + `"`
				//	}
				//}
				//return fmt.Sprintf("%s + %s", str1, str2)
			}, func() types.JavaType {
				return typ
			}), nil
		}
	},
	"java.lang.invoke.LambdaMetafactory.metafactory": func(args1 ...values.JavaValue) BuildinBootstrapMethod {
		return func(d *Decompiler, sim StackSimulation, typ types.JavaType, args2 ...values.JavaValue) (values.JavaValue, error) {
			// args1 are the bootstrap static arguments:
			//   args1[0] = samMethodType, args1[1] = implMethod(MethodHandle), args1[2] = instantiatedMethodType
			// args2 are the dynamic captured arguments.
			if len(args1) < 2 {
				return nil, fmt.Errorf("lambda metafactory requires at least 2 bootstrap args, got %d", len(args1))
			}
			classMember, ok := args1[1].(*values.JavaClassMember)
			if !ok {
				return nil, fmt.Errorf("lambda metafactory: unexpected impl method handle type %T", args1[1])
			}
			member := classMember.Member
			implClassName := strings.ReplaceAll(classMember.Name, "/", ".")
			currentClassName := strings.ReplaceAll(d.FunctionContext.ClassName, "/", ".")
			// Synthetic lambda bodies are emitted by javac as private methods named "lambda$...".
			// Only those should be inlined as lambda expressions; everything else is a method reference.
			isSyntheticLambda := strings.HasPrefix(member, "lambda$")
			if isSyntheticLambda && implClassName == currentClassName {
				methodStr, err := d.DumpClassLambdaMethod(member, classMember.Description, sim.GetVarId())
				if err != nil {
					return nil, fmt.Errorf("dump lambda method `%s.%s` error: %w", classMember.Name, member, err)
				}
				return values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
					return methodStr
				}, func() types.JavaType {
					return typ
				}), nil
			}

			// Method reference: constructor / static / (bound|unbound) instance method.
			capturedArgs := append([]values.JavaValue{}, args2...)
			return values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
				refMember := member
				if member == "<init>" {
					// constructor method reference: ClassName::new
					refMember = "new"
				} else if len(capturedArgs) > 0 {
					// bound instance method reference: receiver::method
					return capturedArgs[0].String(funcCtx) + "::" + refMember
				}
				return funcCtx.ShortTypeName(implClassName) + "::" + refMember
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

func init() {
	buildinBootstrapMethods["java.lang.invoke.LambdaMetafactory.altMetafactory"] = buildinBootstrapMethods["java.lang.invoke.LambdaMetafactory.metafactory"]
}
