package vulinbox

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"net/http"
	"strings"
	"time"
)

//go:embed html/vul_fake_ip.html
var successLogin []byte

//go:embed html/vul_fakeIp_login.html
var Login []byte

func (s *VulinServer) registerFakeIp() {
	token := uuid.NewString()
	router := s.router
	sessionStore := utils.NewTTLCacheWithKey[string, int](time.Second * time.Duration(60*15))
	fakeIpGroup := router.PathPrefix("/fakeIp").Name("ip伪造").Subrouter()
	defaultHandler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var route string
		switch LoadFromGetParams(request, "case") {
		case "local":
			route = "local"
		case "random":
			fallthrough
		default:
			route = "random"
		}
		unsafeTemplateRender(writer, request, string(Login), map[string]any{
			"action": "/fakeIp/" + route,
		})
	})

	fakeIpRoutes := []*VulInfo{
		{
			DefaultQuery: "case=local",
			Path:         "/login",
			Title:        "ip伪造登陆（本地IP伪造）",
			Handler:      defaultHandler,
		},
		{
			DefaultQuery: "case=random",
			Path:         "/login",
			Title:        "ip伪造（爆破）",
			Handler:      defaultHandler,
		},
	}
	router.Handle("/fakeIp/local", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		ip := request.Header.Get("X-Forwarded-For")
		if ip == "" {
			ip = request.URL.Host
		}
		var user = struct {
			Username string
			Password string
		}{}
		var response = struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{
			Success: false,
			Message: "登陆失败",
		}
		ip = strings.TrimSpace(ip)
		body, err := io.ReadAll(request.Body)
		if err != nil {
			marshal, _ := json.Marshal(response)
			writer.Write(marshal)
			return
		}
		err = json.Unmarshal(body, &user)
		if err != nil {
			response.Message = err.Error()
			bytes, _ := json.Marshal(response)
			writer.Write(bytes)
			return
		}
		if strings.ToLower(ip) == "127.0.0.1" {
			response.Success = true
			response.Message = "登陆成功"
			bytes, _ := json.Marshal(response)
			http.SetCookie(writer, &http.Cookie{
				Name:  "token",
				Value: token,
			})
			writer.Write(bytes)
			return
		} else {
			response.Success = false
			response.Message = "需要本地IP登陆"
			bytes, _ := json.Marshal(response)
			writer.Write(bytes)
			return
		}
	}))
	router.Handle("/fakeIp/success", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie("token")
		if err != nil {
			writer.WriteHeader(403)
			return
		}
		if cookie.Value == "" || cookie.Value != token {
			writer.WriteHeader(403)
			return
		} else {
			writer.Write(successLogin)
		}
	}))
	router.Handle("/fakeIp/random", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		ip := request.Header.Get("X-Forwarded-For")
		if ip == "" {
			ip = request.URL.Host
		}
		var response = struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{
			Success: false,
			Message: "登陆失败",
		}
		writeRespose := func() {
			bytes, _ := json.Marshal(response)
			writer.Write(bytes)
		}
		if count, exists := sessionStore.Get(ip); exists {
			if count > 5 {
				response.Message = "连续登陆，用户被锁定"
				writeRespose()
				return
			}
		}
		var user = struct {
			Username string
			Password string
		}{}
		ip = strings.TrimSpace(ip)
		body, err := io.ReadAll(request.Body)
		if err != nil {
			response.Message = fmt.Sprintf("登陆失败： %s", err.Error())
			writeRespose()
			return
		}
		err = json.Unmarshal(body, &user)
		if err != nil {
			response.Message = fmt.Sprintf("unmarshal username or password fail: %s", err)
			writeRespose()
			return
		}
		if strings.ToLower(user.Username) == "admin" && strings.ToLower(user.Password) == "111111" {
			response.Success = true
			response.Message = "登陆成功"
			http.SetCookie(writer, &http.Cookie{
				Name:  "token",
				Value: token,
			})
			writeRespose()
			return
		} else {
			if count, exists := sessionStore.Get(ip); exists {
				count++
				sessionStore.Set(ip, count)
			} else {
				sessionStore.Set(ip, 1)
			}
		}
		response.Message = "登陆失败"
		writeRespose()
	}))
	for _, v := range fakeIpRoutes {
		addRouteWithVulInfo(fakeIpGroup, v)
	}
}
