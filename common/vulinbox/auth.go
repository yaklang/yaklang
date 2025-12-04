package vulinbox

import (
	"net/http"
)

func (s *dbm) Authenticate(writer http.ResponseWriter, request *http.Request) (*VulinUser, error) {
	session, err := request.Cookie("_cookie")
	if err != nil {
		//writer.Header().Set("Location", "/logic/user/login")
		writer.WriteHeader(http.StatusOK)
		writer.Write([]byte(`
<script>
alert("请先登录");
window.location.href = '/logic/user/login?from=` + request.URL.Path + `';
</script>
`))
		return nil, err
	}

	auth := session.Value
	se, err := s.GetUserBySession(auth)
	if err != nil {
		//writer.Header().Set("Location", "/logic/user/login")
		writer.WriteHeader(http.StatusOK)
		writer.Write([]byte(`
<script>
alert("session 异常，请重新登录");
window.location.href = '/logic/user/login?from=` + request.URL.Path + `';
</script>
`))
		return nil, err
	}

	// 在这里执行获取用户详细信息的逻辑
	// 假设根据用户名查询用户信息
	users, err := s.GetUserByUsername(se.Username)
	if err != nil {
		//writer.Header().Set("Location", "/logic/user/login")
		writer.WriteHeader(http.StatusOK)
		writer.Write([]byte(`
<script>
alert("用户信息异常，请重新登录");
window.location.href = '/logic/user/login?from=` + request.URL.Path + `';
</script>
`))
		return nil, err
	}
	user := users[0]

	return user, nil
}

// 购物商城的认证
func (s *dbm) mallAuthenticate(writer http.ResponseWriter, request *http.Request) (*VulinUser, error) {
	session, err := request.Cookie("sessionID")
	if err != nil {
		writer.WriteHeader(http.StatusOK)
		writer.Write([]byte(`
<script>
alert("请先登录");
window.location.href = '/mall/user/login';
</script>
`))
		return nil, err
	}

	auth := session.Value
	se, err := s.GetUserBySession(auth)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		writer.Write([]byte("Internal error session " + err.Error()))
		return nil, err
	}

	// 在这里执行获取用户详细信息的逻辑
	// 假设根据用户名查询用户信息
	users, err := s.GetUserByUsername(se.Username)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		writer.Write([]byte("Internal error, cannot retrieve user information"))
		return nil, err
	}
	user := users[0]

	return user, nil
}

//func (s *dbm) IsAdmin(username string) bool {
//	var user VulinUser
//	if err := um.db.Where("username = ? AND role = ?", username, "admin").First(&user).Error; err != nil {
//		return false
//	}
//	return true
//}
