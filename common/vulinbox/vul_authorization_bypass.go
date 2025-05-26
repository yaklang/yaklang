package vulinbox

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed html/vul_auth_bypass_login.html
var authBypassLoginPage []byte

//go:embed html/vul_auth_bypass_dashboard.html
var authBypassDashboardPage []byte

func (s *VulinServer) registerAuthorizationBypass() {
	group := s.router.PathPrefix("/vul/auth-bypass").Name("认证绕过").Subrouter()

	vulRoutes := []*VulInfo{
		{
			DefaultQuery: "",
			Path:         "/safe",
			Title:        "认证绕过(安全版本)",
			Handler:      s.authBypassSafeLogin,
		},
		{
			DefaultQuery: "",
			Path:         "/safe/api/user",
			Handler:      s.authBypassSafeAPI,
		},
		{
			DefaultQuery: "",
			Path:         "/safe/api/cmd",
			Handler:      s.authBypassSafeCMD,
		},
		{
			DefaultQuery: "user=1",
			Path:         "/unsafe",
			Title:        "认证绕过(存在漏洞)",
			Handler:      s.authBypassVulnLogin,
		},
		{
			DefaultQuery: "username=admin",
			Path:         "/unsafe/api/user",
			Handler:      s.authBypassVulnAPI,
		},
		{
			DefaultQuery: "",
			Path:         "/unsafe/api/cmd",
			Handler:      s.authBypassVulnCMD,
		},
	}

	for _, v := range vulRoutes {
		addRouteWithVulInfo(group, v)
	}
}

// 安全版本的登录页面
func (s *VulinServer) authBypassSafeLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		w.Header().Set("Content-Type", "text/html")
		data, _ := mutate.FuzzTagExec(authBypassLoginPage, mutate.Fuzz_WithParams(map[string]any{
			"loginPath":  "/vul/auth-bypass/safe",
			"vulnNotice": "", // 安全版本不显示漏洞提示
		}))
		w.Write([]byte(data[0]))
		return
	}

	if r.Method == "POST" {
		username := r.FormValue("username")
		password := r.FormValue("password")

		if username == "" || password == "" {
			w.WriteHeader(400)
			w.Write([]byte("用户名或密码不能为空"))
			return
		}

		users, err := s.database.GetUserByUsername(username)
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte("内部错误，无法找到用户: " + username))
			return
		}
		if len(users) == 0 {
			w.WriteHeader(400)
			w.Write([]byte("用户名或密码不正确"))
			return
		}

		user := users[0]
		if user.Password != password {
			w.WriteHeader(400)
			w.Write([]byte("用户名或密码不正确"))
			return
		}

		// 创建session token
		sessionToken := utils.RandStringBytes(32)

		http.SetCookie(w, &http.Cookie{
			Name:     "session_token",
			Value:    sessionToken + ":" + username + ":" + strconv.Itoa(int(user.ID)),
			HttpOnly: true,
			Path:     "/",
		})

		w.Header().Set("Content-Type", "text/html")
		data, _ := mutate.FuzzTagExec(authBypassDashboardPage, mutate.Fuzz_WithParams(map[string]any{
			"apiPath":     "/vul/auth-bypass/safe/api/user",
			"cmdPath":     "/vul/auth-bypass/safe/api/cmd",
			"vulnVersion": "false",
		}))
		w.Write([]byte(data[0]))
		return
	}

	w.WriteHeader(405)
	w.Write([]byte("方法不允许"))
}

