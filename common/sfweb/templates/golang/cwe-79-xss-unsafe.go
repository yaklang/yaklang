package example

import (
	"html/template"
	"net/http"
)

func handler(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.New("example").Parse(`
        <html>
        <body>
            <h1>Hello, {{ .Name }}</h1>
        </body>
        </html>
    `))

	data := struct {
		Name string
	}{
		Name: r.FormValue("name"), // 从用户输入获取 name 参数
	}

	tmpl.Execute(w, data) // 自动对输出进行转义
}

func main() {
	http.HandleFunc("/", handler)
	http.ListenAndServe(":8080", nil)
}
