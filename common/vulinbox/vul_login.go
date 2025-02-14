package vulinbox

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/dgrijalva/jwt-go"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"net/http"
	"strings"
)

//go:embed html/vul_login_jwt_login.html
var jwtLoginPage []byte

//go:embed html/vul_login_jwt_profile.html
var jwtLoginProfilePage []byte

//go:embed html/vul_login_login_setjwt.html
var jwtLoginProfileSetJWTPage []byte

func buildLoginHandles(s *VulinServer, key []byte, profileUrl string) func(writer http.ResponseWriter, request *http.Request) {
	var keyF jwt.Keyfunc = func(token *jwt.Token) (interface{}, error) {
		return []byte(key), nil
	}
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == "GET" {
			// 不存在登录信息
			writer.Header().Set("Content-Type", "text/html")
			data, _ := mutate.FuzzTagExec(jwtLoginPage, mutate.Fuzz_WithParams(map[string]any{
				"profileUrl": "/jwt" + profileUrl,
			}))
			writer.Write([]byte(data[0]))
			return
		}

		if request.Method == "POST" {
			// 登录
			username := request.FormValue("username")
			password := request.FormValue("password")
			if username == "" || password == "" {
				writer.WriteHeader(400)
				writer.Write([]byte("username or password cannot be empty"))
				return
			}

			users, err := s.database.GetUserByUsername(username)
			if err != nil {
				writer.WriteHeader(500)
				writer.Write([]byte("internal error, cannot found user: " + username))
				return
			}
			if len(users) == 0 {
				writer.WriteHeader(400)
				writer.Write([]byte("username or password incorrect"))
				return
			}

			user := users[0]
			if user.Password != password {
				writer.WriteHeader(400)
				writer.Write([]byte("username or password incorrect"))
				return
			}

			token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"username": user.Username,
			})
			token.Header["kid"] = user.ID
			token.Header["username"] = user.Username
			token.Header["age"] = user.Age

			k, _ := keyF(token)
			tokenString, err := token.SignedString(k)
			if err != nil {
				writer.WriteHeader(500)
				writer.Write([]byte("internal error, cannot sign token: " + err.Error() + "\n " + spew.Sdump(key)))
				return
			}

			writer.Header().Set("Content-Type", "text/html")
			//jsonBytes := []byte(`{"token": "` + string(tokenString) + `"}`)
			data, _ := mutate.FuzzTagExec(jwtLoginProfileSetJWTPage, mutate.Fuzz_WithParams(map[string]any{
				"jsonRaw": fmt.Sprintf("%s", tokenString),
			}))
			writer.Write([]byte(data[0]))
			return
		}

		writer.WriteHeader(405)
		writer.Write([]byte("method not allowed"))
	}
}
func buildProfileHandle(s *VulinServer, tokenParser func(s string) (*jwt.Token, error)) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		authToken := request.Header.Get("Authorization")
		if authToken != "" {
			token, err := tokenParser(authToken)
			if err != nil {
				writer.WriteHeader(401)
				writer.Write([]byte(err.Error()))
				return
			}

			writer.Header().Set("Content-Type", "application/json")
			flag := utils.MapGetRawOr(token.Header, "flag", nil)
			if flag != nil {
				data, _ := json.Marshal(map[string]any{
					"flag": flag,
				})
				writer.Write(data)
				return
			}
			name := utils.MapGetString(token.Header, "username")
			users, err := s.database.GetUserByUsername(name)
			if err != nil {
				writer.WriteHeader(500)
				writer.Write([]byte("internal error, cannot found user: " + name))
				return
			}

			profileData := funk.Map(users, func(u *VulinUser) map[string]interface{} {
				return map[string]interface{}{
					"username":   u.Username,
					"id":         u.ID,
					"age":        u.Age,
					"updated_at": u.UpdatedAt.String(),
					"created_at": u.CreatedAt.String(),
				}
			})
			jsonData, err := json.Marshal(profileData)
			if err != nil {
				writer.WriteHeader(500)
				writer.Write([]byte("internal error, cannot found user: " + name + " \n json.Marshal failed: " + err.Error()))
				return
			}

			writer.Write(jsonData)
			return
		}
		writer.WriteHeader(401)
		writer.Write([]byte("invalid auth token"))
		return
	}
}
func (s *VulinServer) registerLoginRoute() {
	var r = s.router

	jwtGroup := r.PathPrefix("/jwt").Name("登陆 JWT").Subrouter()
	key := []byte(utils.RandStringBytes(20))
	jwtRoutes := []*VulInfo{
		// safe jwt
		{
			DefaultQuery: "",
			Path:         "/safe-login",
			Title:        "登陆(Safe JWT)",
			Handler:      buildLoginHandles(s, key, "/safe-login/profile"),
		},
		{
			DefaultQuery: "",
			Path:         "/safe-login/profile",
			Handler: buildProfileHandle(s, func(authToken string) (*jwt.Token, error) {
				before, after, _ := strings.Cut(authToken, " ")
				if before != "Bearer" {
					return nil, errors.New("invalid auth token, use Bearer schema")
				}
				token, err := jwt.Parse(after, func(token *jwt.Token) (interface{}, error) {
					return []byte(key), nil
				})
				if err != nil {
					return nil, errors.New("invalid auth token")
				}
				if !token.Valid {
					return nil, errors.New("invalid auth token")
				}
				return token, nil
			}),
		},
		// unsafe jwt (no validation)
		{
			DefaultQuery: "",
			Path:         "/unsafe-login1",
			Title:        "登陆(未验证算法)",
			Handler:      buildLoginHandles(s, key, "/unsafe-login1/profile"),
		},
		{
			DefaultQuery: "",
			Path:         "/unsafe-login1/profile",
			Handler: buildProfileHandle(s, func(authToken string) (*jwt.Token, error) {
				before, after, _ := strings.Cut(authToken, " ")
				if before != "Bearer" {
					return nil, errors.New("invalid auth token, use Bearer schema")
				}
				token, err := jwt.Parse(after, func(token *jwt.Token) (interface{}, error) {
					switch token.Header["alg"] {
					case "none", "None":
						return jwt.UnsafeAllowNoneSignatureType, nil
					}
					return []byte(key), nil
				})
				if err != nil {
					return nil, errors.New("invalid auth token")
				}
				return token, nil
			}),
		},
		// unsafe jwt (leaking keys in errors)
		{
			DefaultQuery: "",
			Path:         "/unsafe-login2",
			Title:        "登陆(错误中泄漏key)",
			Handler:      buildLoginHandles(s, key, "/unsafe-login2/profile"),
		},
		{
			DefaultQuery: "",
			Path:         "/unsafe-login2/profile",
			Handler: buildProfileHandle(s, func(authToken string) (*jwt.Token, error) {
				before, after, _ := strings.Cut(authToken, " ")
				if before != "Bearer" {
					return nil, errors.New("invalid auth token, use Bearer schema")
				}
				token, err := jwt.Parse(after, func(token *jwt.Token) (interface{}, error) {
					return []byte(key), nil
				})
				if err != nil {
					return nil, fmt.Errorf("parse jwt faild, jwt: %v, key: %v,error: %v", authToken, key, err)
				}
				if !token.Valid {
					return nil, errors.New("invalid auth token")
				}
				return token, nil
			}),
		},
	}
	for _, v := range jwtRoutes {
		addRouteWithVulInfo(jwtGroup, v)
	}

}
