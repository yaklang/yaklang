package vulinbox

import (
	_ "embed"
	"github.com/xiecat/wsm/lib/utils"
	"html/template"
	"net/http"
	"strconv"
)

//go:embed html/vul_csrf_unsafe.html
var unsafeUpdate []byte

//go:embed html/vul_csrf_safe.html
var safeUpdateTp []byte

func (s *VulinServer) registerCsrf() {
	var router = s.router
	csrfGroup := router.PathPrefix("/csrf").Name("表单 CSRF 保护测试").Subrouter()
	csrfRoutes := []*VulInfo{
		{
			DefaultQuery: "",
			Path:         "/unsafe",
			Title:        "没有保护的表单",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				realUser, err := s.database.Authenticate(writer, request)
				if err != nil {
					return
				}

				if request.Method == http.MethodGet {
					writer.Write(unsafeUpdate)
					return
				}

				updateUser := update(writer, request, realUser)

				if updateUser == nil {
					return
				}

				err = s.database.UpdateUser(updateUser)
				if err != nil {
					writer.WriteHeader(500)
					return
				}

				writer.Write(unsafeUpdate)
				writer.Write([]byte(`<script>alert("修改成功")</script>`))
				writer.WriteHeader(http.StatusOK)

			},
		},
		{
			DefaultQuery: "",
			Path:         "/safe",
			Title:        "csrf_token保护的表单",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				realUser, err := s.database.Authenticate(writer, request)
				if err != nil {
					return
				}

				data := struct {
					Token string
				}{
					Token: utils.MD5(realUser.Username + "vulinbox"),
				}

				tmpl := template.Must(template.New("safeTemplate").Parse(string(safeUpdateTp)))
				if request.Method == http.MethodGet {

					err := tmpl.Execute(writer, data)
					if err != nil {
						writer.WriteHeader(http.StatusInternalServerError)
						return
					}
					writer.WriteHeader(http.StatusOK)
					return
				}

				csrfToken := request.FormValue("csrf_token")

				if csrfToken != data.Token {
					writer.Write([]byte("csrf_token error"))
					writer.WriteHeader(http.StatusBadRequest)
					return
				}

				updateUser := update(writer, request, realUser)

				if updateUser == nil {
					return
				}

				err = s.database.UpdateUser(updateUser)
				if err != nil {
					writer.WriteHeader(500)
					return
				}

				err = tmpl.Execute(writer, data)
				if err != nil {
					writer.WriteHeader(http.StatusInternalServerError)
					return
				}
				writer.Write([]byte(`<script>alert("修改成功")</script>`))
				writer.WriteHeader(http.StatusOK)

			},
		},
	}

	for _, v := range csrfRoutes {
		addRouteWithVulInfo(csrfGroup, v)
	}

}

func update(writer http.ResponseWriter, request *http.Request, realUser *VulinUser) *VulinUser {
	passwd := request.FormValue("password")
	age := request.FormValue("age")
	if passwd != "" {
		realUser.Password = passwd
	}

	if age != "" {
		atoi, err := strconv.Atoi(age)
		if err != nil {
			writer.WriteHeader(500)
			return nil
		}
		realUser.Age = atoi
	}
	return realUser
}
