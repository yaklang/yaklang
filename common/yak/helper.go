package yak

import (
	"bytes"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/olekukonko/tablewriter"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"reflect"
	"sort"
	"strings"
)

type PalmScriptEngineHelper struct {
	Libs             map[string]*PalmScriptLib
	BuildInFunctions map[string]*PalmScriptLibFunc
	UserFunctions    map[string]*PalmScriptLibFunc
	Instances        map[string]*PalmScriptLibInstance
}

func (p *PalmScriptEngineHelper) GetAllLibs() []string {
	var k []string
	for name := range p.Libs {
		k = append(k, name)
	}
	sort.Strings(k)
	return k
}

func (p *PalmScriptEngineHelper) HelpInfo() string {
	buffer := bytes.NewBuffer(nil)

	// 内置用户函数
	buffer.WriteString("### 内置用户函数定义\n")
	var items []string
	for _, i := range p.UserFunctions {
		items = append(items, i.String())
	}
	sort.Strings(items)
	for _, i := range items {
		buffer.WriteString(fmt.Sprintf("    %v\n", i))
	}

	buffer.WriteString(fmt.Sprintf("\n%v\n", strings.Repeat("-", 48)))

	t := tablewriter.NewWriter(buffer)
	t.Header([]string{"内置值", "值的类型", "值"})
	for name, item := range p.Instances {
		if item.Value == nil {
			t.Append([]string{name, item.Type, "-"})
		} else {
			t.Append([]string{name, item.Type, spew.Sdump(item.Value)})
		}
	}
	t.Render()

	buffer.WriteString(fmt.Sprintf("\n%v\n", strings.Repeat("-", 48)))

	t = tablewriter.NewWriter(buffer)
	t.Header([]string{"可用依赖库", "依赖库可用元素(值/函数)"})
	for libName, libs := range p.Libs {
		t.Append([]string{libName, fmt.Sprint(len(libs.ElementDocs))})
	}
	t.Render()

	return buffer.String()
}

func (p *PalmScriptEngineHelper) ShowHelpInfo() {
	fmt.Println(p.HelpInfo())
}

func (p *PalmScriptEngineHelper) LibHelpInfo(name string) string {
	lib, ok := p.Libs[name]
	if !ok {
		return ""
	}
	return lib.String()
}

func (p *PalmScriptEngineHelper) ShowLibHelpInfo(name string) {
	fmt.Println(p.LibHelpInfo(name))
}

type PalmScriptLib struct {
	Name             string
	Values           map[string]interface{}
	ElementDocs      []string
	FuncElements     []*PalmScriptLibFunc
	InstanceElements []*PalmScriptLibInstance
}

func (p *PalmScriptLib) String() string {
	buff := bytes.NewBuffer(nil)
	buff.WriteString(fmt.Sprintf("### Palm ExtLib: [%v] - %v elements\n\n", p.Name, len(p.Values)))
	sort.Strings(p.ElementDocs)
	for _, i := range p.ElementDocs {
		buff.WriteString(fmt.Sprintf("    %v\n", i))
	}

	return buff.String()
}

type PalmScriptLibFunc struct {
	LibName    string
	MethodName string
	Params     []string
	Returns    []string
}

func (p *PalmScriptLibFunc) String() string {
	if p == nil {
		return ""
	}

	var end string
	if len(p.Returns) > 1 {
		end = fmt.Sprintf(": (%v)", strings.Join(p.Returns, ", "))
	} else if len(p.Returns) == 1 {
		end = fmt.Sprintf(": %v", strings.Join(p.Returns, ", "))
	}

	if p.LibName == "" || p.LibName == "__GLOBAL__" {
		return fmt.Sprintf(
			"fn %v(%v)%v",
			p.MethodName,
			strings.Join(p.Params, ", "),
			end,
		)
	}

	return fmt.Sprintf(
		"fn %v.%v(%v)%v",
		p.LibName, p.MethodName,
		strings.Join(p.Params, ", "),
		end,
	)
}