// 安全版本的API - 正确验证权限
func (s *VulinServer) authBypassSafeAPI(w http.ResponseWriter, r *http.Request) {
	// 验证session
	cookie, err := r.Cookie("session_token")
	if err != nil {
		w.WriteHeader(401)
		w.Write([]byte("未授权：缺少session"))
		return
	}

	parts := strings.Split(cookie.Value, ":")
	if len(parts) != 3 {
		w.WriteHeader(401)
		w.Write([]byte("未授权：无效的session"))
		return
	}

	currentUsername := parts[1]
	currentUserID := parts[2]

	// 获取请求的用户ID
	requestedUserID := r.URL.Query().Get("user")
	if requestedUserID == "" {
		requestedUserID = currentUserID // 默认获取当前用户信息
	}

	// 安全检查：只能获取自己的信息
	if requestedUserID != currentUserID {
		w.WriteHeader(403)
		w.Write([]byte("权限不足：只能查看自己的用户信息"))
		return
	}

	// 验证用户是否存在
	users, err := s.database.GetUserByUsername(currentUsername)
	if err != nil || len(users) == 0 {
		w.WriteHeader(401)
		w.Write([]byte("未授权：无效用户"))
		return
	}

	user := users[0]
	userData := map[string]interface{}{
		"username":   user.Username,
		"id":         user.ID,
		"age":        user.Age,
		"role":       user.Role,
		"updated_at": user.UpdatedAt.String(),
		"created_at": user.CreatedAt.String(),
	}

	w.Header().Set("Content-Type", "application/json")
	jsonData, _ := json.Marshal(userData)
	w.Write(jsonData)
}

// 存在漏洞的登录页面
func (s *VulinServer) authBypassVulnLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		w.Header().Set("Content-Type", "text/html")
		vulnNoticeHTML := `<div class="alert alert-warning mt-3">
			<small><strong>注意：</strong>此系统存在用户ID参数漏洞</small>
		</div>`
		data, _ := mutate.FuzzTagExec(authBypassLoginPage, mutate.Fuzz_WithParams(map[string]any{
			"loginPath":  "/vul/auth-bypass/unsafe",
			"vulnNotice": vulnNoticeHTML,
		}))
		w.Write([]byte(data[0]))
		return
	}

	if r.Method == "POST" {
		username := r.FormValue("username")
		password := r.FormValue("password")

		if username == "" || password == "" {
			w.WriteHeader(400)
			w.Write([]byte("用户名或密码不能为空"))
			return
		}

		users, err := s.database.GetUserByUsername(username)
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte("内部错误，无法找到用户: " + username))
			return
		}
		if len(users) == 0 {
			w.WriteHeader(400)
			w.Write([]byte("用户名或密码不正确"))
			return
		}

		user := users[0]
		if user.Password != password {
			w.WriteHeader(400)
			w.Write([]byte("用户名或密码不正确"))
			return
		}

		// 创建session token
		sessionToken := utils.RandStringBytes(32)

		http.SetCookie(w, &http.Cookie{
			Name:     "session_token",
			Value:    sessionToken + ":" + username + ":" + strconv.Itoa(int(user.ID)),
			HttpOnly: true,
			Path:     "/",
		})

		w.Header().Set("Content-Type", "text/html")
		data, _ := mutate.FuzzTagExec(authBypassDashboardPage, mutate.Fuzz_WithParams(map[string]any{
			"apiPath":     "/vul/auth-bypass/unsafe/api/user",
			"cmdPath":     "/vul/auth-bypass/unsafe/api/cmd",
			"vulnVersion": "true",
		}))
		w.Write([]byte(data[0]))
		return
	}

	w.WriteHeader(405)
	w.Write([]byte("方法不允许"))
}

// 存在漏洞的API - 未正确验证权限
func (s *VulinServer) authBypassVulnAPI(w http.ResponseWriter, r *http.Request) {
	// 漏洞：这里不验证session，直接通过username参数获取用户信息
	// 任何人都可以通过传递username参数来获取任意用户的信息

	// 获取请求的用户名
	requestedUsername := r.URL.Query().Get("username")
	if requestedUsername == "" {
		w.WriteHeader(400)
		w.Write([]byte("缺少username参数"))
		return
	}

	// 直接根据username参数返回对应用户的信息，无任何权限验证
	users, err := s.database.GetUserByUsername(requestedUsername)
	if err != nil || len(users) == 0 {
		w.WriteHeader(404)
		w.Write([]byte("用户不存在"))
		return
	}

	user := users[0]
	userData := map[string]interface{}{
		"username":   user.Username,
		"id":         user.ID,
		"age":        user.Age,
		"role":       user.Role,
		"updated_at": user.UpdatedAt.String(),
		"created_at": user.CreatedAt.String(),
	}

	w.Header().Set("Content-Type", "application/json")
	jsonData, _ := json.Marshal(userData)
	w.Write(jsonData)
}

