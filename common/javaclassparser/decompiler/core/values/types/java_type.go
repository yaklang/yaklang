package types

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"
)

type JavaFuncType struct {
	Desc       string
	ParamTypes []JavaType
	ReturnType JavaType
}

func (j JavaFuncType) String(funcCtx *class_context.ClassContext) string {
	return j.Desc
}

func (j JavaFuncType) IsJavaType() {

}

func NewJavaFuncType(desc string, params []JavaType, returnType JavaType) *JavaFuncType {
	return &JavaFuncType{
		ParamTypes: params,
		ReturnType: returnType,
	}
}

type JavaArrayType struct {
	JavaType  JavaType
	Dimension int
}

func (j *JavaArrayType) String(funcCtx *class_context.ClassContext) string {
	return fmt.Sprintf("%s%s", j.JavaType.String(funcCtx), strings.Repeat("[]", j.Dimension))
}

func (j *JavaArrayType) IsJavaType() {

}

func NewJavaArrayType(typ JavaType) JavaType {
	if typ.IsArray() {
		arrayType, ok := typ.RawType().(*JavaArrayType)
		if ok {
			return newJavaTypeWrap(&JavaArrayType{
				JavaType:  arrayType.JavaType,
				Dimension: arrayType.Dimension + 1,
			})
		}
		return newJavaTypeWrap(&JavaArrayType{
			JavaType:  typ.ElementType(),
			Dimension: typ.ArrayDim() + 1,
		})
	}
	return newJavaTypeWrap(&JavaArrayType{
		JavaType:  typ,
		Dimension: 1,
	})
}

type JavaClass struct {
	Name string
	JavaType
}

// ReplaceVar implements values.JavaValue.
func (j *JavaClass) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
}

func (j *JavaClass) IsJavaType() {

}
func (j *JavaClass) Type() JavaType {
	return newJavaTypeWrap(j)
}

func (j *JavaClass) String(funcCtx *class_context.ClassContext) string {
	name := funcCtx.ShortTypeName(j.Name)
	return fmt.Sprintf("%s", name)
}
func NewJavaClass(typeName string) JavaType {
	if strings.HasPrefix(typeName, "[") {
		t, err := ParseDescriptor(typeName)
		if err != nil {
			// Fallback: treat as a plain class name if descriptor parsing fails.
			return newJavaTypeWrap(&JavaClass{Name: typeName})
		}
		return t
	}
	return newJavaTypeWrap(&JavaClass{
		Name: typeName,
	})
}

// JavaMultiCatchType models the static type of a multi-catch exception variable
// (`catch (A | B | C) e`). It renders as the alternatives joined by " | ", each through the
// normal short-name resolution, so the catch clause is reconstructed faithfully.
type JavaMultiCatchType struct {
	Types []JavaType
	JavaType
}

func (j *JavaMultiCatchType) IsJavaType() {}

func (j *JavaMultiCatchType) String(funcCtx *class_context.ClassContext) string {
	parts := make([]string, 0, len(j.Types))
	for _, t := range j.Types {
		parts = append(parts, t.String(funcCtx))
	}
	return strings.Join(parts, " | ")
}

// NewMultiCatchType builds a multi-catch union type. With fewer than two alternatives it returns
// the single type (or Throwable when empty) so callers never need to special-case the degenerate
// forms.
func NewMultiCatchType(alternatives []JavaType) JavaType {
	switch len(alternatives) {
	case 0:
		return NewJavaClass("Throwable")
	case 1:
		return alternatives[0]
	default:
		return newJavaTypeWrap(&JavaMultiCatchType{Types: alternatives})
	}
}
