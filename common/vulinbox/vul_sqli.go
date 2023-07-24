package vulinbox

import (
	_ "embed"
	"encoding/json"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net/http"
	"strconv"
	"strings"
	"time"
)

//go:embed vul_sqli.html
var vulInSQLIViewer []byte

func sqliWriter(writer http.ResponseWriter, request *http.Request, data []*VulinUser, str ...string) {
	sqliWriterEx(false, writer, request, data, str...)
}

func sqliWriterEx(enableDebug bool, writer http.ResponseWriter, request *http.Request, data []*VulinUser, str ...string) {
	raw, err := json.Marshal(data)
	if err != nil {
		Failed(writer, request, err.Error())
		return
	}

	if request.URL.Query().Get("debug") == "" {
		str = nil
	}
	var extraInfo string
	if len(str) > 0 {
		extraInfo = `<pre>` + strconv.Quote(strings.Join(str, "")) + `</pre> <br>`
	}
	var debugstyle string
	if !enableDebug {
		debugstyle = `style='display: none;'`
	} else {
		debugstyle = `style='margin-bottom: 24px;'`
	}
	unsafeTemplateRender(writer, request, string(vulInSQLIViewer), map[string]any{
		"userjson":   string(raw),
		"extra":      extraInfo,
		"debugstyle": debugstyle,
	})
}

