package javascript

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"testing"
)

func Test_JS_XMLHttpRequest(t *testing.T) {
	t.Run("simple get request", func(t *testing.T) {
		code := `
	let xhr1 =new XMLHttpRequest()

	xhr1.open('GET', 'http://*****')
	xhr1.send()
    xhr1.send("123")
    xhr1.addEventListener('load', function () {
      console.log(this.response)
    })

   `
		prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaapi.JS))
		if err != nil {
			t.Fatal("prog parse error", err)
		}
		prog.Show()
		// todo syntax分析应该只能得到XMLHttpRequest.open(),得到多个无关值
		results, err := prog.SyntaxFlowWithError("XMLHttpRequest().open")
		for _, result := range results {
			// 获取所有call被调用的地方
			for _, called := range result.GetCalledBy() {
				//获取参数
				called.GetCallArgs().Show()
			}
		}
	})

	t.Run("simple post request", func(t *testing.T) {
		code := `
	const data = {
       name: 'job',
       age: '12',
    }
    let xhr2 = new XMLHttpRequest()
    xhr2.open('POST', 'http://XXXX')
    const usp = new URLSearchParams(data)
    const query = usp.toString()
    xhr2.setRequestHeader('Content-type', 'application/x-www-form-urlencoded')
    xhr2.send(query)
    xhr2.addEventListener('load', function () {
        console.log(this.response)
    })

   `
		prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaapi.JS))
		if err != nil {
			t.Fatal("prog parse error", err)
		}
		prog.Show()
		// todo syntax分析应该只能得到XMLHttpRequest.open(),得到多个无关值
		// 获取XMLHttpRequest.open()的参数
		open, err := prog.SyntaxFlowWithError("XMLHttpRequest().open")
		for _, result := range open {
			// 获取所有call被调用的地方
			for _, called := range result.GetCalledBy() {
				//获取参数
				called.GetCallArgs().Show()
			}
		}

	})

}

func Test_JS_Fetch(t *testing.T) {}

func Test_JS_JQuery(t *testing.T) {}

func Test_JS_Axios(t *testing.T) {}
