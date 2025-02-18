package ssaapi_test

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func Test_CrossProcess(t *testing.T) {
	t.Run("Test_CrossProcess_Analysis 1", func(t *testing.T) {
		code := `
	func A(num){
		return num
	}	

	func foo(){
		m := {"a":A(1),"b":A(2)}
		print(m)
	}
		`
		/*
			以上代码会进行两次跨过程分析，不会触防放递归机制
			m->
			  -> FreeValue-A(1)
				-> Function-A
				  -> Parameter-num
					-> 1
			  -> FreeValue-A(2)
				-> Function-A
			      -> Parameter-num
					-> 2
		*/
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			rule, err := prog.SyntaxFlowWithError(`print(* #-> as $res)`)
			require.NoError(t, err)
			vals := rule.GetValues("res")
			vals.Show()
			return nil
		})
	})
}

func Test_WithinProcess(t *testing.T) {
	t.Run("Test_WithinProcess_Analysis", func(t *testing.T) {
		code := `
	package main
	
	import (
		"html/template"
		"net/http"
	)
	func handler(w http.ResponseWriter, r *http.Request) {
		tmpl := template.Must(template.New("example").Parse("<h1>Hello, {{ .Name }}  || {{ .Id }}</h1>"))
		data := struct {
			Name string
			Id   template.HTML
		}{
			Name: r.FormValue("name"),
			Id:   template.HTML(r.FormValue("id")), // xss
		}
		tmpl.Execute(w, data)
	}
	
	func main() {
		serve := http.NewServeMux()
		serve.HandleFunc("/", handler)
		http.ListenAndServe(":8080", serve)
	}
	`
		rule := `template?{<fullTypeName()>?{have:"html/template"}}.Must().Execute(* as $input)
$input #{
	hook:<<<HOOK
	* ?{!opcode:call &&  have:'FormValue'}  as $toCheck
HOOK
}-> as $res `
		/*
				以上例子为过程内分析，不会触发防递归机制
			make(struct {string,ClassBluePrint: HTML})
			->make(struct {})
				-> ParameterMember-parameter[1].FormValue("name")
					-> ParameterMember-parameter[1].FormValue
					->"name"
				->Parameter-r
			->Undefined-template.HTML(ParameterMember-parameter[1].FormValue("id"))
			  ->Undefined-template.HTML
				->ExternLib-template
			  ->ParameterMember-parameter[1].FormValue("id")
				->ParameterMember-parameter[1].FormValue
				->"id"
			  -> Parameter-r
				其中`ParameterMember-parameter[1].FormValue`会进去分析两次，虽然两次都是一个过程内，但是他们是不同的路径，
				因此防递归机制不应该进行拦截。
		*/
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			res, err := prog.SyntaxFlowWithError(rule)
			require.NoError(t, err)
			vals := res.GetValues("res")
			require.Contains(t, vals.String(), "id", "name", "FormValue")

			toCheck := res.GetValues("toCheck")
			effect := toCheck[0].GetEffectOn()
			require.Equal(t, effect.Len(), 2)
			return nil
		}, ssaapi.WithLanguage(consts.GO))
	})
}