func (s *VulinServer) registerSQLinj() {
	var router = s.router

	sqli := router.Name("SQL注入漏洞案例（复杂度递增）").Subrouter()

	vroutes := []*VulnInfo{
		{
			DefaultQuery: "id=1",
			Path:         "/user/by-id-safe",
			RouteName:    "不存在SQL注入的情况（数字严格校验）",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				var a = request.URL.Query().Get("id")
				i, err := strconv.ParseInt(a, 10, 64)
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				u, err := s.database.GetUserById(int(i))
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				sqliWriter(writer, request, []*VulinUser{u})
				return
			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "id=1",
			Path:         "/user/id",
			RouteName:    "ID 为数字型的简单边界 SQL注入",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				var a = request.URL.Query().Get("id")
				u, err := s.database.GetUserByIdUnsafe(a)
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				sqliWriter(writer, request, []*VulinUser{u})
				return
			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "id=1",
			Path:         "/user/id-json",
			RouteName:    "参数是 JSON，JSON中字段存在SQL注入",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				var a = request.URL.Query().Get("id")
				var jsonMap map[string]any
				err := json.Unmarshal([]byte(a), &jsonMap)
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				a, ok := jsonMap["id"].(string)
				if !ok {
					writer.Write([]byte("Failed to retrieve the 'id' field"))
					writer.WriteHeader(500)
					return
				}

				u, err := s.database.GetUserByIdUnsafe(a)
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				sqliWriter(writer, request, []*VulinUser{u})
				return
			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "id=1",
			Path:         "/user/id-b64-json",
			RouteName:    "GET参数是被编码的JSON，JSON中字段存在SQL注入",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				var a = request.URL.Query().Get("id")
				decodedB64, err := codec.DecodeBase64(a)
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				var jsonMap map[string]any
				err = json.Unmarshal(decodedB64, &jsonMap)
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				a, ok := jsonMap["id"].(string)
				if !ok {
					writer.Write([]byte("Failed to retrieve the 'id' field"))
					writer.WriteHeader(500)
					return
				}

				u, err := s.database.GetUserByIdUnsafe(a)
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				sqliWriter(writer, request, []*VulinUser{u})
				return
			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "id=1",
			Path:         "/user/id-error",
			RouteName:    "ID 为数字型的简单边界SQL报错检测",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				var a = request.URL.Query().Get("id")
				u, err := s.database.GetUserByIdUnsafe(a)
				if err != nil {
					writer.Write([]byte(`You have an error in your SQL syntax; check the manual that corresponds to your MySQL server version for the right syntax to use near ''1'' LIMIT 0,1' at line 1`))
					writer.WriteHeader(500)
					return
				}
				_, err = json.Marshal(u)
				if err != nil {
					writer.Write([]byte(`You have an error in your SQL syntax; check the manual that corresponds to your MySQL server version for the right syntax to use near ''1'' LIMIT 0,1' at line 1`))
					writer.WriteHeader(500)
					return
				}
				sqliWriter(writer, request, []*VulinUser{u})
				return
			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "",
			Path:         "/user/cookie-id",
			RouteName:    "Cookie-ID SQL注入",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				a, err := request.Cookie("ID")
				if err != nil {
					cookie := http.Cookie{
						Name:     "ID",
						Value:    "1",                                // 设置 cookie 的值
						Expires:  time.Now().Add(7 * 24 * time.Hour), // 设置过期时间
						HttpOnly: false,                              // 仅限 HTTP 访问，不允许 JavaScript 访问
					}
					http.SetCookie(writer, &cookie)
					writer.Header().Set("Location", "/user/cookie-id?skip=1")
					if request.URL.Query().Get("skip") == "1" {
						writer.WriteHeader(200)
						writer.Write([]byte("Cookie set"))
					} else {
						writer.WriteHeader(302)
					}
					return
				}
				u, err := s.database.GetUserByIdUnsafe(a.Value)
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				sqliWriter(writer, request, []*VulinUser{u})
			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "name=admin",
			Path:         "/user/name",
			RouteName:    "字符串为注入点的 SQL注入",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				var a = request.URL.Query().Get("name")
				u, err := s.database.GetUserByUsernameUnsafe(a)
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				sqliWriter(writer, request, u)
				return
			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "name=admin",
			Path:         "/user/name/like",
			RouteName:    "字符串注入点模糊查询",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				db := s.database.db
				var name = LoadFromGetParams(request, "name")
				msg := `select * from vulin_users where username LIKE '%` + name + `%';`
				db = db.Raw(msg)
				if db.Error != nil {
					Failed(writer, request, db.Error.Error())
					return
				}
				var users []*VulinUser
				err := db.Scan(&users).Error
				if err != nil {
					Failed(writer, request, err.Error())
					return
				}
				sqliWriterEx(true, writer, request, users, msg)
			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "name=admin",
			Path:         "/user/name/like/2",
			RouteName:    "字符串注入点模糊查询(括号边界)",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				db := s.database.db
				var name = LoadFromGetParams(request, "name")
				var rowStr = `select * from vulin_users where (username LIKE '%` + name + `%') AND (age > 20);`
				db = db.Raw(rowStr)
				if db.Error != nil {
					Failed(writer, request, db.Error.Error())
					return
				}
				var users []*VulinUser
				err := db.Scan(&users).Error
				if err != nil {
					Failed(writer, request, err.Error())
					return
				}
				sqliWriterEx(true, writer, request, users, rowStr)
			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "nameb64=YQ==",
			Path:         "/user/name/like/b64",
			RouteName:    "参数编码字符串注入点模糊查询(括号边界)",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				db := s.database.db
				var name = LoadFromGetBase64Params(request, "nameb64")
				var rowStr = `select * from vulin_users where (username LIKE '%` + name + `%') AND (age > 20);`
				db = db.Raw(rowStr)
				if db.Error != nil {
					Failed(writer, request, db.Error.Error())
					return
				}
				var users []*VulinUser
				err := db.Scan(&users).Error
				if err != nil {
					Failed(writer, request, err.Error())
					return
				}
				sqliWriterEx(true, writer, request, users, rowStr)
			},
			Detected:      true,
			ExpectedValue: "1",
		}, {
			DefaultQuery: "data=eyJuYW1lYjY0aiI6ImEifQ==",
			Path:         "/user/name/like/b64j",
			RouteName:    "Base64参数(JSON)嵌套字符串注入点模糊查询(括号边界)",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				db := s.database.db
				var name = LoadFromGetBase64JSONParam(request, "data", "nameb64j")
				var rowStr = `select * from vulin_users where (username LIKE '%` + name + `%') AND (age > 20);`
				db = db.Raw(rowStr)
				if db.Error != nil {
					Failed(writer, request, db.Error.Error())
					return
				}
				var users []*VulinUser
				err := db.Scan(&users).Error
				if err != nil {
					Failed(writer, request, err.Error())
					return
				}
				sqliWriterEx(true, writer, request, users, rowStr)
			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "limit=1",
			Path:         "/user/limit/int",
			RouteName:    "LIMIT（语句结尾）注入案例",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				db := s.database.db
				var limit = LoadFromGetParams(request, "limit")
				var rowStr = `select * from vulin_users where (username LIKE '%` + "a" + `%') LIMIT ` + limit + `;`
				db = db.Raw(rowStr)
				if db.Error != nil {
					Failed(writer, request, db.Error.Error())
					return
				}
				var users []*VulinUser
				err := db.Scan(&users).Error
				if err != nil {
					Failed(writer, request, err.Error())
					return
				}
				sqliWriterEx(true, writer, request, users, rowStr)
			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "order=desc",
			Path:         "/user/limit/4/order1",
			RouteName:    "ORDER注入：单个条件排序位于 LIMIT 之前",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				db := s.database.db
				var data = LoadFromGetParams(request, "order")
				var rowStr = `select * from vulin_users where (username LIKE '%` + "a" + `%') ORDER BY username ` + data + ` LIMIT 5;`
				db = db.Raw(rowStr)
				if db.Error != nil {
					Failed(writer, request, db.Error.Error())
					return
				}
				var users []*VulinUser
				err := db.Scan(&users).Error
				if err != nil {
					Failed(writer, request, err.Error())
					return
				}
				sqliWriterEx(true, writer, request, users, rowStr)
			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "order=desc",
			Path:         "/user/limit/4/order2",
			RouteName:    "ORDER注入：多条件排序位于 LIMIT 之前",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				db := s.database.db
				var data = LoadFromGetParams(request, "order")
				var rowStr = `select * from vulin_users where (username LIKE '%` + "a" + `%') ORDER BY username ` + data + `, created_at LIMIT 5;`
				db = db.Raw(rowStr)
				if db.Error != nil {
					Failed(writer, request, db.Error.Error())
					return
				}
				var users []*VulinUser
				err := db.Scan(&users).Error
				if err != nil {
					Failed(writer, request, err.Error())
					return
				}
				sqliWriterEx(true, writer, request, users, rowStr)
			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "order=desc",
			Path:         "/user/order3",
			RouteName:    "注入：多条件排序位（无LIMIT）",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				db := s.database.db
				var data = LoadFromGetParams(request, "order")
				var rowStr = `select * from vulin_users where (username LIKE '%` + "a" + `%') ORDER BY created_at desc, username ` + data + `;`
				db = db.Raw(rowStr)
				if db.Error != nil {
					Failed(writer, request, db.Error.Error())
					return
				}
				var users []*VulinUser
				err := db.Scan(&users).Error
				if err != nil {
					Failed(writer, request, err.Error())
					return
				}
				sqliWriterEx(true, writer, request, users, rowStr)
			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "orderby=username",
			Path:         "/user/limit/4/orderby",
			RouteName:    "ORDERBY 注入：多字段",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				db := s.database.db
				var data = LoadFromGetParams(request, "orderby")
				var rowStr = `select * from vulin_users where (username LIKE '%` + "a" + `%') ORDER BY ` + data + ` desc LIMIT 5;`
				db = db.Raw(rowStr)
				if db.Error != nil {
					Failed(writer, request, db.Error.Error())
					return
				}
				var users []*VulinUser
				err := db.Scan(&users).Error
				if err != nil {
					Failed(writer, request, err.Error())
					return
				}
				sqliWriterEx(true, writer, request, users, rowStr)
			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "orderby=id",
			Path:         "/user/limit/4/orderby1",
			RouteName:    "ORDER BY注入：反引号+排序",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				db := s.database.db
				var data = LoadFromGetParams(request, "orderby")
				var rowStr = `select * from vulin_users where (username LIKE '%` + "a" + `%') ORDER BY ` + "`" + data + "` desc" + ` LIMIT 5;`
				db = db.Raw(rowStr)
				if db.Error != nil {
					Failed(writer, request, db.Error.Error())
					return
				}
				var users []*VulinUser
				err := db.Scan(&users).Error
				if err != nil {
					Failed(writer, request, err.Error())
					return
				}
				sqliWriterEx(true, writer, request, users, rowStr)
			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "orderby=id",
			Path:         "/user/limit/4/orderby2",
			RouteName:    "ORDER BY 注入：反引号+多字段",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				db := s.database.db
				var data = LoadFromGetParams(request, "orderby")
				var rowStr = `select * from vulin_users where (username LIKE '%` + "a" + `%') ORDER BY ` + "`" + data + "`,created_at" + ` LIMIT 5;`
				db = db.Raw(rowStr)
				if db.Error != nil {
					Failed(writer, request, db.Error.Error())
					return
				}
				var users []*VulinUser
				err := db.Scan(&users).Error
				if err != nil {
					Failed(writer, request, err.Error())
					return
				}
				sqliWriterEx(true, writer, request, users, rowStr)
			},
			Detected:      true,
			ExpectedValue: "1",
		},
	}
	for _, v := range vroutes {
		addRouteWithComment(sqli, v)
	}

}
