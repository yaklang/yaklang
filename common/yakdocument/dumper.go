package yakdocument

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
)

func DumpReturnTypes(types []reflect.Type) string {
	var strs []string
	for _, t := range types {
		strs = append(strs, DumpReflectType(t))
	}
	return strings.Join(strs, ", ")
}

func DumpTypes(types []reflect.Type, isVariadic bool) string {
	var strs []string
	for i, t := range types {
		if i+1 == len(types) && isVariadic {
			tStr := DumpReflectType(t.Elem())
			strs = append(strs, fmt.Sprintf("v%v ...%v", i+1, tStr))
		} else {
			strs = append(strs, fmt.Sprintf("v%v: %v", i+1, DumpReflectType(t)))
		}
	}
	return strings.Join(strs, ", ")
}

func DumpReflectType(t reflect.Type, verbose ...string) string {
	return DumpReflectTypeEx(t, false, verbose...)
}

func DumpReflectTypeEx(t reflect.Type, method bool, verbose ...string) string {
	switch t.Kind() {
	case reflect.Bool:
		fallthrough
	case reflect.Int:
		fallthrough
	case reflect.Int8:
		fallthrough
	case reflect.Int16:
		fallthrough
	case reflect.Int32:
		fallthrough
	case reflect.Int64:
		fallthrough
	case reflect.Uint:
		fallthrough
	case reflect.Uint8:
		fallthrough
	case reflect.Uint16:
		fallthrough
	case reflect.Uint32:
		fallthrough
	case reflect.Uint64:
		fallthrough
	case reflect.Uintptr:
		fallthrough
	case reflect.Float32:
		fallthrough
	case reflect.Float64:
		fallthrough
	case reflect.Complex64:
		fallthrough
	case reflect.Complex128:
		fallthrough
	case reflect.String:
		return fmt.Sprintf("%v", t)
	case reflect.Array:
		return fmt.Sprintf("%v", t)
	case reflect.Chan:
		return fmt.Sprintf("%v", t)
	case reflect.Func:
		ret := t
		var inParams []reflect.Type
		var isVariadic = ret.IsVariadic()
		for i := range make([]int, ret.NumIn()) {
			p := ret.In(i)
			inParams = append(inParams, p)
		}
		if len(inParams) > 0 && method {
			inParams = inParams[1:]
		}
		params := DumpTypes(inParams, isVariadic)

		var returns []reflect.Type
		for i := range make([]int, ret.NumOut()) {
			p := ret.Out(i)
			returns = append(returns, p)
		}
		returnStr := DumpReturnTypes(returns)
		if returnStr != "" {
			returnStr = fmt.Sprintf(" return(%v)", returnStr)
		}
		var name = t.Name()
		if name == "" {
			name = strings.Join(verbose, ".")
		}
		return fmt.Sprintf("func %v(%v)%v ", name, params, returnStr)
	case reflect.Interface:
		return fmt.Sprintf("%v", t)
	case reflect.Map:
		return fmt.Sprintf("%v", t)
	case reflect.Ptr:
		return fmt.Sprintf("%v", t)
	case reflect.Slice:
		return fmt.Sprintf("%v", t)
	case reflect.Struct:
		return fmt.Sprintf("%v", t)
	case reflect.UnsafePointer:
		return fmt.Sprintf("%v", t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func DescPtrStruct(ret reflect.Type) (_ *StructHelper, fErr error) {
	defer func() {
		if err := recover(); err != nil {
			fErr = utils.Errorf("failed: %v", err)
			return
		}
	}()

	switch ret.Kind() {
	case reflect.Ptr:
		stct := ret.Elem()
		sh, err := DescStruct(stct)
		if err != nil {
			return nil, utils.Errorf("not a struct ERR: %v", err)
		}
		max := ret.NumMethod()
		for i := 0; i < max; i++ {
			sh.PtrMethods = append(sh.PtrMethods, ret.Method(i))
		}
		return sh, nil
	default:
		if fErr == nil {
			return nil, utils.Errorf("cannot fetch struct helper for %v", spew.Sdump(ret))
		}
		return nil, fErr
	}
}

func DescStruct(ret reflect.Type) (*StructHelper, error) {
	switch ret.Kind() {
	case reflect.Struct:
		sh := &StructHelper{
			PkgPath: ret.PkgPath(),
			Name:    ret.Name(),
		}
		max := ret.NumField()
		for i := 0; i < max; i++ {
			field := ret.Field(i)
			if len(field.Name) > 0 {
				firstAlphabet := field.Name[0]
				if strings.Contains("abcdefghijklmnopqrstuvwxyz_", string(firstAlphabet)) {
					continue
				}
			}
			sh.Fields = append(sh.Fields, field)
		}

		max = ret.NumMethod()
		for i := 0; i < max; i++ {
			method := ret.Method(i)
			sh.Methods = append(sh.Methods, method)
		}
		return sh, nil
	default:
		return nil, utils.Errorf("not a struct: %v", ret.Kind())
	}
}

func Dir(i interface{}) (*StructHelper, error) {
	switch ret := i.(type) {
	case reflect.Type:
		switch ret.Kind() {
		case reflect.Ptr:
			return DescPtrStruct(ret)
		case reflect.Struct:
			return DescStruct(ret)
		case reflect.Chan:
			return Dir(ret.Elem())
		case reflect.Slice:
			return Dir(ret.Elem())
		case reflect.Map:
			return Dir(ret.Elem())
		case reflect.Array:
			return Dir(ret.Elem())
		default:
			return nil, nil
		}
	default:
		return Dir(reflect.TypeOf(i))
	}
}

func MethodToDoc(m reflect.Method, ptr bool, structName string) *MethodDoc {
	return convertMethodToDoc(m, ptr, structName, new(sync.Map))
}

// MethodToDocWithCache converts reflect.Method to MethodDoc with shared cache
func MethodToDocWithCache(m reflect.Method, ptr bool, structName string, cache *sync.Map) *MethodDoc {
	if cache == nil {
		cache = new(sync.Map)
	}
	return convertMethodToDoc(m, ptr, structName, cache)
}

func convertMethodToDoc(m reflect.Method, ptr bool, structName string, cache *sync.Map) *MethodDoc {
	isMethod := structName != "" && structName != "."
	doc, err := convertFuncToDoc(m.Func.Type(), ptr, cache, isMethod)
	if err != nil {
		return &MethodDoc{
			Ptr:         ptr,
			Description: fmt.Sprintf("`%v` 方法：", m.Name),
			StructName:  structName,
			Name:        m.Name,
			Params:      nil,
			Returns:     nil,
		}
	}
	doc.StructName = structName
	doc.Name = m.Name

	if structName != "" && structName != "." {
		if len(doc.Params) > 0 {
			doc.Params = doc.Params[1:]
		}
	}
	return doc
}

func StructHelperToDoc(h *StructHelper) []*StructDoc {
	var cached = new(sync.Map)
	return convertStructHelperToDoc(h, cached)
}

// StructHelperToDocWithCache converts StructHelper to StructDoc with shared cache to avoid repeated parsing
func StructHelperToDocWithCache(h *StructHelper, cache *sync.Map) []*StructDoc {
	if cache == nil {
		cache = new(sync.Map)
	}
	return convertStructHelperToDoc(h, cache)
}

func convertStructHelperToDoc(h *StructHelper, cache *sync.Map) []*StructDoc {
	var docs []*StructDoc

	d := &StructDoc{
		StructName:   StructName("", h.PkgPath, h.Name),
		Description:  "",
		Fields:       nil,
		MethodsDoc:   nil,
		PtrMethodDoc: nil,
	}
	d.IsBuildInStruct = d.IsBuildInLib()
	_, ok := cache.Load(d.StructName)
	if ok {
		return nil
	}
	cache.Store(d.StructName, d)

	docs = append(docs, d)

	for _, f := range h.Fields {
		var rStructName string
		s, _ := Dir(f.Type)
		if s != nil {
			rStructName = StructName("", s.PkgPath, s.Name)
		}

		d.Fields = append(d.Fields, &FieldDoc{
			StructName:         d.StructName,
			Name:               f.Name,
			Description:        "",
			RelativeStructName: rStructName,
			TypeStr:            DumpReflectType(f.Type),
		})
	}

	for _, f := range h.Methods {
		d.MethodsDoc = append(d.MethodsDoc, convertMethodToDoc(f, false, d.StructName, cache))
	}

	for _, f := range h.PtrMethods {
		d.MethodsDoc = append(d.MethodsDoc, convertMethodToDoc(f, true, d.StructName, cache))
	}

	sort.SliceStable(d.Fields, func(i, j int) bool {
		return d.Fields[i].Name < d.Fields[j].Name
	})
	sort.SliceStable(d.MethodsDoc, func(i, j int) bool {
		return d.MethodsDoc[i].Name < d.MethodsDoc[j].Name
	})
	sort.SliceStable(d.PtrMethodDoc, func(i, j int) bool {
		return d.PtrMethodDoc[i].Name < d.PtrMethodDoc[j].Name
	})

	for _, f := range h.Fields {
		fieldDoc, _ := Dir(f.Type)
		if fieldDoc != nil {
			docs = append(docs, convertStructHelperToDoc(fieldDoc, cache)...)
		}
	}

	for _, m := range d.MethodsDoc {
		docs = append(docs, m.RelativeStructName...)
	}

	for _, m := range d.PtrMethodDoc {
		docs = append(docs, m.RelativeStructName...)
	}

	return docs
}