// 安全版本的命令执行API - 需要admin权限
func (s *VulinServer) authBypassSafeCMD(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(405)
		w.Write([]byte("方法不允许"))
		return
	}

	// 验证session
	cookie, err := r.Cookie("session_token")
	if err != nil {
		w.WriteHeader(401)
		w.Write([]byte("未授权：缺少session"))
		return
	}

	parts := strings.Split(cookie.Value, ":")
	if len(parts) != 3 {
		w.WriteHeader(401)
		w.Write([]byte("未授权：无效的session"))
		return
	}

	currentUsername := parts[1]

	// 验证用户是否存在并且是admin
	users, err := s.database.GetUserByUsername(currentUsername)
	if err != nil || len(users) == 0 {
		w.WriteHeader(401)
		w.Write([]byte("未授权：无效用户"))
		return
	}

	user := users[0]
	if user.Role != "admin" {
		w.WriteHeader(403)
		w.Write([]byte("权限不足：需要管理员权限"))
		return
	}

	// 解析命令
	var cmdReq struct {
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&cmdReq); err != nil {
		w.WriteHeader(400)
		w.Write([]byte("无效的请求格式"))
		return
	}

	// 模拟执行ls命令
	var result string
	if cmdReq.Command == "ls" {
		result = `total 28
drwxr-xr-x  3 user user 4096 Dec 15 10:30 documents
drwxr-xr-x  2 user user 4096 Dec 15 10:25 downloads
-rw-r--r--  1 user user 1024 Dec 15 10:20 config.txt
-rw-r--r--  1 user user  512 Dec 15 10:15 readme.md
-rw-r--r--  1 root root   64 Dec 15 10:10 flag.txt`
	} else {
		result = "bash: " + cmdReq.Command + ": command not found"
	}

	response := map[string]interface{}{
		"command": cmdReq.Command,
		"result":  result,
	}

	w.Header().Set("Content-Type", "application/json")
	jsonData, _ := json.Marshal(response)
	w.Write(jsonData)
}

// 存在漏洞的命令执行API - 未验证认证
func (s *VulinServer) authBypassVulnCMD(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(405)
		w.Write([]byte("方法不允许"))
		return
	}

	// 漏洞：这里没有验证用户是否已登录，也没有验证权限
	// 任何人都可以直接调用这个接口执行命令
	// 前端的权限检查可以被绕过

	// 解析命令
	var cmdReq struct {
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&cmdReq); err != nil {
		w.WriteHeader(400)
		w.Write([]byte("无效的请求格式"))
		return
	}

	// 模拟执行ls命令
	var result string
	if cmdReq.Command == "ls" {
		result = `total 36
drwxr-xr-x  4 root root 4096 Dec 15 10:30 admin
drwxr-xr-x  3 user user 4096 Dec 15 10:30 documents
drwxr-xr-x  2 user user 4096 Dec 15 10:25 downloads
-rw-r--r--  1 root root 1024 Dec 15 10:20 config.txt
-rw-r--r--  1 root root  512 Dec 15 10:15 secret.txt
-rw-r--r--  1 user user  256 Dec 15 10:10 readme.md
-rw-r--r--  1 root root   64 Dec 15 10:05 flag.txt`
	} else {
		result = "bash: " + cmdReq.Command + ": command not found"
	}

	response := map[string]interface{}{
		"command": cmdReq.Command,
		"result":  result,
	}

	w.Header().Set("Content-Type", "application/json")
	jsonData, _ := json.Marshal(response)
	w.Write(jsonData)
}
