package types

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
)

type JavaType interface {
	javaType
	ResetType(t JavaType)
	ResetTypeRef(t JavaType)
	IsArray() bool
	ElementType() JavaType
	ArrayDim() int
	FunctionType() *JavaFuncType
	RawType() javaType
	Copy() JavaType
	GetJavaTypeRef() *javaTypeRef
}
type javaTypeRef struct {
	javaType
}
type JavaTypeWrap struct {
	*javaTypeRef
}

func (j *JavaTypeWrap) Copy() JavaType {
	return &JavaTypeWrap{
		javaTypeRef: &javaTypeRef{j.javaType},
	}
}
func (j *JavaTypeWrap) ArrayDim() int {
	v, ok := j.javaType.(*JavaArrayType)
	if ok {
		return v.Dimension
	}
	return 0
}
func (j *JavaTypeWrap) RawType() javaType {
	return j.javaType
}
func (j *JavaTypeWrap) FunctionType() *JavaFuncType {
	v, ok := j.javaType.(*JavaFuncType)
	if ok {
		return v
	}
	return nil
}
func (j *JavaTypeWrap) IsArray() bool {
	_, ok := j.javaType.(*JavaArrayType)
	return ok
}
func (j *JavaTypeWrap) ElementType() JavaType {
	v, ok := j.javaType.(*JavaArrayType)
	if ok {
		if v.Dimension == 1 {
			return v.JavaType
		} else {
			return newJavaTypeWrap(&JavaArrayType{
				JavaType:  v.JavaType,
				Dimension: v.Dimension - 1,
			})
		}
	}
	return nil
}
func (j *JavaTypeWrap) GetJavaTypeRef() *javaTypeRef {
	return j.javaTypeRef
}
func (j *JavaTypeWrap) ResetTypeRef(t JavaType) {
	j.javaTypeRef = t.GetJavaTypeRef()
}
func (j *JavaTypeWrap) ResetType(t JavaType) {
	if j.String(&class_context.ClassContext{}) == JavaInteger && t.String(&class_context.ClassContext{}) == JavaBoolean {
		j.javaType = t.RawType()
	}
	if j.String(&class_context.ClassContext{}) == JavaVoid {
		j.javaType = t.RawType()
	}
}
func newJavaTypeWrap(t javaType) *JavaTypeWrap {
	return &JavaTypeWrap{
		javaTypeRef: &javaTypeRef{t},
	}
}

type javaType interface {
	String(funcCtx *class_context.ClassContext) string
	IsJavaType()
}

func MergeTypes(types ...JavaType) JavaType {
	typesMap := map[string][]JavaType{}
	for _, j := range types {
		typesMap[j.String(&class_context.ClassContext{})] = append(typesMap[j.String(&class_context.ClassContext{})], j)
	}
	if len(typesMap) == 1 {
		for _, javaTypes := range typesMap {
			if len(javaTypes) > 0 {
				baseRef := javaTypes[0]
				for _, j := range javaTypes[1:] {
					j.ResetTypeRef(baseRef)
				}
			}
		}
	}
	if v, ok := typesMap[JavaBoolean]; ok {
		baseRef := v[0]
		for _, j := range v[1:] {
			j.ResetTypeRef(baseRef)
		}
		for key, javaTypes := range typesMap {
			if key == JavaBoolean {
				continue
			}
			if key == JavaInteger || key == JavaVoid {
				for _, j := range javaTypes {
					j.ResetTypeRef(baseRef)
				}
			} else {
				panic("unsupported type")
			}
		}
	}
	if len(types) > 0 {
		return types[0]
	}
	return nil
}

var _ javaType = &JavaClass{}
var _ javaType = &JavaPrimer{}
var _ javaType = &JavaArrayType{}
var _ javaType = &JavaFuncType{}
