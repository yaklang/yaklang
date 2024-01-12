package vulinbox

import (
	_ "embed"
	"encoding/json"
	"github.com/yaklang/yaklang/common/log"
	"net/http"
	"strconv"
	"strings"
	"text/template"
	"time"
)

//go:embed html/mall/vul_mall_register.html
var mallRegisterPage []byte

//go:embed html/mall/vul_mall_login.html
var mallLoginPage []byte

//go:embed html/mall/vul_mall_userProfile.html
var mallUserProfilePage []byte

func (s *VulinServer) mallUserRoute() {
	// var router = s.router
	// malloginGroup := router.PathPrefix("/mall").Name("购物商城").Subrouter()
	mallloginRoutes := []*VulInfo{
		//登陆功能
		{
			DefaultQuery: "",
			Path:         "/user/login",
			// Title:        "商城登陆",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodGet {
					// 返回登录页面
					writer.Header().Set("Content-Type", "text/html")
					writer.Write(mallLoginPage)
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
				// sql 注入 , 万能密码
				users, err := s.database.GetUserByUnsafe(username, password)
				if err != nil {
					writer.WriteHeader(500)
					writer.Write([]byte("internal error: " + err.Error()))
					return
				}
				user := users[0]

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
				var sessionID = "fixedSessionID"
				// session, err := user.CreateSession(s.database)
				if err != nil {
					return
				}
				http.SetCookie(writer, &http.Cookie{
					Name:    "sessionID",
					Value:   sessionID,
					Path:    "/",
					Expires: time.Now().Add(15 * time.Minute),

					//HttpOnly: true,
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
			},
			RiskDetected: true,
		},
		//注册功能
		{
			DefaultQuery: "",
			Path:         "/user/register",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodGet {
					// 返回登录页面
					writer.Header().Set("Content-Type", "text/html")
					writer.Write(mallRegisterPage)
					return
				} else if request.Method == http.MethodPost {
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

					remake := strings.ToLower(user.Remake)
					filterRemake := strings.ReplaceAll(remake, "<", "")
					filterRemake = strings.ReplaceAll(filterRemake, ">", "")
					filterRemake = strings.ReplaceAll(filterRemake, "script", "")
					user.Remake = filterRemake

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
				}
			},
		},
		//用户信息
		{
			DefaultQuery: "",
			Path:         "/user/profile",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				realUser, err := s.database.mallAuthenticate(writer, request)
				if err != nil {
					return
				}

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

				// 水平越权
				if realUser.Role != "admin" && realUser.Role != userInfo.Role {
					writer.Write([]byte("Not Enough Permissions"))
					writer.WriteHeader(http.StatusBadRequest)
					return
				}

				// 返回用户个人页面
				writer.Header().Set("Content-Type", "text/html")
				tmpl, err := template.New("userProfile").Parse(string(mallUserProfilePage))

				//获取购物车商品数量
				Cartsum, err := s.database.GetUserCartCount(int(userInfo.ID))
				//个人信息页面显示购物车商品数量
				type UserProfile struct {
					ID       int
					Username string
					Cartsum  int
				}
				profile := UserProfile{
					// ID:       int(userInfo.ID),
					Username: userInfo.Username,
					Cartsum:  int(Cartsum),
				}

				err = tmpl.Execute(writer, profile)

				if err != nil {
					writer.WriteHeader(http.StatusInternalServerError)
					log.Println("执行模板失败:", err)
					writer.Write([]byte("Internal error, cannot render user profile"))
					return
				}
			},
		},
		//退出登陆
		{
			DefaultQuery: "",
			Path:         "/user/logout",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				cookie, err := request.Cookie("_cookie")
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(http.StatusBadRequest)
					return
				}
				uuid := cookie.Value
				err = s.database.DeleteSession(uuid)
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(http.StatusInternalServerError)
					return
				}
				writer.WriteHeader(http.StatusOK)
				return
			},
		},
	}

	for _, v := range mallloginRoutes {
		addRouteWithVulInfo(MallGroup, v)
	}

}
