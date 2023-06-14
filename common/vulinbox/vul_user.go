package vulinbox

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"strconv"
	"text/template"
)

//go:embed vul_user_register.html
var registerPage []byte

//go:embed vul_user_login.html
var loginPage []byte

//go:embed vul_user_profile.html
var profilePage []byte

func (s *VulinServer) registerUserRoute() {
	var router = s.router

	router.HandleFunc("/user/register", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		writer.Write(registerPage)
	}).Methods(http.MethodGet)
	// 用户注册
	router.HandleFunc("/user/register", func(writer http.ResponseWriter, request *http.Request) {

		// 解析请求体中的 JSON 数据
		user := &VulinUser{
			Role: "user",
		}
		err := json.NewDecoder(request.Body).Decode(user)
		if err != nil {
			writer.Write([]byte(err.Error()))
			writer.WriteHeader(http.StatusBadRequest)
			return
		}

		// 在这里执行用户注册逻辑，将用户信息存储到数据库
		err = s.database.CreateUser(user)
		if err != nil {
			writer.Write([]byte(err.Error()))
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}

		// 假设验证通过，返回登录成功消息
		responseData, err := json.Marshal(user)
		if err != nil {
			writer.Write([]byte(err.Error()))
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		response := struct {
			Id      uint   `json:"id"`
			Success bool   `json:"success"`
			Message string `json:"message"`
			Data    string `json:"data"`
		}{
			Id:      user.ID,
			Success: true,
			Message: "Register successful",
			Data:    string(responseData),
		}
		writer.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(writer).Encode(response)
		if err != nil {
			writer.Write([]byte(err.Error()))
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		writer.WriteHeader(http.StatusOK)
		return
	}).Methods(http.MethodPost)
	// 用户登录
	router.HandleFunc("/user/login", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			// 返回登录页面
			writer.Header().Set("Content-Type", "text/html")
			writer.Write(loginPage)
			return
		}

		// 解析请求体中的 JSON 数据
		var loginRequest struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}

		err := json.NewDecoder(request.Body).Decode(&loginRequest)
		if err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			writer.Write([]byte("Invalid request"))
			return
		}

		username := loginRequest.Username
		password := loginRequest.Password

		// 在这里执行用户登录逻辑，验证用户名和密码是否正确
		// 检查数据库中是否存在匹配的用户信息
		if username == "" || password == "" {
			writer.WriteHeader(400)
			writer.Write([]byte("username or password cannot be empty"))
			return
		}

		users, err := s.database.GetUserByUsernameUnsafe(username)
		if err != nil {
			writer.WriteHeader(500)
			writer.Write([]byte("internal error, cannot found user: " + username))
			return
		}
		if len(users) == 0 {
			writer.WriteHeader(400)
			// 用户名可爆破
			writer.Write([]byte("Incorrect username"))
			return
		}

		user := users[0]
		if user.Password != password {
			writer.WriteHeader(400)
			// 密码可爆破
			writer.Write([]byte("Incorrect password"))
			return
		}

		// 假设验证通过，返回登录成功消息
		response := struct {
			Id      uint   `json:"id"`
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{
			Id:      user.ID,
			Success: true,
			Message: "Login successful",
		}
		session, err := user.CreateSession(s.database)
		if err != nil {
			return
		}
		http.SetCookie(writer, &http.Cookie{
			Name:  "_cookie",
			Value: session.Uuid,

			HttpOnly: true,
		})
		writer.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(writer).Encode(response)
		if err != nil {
			writer.Write([]byte(err.Error()))
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		writer.WriteHeader(http.StatusOK)
		return
	})
	// 用户信息
	router.HandleFunc("/user/profile", func(writer http.ResponseWriter, request *http.Request) {
		// 通过 id 获取用户信息
		var a = request.URL.Query().Get("id")
		i, err := strconv.ParseInt(a, 10, 64)
		if err != nil {
			writer.Write([]byte(err.Error()))
			writer.WriteHeader(500)
			return
		}
		userInfo, err := s.database.GetUserById(int(i))
		if err != nil {
			writer.Write([]byte(err.Error()))
			writer.WriteHeader(500)
			return
		}

		// 通过 cookie 登录用户的信息
		//session, err := request.Cookie("_cookie")
		//if err != nil {
		//	writer.WriteHeader(http.StatusUnauthorized)
		//	writer.Write([]byte("Unauthorized"))
		//	return
		//}
		//
		//// 解析 Cookie 中的用户信息
		//auth := session.Value
		//userInfo, err := s.database.GetUserBySession(auth)
		//if err != nil {
		//	writer.WriteHeader(http.StatusInternalServerError)
		//	writer.Write([]byte("Internal error session " + err.Error()))
		//	return
		//}

		// 在这里执行获取用户详细信息的逻辑
		// 假设根据用户名查询用户信息
		users, err := s.database.GetUserByUsernameUnsafe(userInfo.Username)
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			writer.Write([]byte("Internal error, cannot retrieve user information"))
			return
		}
		user := users[0]

		writer.Header().Set("Content-Type", "text/html")

		tmpl, err := template.New("profile").Parse(string(profilePage)) // 请将文件名替换为你保存的 HTML 文件名
		err = tmpl.Execute(writer, user)
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			writer.Write([]byte("Internal error, cannot render user profile"))
			return
		}
	})

	router.HandleFunc("/user/update", func(writer http.ResponseWriter, request *http.Request) {
		// 解析请求体中的 JSON 数据
		var oldUser VulinUser
		err := json.NewDecoder(request.Body).Decode(&oldUser)
		if err != nil {
			writer.Write([]byte(err.Error()))
			writer.WriteHeader(http.StatusBadRequest)
			return
		}

		userInfo, err := s.database.GetUserById(int(oldUser.ID))
		if err != nil {
			writer.Write([]byte(err.Error()))
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		userInfo.Remake = oldUser.Remake
		err = s.database.UpdateUser(userInfo)
		if err != nil {
			writer.Write([]byte(err.Error()))
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "text/html")

		tmpl, err := template.New("profile").Parse(string(profilePage)) // 请将文件名替换为你保存的 HTML 文件名
		err = tmpl.Execute(writer, userInfo)
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			writer.Write([]byte("Internal error, cannot render user profile"))
			return
		}
	})

}
