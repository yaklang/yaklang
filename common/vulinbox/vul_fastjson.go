package vulinbox

import (
	"encoding/json"
	"errors"
	utils2 "github.com/yaklang/yaklang/common/utils"
	"io"
	"net/http"
)

func fastjsonParser(data string) (map[string]any, error) {
	var js map[string]any
	err := json.Unmarshal([]byte(data), &js)
	if err != nil {
		return nil, err
	}
	return js, nil
}
func mockController(data string) string {
	newErrorResponse := func(err error) string {
		response, _ := json.Marshal(map[string]any{
			"code": 0,
			"err":  err.Error(),
		})
		return string(response)
	}
	js, err := fastjsonParser(data)
	user := utils2.MapGetString(js, "user")
	password := utils2.MapGetString(js, "password")
	if user == "admin" && password == "password" {
		return "ok"
	}
	if err != nil {
		return newErrorResponse(err)
	}
	err = errors.New("user or password error")
	return newErrorResponse(err)
}
func (s *VulinServer) registerFastjson() {
	r := s.router
	var fastjsonGroup = r.PathPrefix("/fastjson").Name("Fastjson 案例").Subrouter()
	var vuls = []*VulInfo{
		{
			Title:        "GET 传参案例案例",
			Path:         "/json-in-query",
			DefaultQuery: `auth={"user":"admin","password":"password"}`,
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodGet {
					auth := request.URL.Query().Get("auth")
					if auth == "" {
						writer.Write([]byte("auth 参数不能为空"))
						return
					}
					response := mockController(auth)
					writer.Write([]byte(response))
				} else {
					writer.WriteHeader(http.StatusMethodNotAllowed)
				}
			},
		},
		{
			Title: "POST 传参案例案例",
			Path:  "/json-in-body",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodPost {
					body, err := io.ReadAll(request.Body)
					if err != nil {
						writer.WriteHeader(http.StatusBadRequest)
						writer.Write([]byte("Invalid request"))
						return
					}
					defer request.Body.Close()
					response := mockController(string(body))
					writer.Write([]byte(response))
				} else {
					writer.WriteHeader(http.StatusMethodNotAllowed)
				}
			},
		},
		{
			Title: "COOKIE 传参案例案例",
			Path:  "/json-in-cookie",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodGet {
					cookie, err := request.Cookie("auth")
					if err != nil {
						writer.Write([]byte("auth 参数不能为空"))
						return
					}
					response := mockController(cookie.Value)
					writer.Write([]byte(response))
				} else {
					writer.WriteHeader(http.StatusMethodNotAllowed)
				}
			},
		},
		{
			Title: "COOKIE 传参案例案例",
			Path:  "/form-in-body",
			Handler: func(writer http.ResponseWriter, request *http.Request) {

			},
		},
	}
	for _, v := range vuls {
		addRouteWithVulInfo(fastjsonGroup, v)
	}
}
