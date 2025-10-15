package yakdoc

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/yaklang/yaklang/common/utils"
)

var (
	defaultPackageName        = "github.com/yaklang/yaklang"
	defaultHooks              = make([]func(h *DocumentHelper), 0)
	projectPath        string = ""
)

func GetProjectPath() string {
	if projectPath == "" {
		_, filename, _, ok := runtime.Caller(0)
		if ok {
			projectPath, _ = filepath.Abs(filepath.Join(filename, "../../../../"))
		}
	}
	return projectPath
}

func GetProjectAstPackages() (map[string]*ast.Package, *token.FileSet, error) {
	rootDir := GetProjectPath()
	fset := token.NewFileSet()
	packages := make(map[string]*ast.Package)

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			pkgs, err := parser.ParseDir(fset, path, nil, parser.ParseComments|parser.AllErrors)
			if err != nil {
				log.Errorf("parser package path error:%v", err) // ignore error
				return nil
			}
			for name, pkg := range pkgs {
				// skip test pkg
				if strings.HasSuffix(name, "_test") {
					continue
				}
				path, _ = filepath.Rel(rootDir, path)
				// only path, remove last part, use pkg name instead
				path, _ = filepath.Split(path)
				path = filepath.Join(path, name)
				path = fmt.Sprintf("%s/%s", defaultPackageName, path)
				path = strings.ReplaceAll(path, string(filepath.Separator), "/")
				packages[path] = pkg
			}
		}

		return nil
	})

	return packages, fset, err
}

type DocumentHelper struct {
	Libs                map[string]*ScriptLib
	Functions           map[string]*FuncDecl
	Instances           map[string]*LibInstance
	StructMethods       map[string]*ScriptLib // 结构体方法，名字 -> 所有结构体与结构体指针方法
	DeprecatedFunctions []*DeprecateFunction
	hooks               []func(h *DocumentHelper)
}

func RegisterHook(hook func(h *DocumentHelper)) {
	defaultHooks = append(defaultHooks, hook)
}

func (h *DocumentHelper) Callback() {
	for _, hook := range defaultHooks {
		hook(h)
	}

	for _, hook := range h.hooks {
		hook(h)
	}
}

func (h *DocumentHelper) InjectInterfaceDocumentManually(interfacePath, sourceCodePath string) error {
	lib, ok := h.StructMethods[interfacePath]
	if !ok {
		return utils.Errorf("interface not found in document helper: %v", interfacePath)
	}
	_ = lib
	if !filepath.IsAbs(sourceCodePath) {
		sourceCodePath, _ = filepath.Abs(filepath.Join(GetProjectPath(), sourceCodePath))
	}

	bundle, err := GetCacheAstBundle(sourceCodePath, "")
	if err != nil {
		return err
	}
	splited := strings.Split(interfacePath, ".")
	interfaceName := splited[len(splited)-1]

	for _, decl := range bundle.parsedFile.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != interfaceName {
				continue
			}
			iface, ok := typeSpec.Type.(*ast.InterfaceType)
			if !ok {
				continue
			}
			for _, field := range iface.Methods.List {
				if field.Doc == nil {
					continue
				}
				methodName := field.Names[0].Name
				f, ok := lib.Functions[methodName]
				if !ok {
					continue
				}
				f.Document = strings.TrimSpace(field.Doc.Text())
			}
		}
	}

	return nil
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
	ValueStr     string `json:"value,omitempty"`
}

func (i *LibInstance) String() string {
	if i == nil {
		return ""
	}
	return fmt.Sprintf("%v.%v: %v = %v", i.LibName, i.InstanceName, i.Type, i.ValueStr)
}

type ScriptLib struct {
	Name        string
	Instances   map[string]*LibInstance
	Functions   map[string]*FuncDecl
	ElementDocs []string
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
	Name    string
	Type    string
	RefType reflect.Type `json:"-"`
}

type FuncDecl struct {
	LibName    string
	MethodName string
	Document   string `json:"document,omitempty"`
	Decl       string

	Params  []*Field
	Results []*Field

	VSCodeSnippets string
}

func (f *FuncDecl) String() string {
	decl, doc := f.Decl, f.Document
	decl = fmt.Sprintf("%s.%s", f.LibName, decl)
	return fmt.Sprintf("`%s`\n\n%s", decl, doc)
}

type DeprecateFunction struct {
	Name string
	Self *FuncDecl
	Msg  string
}

// func FuncToFuncDecl(libName, methodName string, refType reflect.Type, f interface{}) *FuncDecl {

// }

func CustomHandleTypeName(typName string) string {
	// 这里需要手动处理context
	if strings.Contains(typName, "context") {
		return "context.Context"
	}

	return typName
}

func AnyTypeToLibInstance(libName, name string, typ reflect.Type, value interface{}) *LibInstance {
	var (
		typKind          = typ.Kind()
		pkgPath, typName string
	)
	if typKind == reflect.Struct || typKind == reflect.Interface {
		pkgPath = typ.PkgPath()
		typName = typ.Name()
	} else if typKind == reflect.Ptr {
		pkgPath = typ.Elem().PkgPath()
		typName = typ.Elem().Name()
	}
	if typName == "" {
		typName = typ.String()
	}
	if pkgPath != "" {
		typName = fmt.Sprintf("%s.%s", pkgPath, typName)
	}

	return &LibInstance{
		LibName:      libName,
		InstanceName: name,
		Type:         CustomHandleTypeName(typName),
		ValueStr:     utils.AsDebugString(value),
	}
}
