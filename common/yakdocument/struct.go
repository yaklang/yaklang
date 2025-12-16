package yakdocument

import (
	"bytes"
	"fmt"
	"reflect"
	"sync"

	"github.com/dave/jennifer/jen"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type StructHelper struct {
	PkgPath    string
	Name       string
	Fields     []reflect.StructField
	Methods    []reflect.Method
	PtrMethods []reflect.Method
}

func (s *StructHelper) Show() {
	fmt.Println(s.String())
}

func (s *StructHelper) String() string {
	var buf bytes.Buffer
	bufHandler := &buf
	_, _ = fmt.Fprintf(bufHandler, "type %v.(%v) struct {\n", s.PkgPath, s.Name)
	_, _ = fmt.Fprintf(bufHandler, "  Fields(可用字段): \n")
	prefix := "      "
	for _, f := range s.Fields {
		suffix := ""
		if f.Tag.Get("ydoc") != "" {
			suffix = " (DOC: %v)"
		}
		_, _ = fmt.Fprintf(bufHandler, "%v%v: %v  %v\n", prefix, f.Name, DumpReflectType(f.Type), suffix)
	}

	_, _ = fmt.Fprintf(bufHandler, "  StructMethods(结构方法/函数): \n")
	for _, m := range s.Methods {
		suffix := ""
		//if m.Tag.Get("ydoc") != "" {
		//	suffix = " (DOC: %v)"
		//}
		_, _ = fmt.Fprintf(bufHandler, "%v%v%v\n", prefix, DumpReflectTypeEx(m.Func.Type(), true, m.Name), suffix)
	}

	_, _ = fmt.Fprintf(bufHandler, "  PtrStructMethods(指针结构方法/函数): \n")
	for _, m := range s.PtrMethods {
		suffix := ""
		//if m.Tag.Get("ydoc") != "" {
		//	suffix = " (DOC: %v)"
		//}
		_, _ = fmt.Fprintf(bufHandler, "%v%v%v\n", prefix, DumpReflectTypeEx(m.Func.Type(), true, m.Name), suffix)
	}

	_, _ = fmt.Fprintf(bufHandler, "}")
	return buf.String()
}

func (s *StructHelper) ShowAddDocHelper() {
	var mStr []string
	for _, m := range s.Methods {
		mStr = append(
			mStr,
			fmt.Sprintf(`{Name: "%v", Params: []FieldDoc{}, Returns: []FieldDoc{}}`, m.Name),
		)
	}

	var fCodes []jen.Code
	for _, f := range s.Fields {
		fCodes = append(fCodes, jen.Block(jen.Dict{
			jen.Id("Name"):    jen.Lit(f.Name),
			jen.Id("Content"): jen.Lit(" "),
			jen.Id("TypeStr"): jen.Lit(DumpReflectType(f.Type)),
		}))
	}

	var mCodes []jen.Code
	for _, m := range s.Methods {
		f, err := FuncToDoc(m.Func.Type(), false, true)
		if err != nil {
			log.Error("gen methods failed: ", err)
			continue
		}
		f.Name = m.Name
		_ = f
		mCodes = append(mCodes, jen.Qual("palm/common/yakdocument", "MethodDoc").Values())
	}

	for _, m := range s.PtrMethods {
		f, err := FuncToDoc(m.Func.Type(), false, true)
		if err != nil {
			log.Error("gen methods failed: ", err)
			continue
		}
		f.Name = m.Name
		_ = f

		var fCodes []jen.Code
		for _, f := range f.Params {
			fCodes = append(fCodes, jen.Block(jen.Dict{
				jen.Id("Name"):    jen.Lit(f.Name),
				jen.Id("Content"): jen.Lit(" "),
				jen.Id("TypeStr"): jen.Lit(f.TypeStr),
			}))
		}

		var rCodes []jen.Code
		for _, f := range f.Returns {
			rCodes = append(rCodes, jen.Block(jen.Dict{
				jen.Id("Name"):    jen.Lit(f.Name),
				jen.Id("Content"): jen.Lit(" "),
				jen.Id("TypeStr"): jen.Lit(f.TypeStr),
			}))
		}

		mCodes = append(mCodes, jen.Qual("palm/common/yakdocument", "MethodDoc").Values(
			jen.Dict{
				jen.Id("Ptr"):     jen.Lit(true),
				jen.Id("Name"):    jen.Lit(m.Name),
				jen.Id("Params"):  jen.Index().Qual("palm/common/yakdocument", "FieldDoc").Values(fCodes...),
				jen.Id("Returns"): jen.Index().Qual("palm/common/yakdocument", "FieldDoc").Values(rCodes...),
			},
		))
	}

	code := jen.Qual("palm/common/yakdocument", "AddDoc").Call(
		jen.Lit(s.PkgPath), jen.Lit(s.Name),
		jen.Index().Qual("palm/common/yakdocument", "FieldDoc").Values(jen.List(
			fCodes...,
		)),
		jen.List(mCodes...),
	)

	f := jen.NewFile("yakdoc")
	f.ImportNames(map[string]string{
		"palm/common/yakdocument": "yakdocument",
	})
	f.Func().Id("init").Parens(jen.List()).Block(code)
	println(f.GoString())
}

func FuncToDoc(ret reflect.Type, ptr bool, isMethod bool) (*MethodDoc, error) {
	return convertFuncToDoc(ret, ptr, new(sync.Map), isMethod)
}

// FuncToDocWithCache converts reflect.Type to MethodDoc with shared cache
func FuncToDocWithCache(ret reflect.Type, ptr bool, isMethod bool, cache *sync.Map) (*MethodDoc, error) {
	if cache == nil {
		cache = new(sync.Map)
	}
	return convertFuncToDoc(ret, ptr, cache, isMethod)
}

func convertFuncToDoc(ret reflect.Type, ptr bool, cache *sync.Map, isMethod bool) (*MethodDoc, error) {
	switch ret.Kind() {
	case reflect.Func:
		var params []*FieldDoc
		var returnTypes []*FieldDoc
		var structs []*StructDoc

		var inParams []reflect.Type
		var isVariadic = ret.IsVariadic()
		for i := range make([]int, ret.NumIn()) {
			p := ret.In(i)
			inParams = append(inParams, p)
		}

		for _index, i := range inParams {
			field := &FieldDoc{
				Name:        fmt.Sprintf("v%v", _index+1),
				Description: "",
				TypeStr:     DumpReflectType(i),
				IsVariadic:  _index+1 == len(inParams) && isVariadic,
			}
			sHelper, err := Dir(i)
			if err != nil {
				//log.Debugf("inspect type:[%v] failed: %v", i.Kind(), err)
			}
			if sHelper != nil {
				if !(_index == 0 && isMethod) {
					res := convertStructHelperToDoc(sHelper, cache)
					if len(res) == 1 {
						field.RelativeStructName = res[0].StructName
					}
					structs = append(structs, res...)
				}
			}
			params = append(params, field)

		}

		var returns []reflect.Type
		for i := range make([]int, ret.NumOut()) {
			p := ret.Out(i)
			returns = append(returns, p)
		}

		for _index, i := range returns {
			field := &FieldDoc{
				Name:        fmt.Sprintf("r%v", _index),
				Description: "",
				TypeStr:     DumpReflectType(i),
			}
			sHelper, err := Dir(i)
			if err != nil {
				//log.Errorf("inspect type:[%v] failed: %v", i.Kind(), err)
			}
			if sHelper != nil {
				res := convertStructHelperToDoc(sHelper, cache)
				if len(res) == 1 {
					field.RelativeStructName = res[0].StructName
				}
				structs = append(structs, res...)
			}
			returnTypes = append(returnTypes, field)
		}

		return &MethodDoc{
			Ptr:                ptr,
			Name:               ret.Name(),
			Params:             params,
			Returns:            returnTypes,
			RelativeStructName: structs,
		}, nil
	}
	return nil, utils.Errorf("not a func")
}
