package yakdoc

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/log"

	"github.com/davecgh/go-spew/spew"
	"github.com/olekukonko/tablewriter"
)

type DocumentHelper struct {
	Libs      map[string]*ScriptLib
	Functions map[string]*FuncDecl
	Instances map[string]*LibInstance
}

func (h *DocumentHelper) GetAllLibs() []string {
	var k []string
	if h == nil {
		return k
	}
	for name := range h.Libs {
		k = append(k, name)
	}
	sort.Strings(k)
	return k
}

func (h *DocumentHelper) HelpInfo() string {
	if h == nil {
		return ""
	}

	buffer := bytes.NewBuffer(nil)

	// 内置用户函数
	buffer.WriteString("### 函数定义\n")
	var items []string
	for _, i := range h.Functions {
		items = append(items, i.String())
	}
	sort.Strings(items)
	for _, i := range items {
		buffer.WriteString(fmt.Sprintf("    %v\n", i))
	}

	buffer.WriteString(fmt.Sprintf("\n%v\n", strings.Repeat("-", 48)))

	t := tablewriter.NewWriter(buffer)
	t.SetHeader([]string{"内置值", "值的类型", "值"})
	for name, item := range h.Instances {
		if item.ValueStr == "" {
			t.Append([]string{name, item.Type, "-"})
		} else {
			t.Append([]string{name, item.Type, item.ValueStr})
		}
	}
	t.Render()

	buffer.WriteString(fmt.Sprintf("\n%v\n", strings.Repeat("-", 48)))

	t = tablewriter.NewWriter(buffer)
	t.SetHeader([]string{"可用依赖库", "依赖库可用元素(值/函数)"})
	for libName, libs := range h.Libs {
		t.Append([]string{libName, fmt.Sprint(len(libs.ElementDocs))})
	}
	t.Render()

	return buffer.String()
}

func (h *DocumentHelper) ShowHelpInfo() {
	if h == nil {
		return
	}
	fmt.Println(h.HelpInfo())
}

func (h *DocumentHelper) LibHelpInfo(name string) string {
	if h == nil {
		return ""
	}

	lib, ok := h.Libs[name]
	if !ok {
		return ""
	}
	return lib.String()
}

func (h *DocumentHelper) ShowLibHelpInfo(name string) {
	if h == nil {
		return
	}

	fmt.Println(h.LibHelpInfo(name))
}

func (h *DocumentHelper) LibFuncDefinitionStr(libName, funcName string) string {
	return h.libFuncToStr(libName, funcName, "def")
}

func (h *DocumentHelper) LibFuncHelpInfo(libName, funcName string) string {
	return h.libFuncToStr(libName, funcName, "help")
}

func (h *DocumentHelper) LibFuncAutoCompletion(libName, funcName string) string {
	return h.libFuncToStr(libName, funcName, "completion")
}

func (h *DocumentHelper) libFuncToStr(libName, funcName string, strType string) string {
	if h == nil {
		return ""
	}

	var (
		lib *ScriptLib
		f   *FuncDecl
		ok  bool
	)
	if libName == "__GLOBAL__" || libName == "__global__" {
		f, ok = h.Functions[funcName]
		if !ok {
			return ""
		}
	} else {
		lib, ok = h.Libs[libName]
		if !ok {
			return ""
		}

		f, ok = lib.Functions[funcName]
		if !ok {
			return ""
		}
	}

	switch strType {
	case "help":
		return fmt.Sprintf("函 数 名: %s.%s\n函数声明: %s\n函数文档: %s", libName, funcName, f.Decl, f.Document)
	case "completion":
		return f.VSCodeSnippets
	default:
		if f.Document == "" {
			return f.Decl
		}
		return fmt.Sprintf("%v  doc:%v", f.Decl, f.Document)
	}

}

func (h *DocumentHelper) ShowLibFuncHelpInfo(libName, funcName string) {
	if h == nil {
		return
	}

	fmt.Println(h.LibFuncHelpInfo(libName, funcName))
}

type LibInstance struct {
	LibName      string
	InstanceName string
	Type         string
	ValueStr     string
}

func (i *LibInstance) String() string {
	if i == nil {
		return ""
	}
	return fmt.Sprintf("%v.%v: %v = %v", i.LibName, i.InstanceName, i.Type, i.ValueStr)
}

type ScriptLib struct {
	Name          string
	LibsInstances []*LibInstance
	Functions     map[string]*FuncDecl
	ElementDocs   []string
}

func (l *ScriptLib) String() string {
	buff := bytes.NewBuffer(nil)
	lenOfElements := len(l.ElementDocs)
	buff.WriteString(fmt.Sprintf("### Palm ExtLib: [%v] - %v elements\n\n", l.Name, lenOfElements))

	for _, i := range l.ElementDocs {
		buff.WriteString(fmt.Sprintf("    %v\n", i))
	}

	return buff.String()
}

type Field struct {
	Name string
	Type string
}

type FuncDecl struct {
	LibName    string
	MethodName string
	Document   string
	Decl       string

	Params  []*Field
	Results []*Field

	VSCodeSnippets string
}

func (f *FuncDecl) String() string {
	decl, doc := f.Decl, f.Document
	if doc != "" {
		doc = fmt.Sprintf(`: "%s"`, doc)
	}

	decl = fmt.Sprintf("%s.%s", f.LibName, decl)

	return fmt.Sprintf("%s%s", decl, doc)
}

func FuncToFuncDecl(libName, methodName string, f interface{}) *FuncDecl {
	funcDecl, err := funcDescriptionAndDeclaration(f, libName, methodName)
	if err != nil {
		log.Warnf("funcToFuncDecl error: %v", err)
		return &FuncDecl{}
	}
	if funcDecl == nil {
		return &FuncDecl{}
	}

	return funcDecl
}

func AnyTypeToLibInstance(libName, name string, typ reflect.Type, value interface{}) *LibInstance {
	return &LibInstance{
		LibName:      libName,
		InstanceName: name,
		Type:         typ.String(),
		ValueStr:     spew.Sdump(value),
	}
}