type PalmScriptLibInstance struct {
	LibName      string
	InstanceName string
	Type         string
	Value        interface{}
}

func (p *PalmScriptLibInstance) String() string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%v.%v: %v = %v", p.LibName, p.InstanceName, p.Type, p.Value)
}

func funcTypeToPalmScriptLibFunc(libName, method string, methodType reflect.Type) *PalmScriptLibFunc {
	if methodType.Kind() != reflect.Func {
		return nil
	}

	methodItem := &PalmScriptLibFunc{
		LibName:    libName,
		MethodName: method,
	}

	// 遍历参数
	for i := range make([]int, methodType.NumIn()) {
		paramType := methodType.In(i)
		text := fmt.Sprintf("var_%v: %v", i+1, paramType.String())

		if i+1 == methodType.NumIn() && methodType.IsVariadic() {
			methodItem.Params = append(
				methodItem.Params,
				fmt.Sprintf("vars: ...%v", paramType.Elem()),
			)
		} else {
			methodItem.Params = append(
				methodItem.Params,
				text,
			)
		}
	}

	// 遍历返回值
	for i := range make([]int, methodType.NumOut()) {
		paramType := methodType.Out(i)
		methodItem.Returns = append(
			methodItem.Returns,
			fmt.Sprintf("%v", paramType.String()),
		)
	}
	return methodItem
}
func anyTypeToPalmScriptLibInstance(libName, name string, methodType reflect.Type) *PalmScriptLibInstance {
	return &PalmScriptLibInstance{
		LibName:      libName,
		InstanceName: name,
		Type:         methodType.String(),
	}
}

func EngineToHelper(engine *antlr4yak.Engine) *PalmScriptEngineHelper {
	helper := &PalmScriptEngineHelper{
		Libs:             make(map[string]*PalmScriptLib),
		BuildInFunctions: make(map[string]*PalmScriptLibFunc),
		UserFunctions:    make(map[string]*PalmScriptLibFunc),
		Instances:        make(map[string]*PalmScriptLibInstance),
	}

	var extLibs []*PalmScriptLib
	for name, item := range engine.GetFntable() {
		iTy := reflect.TypeOf(item)
		iVl := reflect.ValueOf(item)
		_, _ = iTy, iVl

		switch iTy {
		case reflect.TypeOf(make(map[string]interface{})):
			res := item.(map[string]interface{})
			if res == nil && len(res) <= 0 {
				continue
			}

			extLib := &PalmScriptLib{
				Name:   name,
				Values: res,
			}
			extLibs = append(extLibs, extLib)
			helper.Libs[extLib.Name] = extLib

			for elementName, value := range res {
				switch methodType := reflect.TypeOf(value); methodType.Kind() {
				case reflect.Func:
					methodItem := funcTypeToPalmScriptLibFunc(name, elementName, methodType)
					if methodItem == nil {
						continue
					}

					extLib.ElementDocs = append(extLib.ElementDocs, methodItem.String())
					extLib.FuncElements = append(extLib.FuncElements, methodItem)
				default:
					item := anyTypeToPalmScriptLibInstance(
						extLib.Name, elementName,
						methodType,
					)
					extLib.ElementDocs = append(extLib.ElementDocs, item.String())
					extLib.InstanceElements = append(extLib.InstanceElements, item)
				}
			}
		default:
			if iTy == nil {
				continue
			}

			globalBanner := "__GLOBAL__"
			switch iTy.Kind() {
			case reflect.Func:
				if strings.HasPrefix(name, "$") || strings.HasPrefix(name, "_") {
					helper.BuildInFunctions[name] = funcTypeToPalmScriptLibFunc(globalBanner, name, iTy)
				} else {
					helper.UserFunctions[name] = funcTypeToPalmScriptLibFunc(globalBanner, name, iTy)
				}
			default:
				helper.Instances[name] = anyTypeToPalmScriptLibInstance(globalBanner, name, iTy)
			}
		}
	}
	return helper
}
